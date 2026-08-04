package compiler

// Version subsystem — the lazy first pass. An INDEPENDENT parser over the
// config XML: stdlib encoding/xml, no cargoxml, no cargo — it skims the one
// file for the version universe (project skeleton, labels, version-spec,
// version, ts) and nothing else. The v-verbs live entirely on it: parse one
// file, resolve what is reachable, exit. The full cargoxml parser (Root) is
// the third pass and another business.
//
// Second pass: what was captured resolves over itself — the skeleton yields
// every unit's computed base, the maps answer references on demand (lazy),
// computed labels render through the shared primitives with one cycle guard.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
)

// LazyLabel is one <label> wherever it lives — the two exclusive classes in
// one shape: a VALUE label carries Value (its text node); a COMPUTED one
// carries Format+Params (+When). Position gave it its domain already.
type LazyLabel struct {
	Key         string
	Value       string // value class: the text node IS the current value
	Format      string // computed class: positional format…
	Params      string // …over these reference items
	When        string // computed only: guard reference; empty value, empty label
	Volatile    bool   // version-spec only: bump semantics
	Default     string // volatile reset literal…
	DefaultToTs bool   // …or the materialized ts; mutually exclusive
	Line        int    // the document line it was declared at; 0 = unknown
}

// Computed reports the label's class — format/params present is what decides.
func (l *LazyLabel) Computed() bool { return l.Format != "" || l.Params != "" }

// LazyComponent is one declared component: its domain and current value.
type LazyComponent struct {
	Name     string
	Min, Max int
	Value    int
	Locked   bool
}

// LazySpec is the captured <version-spec>: format, ordered components with
// their domains and values, ts, its labels, and the destinations.
type LazySpec struct {
	Format     string // optional canonical override; empty = "v" + dots
	Components []LazyComponent
	Ts         string
	Labels     map[string]*LazyLabel
	LabelOrder []string // declaration order — the history archives by it
	Dests      []any    // *LazyUpdate | *LazyCreate, declaration order
}

// LazyUpdate is one <update>: patch a file other content also owns.
type LazyUpdate struct {
	File    string // project-root-relative; may carry bare-ref macros
	Type    string // "json" | "xml" | "replace" (the default)
	Entries []*LazyEntry
}

// LazyEntry is one <entry> of an update. Text is the RAW text node —
// trim is applied at render time, per mode.
type LazyEntry struct {
	Key    string // json/xml address (slash path); empty on replace
	Attrib string // xml only: target an attribute instead of the text
	Trim   string // all (default) | left | right | none
	Indent string // "" = keep the file line's own; else spaces count
	Text   string
}

// LazyCreate is one <create>: a file wholly owned by the version.
type LazyCreate struct {
	File      string // project-root-relative; may carry bare-ref macros
	Src       string // external template — XOR with the text node
	Text      string // inline template, raw
	GitIgnore bool
	Overwrite string // "true" (default) | "warn" | "false"
	Eol       string // "" (as the template arrives) | "lf" | "crlf"
}

// Component finds a declared component by name; nil when absent.
func (s *LazySpec) Component(name string) *LazyComponent {
	for i := range s.Components {
		if s.Components[i].Name == name {
			return &s.Components[i]
		}
	}
	return nil
}

// LazyUnit is one project of the skeleton: the literal structural attributes
// (law 1: never expanded), the computed Base, and the unit's labels.
type LazyUnit struct {
	Id, Name, Type string
	Path           string // declared, literal, relative to the parent
	Base           string // RESOLVED unit root: parent base + path; "" for main
	Labels         map[string]*LazyLabel
	EmbedVersion   string // the unit's <embed embed-version="…"> template, raw
}

// LazyDoc is the whole first pass: the skeleton, the label domains, the spec
// and the load warnings. Everything the v-verbs can reach lives here.
type LazyDoc struct {
	Units []*LazyUnit // document order: main first
	ByID  map[string]*LazyUnit
	Glb   map[string]*LazyLabel
	Spec  *LazySpec
	Warns []string
}

