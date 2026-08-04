package compiler

// Version subsystem — the verbs, as compiler API. The CLI is a shell: it
// parses arguments, calls here, prints. Phase 1 ships the read side (-vout
// and its -tag alias); the mutators arrive with the cargoxml write path.

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
)

// VersionConfigFile is the conventional config location under a project
// base — the same convention Root.Load follows.
func VersionConfigFile(base string) string {
	if base == "" {
		base = "."
	}
	return filepath.Join(base, _SkillFolder, _ConfigFileName)
}

// VOut is the -vout command: load the config lazily, resolve ONE bare
// reference, hand back its render. No verbs, no connectors, no formats on
// the command line — formatting lives in the XML; declare a computed label
// and print it by name. Without a version-spec every v-command errors.
func VOut(base, ref string) (string, error) {
	doc, err := LazyLoad(VersionConfigFile(base))
	if err != nil {
		return "", err
	}
	return doc.VOut(ref)
}

// VTag is the -tag command, the daily alias of -vout version.
func VTag(base string) (string, error) { return VOut(base, "version") }

// versionLazy loads (once) the version universe of the same config file —
// the third pass borrowing the first: embed-version and friends resolve
// against it at deploy time.
func (c *Root) versionLazy() (*LazyDoc, error) {
	if !c.lazyTried {
		c.lazyTried = true
		c.lazyDoc, c.lazyErr = LazyLoad(c.ConfigFile)
	}
	return c.lazyDoc, c.lazyErr
}

// VTrace is the -vtrace command: resolve ONE reference over an UNCHECKED
// load, recording the whole chain — every nested lookup with its render,
// or the exact node where it died. Broken configs welcome: inspecting a
// config that misbehaves is what a debugger is for.
func VTrace(base, ref string) (string, []string, error) {
	doc, err := LazyLoadUnchecked(VersionConfigFile(base))
	if err != nil {
		return "", nil, err
	}
	return doc.Trace(ref)
}

// VTraceAll is -vtrace without a reference: the whole radiography — the
// canonical, ts and every label of every domain, each with its full
// chain, broken ones included (they are the point of the picture).
// One-by-one is a chore; print it all and let the eye search.
func VTraceAll(base string) ([]string, error) {
	doc, err := LazyLoadUnchecked(VersionConfigFile(base))
	if err != nil {
		return nil, err
	}
	var out []string
	trace := func(ref string) {
		_, lines, _ := doc.Trace(ref) // a failure is part of the picture
		out = append(out, lines...)
	}
	if doc.Spec != nil {
		trace("version")
		trace("ts")
		for _, k := range doc.Spec.LabelOrder {
			trace("label:" + k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(doc.Glb)) {
		trace("glb:label:" + k)
	}
	for _, u := range doc.Units {
		for _, k := range slices.Sorted(maps.Keys(u.Labels)) {
			trace("prj:" + u.Id + ":label:" + k)
		}
	}

	// and everything the full load resolves — the Expandables, each
	// carrying its own TraceInfo. A config too broken for Root.Load still
	// gets the label radiography above; this section just says why it
	// stops there.
	root := &Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		out = append(out, "", "expandables not reported: "+err.Error())
		return out, nil
	}
	if rep := root.Family.Report(); len(rep) > 0 {
		out = append(out, "")
		out = append(out, rep...)
	}
	return out, nil
}

// VLabels is the -vlabels command: every label of every domain with its
// render on one line — or its failure, marked. The overview the trace
// then digs into. Unchecked load, same reason.
func VLabels(base string) ([]string, error) {
	doc, err := LazyLoadUnchecked(VersionConfigFile(base))
	if err != nil {
		return nil, err
	}
	var lines []string
	add := func(qual string, l *LazyLabel, unit *LazyUnit, ref VersionRef) {
		if l != nil && l.Line > 0 {
			qual += fmt.Sprintf(" (line %d)", l.Line)
		}
		v, err := doc.ResolverFor(unit)(ref)
		if err != nil {
			lines = append(lines, qual+" !! "+err.Error())
			return
		}
		lines = append(lines, qual+" = "+strconv.Quote(v.Text))
	}
	for _, k := range slices.Sorted(maps.Keys(doc.Glb)) {
		add("glb:label:"+k, doc.Glb[k], nil, VersionRef{Domain: "glb", Label: true, Name: k})
	}
	if doc.Spec != nil {
		for _, k := range doc.Spec.LabelOrder {
			add("ver:label:"+k, doc.Spec.Labels[k], nil, VersionRef{Domain: "ver", Label: true, Name: k})
		}
	}
	for _, u := range doc.Units {
		for _, k := range slices.Sorted(maps.Keys(u.Labels)) {
			add("prj:"+u.Id+":label:"+k, u.Labels[k], u, VersionRef{Domain: "prj", Unit: u.Id, Label: true, Name: k})
		}
	}
	return lines, nil
}

// VOut resolves one bare reference against an already loaded doc — the
// v-verbs' context: the version-spec is the local domain.
func (doc *LazyDoc) VOut(ref string) (string, error) {
	if doc.Spec == nil {
		return "", fmt.Errorf("no version-spec in the config")
	}
	r, err := ParseVersionRef(ref)
	if err != nil {
		return "", err
	}
	v, err := doc.ResolverFor(nil)(r)
	if err != nil {
		return "", err
	}
	return v.Text, nil
}
