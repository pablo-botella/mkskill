// Package mkskill is the minimal face a host application imports: its docs
// come already composed (the compiler package renders them at build time
// and the generated embed carries them in the binary); this package only
// handles the parameters — recognizing the generate commands on the host's
// command line, writing or installing the documents, and lending the usage
// text for the host's own help.
package mkskill

// The documentation is not automatic — this makes it: `go generate ./...`
// rebuilds every artifact from _mkskill/, and the golden test fails when
// anything went stale. The second line propagates the version-spec's
// destinations (__publish.bat and friends) without touching the version.
//go:generate go run ./cmd/mkskill -q build
//go:generate go run ./cmd/mkskill -q -vbuild

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Spec is everything the host carries: its name, the trigger description,
// and the composed documents — ready to write, never rendered here.
type Spec struct {
	Name        string // skill name; also the skill directory name
	Description string // the trigger text Claude uses to decide when to load the skill
	Version     string // the release this build claims to be — embed-version's render; empty when the unit opted out
	Readme      string // the composed README.md
	Agents      string // the composed AGENTS.md
	Skill       string // the composed SKILL.md, wrapper included
}

// commandUsage is the single source for the help text Usage returns — add a
// command, flag, or description here and every host's usage stays in sync.
var commandUsage = []struct{ Name, Flags, Desc string }{
	{"-generate-claude-skill", "[-dst f] [-global] [-force]", "write the Claude Code SKILL.md (-global installs ~/.claude/skills/<name>/SKILL.md)"},
	{"-generate-agent-docs", "[-dst f] [-force]", "write the agent-agnostic AGENTS.md"},
	{"-generate-readme", "[-dst f] [-force]", "write the README.md"},
}

// Usage returns mkskill's command help for a host to embed in its own help,
// so it stays in sync as mkskill gains commands or flags. detail=false is
// the compact header (one aligned "name  flags" line per command);
// detail=true adds a description line under each. Both have no leading
// indent or trailing newline — the host positions them. It is a method of
// Spec so a host's main needs no mkskill import of its own: the generated
// embed already brings the type, and MkskillSpec.Usage(true) just works.
func (s *Spec) Usage(detail bool) string {
	_ = s // avoid "unused" when the host doesn't use the spec
	w := 0
	for _, c := range commandUsage {
		if len(c.Name) > w {
			w = len(c.Name)
		}
	}
	var b strings.Builder
	for i, c := range commandUsage {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%-*s  %s", w, c.Name, c.Flags)
		if detail {
			fmt.Fprintf(&b, "\n    %s", c.Desc)
		}
	}
	return b.String()
}

// Dispatch routes the generate commands. They are flag-shaped
// (-generate-claude-skill, one or two dashes) so they never collide with
// the host's own subcommands; the bare form is accepted too. It returns
// handled=false for anything else so a host CLI can fall through to its
// own commands:
//
//	if ok, err := spec.Dispatch(cmd, args); ok { return err }
func (s Spec) Dispatch(cmd string, args []string) (handled bool, err error) {
	switch strings.TrimLeft(cmd, "-") {
	case "generate-claude-skill":
		return true, s.RunGenerateClaudeSkill(args)
	case "generate-agent-docs":
		return true, s.RunGenerateAgentDocs(args)
	case "generate-readme":
		return true, s.RunGenerateReadme(args)
	}
	return false, nil
}

// CheckParams is the one-call host integration: it inspects os.Args and
// runs a generate command if the program was invoked as one. It returns
// done=true when a generate command ran successfully (the host should
// stop), err!=nil when a generate command was recognised but failed, and
// both zero when os.Args is not a generate command (so the host falls
// through to its own argument handling):
//
//	if err, done := Skill.CheckParams(); done {
//		return
//	} else if err != nil {
//		fmt.Fprintf(os.Stderr, "error: %v\n", err)
//		os.Exit(1)
//	}
//	// …not a mkskill command: the host handles os.Args itself…
func (s Spec) CheckParams() (err error, done bool) {
	if len(os.Args) < 2 {
		return nil, false
	}
	handled, e := s.Dispatch(os.Args[1], os.Args[2:])
	if !handled {
		return nil, false
	}
	return e, e == nil
}

// RunGenerateClaudeSkill parses [-dst f] [-global] [-force] and writes the
// composed SKILL.md. Default destination is the project-local
// .claude/skills/<name>/SKILL.md; -global writes
// ~/.claude/skills/<name>/SKILL.md (available from every project, no
// elevation — it is under the user's own home); -global and -dst are
// mutually exclusive.
func (s Spec) RunGenerateClaudeSkill(args []string) error {
	if s.Name == "" {
		return fmt.Errorf("generate-claude-skill: spec has no name")
	}
	if s.Skill == "" {
		return fmt.Errorf("generate-claude-skill: spec carries no composed SKILL.md")
	}
	fs := flag.NewFlagSet("generate-claude-skill", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dst := fs.String("dst", "", "destination path (default: .claude/skills/<name>/SKILL.md)")
	global := fs.Bool("global", false, "install into ~/.claude/skills/<name>/SKILL.md (available from every project)")
	force := fs.Bool("force", false, "overwrite an existing destination file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := *dst
	switch {
	case *global && target != "":
		return fmt.Errorf("generate-claude-skill: -global and -dst are mutually exclusive")
	case *global:
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("generate-claude-skill: cannot resolve home dir for -global: %w", err)
		}
		target = filepath.Join(home, ".claude", "skills", s.Name, "SKILL.md")
	case target == "":
		target = filepath.Join(".claude", "skills", s.Name, "SKILL.md")
	}
	return writeGenerated(target, s.Skill, *force)
}

// RunGenerateAgentDocs parses [-dst f] [-force] and writes the composed
// AGENTS.md (default destination AGENTS.md).
func (s Spec) RunGenerateAgentDocs(args []string) error {
	if s.Agents == "" {
		return fmt.Errorf("generate-agent-docs: spec carries no composed AGENTS.md")
	}
	fs := flag.NewFlagSet("generate-agent-docs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dst := fs.String("dst", "AGENTS.md", "destination path")
	force := fs.Bool("force", false, "overwrite an existing destination file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return writeGenerated(*dst, s.Agents, *force)
}

// RunGenerateReadme parses [-dst f] [-force] and writes the composed
// README.md (default destination README.md).
func (s Spec) RunGenerateReadme(args []string) error {
	if s.Readme == "" {
		return fmt.Errorf("generate-readme: spec carries no composed README.md")
	}
	fs := flag.NewFlagSet("generate-readme", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dst := fs.String("dst", "README.md", "destination path")
	force := fs.Bool("force", false, "overwrite an existing destination file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return writeGenerated(*dst, s.Readme, *force)
}

// writeGenerated writes content to dst (creating parent dirs), refusing to
// overwrite an existing file unless force is set.
func writeGenerated(dst, content string, force bool) error {
	if !force {
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("destination %s already exists (use -force to overwrite)", dst)
		}
	}
	if dir := filepath.Dir(dst); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", dst)
	return nil
}