// reserved bare names of the version domain.
func isReservedVersionName(s string) bool { return s == "version" || s == "ts" }

// --- first pass: the skim ---

// LazyLoad reads filename and runs the two passes: capture, then the
// skeleton resolution (auto ids, bases, domain checks) and the eager
// label verification. Reference resolution proper stays on demand —
// ResolverFor hands it out.
func LazyLoad(filename string) (*LazyDoc, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LazyParse(xml.NewDecoder(f))
}

// LazyLoadUnchecked loads WITHOUT the eager label verification — the
// debugger's door: inspecting a config that misbehaves is the whole
// point, so the load must not refuse it first.
func LazyLoadUnchecked(filename string) (*LazyDoc, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return lazyParseCore(xml.NewDecoder(f))
}

// resolverTraced hands out a resolver wired to a fresh recorder: every
// reference it touches lands in the trace — the Expandables keep theirs
// at resolution, so the value carries its own explanation.
func (doc *LazyDoc) resolverTraced(unit *LazyUnit) (VersionResolver, *vTrace) {
	st := &lazyState{inFlight: map[vScopeKey]bool{}, trace: &vTrace{}}
	res := &lazyResolution{doc: doc, scope: vScope{unit: unit}, st: st}
	return res.resolve, st.trace
}

// Trace resolves one reference recording the whole chain — every nested
// lookup with its render, or the exact node where it died. The lines
// come back even on error: the trace of a failure IS the diagnosis.
func (doc *LazyDoc) Trace(ref string) (string, []string, error) {
	r, err := ParseVersionRef(ref)
	if err != nil {
		return "", nil, err
	}
	st := &lazyState{inFlight: map[vScopeKey]bool{}, trace: &vTrace{}}
	res := &lazyResolution{doc: doc, st: st}
	v, err := res.resolve(r)
	return v.Text, st.trace.Lines, err
}

// LazyParse runs the first pass over a decoder and verifies every label.
func LazyParse(d *xml.Decoder) (*LazyDoc, error) {
	doc, err := lazyParseCore(d)
	if err != nil {
		return nil, err
	}
	if err := doc.checkAllLabels(); err != nil {
		return nil, err
	}
	return doc, nil
}

