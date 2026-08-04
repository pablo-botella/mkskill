package compiler

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// Deploy writes this project's composed views under its own base — every
// unit gets its own output: README.md and AGENTS.md at the base itself,
// SKILL.md in .claude/skills/<name>/. The artifacts attribute says which
// views; global-artifacts adds the PARENT's views deployed here too, so a
// unit opened standalone still carries the whole project's docs. An empty
// view is never written. A preserved destination is honored by method:
// alias writes mkskill's version under the alias name, alt does not write
// at all (the sidelined copy under _mkskill/alt/files/ is still to come).
// Everything on the record through log.
func (p *Project) Deploy(log io.Writer) error {
	written := make(map[string]bool) // targets deployed this run: collisions warn, never pile up
	for i := range Views {
		v := &Views[i]
		if !p.DeploysView(v) {
			fmt.Fprintf(log, "[%s] deploy %s: not an artifact of this unit\n", p.Id, v.FileName)
			continue
		}
		doc, err := p.Compose(log, v)
		if err != nil {
			return err
		}
		if err := p.deployDoc(log, viewTarget(v, p), doc, v, written); err != nil {
			return err
		}
	}

	// the parent's views this unit also wants under its own base — never
	// the readme: that one belongs to the unit itself
	if !p.GlobalArtifacts.Empty() {
		for i := range Views {
			v := &Views[i]
			if v.Name == "readme" || !inArtifactList(p.GlobalArtifacts.Get(), v.Name) {
				continue
			}
			doc, err := p.Parent.Compose(log, v)
			if err != nil {
				return err
			}
			fmt.Fprintf(log, "[%s] global artifact %s: views of %s\n", p.Id, v.FileName, p.Parent.Id)
			if err := p.deployDoc(log, viewTarget(v, p.Parent), doc, v, written); err != nil {
				return err
			}
		}
	}
	return p.deployEmbed(log)
}

// viewTarget is the base-relative destination of a view: the skill keeps
// the owner's identity in its folder name, the rest are just their file.
func viewTarget(v *View, owner *Project) string {
	if v.Name == "skill" {
		return ".claude/skills/" + string(owner.Name) + "/" + v.FileName
	}
	return v.FileName
}

// inArtifactList tells whether a view name appears in a comma list.
func inArtifactList(list, name string) bool {
	for _, tok := range splitTrimmed(list) {
		if tok == name {
			return true
		}
	}
	return false
}

// deployDoc writes one composed document to its base-relative target,
// honoring preservation: alias redirects mkskill's version next to the
// original; alt (the default) sidelines it to the _mkskill/alt/files/
// mirror — the preserved file is never touched, but mkskill's version is
// always there to compare. A target already deployed this run is a
// collision: warning, the first write stands.
func (p *Project) deployDoc(log io.Writer, target, doc string, v *View, written map[string]bool) error {
	if doc == "" {
		fmt.Fprintf(log, "[%s] deploy %s: empty view, nothing written\n", p.Id, v.FileName)
		return nil
	}
	if pf := p.preservedFile(target); pf != nil {
		if pf.Item.Method.Get() == "alias" && !pf.Item.Alias.Empty() {
			alias := path.Join(path.Dir(target), pf.Item.Alias.Get()) // the alias lives next to the original
			fmt.Fprintf(log, "[%s] deploy %s: preserved, using alias %s\n", p.Id, target, alias)
			target = alias
		} else {
			mirror := _SkillFolder + "/alt/files/" + target // the original's path, mirrored
			fmt.Fprintf(log, "[%s] deploy %s: preserved (%s), mkskill's version to %s\n", p.Id, target, pf.Item.Method, mirror)
			if err := p.ensureAltFilesIgnore(); err != nil {
				return err
			}
			target = mirror
		}
	}
	if written[target] {
		fmt.Fprintf(log, "[%s] WARN: %s collides with an artifact already deployed, not written\n", p.Id, target)
		return nil
	}
	abs := filepath.Join(p.Base, filepath.FromSlash(target))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(abs, []byte(doc), 0o644); err != nil {
		return err
	}
	written[target] = true
	fmt.Fprintf(log, "[%s] deploy %s: %d bytes\n", p.Id, target, len(doc))
	return nil
}

// ensureAltFilesIgnore opens the alt/files compartment: the folder with its
// section .gitignore, written only the first time.
func (p *Project) ensureAltFilesIgnore() error {
	dir := filepath.Join(p.Base, _SkillFolder, "alt", "files")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gitignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignore); err != nil {
		return os.WriteFile(gitignore, []byte("*\n!.gitignore\n"), 0o644)
	}
	return nil
}

// preservedFile finds the preservation entry matching a base-relative
// target; nil when the target is free.
func (p *Project) preservedFile(target string) *PreservedFile {
	if p.Preserve == nil {
		return nil
	}
	for _, f := range p.Preserve.Files {
		if f.Path == target {
			return f
		}
	}
	return nil
}
