package compiler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WriteTips drops a starter recipe for every unit under the root's
// _mkskill/alt/tips/ — cheat sheets by project-type, regenerated on demand,
// never part of the build. The folder gets its section .gitignore the first
// time, as every alt/ compartment does.
func (c *Root) WriteTips(log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	dir := filepath.Join(c.ProjectBase, _SkillFolder, "alt", "tips")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gitignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignore); err != nil {
		if err := os.WriteFile(gitignore, []byte("*\n!.gitignore\n"), 0o644); err != nil {
			return err
		}
	}
	for _, p := range c.GetProjectList() {
		file := string(p.Id) + "-" + string(p.ProjectType) + ".md"
		if err := os.WriteFile(filepath.Join(dir, file), []byte(tipFor(p)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(log, "[%s] tip _mkskill/alt/tips/%s\n", p.Id, file)
	}
	return nil
}

// tipFor picks the recipe by project-type and fills in the unit's names.
func tipFor(p *Project) string {
	var t string
	switch p.ProjectType {
	case "Go-CLI":
		t = tipGoCli
	case "Go-Module", "Go":
		t = tipGoLibrary
	case "JS-Bundle":
		t = tipJsBundle
	default:
		t = tipOther
	}
	embedFile := "{name}_generated_embed.go"
	if p.Embed != nil && !p.Embed.Filename.Empty() {
		embedFile = p.Embed.Filename.Get()
	}
	path := string(p.Path)
	if path == "" {
		path = "./cmd/" + string(p.Name) // a suggestion; a real child brings its own
	}
	return strings.NewReplacer(
		"{name}", string(p.Name),
		"{id}", string(p.Id),
		"{path}", path,
		"{embed-file}", strings.ReplaceAll(embedFile, "{name}", string(p.Name)),
	).Replace(t)
}

const tipGoCli = `# Starter recipe — {name} ({id}, Go-CLI)

A CLI binary can carry the whole project's documentation and act in its
own name: generate and install its docs with no mkskill around.

## 1. Declare the embed in the config

    <child-project id="{id}" name="{name}" path="{path}" project-type="Go-CLI">
      <embed filename="{embed-file}" module-name="main" embed-parent="*"/>
    </child-project>

embed-parent="*" means the binary embeds the PARENT's composed views —
installing this tool's skill documents the whole project.

## 2. Build the docs

    mkskill build

This deploys every artifact and generates {embed-file} plus the
mkskill_docs/ folder next to it (//go:embed feeds the Spec from there).

The documentation is not automatic — make it: wire the build into
go generate (e.g. in your main.go) so "go generate ./..." refreshes it,

    //go:generate go run github.com/pablo-botella/mkskill/cmd/mkskill build

and consider a golden test that composes in memory and fails when a
deployed file went stale — CI then keeps the docs honest.

## 3. Wire your main

    if err, done := MkskillSpec.CheckParams(); done {
        return
    } else if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
    // …not a mkskill command: your own argument handling…

## 4. Lend the usage

Embed MkskillSpec.Usage(true) in your own help text so the generate
commands stay documented as mkskill gains commands or flags — no mkskill
import needed: the generated embed already brings the type.

## 5. Install the skill

    {name} -generate-claude-skill -global

Writes ~/.claude/skills/<name>/SKILL.md — available from every project.
`

const tipGoLibrary = `# Starter recipe — {name} ({id}, Go library)

A library documents itself with mkskill alone — no binary, no dependency:
_mkskill/ is invisible to the Go toolchain and nothing here touches go.mod.

## 1. Write the sections

    _mkskill/src/*.md — one file per section, markdown first. The engine's
    directives ride the front matter, namespaced and invisible on compose:

    ---
    mkskill:
      pos: 10
      in: "*"
    ---

    ## Overview
    …

## 2. Build the docs

    mkskill build

README.md, AGENTS.md and .claude/skills/{name}/SKILL.md come out composed
from the same sections — they cannot drift apart.

The documentation is not automatic — make it: put the build behind
go generate (e.g. in a doc.go),

    //go:generate go run github.com/pablo-botella/mkskill/cmd/mkskill build

and consider a golden test that fails when a deployed file went stale.

## 3. Install the skill globally (no binary needed)

    mkskill -C . -generate-claude-skill -global

The repo speaks for itself: composed on the fly, installed under
~/.claude/skills/{name}/.
`

const tipJsBundle = `# Starter recipe — {name} ({id}, JS bundle)

No Go here — mkskill does everything from outside; the project only
carries its _mkskill/ folder.

## 1. Write the sections under _mkskill/src/*.md

## 2. Build the docs

    mkskill build          # or: mkskill -C <this folder> build

The documentation is not automatic — wire "mkskill build" into whatever
runs your builds (an npm script, a make target) so it refreshes itself.

## 3. Install the skill globally

    mkskill -C . -generate-claude-skill -global

Tune what this unit deploys with artifacts="readme,agents,skill" on its
<project> element.
`

const tipOther = `# Starter recipe — {name} ({id})

The generic path: content in _mkskill/src/*.md, one section per file,
directives in the mkskill: front matter block (pos, in, after/before,
include, replace-macros), views selected with in: "*" | readme | ai*.

    mkskill build                                  write every artifact
    mkskill check                                  dry look: warnings and order
    mkskill -C . -generate-claude-skill -global    install this repo's skill
`
