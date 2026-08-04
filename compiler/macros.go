package compiler

import (
	"fmt"
	"io"
	"strings"

	"github.com/pablo-botella/mkskill"
)

const macroOpen = "<$$$msk."
const macroClose = "$$$>"

// expandMacros replaces the <$$$msk.…$$$> macros in a section body. A
// plain key resolves with one precedence: the section's own front matter
// first, the project attributes (name, id, type, description) next, the
// unit's <meta> tag last — no whitelist, whatever is declared answers.
// msk.view is the view being composed and msk.meta.X reads the front
// matter explicitly. Only sections asking for it (replace-macros) get
// here. An unknown macro warns and stays in the text — a visible hole
// beats a silent one.
func expandMacros(log io.Writer, p *Project, v *View, sec *Section, body string) string {
	var b strings.Builder
	for {
		i := strings.Index(body, macroOpen)
		if i < 0 {
			break
		}
		j := strings.Index(body[i+len(macroOpen):], macroClose)
		if j < 0 {
			break // an opener that never closes: literal text
		}
		key := body[i+len(macroOpen) : i+len(macroOpen)+j]
		whole := body[i : i+len(macroOpen)+j+len(macroClose)]
		b.WriteString(body[:i])
		body = body[i+len(whole):]

		value, ok := "", false
		switch {
		case key == "view":
			value, ok = v.FileName, true // run context, nobody else's
		case strings.HasPrefix(key, "meta."):
			value, ok = sec.Meta[strings.TrimPrefix(key, "meta.")] // explicit: front matter only
		default:
			value, ok = metaLookup(p, sec, key)
			if !ok {
				value, ok = derivedMacro(p, sec, key) // cooked only when nobody declared it
			}
		}
		if !ok {
			fmt.Fprintf(log, "[%s] WARN: %s: unknown macro %q left as is\n", p.Id, secName(sec), whole)
			b.WriteString(whole)
			continue
		}
		b.WriteString(value)
	}
	b.WriteString(body)
	return b.String()
}

// metaLookup resolves a plain key with the one precedence: the section's
// own front matter, then the structural project attributes (name, id,
// type), then the unit's <meta> tag — where everything else lives,
// description included. Whatever is declared answers — no whitelist.
func metaLookup(p *Project, sec *Section, key string) (string, bool) {
	if v, hit := sec.Meta[key]; hit {
		return v, true
	}
	switch key {
	case "name":
		return string(p.Name), true
	case "id":
		return string(p.Id), true
	case "type":
		return string(p.ProjectType), true
	}
	if v := p.MetaValue(key); v != "" {
		return v, true
	}
	return "", false
}

// derivedMacro cooks the computed macros — the ones nobody stores: the Go
// Reference badge and the install block from the module, and the generate
// commands' usage from the root package itself (in sync by construction).
func derivedMacro(p *Project, sec *Section, key string) (string, bool) {
	switch key {
	case "badge":
		module, _ := metaLookup(p, sec, "module")
		if module == "" {
			return "", false
		}
		return fmt.Sprintf("[![Go Reference](https://pkg.go.dev/badge/%s.svg)](https://pkg.go.dev/%s)", module, module), true
	case "install":
		return installBlock(p, sec)
	case "skill.usage.short":
		return (&mkskill.Spec{}).Usage(false), true
	case "skill.usage.full":
		return (&mkskill.Spec{}).Usage(true), true
	}
	return "", false
}

// installBlock cooks the msk.install snippet: go get for the library plus
// one go install per Go-CLI unit hanging below — a child without its own
// <module> derives it from this one's module and its path.
func installBlock(p *Project, sec *Section) (string, bool) {
	module, _ := metaLookup(p, sec, "module")
	if module == "" {
		return "", false
	}
	lines := []string{"go get " + module + "    # library"}
	units := append([]*Project{}, p.Children...)
	for i := 0; i < len(units); i++ { // the list is its own worklist
		c := units[i]
		units = append(units, c.Children...)
		if c.ProjectType != "Go-CLI" {
			continue
		}
		cm := c.MetaValue("module")
		if cm == "" && c.Path != "" {
			cm = module + "/" + strings.TrimPrefix(strings.TrimPrefix(string(c.Path), "./"), "/")
		}
		if cm == "" {
			continue
		}
		lines = append(lines, "go install "+cm+"@latest    # CLI")
	}
	return "```sh\n" + strings.Join(lines, "\n") + "\n```", true
}