// lazyParseCore is the capture and skeleton, label verification excluded.
// Structural attributes are checked against law 1 (no macro may live in
// id, path, project-type).
func lazyParseCore(d *xml.Decoder) (*LazyDoc, error) {
	doc := &LazyDoc{ByID: map[string]*LazyUnit{}, Glb: map[string]*LazyLabel{}}

	var unitStack []*LazyUnit // open <project>/<child-project> elements
	var specDepth int         // >0: inside <version-spec> at that stack depth
	var curVersionEl bool     // inside <version>
	var curTsEl bool          // inside <version><ts>
	var curLabel *LazyLabel   // open <label> element, wherever it lives
	var curUpdate *LazyUpdate // open <update> element (spec only)
	var curEntry *LazyEntry   // open <entry> inside the open update
	var curCreate *LazyCreate // open <create> element (spec only)
	var labelText strings.Builder
	var tsText strings.Builder
	var entryText strings.Builder
	var createText strings.Builder
	depth := 0

	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "project", "child-project":
				if specDepth > 0 {
					break // foreign inside the spec: not ours here
				}
				u := &LazyUnit{Labels: map[string]*LazyLabel{}}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "id", "name", "path", "project-type":
						if HasVersionMacro(a.Value) {
							return nil, fmt.Errorf("law 1: %s=%q must not carry a macro — it is a source of the version universe", a.Name.Local, a.Value)
						}
					}
					switch a.Name.Local {
					case "id":
						u.Id = a.Value
					case "name":
						u.Name = a.Value
					case "path":
						u.Path = a.Value
					case "project-type":
						u.Type = a.Value
					}
				}
				if len(unitStack) > 0 {
					parent := unitStack[len(unitStack)-1]
					u.Base = joinBase(parent.Base, u.Path)
				}
				doc.Units = append(doc.Units, u)
				unitStack = append(unitStack, u)

			case "version-spec":
				if len(unitStack) > 0 {
					doc.Warns = append(doc.Warns, "version-spec inside a project is ignored (reserved)")
					if err := d.Skip(); err != nil {
						return nil, err
					}
					depth--
					break
				}
				if doc.Spec != nil {
					return nil, fmt.Errorf("multiple <version-spec> elements")
				}
				doc.Spec = &LazySpec{Labels: map[string]*LazyLabel{}}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "format":
						doc.Spec.Format = a.Value
					case "params":
						comps, err := parseComponentDomains(a.Value)
						if err != nil {
							return nil, err
						}
						doc.Spec.Components = comps
					}
				}
				specDepth = depth

			case "version":
				if doc.Spec == nil || specDepth == 0 {
					break
				}
				curVersionEl = true
				var lock string
				for _, a := range t.Attr {
					if a.Name.Local == "lock" {
						lock = a.Value
						continue
					}
					c := doc.Spec.Component(a.Name.Local)
					if c == nil {
						return nil, fmt.Errorf("<version> attribute %q is not a declared component", a.Name.Local)
					}
					n, err := strconv.Atoi(a.Value)
					if err != nil {
						return nil, fmt.Errorf("<version> %s=%q is not a number", a.Name.Local, a.Value)
					}
					if n < c.Min || n > c.Max {
						return nil, fmt.Errorf("<version> %s=%d out of its domain [%d-%d]", a.Name.Local, n, c.Min, c.Max)
					}
					c.Value = n
				}
				if lock != "" {
					c := doc.Spec.Component(lock)
					if c == nil {
						return nil, fmt.Errorf("lock=%q is not a declared component", lock)
					}
					c.Locked = true
				}

			case "ts":
				if curVersionEl {
					curTsEl = true
					tsText.Reset()
				}

			case "embed":
				if specDepth == 0 && len(unitStack) > 0 {
					for _, a := range t.Attr {
						if a.Name.Local == "embed-version" {
							unitStack[len(unitStack)-1].EmbedVersion = a.Value
						}
					}
				}

			case "update":
				if doc.Spec == nil || specDepth == 0 {
					break
				}
				curUpdate = &LazyUpdate{Type: "replace"}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "file":
						curUpdate.File = a.Value
					case "type":
						switch a.Value {
						case "json", "xml", "replace":
							curUpdate.Type = a.Value
						default:
							return nil, fmt.Errorf("<update> type %q: want json, xml or replace", a.Value)
						}
					}
				}
				if curUpdate.File == "" {
					return nil, fmt.Errorf("<update> without file")
				}
				doc.Spec.Dests = append(doc.Spec.Dests, curUpdate)

			case "entry":
				if curUpdate == nil {
					break
				}
				curEntry = &LazyEntry{Trim: "all"}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "key":
						curEntry.Key = a.Value
					case "attrib":
						curEntry.Attrib = a.Value
					case "trim":
						switch a.Value {
						case "all", "left", "right", "none":
							curEntry.Trim = a.Value
						default:
							return nil, fmt.Errorf("<entry> trim %q: want all, left, right or none", a.Value)
						}
					case "indent":
						if _, err := strconv.Atoi(a.Value); err != nil {
							return nil, fmt.Errorf("<entry> indent %q is not a number", a.Value)
						}
						curEntry.Indent = a.Value
					}
				}
				switch curUpdate.Type {
				case "json", "xml":
					if curEntry.Key == "" {
						return nil, fmt.Errorf("<entry> of a %s update needs a key", curUpdate.Type)
					}
				case "replace":
					if curEntry.Key != "" {
						return nil, fmt.Errorf("<entry> of a replace update takes no key — it self-anchors")
					}
				}
				if curEntry.Attrib != "" && curUpdate.Type != "xml" {
					return nil, fmt.Errorf("attrib is xml-only, this update is %s", curUpdate.Type)
				}
				entryText.Reset()

			case "create":
				if doc.Spec == nil || specDepth == 0 {
					break
				}
				curCreate = &LazyCreate{Overwrite: "true"}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "file":
						curCreate.File = a.Value
					case "src":
						curCreate.Src = a.Value
					case "git-ignore":
						curCreate.GitIgnore = a.Value == "true"
					case "overwrite":
						switch a.Value {
						case "true", "warn", "false":
							curCreate.Overwrite = a.Value
						default:
							return nil, fmt.Errorf("<create> overwrite %q: want true, warn or false", a.Value)
						}
					case "eol":
						switch a.Value {
						case "lf", "crlf":
							curCreate.Eol = a.Value
						default:
							return nil, fmt.Errorf("<create> eol %q: want lf or crlf", a.Value)
						}
					}
				}
				if curCreate.File == "" {
					return nil, fmt.Errorf("<create> without file")
				}
				doc.Spec.Dests = append(doc.Spec.Dests, curCreate)
				createText.Reset()

			case "label":
				l := &LazyLabel{}
				l.Line, _ = d.InputPos()
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "key":
						l.Key = a.Value
					case "format":
						l.Format = a.Value
					case "params":
						l.Params = a.Value
					case "when":
						l.When = a.Value
					case "volatile":
						l.Volatile = a.Value == "true"
					case "default":
						l.Default = a.Value
					case "default-to-ts":
						l.DefaultToTs = a.Value == "true"
					}
				}
				if l.Key == "" {
					return nil, fmt.Errorf("<label> without key")
				}
				curLabel = l
				labelText.Reset()
			}

		case xml.CharData:
			if curLabel != nil {
				labelText.Write(t)
			}
			if curTsEl {
				tsText.Write(t)
			}
			if curEntry != nil {
				entryText.Write(t)
			}
			if curCreate != nil {
				createText.Write(t)
			}

		case xml.EndElement:
			depth--
			switch t.Name.Local {
			case "project", "child-project":
				if specDepth == 0 && len(unitStack) > 0 {
					unitStack = unitStack[:len(unitStack)-1]
				}
			case "version-spec":
				if specDepth > 0 && depth < specDepth {
					specDepth = 0
				}
			case "version":
				curVersionEl = false
			case "ts":
				if curTsEl {
					doc.Spec.Ts = strings.TrimSpace(tsText.String())
					curTsEl = false
				}
			case "label":
				if curLabel == nil {
					break
				}
				curLabel.Value = strings.TrimSpace(labelText.String())
				if err := doc.placeLabel(curLabel, unitStack, specDepth > 0); err != nil {
					return nil, err
				}
				curLabel = nil
			case "entry":
				if curEntry != nil {
					curEntry.Text = entryText.String() // raw: trim happens per mode at render
					curUpdate.Entries = append(curUpdate.Entries, curEntry)
					curEntry = nil
				}
			case "update":
				curUpdate = nil
			case "create":
				if curCreate != nil {
					curCreate.Text = createText.String()
					if curCreate.Src != "" && strings.TrimSpace(curCreate.Text) != "" {
						return nil, fmt.Errorf("<create %s>: inline template and src are mutually exclusive", curCreate.File)
					}
					curCreate = nil
				}
			}
		}
	}

	if err := doc.finishSkeleton(); err != nil {
		return nil, err
	}
	return doc, nil
}

