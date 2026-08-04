package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill"
)

// demoSpec is a host's Spec as the generated embed would fill it: composed
// documents, ready to write.
func demoSpec() mkskill.Spec {
	return mkskill.Spec{
		Name:        "demo",
		Description: "does the demo thing",
		Readme:      "# Demo\n\nreadme body\n",
		Agents:      "# Demo\n\nagents body\n",
		Skill:       "---\nname: demo\n---\n\n# Demo\n\nskill body\n",
	}
}

// outDir gives a clean per-test corner under test/_out.
func outDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join("..", "_out", "spec", name)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestUsage checks both shapes of the shared help block: the compact header
// and the detailed one — flag-shaped names, no trailing newline.
func TestUsage(t *testing.T) {
	var s mkskill.Spec // the method needs no spec data: usable from any host
	short := s.Usage(false)
	if !strings.Contains(short, "-generate-claude-skill  [-dst f] [-global] [-force]") {
		t.Errorf("compact usage misses the aligned skill line:\n%s", short)
	}
	if strings.Contains(short, "write the") {
		t.Errorf("compact usage should carry no descriptions:\n%s", short)
	}
	if strings.HasSuffix(short, "\n") {
		t.Error("usage must not end with a newline — the host positions it")
	}
	full := s.Usage(true)
	if !strings.Contains(full, "\n    write the agent-agnostic AGENTS.md") {
		t.Errorf("detailed usage misses a description line:\n%s", full)
	}
}

// TestDispatch checks the routing: one dash, two dashes or none all reach
// the command; anything unknown falls through for the host.
func TestDispatch(t *testing.T) {
	dir := outDir(t, "dispatch")
	s := demoSpec()
	for i, cmd := range []string{"generate-readme", "-generate-readme", "--generate-readme"} {
		dst := filepath.Join(dir, "readme", string(rune('a'+i))+".md")
		handled, err := s.Dispatch(cmd, []string{"-dst", dst})
		if !handled || err != nil {
			t.Errorf("%s: handled=%v err=%v", cmd, handled, err)
		}
	}
	if handled, err := s.Dispatch("serve", nil); handled || err != nil {
		t.Errorf("a host command must fall through: handled=%v err=%v", handled, err)
	}
}

// TestGenerateReadme checks the write cycle: content verbatim, the -force
// lock against overwriting, and the no-document error.
func TestGenerateReadme(t *testing.T) {
	dir := outDir(t, "readme")
	dst := filepath.Join(dir, "README.md")
	s := demoSpec()

	if err := s.RunGenerateReadme([]string{"-dst", dst}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != s.Readme {
		t.Errorf("written readme = %q, want %q", data, s.Readme)
	}

	err = s.RunGenerateReadme([]string{"-dst", dst})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("want the already-exists lock, got %v", err)
	}
	if err := s.RunGenerateReadme([]string{"-dst", dst, "-force"}); err != nil {
		t.Errorf("-force should overwrite: %v", err)
	}

	empty := mkskill.Spec{Name: "demo"}
	err = empty.RunGenerateReadme([]string{"-dst", filepath.Join(dir, "x.md")})
	if err == nil || !strings.Contains(err.Error(), "carries no composed README.md") {
		t.Errorf("want the no-document error, got %v", err)
	}
}

// TestGenerateClaudeSkill checks the skill's own rules: the default
// destination under .claude/skills/<name>/, -global vs -dst exclusion, and
// the no-name error.
func TestGenerateClaudeSkill(t *testing.T) {
	dir := outDir(t, "skill")
	s := demoSpec()

	dst := filepath.Join(dir, "SKILL.md")
	if err := s.RunGenerateClaudeSkill([]string{"-dst", dst}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != s.Skill {
		t.Errorf("written skill = %q, want %q", data, s.Skill)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(abs)
	if err := s.RunGenerateClaudeSkill(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".claude", "skills", "demo", "SKILL.md")); err != nil {
		t.Error("default destination .claude/skills/demo/SKILL.md missing")
	}

	err = s.RunGenerateClaudeSkill([]string{"-global", "-dst", "x"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("want the -global/-dst exclusion, got %v", err)
	}
	noName := mkskill.Spec{Skill: "s"}
	if err := noName.RunGenerateClaudeSkill(nil); err == nil || !strings.Contains(err.Error(), "no name") {
		t.Errorf("want the no-name error, got %v", err)
	}
}

// TestCheckParams checks the one-call host integration over os.Args: ran
// and done, not-mine fall-through, and recognised-but-failed.
func TestCheckParams(t *testing.T) {
	dir := outDir(t, "checkparams")
	s := demoSpec()
	saved := os.Args
	defer func() { os.Args = saved }()

	dst := filepath.Join(dir, "AGENTS.md")
	os.Args = []string{"host", "-generate-agent-docs", "-dst", dst}
	if err, done := s.CheckParams(); err != nil || !done {
		t.Errorf("run+done expected, got err=%v done=%v", err, done)
	}

	os.Args = []string{"host"}
	if err, done := s.CheckParams(); err != nil || done {
		t.Errorf("bare call must fall through, got err=%v done=%v", err, done)
	}
	os.Args = []string{"host", "serve", "-x"}
	if err, done := s.CheckParams(); err != nil || done {
		t.Errorf("a host command must fall through, got err=%v done=%v", err, done)
	}

	os.Args = []string{"host", "-generate-agent-docs", "-dst", dst} // exists now, no -force
	if err, done := s.CheckParams(); err == nil || done {
		t.Errorf("recognised-but-failed expected, got err=%v done=%v", err, done)
	}
}
