package resolve

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestResolve runs the whole chain — Load, Scan, Prepare, Resolve — on a
// working copy of the demo fixture and checks the sections it yields: the
// internal fenced header (typed fields + Meta, body stripped), the sibling
// .fm, the no-front-matter file with its first line reconstituted, the
// harvested item resolved from its materialized copy, the preserved
// destination yielding no section, and the unknown-key warning.
func TestResolve(t *testing.T) {
	base := filepath.Join("..", "_out", "resolve", "demo")
	if err := os.RemoveAll(filepath.Join("..", "_out", "resolve")); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join("..", "_data", "demo"), base)

	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if err := root.Prepare(nil); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := root.Resolve(&buf); err != nil {
		t.Fatal(err)
	}
	logs := buf.String()

	main := root.ProjectMap["main"]
	cli := root.ProjectMap["cli"]
	if main == nil || cli == nil {
		t.Fatalf("ProjectMap misses main/cli: %v", root.ProjectMap)
	}
	if len(main.Sections) != 4 {
		t.Fatalf("main sections = %d, want 4 (overview, usage, notes, extra)", len(main.Sections))
	}
	for i, want := range []string{"overview.md", "usage.md", "notes.md", "extra.md"} {
		if got := main.Sections[i].Item.DstFileName; got != want {
			t.Errorf("section %d is %s, want %s (pos 10, 20, 30, unset=500)", i, got, want)
		}
	}
	if len(cli.Sections) != 1 {
		t.Fatalf("cli sections = %d, want 1", len(cli.Sections))
	}

	// overview: internal fenced header — typed pos, title to Meta, body clean
	overview := findSection(t, main, "overview.md")
	if overview.Pos != 10 {
		t.Errorf("overview.Pos = %d, want 10", overview.Pos)
	}
	if overview.Meta["title"] != "Demo overview" {
		t.Errorf("overview.Meta[title] = %q, want unquoted scalar", overview.Meta["title"])
	}
	if overview.In != "*" {
		t.Errorf("overview.In = %q, want the * default", overview.In)
	}
	if strings.Contains(overview.Body, "---") || strings.Contains(overview.Body, "title:") {
		t.Errorf("overview body still carries front matter:\n%s", overview.Body)
	}
	if !strings.Contains(overview.Body, "# Demo overview") {
		t.Errorf("overview body lost its content:\n%s", overview.Body)
	}

	// usage: the front matter lives in the sibling .fm; the .md is all body
	usage := findSection(t, main, "usage.md")
	if usage.Pos != 20 {
		t.Errorf("usage.Pos = %d, want 20 (from usage.fm)", usage.Pos)
	}
	if !strings.HasPrefix(usage.Body, "# Usage") {
		t.Errorf("usage body should be the whole .md:\n%s", usage.Body)
	}

	// extra: no front matter at all — the consumed first line comes back
	extra := findSection(t, main, "extra.md")
	if !strings.HasPrefix(extra.Body, "# Extra") {
		t.Errorf("extra body lost its first line:\n%s", extra.Body)
	}
	if !strings.Contains(extra.Body, "subfolder") {
		t.Errorf("extra body lost its content:\n%s", extra.Body)
	}
	if extra.Pos != 0 || len(extra.Meta) != 0 {
		t.Errorf("extra should carry no directives: pos=%d meta=%v", extra.Pos, extra.Meta)
	}

	// notes: harvested from miniskin, resolved from its materialized copy
	notes := findSection(t, main, "notes.md")
	if notes.Pos != 30 {
		t.Errorf("notes.Pos = %d, want 30 (from the generated .fm)", notes.Pos)
	}
	if !strings.Contains(notes.Body, "# Demo notes") {
		t.Errorf("notes body lost its content:\n%s", notes.Body)
	}

	// the child keeps its own
	if !strings.HasPrefix(cli.Sections[0].Body, "# Tool CLI") {
		t.Errorf("cli section body wrong:\n%s", cli.Sections[0].Body)
	}

	for _, want := range []string{
		"[main] WARN: _mkskill/src/demo-content.md not materialized, no section",
		`[main] WARN: _mkskill/src/overview.md: unknown mkskill key "weight" ignored`,
		"[main] resolve _mkskill/src/overview.md: 6 fm lines",
		"[main] resolve _mkskill/src/notes/notes.md: 2 fm lines",
		"[cli] resolve _mkskill/src/cli.md",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("log misses %q:\n%s", want, logs)
		}
	}

	// idempotence: a SECOND full run finds last run's materializations in
	// _mkskill/src — the harvest supersedes them, sections never double
	buf.Reset()
	if err := root.Scan(&buf); err != nil {
		t.Fatal(err)
	}
	if err := root.Prepare(&buf); err != nil {
		t.Fatal(err)
	}
	if err := root.Resolve(&buf); err != nil {
		t.Fatalf("second run must stay clean: %v", err)
	}
	if len(root.ProjectMap["main"].Sections) != 4 {
		t.Errorf("second run sections = %d, want 4 (no doubles)", len(root.ProjectMap["main"].Sections))
	}
	if !strings.Contains(buf.String(), "superseded by the harvest, dropped") {
		t.Errorf("log misses the superseded native:\n%s", buf.String())
	}
}

// findSection locates a section by its item's file name.
func findSection(t *testing.T, p *compiler.Project, name string) *compiler.Section {
	t.Helper()
	for _, sec := range p.Sections {
		if sec.Item.DstFileName == name {
			return sec
		}
	}
	t.Fatalf("no section for %s", name)
	return nil
}

// copyTree clones the fixture into the working area, file by file.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