// checkAllLabels resolves EVERY label of every domain once, discarding
// the renders: the lazy pass leaves no label unverified — a broken or
// cyclic one fails the load, cited or not, so every verb catches it, not
// just the guard. Values are deliberately NOT memoized: the mutators
// edit the in-memory state and read again — correctness beats caching a
// handful of strings.
func (doc *LazyDoc) checkAllLabels() error {
	for _, k := range slices.Sorted(maps.Keys(doc.Glb)) {
		if _, err := doc.ResolverFor(nil)(VersionRef{Domain: "glb", Label: true, Name: k}); err != nil {
			return err
		}
	}
	if doc.Spec != nil {
		for _, k := range doc.Spec.LabelOrder {
			if _, err := doc.ResolverFor(nil)(VersionRef{Label: true, Name: k}); err != nil {
				return err
			}
		}
	}
	for _, u := range doc.Units {
		for _, k := range slices.Sorted(maps.Keys(u.Labels)) {
			if _, err := doc.ResolverFor(u)(VersionRef{Label: true, Name: k}); err != nil {
				return err
			}
		}
	}
	return nil
}

// placeLabel files a closed <label> under its domain — POSITION DEFINES THE
// DOMAIN — and validates its class right here: the two classes are
// exclusive, bump semantics live in the spec only.
func (doc *LazyDoc) placeLabel(l *LazyLabel, unitStack []*LazyUnit, inSpec bool) error {
	if l.Computed() {
		if l.Volatile || l.Default != "" || l.DefaultToTs || l.Value != "" {
			return fmt.Errorf("label %q: computed and value traits are exclusive", l.Key)
		}
	} else if l.When != "" {
		return fmt.Errorf("label %q: when guards computed labels only", l.Key)
	}
	if l.Default != "" && l.DefaultToTs {
		return fmt.Errorf("label %q: default and default-to-ts are mutually exclusive", l.Key)
	}
	switch {
	case inSpec:
		if doc.Spec.Labels[l.Key] != nil {
			return fmt.Errorf("duplicate spec label %q", l.Key)
		}
		doc.Spec.Labels[l.Key] = l
		doc.Spec.LabelOrder = append(doc.Spec.LabelOrder, l.Key)
	case len(unitStack) > 0:
		if l.Volatile || l.DefaultToTs {
			return fmt.Errorf("label %q: volatile/default-to-ts are version-spec only", l.Key)
		}
		u := unitStack[len(unitStack)-1]
		if u.Labels[l.Key] != nil {
			return fmt.Errorf("duplicate label %q in unit %q", l.Key, u.Id)
		}
		u.Labels[l.Key] = l
	default:
		if l.Volatile || l.DefaultToTs {
			return fmt.Errorf("label %q: volatile/default-to-ts are version-spec only", l.Key)
		}
		if doc.Glb[l.Key] != nil {
			return fmt.Errorf("duplicate global label %q", l.Key)
		}
		doc.Glb[l.Key] = l
	}
	return nil
}

