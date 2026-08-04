package compiler

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/pablo-botella/fmlines"
)

// Section is one piece of content, resolved from a SourceItem: the markdown
// body plus its attributes, already interpreted and typed — what ordering
// and view routing work with. Resolving is where raw front matter lines
// become meaning; everything before this (scan, prepare) never opened the
// content.
type Section struct {
	Item *SourceItem // the source this section was resolved from
	Body string      // the markdown content, front matter stripped

	// the mkskill: directives, clear and typed
	Pos           int    // ordering weight (1 = top … 999 = bottom); 0 = unset
	After         string // order right after that section file (anchor by filename)
	Before        string // order right before that section file
	In            string // target views; "*" (the default) means all of them
	ReplaceMacros bool   // expand the <$$$msk.…$$$> macros in the body
	Include       string // nest another unit's content here (e.g. "cmd/x"), headings demoted

	Meta map[string]string // the top-level front matter keys — each section keeps its own, repeats across files never clash
}

// secName is the section's file in the anchor namespace: path/name when it
// lives in a subfolder, the bare name otherwise.
func secName(s *Section) string {
	if s.Item.DstPath != "" {
		return s.Item.DstPath + "/" + s.Item.DstFileName
	}
	return s.Item.DstFileName
}

// findAnchor resolves an after/before reference to its section: a bare
// name matches by file name, one with a slash matches path/name. Not found
// or found twice is an error.
func findAnchor(unit, anchor string, secs []*Section) (*Section, error) {
	var found *Section
	for _, s := range secs {
		name := s.Item.DstFileName
		if strings.Contains(anchor, "/") {
			name = secName(s)
		}
		if name != anchor {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("[%s] anchor %q is ambiguous: %s and %s", unit, anchor, secName(found), secName(s))
		}
		found = s
	}
	if found == nil {
		return nil, fmt.Errorf("[%s] anchor %q not found", unit, anchor)
	}
	return found, nil
}

// orderSections puts the sections in their final order: first the weighted
// flow — pos 1 = top … 999 = bottom, 500 for the unset, scan order among
// equals — then every after/before section woven right next to its anchor
// (an anchored anchor drags its own followers along). Duplicate explicit
// pos, a missing or ambiguous anchor, after+before together, and anchor
// chains that never reach the flow (cycles) are errors.
func orderSections(log io.Writer, unit string, secs []*Section) ([]*Section, error) {
	afters := make(map[*Section][]*Section)
	befores := make(map[*Section][]*Section)
	posSeen := make(map[int]*Section)
	var flow []*Section

	for _, s := range secs {
		switch {
		case s.After != "" && s.Before != "":
			return nil, fmt.Errorf("[%s] %s: after and before together, pick one", unit, secName(s))
		case s.After != "":
			a, err := findAnchor(unit, s.After, secs)
			if err != nil {
				return nil, fmt.Errorf("%w (after in %s)", err, secName(s))
			}
			if s.Pos != 0 {
				fmt.Fprintf(log, "[%s] WARN: %s: pos ignored, the section is anchored\n", unit, secName(s))
			}
			afters[a] = append(afters[a], s)
		case s.Before != "":
			a, err := findAnchor(unit, s.Before, secs)
			if err != nil {
				return nil, fmt.Errorf("%w (before in %s)", err, secName(s))
			}
			if s.Pos != 0 {
				fmt.Fprintf(log, "[%s] WARN: %s: pos ignored, the section is anchored\n", unit, secName(s))
			}
			befores[a] = append(befores[a], s)
		default:
			if s.Pos != 0 {
				if other := posSeen[s.Pos]; other != nil {
					return nil, fmt.Errorf("[%s] duplicate pos %d: %s and %s", unit, s.Pos, secName(other), secName(s))
				}
				posSeen[s.Pos] = s
			}
			flow = append(flow, s)
		}
	}

	weight := func(s *Section) int {
		if s.Pos != 0 {
			return s.Pos
		}
		return 500
	}
	sort.SliceStable(flow, func(i, j int) bool { return weight(flow[i]) < weight(flow[j]) })

	// weave: emit each flow section with its befores in front and its
	// afters behind, recursively — a chain hangs together from its root
	out := make([]*Section, 0, len(secs))
	var emit func(s *Section)
	emit = func(s *Section) {
		for _, b := range befores[s] {
			emit(b)
		}
		out = append(out, s)
		for _, a := range afters[s] {
			emit(a)
		}
	}
	for _, s := range flow {
		emit(s)
	}

	if len(out) != len(secs) {
		emitted := make(map[*Section]bool, len(out))
		for _, s := range out {
			emitted[s] = true
		}
		var stuck []string
		for _, s := range secs {
			if !emitted[s] {
				stuck = append(stuck, secName(s))
			}
		}
		return nil, fmt.Errorf("[%s] anchor cycle, these sections never reach the flow: %s", unit, strings.Join(stuck, ", "))
	}

	names := make([]string, len(out))
	for i, s := range out {
		names[i] = secName(s)
	}
	fmt.Fprintf(log, "[%s] order: %s\n", unit, strings.Join(names, " "))
	return out, nil
}

