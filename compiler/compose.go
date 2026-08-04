package compiler

import (
	"fmt"
	"io"
	"strings"
)

// Compose renders one view of this project: the sections that belong to it,
// in their resolved order, joined by a blank line — the raw document. A
// section with an include nests the referenced unit's own composition of
// the same view right after its body, headings demoted one level per depth.
// Macros and the view's wrapping come later; deploy decides where the
// document lands. An empty view composes to the empty string. An include
// pointing to no unit, or a cycle of includes, is an error.
func (p *Project) Compose(log io.Writer, v *View) (string, error) {
	doc, err := p.composeWorker(log, v, map[string]bool{string(p.Id): true})
	if err != nil || doc == "" {
		return doc, err
	}
	if v.Wrap {
		// the skill front matter: always both lines; the description lives
		// in the <meta> tag — and defaults to the name, never an empty hole
		description := p.MetaValue("description")
		if description == "" {
			description = string(p.Name)
		}
		doc = "---\nname: " + yamlScalar(string(p.Name)) + "\ndescription: " + yamlScalar(description) + "\n---\n\n" + doc
		fmt.Fprintf(log, "[%s] wrap %s: skill front matter\n", p.Id, v.FileName)
	}
	return doc, nil
}

// composeWorker is the recursive worker: busy holds the ids along the include
// path — the same unit twice in the path is a cycle; twice as siblings is
// fine.
func (p *Project) composeWorker(log io.Writer, v *View, busy map[string]bool) (string, error) {
	var parts []string
	for _, sec := range p.Sections {
		if !sec.InView(v) {
			continue
		}
		body := strings.Trim(sec.Body, "\r\n") // bare edges; the inside stays verbatim
		if sec.ReplaceMacros {
			body = expandMacros(log, p, v, sec, body)
		}
		if body != "" {
			parts = append(parts, body)
		}
		if sec.Include == "" {
			continue
		}
		other := p.Root.ProjectMap[sec.Include]
		if other == nil {
			return "", fmt.Errorf("[%s] %s: include %q: no unit with that id", p.Id, secName(sec), sec.Include)
		}
		if busy[string(other.Id)] {
			return "", fmt.Errorf("[%s] %s: include %q: include cycle", p.Id, secName(sec), sec.Include)
		}
		busy[string(other.Id)] = true
		inc, err := other.composeWorker(log, v, busy)
		if err != nil {
			return "", err
		}
		delete(busy, string(other.Id))
		if inc = strings.Trim(demoteHeadings(inc), "\r\n"); inc != "" {
			parts = append(parts, inc)
			fmt.Fprintf(log, "[%s] include %s at %s (%s)\n", p.Id, sec.Include, secName(sec), v.FileName)
		}
	}
	if len(parts) == 0 {
		fmt.Fprintf(log, "[%s] compose %s: empty\n", p.Id, v.FileName)
		return "", nil
	}
	doc := strings.Join(parts, "\n\n") + "\n"
	fmt.Fprintf(log, "[%s] compose %s: %d sections, %d bytes\n", p.Id, v.FileName, len(parts), len(doc))
	return doc, nil
}

// demoteHeadings pushes every markdown heading one level down (# → ##,
// ###### stays — there is no deeper), leaving fenced code blocks alone.
func demoteHeadings(doc string) string {
	lines := strings.Split(doc, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "```"):
			fence = "```"
		case strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
		case strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "######"):
			lines[i] = "#" + line
		}
	}
	return strings.Join(lines, "\n")
}