// finishSkeleton is the second pass over the structure: auto ids (same rule
// as the full parser: P<n>, with a warning), the id index, name defaults,
// and the reserved-name checks the maps depend on.
func (doc *LazyDoc) finishSkeleton() error {
	seen := map[string]bool{}
	dupes := []string{}
	for _, u := range doc.Units {
		if u.Id != "" {
			if seen[u.Id] {
				dupes = append(dupes, u.Id)
			}
			seen[u.Id] = true
		}
	}
	if len(dupes) > 0 {
		return fmt.Errorf("duplicate project ids: %v", dupes)
	}
	for n, u := range doc.Units {
		if u.Id == "" {
			id := "P" + strconv.Itoa(n)
			for nn := '1'; seen[id]; nn++ {
				id = "P" + strconv.Itoa(n) + "_" + string(nn)
			}
			u.Id = id
			doc.Warns = append(doc.Warns, fmt.Sprintf("project %q has no id, assigned %q", u.Name, u.Id))
		}
		doc.ByID[u.Id] = u
	}
	if doc.Spec != nil {
		for i := range doc.Spec.Components {
			if isReservedVersionName(doc.Spec.Components[i].Name) {
				return fmt.Errorf("component %q: reserved name", doc.Spec.Components[i].Name)
			}
		}
		for k := range doc.Spec.Labels {
			if isReservedVersionName(k) {
				return fmt.Errorf("label %q: reserved name", k)
			}
		}
	}
	return nil
}

