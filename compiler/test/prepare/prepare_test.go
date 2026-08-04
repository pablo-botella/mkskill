package prepare

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestPrepare materializes the demo fixture — on a working copy under
// test/_out, never on the tracked fixture — and checks every behavior with
// its log line: the copy with its generated .fm, the preserved destination
// left untouched, the hand-edited .fm respected, and the two flavors of the
// missing-source warning.
func TestPrepare(t *testing.T) {
	base := filepath.Join("..", "_out", "prepare", "demo")
	if err := os.RemoveAll(filepath.Join("..", "_out", "prepare")); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join("..", "_data", "demo"), base)

	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}

	// run 1: fresh copy — notes gets copied and its .fm generated; the
	// preserved demo-content destination is not written
	logs := scanAndPrepare(t, root)
	notesMD := filepath.Join(base, "_mkskill", "src", "notes", "notes.md")
	notesFM := filepath.Join(base, "_mkskill", "src", "notes", "notes.fm")
	if !strings.Contains(mustRead(t, notesMD), "# Demo notes") {
		t.Error("notes.md not copied from its source")
	}
	if got := mustRead(t, notesFM); got != "mkskill:\n  pos: 30\n" {
		t.Errorf("notes.fm generated wrong: %q", got)
	}
	if _, err := os.Stat(filepath.Join(base, "_mkskill", "src", "demo-content.md")); err == nil {
		t.Error("preserved demo-content.md was written")
	}
	for _, want := range []string{
		"[main] copy src/generated/notes.md -> _mkskill/src/notes/notes.md",
		"[main] fm: generated _mkskill/src/notes/notes.fm",
		"[main] preserve: _mkskill/src/demo-content.md is protected (alt), not written",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("log misses %q:\n%s", want, logs)
		}
	}

	// run 2: the .fm was hand-edited — fm-gen ",preserve" keeps it
	if err := os.WriteFile(notesFM, []byte("custom: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logs = scanAndPrepare(t, root)
	if got := mustRead(t, notesFM); got != "custom: true\n" {
		t.Errorf("hand-edited notes.fm was regenerated: %q", got)
	}
	if !strings.Contains(logs, "[main] fm: existing _mkskill/src/notes/notes.fm preserved") {
		t.Errorf("log misses the fm-preserved line:\n%s", logs)
	}

	// run 3: source gone, copy present — warns and keeps the copy
	if err := os.Remove(filepath.Join(base, "src", "generated", "notes.md")); err != nil {
		t.Fatal(err)
	}
	logs = scanAndPrepare(t, root)
	if !strings.Contains(logs, "[main] WARN: source src/generated/notes.md missing, using the existing copy _mkskill/src/notes/notes.md") {
		t.Errorf("log misses the existing-copy warning:\n%s", logs)
	}
	if _, err := os.Stat(notesMD); err != nil {
		t.Error("the existing copy disappeared")
	}

	// run 4: source and copy both gone — warns and skips
	if err := os.Remove(notesMD); err != nil {
		t.Fatal(err)
	}
	logs = scanAndPrepare(t, root)
	if !strings.Contains(logs, "[main] WARN: source src/generated/notes.md missing, item _mkskill/src/notes/notes.md skipped") {
		t.Errorf("log misses the skipped warning:\n%s", logs)
	}
}

// scanAndPrepare runs the two phases with a fresh log and hands it back.
func scanAndPrepare(t *testing.T, root *compiler.Root) string {
	t.Helper()
	var buf bytes.Buffer
	if err := root.Scan(&buf); err != nil {
		t.Fatal(err)
	}
	if err := root.Prepare(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
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

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
