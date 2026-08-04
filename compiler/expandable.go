package compiler

// Expandable — one config value that may carry {$$$ ref $$$} macros. The
// stream is never mutated: expansion is NOT reversible, and expanding
// then saving would freeze the template into the file. The type keeps
// the two truths apart:
//
//	Raw       what the document declares — and what every rewrite emits
//	Get()     today's render — what consumers consume (String() casts)
//
// It also carries its CONTEXT: where its references resolve (a unit, or
// the version-spec) and whether it lives in an attribute (law 5: bare
// references only there).
//
// The family is built by the linking walk — the one place that already
// touches every adopter — and is traversable: one pass resolves them all
// (against the same lazy universe, errors at load time — law 4), and an
// exhaustive check walks them without missing one. Capture is a plain
// SetRaw: no decoder plumbing, no registration at birth.

import (
	"fmt"

	"github.com/pablo-botella/cargoxml"
)

// xmlLine answers the decoder's current input line — the capture sites
// stamp it on what they capture, so a value can point at its document.
func xmlLine(d *cargoxml.DecoderWithCargo) int {
	line, _ := d.Decoder.InputPos()
	return line
}

// Literal is Expandable's forbidden twin: a config value the version
// universe is BUILT from — id, name, path, project-type. It never
// expands, because the tree and the reference space derive from these
// before any resolution exists (name feeds prj:id:name: expanding it
// would be circular). The distinct type is the restriction made visible:
// where the project's life is at stake, a plain string cannot slip in,
// and a macro fails LOUDLY at parse (law 1) — never a silent literal.
type Literal string

// CheckLiteral is the law-1 guard a consumer runs over its own sources.
func CheckLiteral(what string, l Literal) error {
	if HasVersionMacro(string(l)) {
		return fmt.Errorf("law 1: %s=%q must not carry a macro — it is a source of the version universe", what, string(l))
	}
	return nil
}

// ExpandableContext is where an Expandable's references resolve — and
// the NODE it came from, so every message and dump can point at the
// document instead of quoting a loose string.
type ExpandableContext struct {
	UnitID  string // the owning unit; empty = the version-spec is local
	Element string // the owning element: "embed", "item", "import-miniskin", "project"
	Name    string // the attribute this value came from
	Attr    bool   // it lives in an attribute: bare references only
}

// Expandable is the value itself — a plain field, no pointers: an absent
// attribute is an empty Raw.
type Expandable struct {
	Raw       string
	Expanded  string
	Line      int // the document line it was captured at; 0 = unknown
	Ctx       ExpandableContext
	TraceInfo []string // how the render came to be: every reference touched, with its value — filled at resolution when the raw carries macros
	resolved  bool
}

// SetRaw is the capture: what the document says, verbatim, and where.
func (e *Expandable) SetRaw(raw string) { e.Raw = raw }

// SetRawAt is SetRaw plus the document line — the capture sites have the
// decoder at hand; a value that knows its line points at the document.
func (e *Expandable) SetRawAt(raw string, line int) {
	e.Raw = raw
	e.Line = line
}

// Get answers the render when resolved, the raw otherwise — a value
// without macros is its own render either way. Value receiver: reading
// costs nothing and works on fields and copies alike.
func (e Expandable) Get() string {
	if e.resolved {
		return e.Expanded
	}
	return e.Raw
}

// String is the cast-to-string: an Expandable in any string context — a
// printf, an interpolation, a %s — IS its resolved value, so consuming
// code reads as if the field were a plain string. The raw face is only
// ever reached by name (Raw), which is exactly the asymmetry wanted:
// consuming is effortless, freezing takes intent.
func (e Expandable) String() string { return e.Get() }

// Empty reports an absent or blank value.
func (e Expandable) Empty() bool { return e.Raw == "" }

// Where names the node the value came from — for messages and dumps:
// `prj:cli <embed embed-version> (line 12)`.
func (e Expandable) Where() string {
	w := "<" + e.Ctx.Element + " " + e.Ctx.Name + ">"
	if e.Ctx.UnitID != "" {
		w = "prj:" + e.Ctx.UnitID + " " + w
	}
	if e.Line > 0 {
		w += fmt.Sprintf(" (line %d)", e.Line)
	}
	return w
}

// HasMacro reports whether the raw actually carries a hole.
func (e Expandable) HasMacro() bool { return HasVersionMacro(e.Raw) }

// ExpandableFamily is the ledger of one load, filled by the linking walk.
type ExpandableFamily struct {
	All []*Expandable
}

// Adopt files one member with its context — the walk calls it for every
// adopter field; empty values stay out (an absent attribute is nobody).
func (f *ExpandableFamily) Adopt(e *Expandable, ctx ExpandableContext) {
	if e.Empty() {
		return
	}
	e.Ctx = ctx
	f.All = append(f.All, e)
}

// Report prints everything the load resolved: every macro-carrying
// member with its node, its raw, its render, and the chain it followed —
// reading what the values already carry, re-running nothing.
func (f *ExpandableFamily) Report() []string {
	var out []string
	for _, e := range f.All {
		if len(e.TraceInfo) == 0 {
			continue // no macros: nothing was resolved, nothing to explain
		}
		out = append(out, e.Where()+" = "+fmt.Sprintf("%q -> %q", e.Raw, e.Get()))
		for _, l := range e.TraceInfo {
			out = append(out, "  "+l)
		}
	}
	return out
}

// NeedsResolution reports whether any member actually carries a macro —
// a family of plain values costs nothing.
func (f *ExpandableFamily) NeedsResolution() bool {
	for _, e := range f.All {
		if e.HasMacro() {
			return true
		}
	}
	return false
}

// Resolve walks the family against the version universe: each member
// renders in its own context AND keeps its own trace — the value carries
// its explanation, debugging is reading what is already there.
// Unresolvable is a hard error — this runs at load, where law 4 wants it
// — and the failed member keeps the trace too: it shows where it died.
func (f *ExpandableFamily) Resolve(doc *LazyDoc) error {
	for _, e := range f.All {
		if !e.HasMacro() {
			e.Expanded = e.Raw
			e.resolved = true
			continue
		}
		var unit *LazyUnit
		if e.Ctx.UnitID != "" {
			if unit = doc.ByID[e.Ctx.UnitID]; unit == nil {
				return fmt.Errorf("expandable of unit %q: unit not in the version universe", e.Ctx.UnitID)
			}
		}
		resolve, trace := doc.resolverTraced(unit)
		text, err := ExpandVersionMacros(e.Raw, e.Ctx.Attr, resolve)
		e.TraceInfo = trace.Lines
		if err != nil {
			return fmt.Errorf("expanding %s = %q: %w", e.Where(), e.Raw, err)
		}
		e.Expanded = text
		e.resolved = true
	}
	return nil
}