// joinBase chains a parent's resolved base with a declared path — forward
// slashes, no leading "./", the config's own convention.
func joinBase(parentBase, path string) string {
	p := strings.TrimPrefix(strings.TrimPrefix(path, "./"), "/")
	p = strings.TrimSuffix(p, "/")
	if parentBase == "" {
		return p
	}
	if p == "" {
		return parentBase
	}
	return parentBase + "/" + p
}

// parseComponentDomains reads "major:byte,minor:[0-99],build:word".
func parseComponentDomains(s string) ([]LazyComponent, error) {
	var out []LazyComponent
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		name, domain, ok := strings.Cut(item, ":")
		if !ok || name == "" {
			return nil, fmt.Errorf("params item %q: want name:domain", item)
		}
		c := LazyComponent{Name: name}
		switch {
		case domain == "byte":
			c.Min, c.Max = 0, 255
		case domain == "word":
			c.Min, c.Max = 0, 65535
		case strings.HasPrefix(domain, "[") && strings.HasSuffix(domain, "]"):
			lo, hi, ok := strings.Cut(domain[1:len(domain)-1], "-")
			if !ok {
				return nil, fmt.Errorf("params item %q: want [lo-hi]", item)
			}
			var err error
			if c.Min, err = strconv.Atoi(lo); err != nil {
				return nil, fmt.Errorf("params item %q: bad lower bound", item)
			}
			if c.Max, err = strconv.Atoi(hi); err != nil {
				return nil, fmt.Errorf("params item %q: bad upper bound", item)
			}
			if c.Min > c.Max {
				return nil, fmt.Errorf("params item %q: empty domain", item)
			}
		default:
			return nil, fmt.Errorf("params item %q: unknown domain %q", item, domain)
		}
		c.Value = c.Min // until <version> says otherwise
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("params declares no components")
	}
	return out, nil
}

// --- second pass proper: lazy resolution over the captured maps ---

// vScopeKey identifies one label for the cycle guard.
type vScopeKey struct {
	domain string // "glb", "ver", or a unit id
	key    string
}

// vScope is one local context — one of the three domains: glb, ver (the
// version-spec) or prj (a unit). Bare and label: references resolve HERE;
// crossing always wears a prefix. The scope is not a property of the run:
// it switches when resolution enters a label, because a computed label's
// params resolve in the label's OWN domain.
type vScope struct {
	unit *LazyUnit // prj: the unit; nil otherwise
	glb  bool      // the global domain; false+nil unit = ver
}

// lazyState is the shared spine of one resolution run: the in-flight set
// and the ordered path of the cycle guard — shared across scope switches,
// because a cycle is a cycle wherever it crosses — plus the optional
// trace recorder of the debugging runs.
type lazyState struct {
	inFlight map[vScopeKey]bool
	path     []vScopeKey
	trace    *vTrace
}

// vTrace records one resolution run as an indented tree: every reference
// touched, in resolution order, with its render or its failure — the
// debugger's answer to "how did this label resolve?".
type vTrace struct {
	Lines []string
	depth int
}

// lazyResolution is one resolution frame: the doc, the current scope, and
// the run's shared state.
type lazyResolution struct {
	doc   *LazyDoc
	scope vScope
	st    *lazyState
}

// ResolverFor hands out the resolver for a starting context: unit == nil
// means the version-spec is the local domain (the v-verbs' own context); a
// unit makes bare names structural and label: that unit's.
func (doc *LazyDoc) ResolverFor(unit *LazyUnit) VersionResolver {
	r := &lazyResolution{doc: doc, scope: vScope{unit: unit}, st: &lazyState{inFlight: map[vScopeKey]bool{}}}
	return r.resolve
}

