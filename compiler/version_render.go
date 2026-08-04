package compiler

// Version subsystem — the render primitives. Pure text: no model, no
// filesystem, no XML. The reference grammar, the {$$$ ... $$$} config skin
// (verbs and connectors included), and the positional format strings of
// version-spec and computed labels. Resolution arrives as a function — who
// owns the maps is somebody else's business.

import (
	"fmt"
	"strconv"
	"strings"
)

// VersionRef is one parsed reference — the inside of a {$$$ $$$} hole or one
// params item, already split by the domain grammar. The first segment picks
// the domain: bare = local, label: = local label, prj:id:var / prj:id:label:key,
// ver:ref, glb:label:key. prj always carries its id — no middle form exists.
type VersionRef struct {
	Domain string // "", "prj", "ver", "glb" — "" is the local domain
	Unit   string // prj only: the unit id, never empty when Domain is "prj"
	Label  bool   // the label: segment was present
	Name   string // the last segment: component, structural or label key
}

func (r VersionRef) String() string {
	var b strings.Builder
	if r.Domain != "" {
		b.WriteString(r.Domain)
		b.WriteByte(':')
	}
	if r.Unit != "" {
		b.WriteString(r.Unit)
		b.WriteByte(':')
	}
	if r.Label {
		b.WriteString("label:")
	}
	b.WriteString(r.Name)
	return b.String()
}

// ParseVersionRef splits a reference by the domain grammar. It is strict:
// a bare "prj:" or "prj:id", a "glb:" without label, an empty name — all
// malformed, all errors. Whitespace around the reference is the caller's
// to trim; inside it is not tolerated.
func ParseVersionRef(s string) (VersionRef, error) {
	bad := func() (VersionRef, error) {
		return VersionRef{}, fmt.Errorf("malformed reference %q", s)
	}
	parts := strings.Split(s, ":")
	for _, p := range parts {
		if p == "" || strings.ContainsAny(p, " \t") {
			return bad()
		}
	}
	switch parts[0] {
	case "label": // label:key — local label
		if len(parts) != 2 {
			return bad()
		}
		return VersionRef{Label: true, Name: parts[1]}, nil
	case "prj": // prj:id:var | prj:id:label:key — always with its id
		switch len(parts) {
		case 3:
			return VersionRef{Domain: "prj", Unit: parts[1], Name: parts[2]}, nil
		case 4:
			if parts[2] != "label" {
				return bad()
			}
			return VersionRef{Domain: "prj", Unit: parts[1], Label: true, Name: parts[3]}, nil
		}
		return bad()
	case "ver": // ver:ref | ver:label:key
		switch len(parts) {
		case 2:
			return VersionRef{Domain: "ver", Name: parts[1]}, nil
		case 3:
			if parts[1] != "label" {
				return bad()
			}
			return VersionRef{Domain: "ver", Label: true, Name: parts[2]}, nil
		}
		return bad()
	case "glb": // glb:label:key — globals hold labels only
		if len(parts) != 3 || parts[1] != "label" {
			return bad()
		}
		return VersionRef{Domain: "glb", Label: true, Name: parts[2]}, nil
	}
	if len(parts) != 1 { // an unknown domain prefix is not a name with colons
		return bad()
	}
	return VersionRef{Name: parts[0]}, nil
}

// VersionValue is one resolved value. Components travel numeric (the fmt
// verbs need the int); everything else is text.
type VersionValue struct {
	Text    string
	Num     int
	Numeric bool
}

// NumValue and TextValue build the two kinds without ceremony.
func NumValue(n int) VersionValue     { return VersionValue{Text: strconv.Itoa(n), Num: n, Numeric: true} }
func TextValue(s string) VersionValue { return VersionValue{Text: s} }
func (v VersionValue) Empty() bool    { return v.Text == "" }

