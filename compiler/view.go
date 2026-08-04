package compiler

import "strings"

// View is one output document: a name, the file it becomes, and the
// content stream it drinks from — human (readme) or AI (ai*). A view's
// peculiarities (the SKILL.md wrapper, its folder) are compose/deploy
// business, never content selection: sections only speak in streams.
type View struct {
	Name     string // canonical name
	FileName string // the file this view becomes
	Ai       bool   // drinks from the ai* stream instead of readme
	Wrap     bool   // gets the skill front matter wrapper (name/description)
}

// Views is the canonical list: three outputs over two content streams —
// README.md drinks from readme; AGENTS.md and SKILL.md both drink ai*, so
// they carry the same sections and differ only in wrapping and destination.
var Views = []View{
	{Name: "readme", FileName: "README.md"},
	{Name: "agents", FileName: "AGENTS.md", Ai: true},
	{Name: "skill", FileName: "SKILL.md", Ai: true, Wrap: true},
}

// knownInToken tells whether a token speaks the selection language:
// * (every view), readme (the human stream), ai* (the AI stream — the
// views of today and whatever AI format joins later).
func knownInToken(tok string) bool {
	switch tok {
	case "*", "readme", "ai*":
		return true
	}
	return false
}

// splitTrimmed splits a comma list into its trimmed, non-empty tokens.
func splitTrimmed(list string) []string {
	var tokens []string
	for _, tok := range strings.Split(list, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			tokens = append(tokens, tok)
		}
	}
	return tokens
}

// DeploysView tells whether this unit deploys the given view: the explicit
// artifacts list decides ("*" takes all); without one, the root deploys
// everything and a child only its README — one project, one skill, one
// AGENTS.
func (p *Project) DeploysView(v *View) bool {
	if p.Artifacts.Empty() {
		return !p.IsChildProject() || v.Name == "readme"
	}
	for _, tok := range splitTrimmed(p.Artifacts.Get()) {
		if tok == "*" || tok == v.Name {
			return true
		}
	}
	return false
}

// InView tells whether the section belongs to the view: any one of its
// comma-separated in tokens matching decides. Unknown tokens never match —
// they already warned when the section was resolved.
func (sec *Section) InView(v *View) bool {
	for _, tok := range strings.Split(sec.In, ",") {
		switch strings.TrimSpace(tok) {
		case "*":
			return true
		case "readme":
			if !v.Ai {
				return true
			}
		case "ai*":
			if v.Ai {
				return true
			}
		}
	}
	return false
}