// vQualLabel prints a label's fully qualified name for messages.
func vQualLabel(domain, key string) string {
	switch domain {
	case "glb", "ver":
		return domain + ":label:" + key
	}
	return "prj:" + domain + ":label:" + key
}

// scopeFor maps a cycle-guard domain back to its scope — the switch that
// happens on entering a label.
func (r *lazyResolution) scopeFor(domain string) vScope {
	switch domain {
	case "glb":
		return vScope{glb: true}
	case "ver":
		return vScope{}
	}
	return vScope{unit: r.doc.ByID[domain]}
}

func (r *lazyResolution) resolve(ref VersionRef) (v VersionValue, err error) {
	if tr := r.st.trace; tr != nil {
		at := len(tr.Lines)
		tr.Lines = append(tr.Lines, "") // filled on the way out, in place
		pad := strings.Repeat("  ", tr.depth)
		tr.depth++
		defer func() {
			tr.depth--
			if err != nil {
				tr.Lines[at] = pad + ref.String() + " !! " + err.Error()
				return
			}
			tr.Lines[at] = pad + ref.String() + " = " + strconv.Quote(v.Text)
		}()
	}
	switch ref.Domain {
	case "glb":
		return r.label("glb", r.doc.Glb, ref.Name)
	case "prj":
		u := r.doc.ByID[ref.Unit]
		if u == nil {
			return VersionValue{}, fmt.Errorf("unknown project id in %q", ref.String())
		}
		if ref.Label {
			return r.label(u.Id, u.Labels, ref.Name)
		}
		return unitStructural(u, ref.Name)
	case "ver":
		return r.verDomain(ref)
	}
	// local domain — the current scope decides, ONE uniform rule:
	// label:key is the local domain's labels, wherever you stand (a
	// computed label's params resolve in the label's own domain — the
	// scope switched on entry). Bare names exist only where something
	// bare lives: components in ver, structurals in prj; glb holds
	// labels and nothing else.
	switch {
	case ref.Label:
		switch {
		case r.scope.glb:
			return r.label("glb", r.doc.Glb, ref.Name)
		case r.scope.unit != nil:
			return r.label(r.scope.unit.Id, r.scope.unit.Labels, ref.Name)
		default:
			if r.doc.Spec == nil {
				return VersionValue{}, fmt.Errorf("no version-spec in the config (resolving %q)", ref.String())
			}
			return r.label("ver", r.doc.Spec.Labels, ref.Name)
		}
	case r.scope.glb:
		return VersionValue{}, fmt.Errorf("the global domain holds labels only: %q", ref.String())
	case r.scope.unit != nil:
		return unitStructural(r.scope.unit, ref.Name)
	}
	return r.verDomain(ref)
}

// verDomain answers the version domain: components, version, ts, labels.
func (r *lazyResolution) verDomain(ref VersionRef) (VersionValue, error) {
	s := r.doc.Spec
	if s == nil {
		return VersionValue{}, fmt.Errorf("no version-spec in the config (resolving %q)", ref.String())
	}
	if ref.Label {
		return r.label("ver", s.Labels, ref.Name)
	}
	switch ref.Name {
	case "version":
		text, err := s.renderCanonical()
		if err != nil {
			return VersionValue{}, err
		}
		return TextValue(text), nil
	case "ts":
		return TextValue(s.Ts), nil
	}
	if c := s.Component(ref.Name); c != nil {
		return NumValue(c.Value), nil
	}
	return VersionValue{}, fmt.Errorf("unresolvable reference %q in the version domain", ref.String())
}