// VersionResolver answers a parsed reference. A non-nil error says WHY it
// did not resolve — unknown reference, a cycle with its whole chain, a
// nested render failure — because a bare "unresolvable" helps nobody.
// This is the config universe: every resolution error is fatal.
type VersionResolver func(ref VersionRef) (VersionValue, error)

// versionRefItem is one reference expression: an optional connector, an
// optional Go fmt verb, and the reference. It is the unit of both the
// {$$$ $$$} hole and the params list item — same grammar, two containers.
type versionRefItem struct {
	Connector    string // the quoted literal; meaningful only with HasConnector
	HasConnector bool
	Verb         string // "%02d", "%x"… empty = plain render
	Ref          VersionRef
}

// parseRefExpr parses one expression: ['literal' +] [%verb] ref.
// % opens a verb, a single quote opens a connector, anything else is a name.
// That is the WHOLE grammar.
func parseRefExpr(s string) (versionRefItem, error) {
	var item versionRefItem
	rest := strings.TrimSpace(s)

	if strings.HasPrefix(rest, "'") { // connector: 'literal' + rest
		end := strings.Index(rest[1:], "'")
		if end < 0 {
			return item, fmt.Errorf("unterminated connector literal in %q", s)
		}
		item.Connector = rest[1 : 1+end]
		item.HasConnector = true
		rest = strings.TrimSpace(rest[2+end:])
		if !strings.HasPrefix(rest, "+") {
			return item, fmt.Errorf("connector without '+' in %q", s)
		}
		rest = strings.TrimSpace(rest[1:])
	}

	if strings.HasPrefix(rest, "%") { // optional Go fmt verb before the name
		sp := strings.IndexAny(rest, " \t")
		if sp < 0 {
			return item, fmt.Errorf("verb without reference in %q", s)
		}
		item.Verb = rest[:sp]
		rest = strings.TrimSpace(rest[sp:])
	}

	ref, err := ParseVersionRef(rest)
	if err != nil {
		return item, err
	}
	item.Ref = ref
	return item, nil
}

// render resolves and renders one expression. The connector rule: empty
// value, the whole piece vanishes — connector included.
func (it versionRefItem) render(resolve VersionResolver) (string, error) {
	v, err := resolve(it.Ref)
	if err != nil {
		return "", err
	}
	if v.Empty() {
		return "", nil // no value, no connector
	}
	text, err := applyVerb(it.Verb, v)
	if err != nil {
		return "", err
	}
	if it.HasConnector {
		return it.Connector + text, nil
	}
	return text, nil
}

// applyVerb renders a value through its Go fmt verb — verbatim fmt, with
// the one discipline: numeric verbs demand numeric values, %s/%q demand
// nothing. An empty verb is the plain render.
func applyVerb(verb string, v VersionValue) (string, error) {
	if verb == "" {
		return v.Text, nil
	}
	kind := verb[len(verb)-1]
	switch kind {
	case 'd', 'x', 'X', 'o', 'b':
		if !v.Numeric {
			return "", fmt.Errorf("numeric verb %q on non-numeric value %q", verb, v.Text)
		}
		return fmt.Sprintf(verb, v.Num), nil
	case 's', 'q':
		return fmt.Sprintf(verb, v.Text), nil
	}
	return "", fmt.Errorf("unsupported verb %q", verb)
}

// --- the {$$$ ... $$$} config skin ---

const vMacroOpen = "{$$$"
const vMacroClose = "$$$}"

