package tips

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestWriteTips drops the starter recipes for the demo tree and checks each
// unit got the recipe of its type, names filled in, with the section
// .gitignore in place.
func TestWriteTips(t *testing.T) {
	base := filepath.Join("..", "_out", "tips", "demo")
	if err := os.RemoveAll(filepath.Join("..", "_out", "tips")); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join("..", "_data", "demo"), base)

	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := root.WriteTips(&buf); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(base, "_mkskill", "alt", "tips")
	if got := mustRead(t, filepath.Join(dir, ".gitignore")); got != "*\n!.gitignore\n" {
		t.Errorf(".gitignore wrong: %q", got)
	}
	main := mustRead(t, filepath.Join(dir, "main-Go-Module.md"))
	if !strings.Contains(main, "# Starter recipe — demo (main, Go library)") {
		t.Errorf("main tip misses its filled title:\n%s", main)
	}
	if !strings.Contains(main, ".claude/skills/demo/SKILL.md") {
		t.Errorf("main tip misses the name substitution:\n%s", main)
	}
	cli := mustRead(t, filepath.Join(dir, "cli-Go-CLI.md"))
	if !strings.Contains(cli, "MkskillSpec.CheckParams()") {
		t.Errorf("cli tip misses the main wiring:\n%s", cli)
	}
	if !strings.Contains(cli, `filename="demo_embed.go"`) {
		t.Errorf("cli tip should use the declared embed filename:\n%s", cli)
	}
	for _, want := range []string{
		"[main] tip _mkskill/alt/tips/main-Go-Module.md",
		"[cli] tip _mkskill/alt/tips/cli-Go-CLI.md",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log misses %q:\n%s", want, buf.String())
		}
	}
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