// fmValue cleans one raw front matter scalar for interpretation: fmlines
// hands it verbatim; mkskill — the consumer — trims and unquotes it here.
func fmValue(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if u, err := strconv.Unquote(v); err == nil {
			return u
		}
	}
	return v
}

// applyFm interprets the front matter lines: the top-level mkskill: block
// becomes the section's typed fields — anything unrecognized inside it is a
// warning, never a stop — and every other top-level key: value goes to
// Meta, each section keeping its own.
func (sec *Section) applyFm(log io.Writer, unit, file string, lines *fmlines.FmLines) {
	for _, l := range *lines {
		if l.Parent != nil {
			continue // nested lines belong to their section, not to us
		}
		if l.Type == fmlines.FmLineKeyValue {
			if sec.Meta == nil {
				sec.Meta = make(map[string]string)
			}
			sec.Meta[strings.TrimSpace(l.Name())] = fmValue(l.Value())
			continue
		}
		if l.Type != fmlines.FmLineSection ||
			strings.TrimSpace(l.Name()) != "mkskill" || l.Children == nil {
			continue
		}
		for _, c := range *l.Children {
			if c.Type != fmlines.FmLineKeyValue {
				fmt.Fprintf(log, "[%s] WARN: %s: unrecognized line %d in mkskill block: %s\n",
					unit, file, c.LineNum, strings.TrimSpace(c.RawLine))
				continue
			}
			key := strings.TrimSpace(c.Name())
			value := fmValue(c.Value())
			switch key {
			case "pos":
				n, err := strconv.Atoi(value)
				if err != nil {
					fmt.Fprintf(log, "[%s] WARN: %s: mkskill pos %q is not a number, ignored\n", unit, file, value)
					continue
				}
				if n < 1 || n > 999 {
					fmt.Fprintf(log, "[%s] WARN: %s: mkskill pos %d out of range 1..999, ignored\n", unit, file, n)
					continue
				}
				sec.Pos = n
			case "after":
				sec.After = value
			case "before":
				sec.Before = value
			case "in":
				if value != "" {
					sec.In = value
					for _, tok := range strings.Split(value, ",") {
						if !knownInToken(strings.TrimSpace(tok)) {
							fmt.Fprintf(log, "[%s] WARN: %s: unknown in view %q\n", unit, file, strings.TrimSpace(tok))
						}
					}
				}
			case "replace-macros", "replace_macros": // both spellings live in the wild
				b, err := strconv.ParseBool(value)
				if err != nil {
					fmt.Fprintf(log, "[%s] WARN: %s: mkskill replace-macros %q is not a bool, ignored\n", unit, file, value)
					continue
				}
				sec.ReplaceMacros = b
			case "include":
				sec.Include = value
			default:
				fmt.Fprintf(log, "[%s] WARN: %s: unknown mkskill key %q ignored\n", unit, file, key)
			}
		}
	}
}