// label resolves one label of one domain. A computed label renders its
// format on demand; a VALUE label's text is itself expandable — the
// document keeps the raw, consumers get the render, same universal rule
// as every known text node. Both recurse under the one cycle guard,
// when-guard first on the computed.
func (r *lazyResolution) label(domain string, m map[string]*LazyLabel, key string) (VersionValue, error) {
	qual := vQualLabel(domain, key)
	l := m[key]
	if l == nil {
		return VersionValue{}, fmt.Errorf("unknown label %s", qual)
	}
	if !l.Computed() && !HasVersionMacro(l.Value) {
		return TextValue(l.Value), nil // the plain case costs nothing
	}
	if l.Line > 0 {
		qual += fmt.Sprintf(" (line %d)", l.Line) // errors point at the document
	}
	sk := vScopeKey{domain: domain, key: key}
	if r.st.inFlight[sk] {
		// a true cycle: report the whole chain, from the first visit back
		// to the offender — "unresolvable" alone helps nobody.
		var chain strings.Builder
		for _, p := range r.st.path {
			chain.WriteString(vQualLabel(p.domain, p.key))
			chain.WriteString(" -> ")
		}
		chain.WriteString(qual)
		return VersionValue{}, fmt.Errorf("label cycle: %s", chain.String())
	}
	r.st.inFlight[sk] = true
	r.st.path = append(r.st.path, sk)
	defer func() {
		delete(r.st.inFlight, sk)
		r.st.path = r.st.path[:len(r.st.path)-1]
	}()

	// the scope switch: params, when and an expandable value text resolve
	// in the label's OWN domain, never in the caller's. The guard state
	// travels — a cycle is a cycle wherever it crosses.
	nested := &lazyResolution{doc: r.doc, scope: r.scopeFor(domain), st: r.st}

	if !l.Computed() { // a value label carrying macros in its text
		text, err := ExpandVersionMacros(l.Value, false, nested.resolve)
		if err != nil {
			return VersionValue{}, fmt.Errorf("label %s: %w", qual, err)
		}
		return TextValue(text), nil
	}

	if l.When != "" {
		ref, err := ParseVersionRef(l.When)
		if err != nil {
			return VersionValue{}, fmt.Errorf("label %s: bad when reference: %w", qual, err)
		}
		v, err := nested.resolve(ref)
		if err != nil {
			return VersionValue{}, fmt.Errorf("label %s: %w", qual, err)
		}
		if v.Empty() {
			return TextValue(""), nil // guard unmet: empty label, no error
		}
	}
	text, err := RenderVersionFormat(l.Format, l.Params, nested.resolve)
	if err != nil {
		return VersionValue{}, fmt.Errorf("label %s: %w", qual, err)
	}
	return TextValue(text), nil
}

// unitStructural answers a unit's structural variables — lookups only; path
// is declared, literal, and deliberately NOT a variable.
func unitStructural(u *LazyUnit, name string) (VersionValue, error) {
	switch name {
	case "id":
		return TextValue(u.Id), nil
	case "name":
		return TextValue(u.Name), nil
	case "type":
		return TextValue(u.Type), nil
	case "base":
		return TextValue(u.Base), nil
	}
	return VersionValue{}, fmt.Errorf("unknown structural variable %q of project %q", name, u.Id)
}

// renderCanonical is {$$$ version $$$} and -tag: the format override, or
// the default "v" + the components joined by dots.
func (s *LazySpec) renderCanonical() (string, error) {
	if s.Format == "" {
		parts := make([]string, len(s.Components))
		for i := range s.Components {
			parts[i] = strconv.Itoa(s.Components[i].Value)
		}
		return "v" + strings.Join(parts, "."), nil
	}
	verbs, err := scanFormatVerbs(s.Format)
	if err != nil {
		return "", err
	}
	if len(verbs) != len(s.Components) {
		return "", fmt.Errorf("format %q has %d verbs, %d components declared", s.Format, len(verbs), len(s.Components))
	}
	args := make([]any, len(s.Components))
	for i := range s.Components {
		switch verbs[i][len(verbs[i])-1] {
		case 'd', 'x', 'X', 'o', 'b':
			args[i] = s.Components[i].Value
		default:
			return "", fmt.Errorf("format %q: verb %q wants a number", s.Format, verbs[i])
		}
	}
	return fmt.Sprintf(s.Format, args...), nil
}