// ExpandVersionMacros expands every {$$$ ... $$$} hole in s. One pass over
// the original text: results are never re-scanned (law 3). attrMode is law 5 —
// in attribute values only bare references are legal: no verbs, no
// connectors; the literal text around the holes is the attribute's own.
// Any unresolvable reference is an error: this is the config universe.
func ExpandVersionMacros(s string, attrMode bool, resolve VersionResolver) (string, error) {
	var b strings.Builder
	for {
		i := strings.Index(s, vMacroOpen)
		if i < 0 {
			break
		}
		j := strings.Index(s[i+len(vMacroOpen):], vMacroClose)
		if j < 0 {
			return "", fmt.Errorf("unterminated macro in %q", s)
		}
		inside := s[i+len(vMacroOpen) : i+len(vMacroOpen)+j]
		b.WriteString(s[:i])
		s = s[i+len(vMacroOpen)+j+len(vMacroClose):]

		item, err := parseRefExpr(inside)
		if err != nil {
			return "", err
		}
		if attrMode && (item.HasConnector || item.Verb != "") {
			return "", fmt.Errorf("attribute macros take bare references only, got %q", strings.TrimSpace(inside))
		}
		text, err := item.render(resolve)
		if err != nil {
			return "", err
		}
		b.WriteString(text)
	}
	b.WriteString(s)
	return b.String(), nil
}

// HasVersionMacro reports whether s contains a macro opener — the law 1
// check: structural attributes must not carry one.
func HasVersionMacro(s string) bool { return strings.Contains(s, vMacroOpen) }

// --- positional format strings (version-spec format=, computed labels) ---

// splitVersionParams splits a params attribute on commas, honoring the
// single-quoted connector literals — a comma inside quotes does not split.
func splitVersionParams(s string) []string {
	var out []string
	var b strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '\'':
			quoted = !quoted
			b.WriteRune(r)
		case r == ',' && !quoted:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 || len(out) > 0 {
		out = append(out, b.String())
	}
	return out
}

// scanFormatVerbs extracts the fmt verbs of a format string, in order.
// %% is a literal percent, not a verb.
func scanFormatVerbs(format string) ([]string, error) {
	var verbs []string
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(format) && (format[j] == '.' || format[j] == '-' || format[j] == '+' ||
			format[j] == '#' || format[j] == '0' || (format[j] >= '1' && format[j] <= '9')) {
			j++
		}
		if j >= len(format) {
			return nil, fmt.Errorf("dangling %% in format %q", format)
		}
		verbs = append(verbs, format[i:j+1])
		i = j
	}
	return verbs, nil
}

// RenderVersionFormat renders a positional format string over its params
// list: verb count must match item count, and each verb must match its
// value's kind — validated before anything renders. The connector rule
// applies per item: an empty value renders as "" through %s (and is an
// error under a numeric verb, which cannot say "nothing").
func RenderVersionFormat(format, params string, resolve VersionResolver) (string, error) {
	items := splitVersionParams(params)
	verbs, err := scanFormatVerbs(format)
	if err != nil {
		return "", err
	}
	if len(verbs) != len(items) {
		return "", fmt.Errorf("format %q has %d verbs, params has %d items", format, len(verbs), len(items))
	}
	args := make([]any, 0, len(items))
	for n, raw := range items {
		item, err := parseRefExpr(raw)
		if err != nil {
			return "", err
		}
		if item.Verb != "" {
			return "", fmt.Errorf("params item %q carries a verb — the verb lives in the format string", strings.TrimSpace(raw))
		}
		v, err := resolve(item.Ref)
		if err != nil {
			return "", err
		}
		kind := verbs[n][len(verbs[n])-1]
		switch kind {
		case 'd', 'x', 'X', 'o', 'b':
			if !v.Numeric {
				return "", fmt.Errorf("format verb %q wants a number, %q is text", verbs[n], item.Ref.String())
			}
			if item.HasConnector {
				return "", fmt.Errorf("connector on numeric item %q — numbers cannot vanish", item.Ref.String())
			}
			args = append(args, v.Num)
		case 's', 'q':
			text := v.Text
			if item.HasConnector {
				if text == "" {
					text = "" // no value, no connector
				} else {
					text = item.Connector + text
				}
			}
			args = append(args, text)
		default:
			return "", fmt.Errorf("unsupported verb %q in format %q", verbs[n], format)
		}
	}
	return fmt.Sprintf(format, args...), nil
}
