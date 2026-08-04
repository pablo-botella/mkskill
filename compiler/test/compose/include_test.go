package compose

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// miniTree writes a throwaway multi-unit tree under test/_out/compose:
// files are base-relative, the config comes verbatim — loads, scans and
// resolves it, ready to compose.
func miniTree(t *testing.T, name, config string, files map[string]string) *compiler.Root {
	t.Helper()
	base := filepath.Join("..", "_out", "compose", name)
	if err := os.RemoveAll(base); err != nil {
		t.Fatal(err)
	}
	files["_mkskill/mkskill.config.xml"] = config
	for file, content := range files {
		target := filepath.Join(base, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if err := root.Resolve(nil); err != nil {
		t.Fatal(err)
	}
	return root
}

const includeConfig = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
	"<mkskill>\n" +
	"  <project id=\"main\" name=\"demo\" project-type=\"Other\">\n" +
	"    <child-project id=\"cli\" name=\"tool\" path=\"./cmd/tool\" project-type=\"Other\"/>\n" +
	"  </project>\n" +
	"</mkskill>\n"

// TestInclude nests the child unit's composition into the main document:
// its headings come demoted one level — except inside code fences, which
// stay verbatim — right after the including section's own body.
func TestInclude(t *testing.T) {
	root := miniTree(t, "include", includeConfig, map[string]string{
		"_mkskill/src/intro.md":        "---\nmkskill:\n  pos: 10\n---\n# Demo\n\nIntro.\n",
		"_mkskill/src/tools.md":        "---\nmkskill:\n  pos: 20\n  include: cli\n---\n# CLI\n",
		"cmd/tool/_mkskill/src/cli.md": "# Tool\n\n```\n# not a heading\n```\n\n## Flags\n\n-v.\n",
	})

	var buf bytes.Buffer
	got, err := root.Project.Compose(&buf, view(t, "readme"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Demo\n\nIntro.\n\n# CLI\n\n## Tool\n\n```\n# not a heading\n```\n\n### Flags\n\n-v.\n"
	if got != want {
		t.Errorf("README.md = %q, want %q", got, want)
	}
	if !strings.Contains(buf.String(), "[main] include cli at tools.md (README.md)") {
		t.Errorf("log misses the include line:\n%s", buf.String())
	}
}

// TestIncludeErrors checks that a dangling include and an include cycle
// stop the composition with their own errors.
func TestIncludeErrors(t *testing.T) {
	t.Run("unknown-unit", func(t *testing.T) {
		root := miniTree(t, "include-unknown", includeConfig, map[string]string{
			"_mkskill/src/a.md": "---\nmkskill:\n  include: ghost\n---\n# A\n",
		})
		_, err := root.Project.Compose(io.Discard, view(t, "readme"))
		if err == nil || !strings.Contains(err.Error(), `include "ghost": no unit with that id`) {
			t.Errorf("want the no-unit error, got %v", err)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		root := miniTree(t, "include-cycle", includeConfig, map[string]string{
			"_mkskill/src/a.md":          "---\nmkskill:\n  include: cli\n---\n# A\n",
			"cmd/tool/_mkskill/src/b.md": "---\nmkskill:\n  include: main\n---\n# B\n",
		})
		_, err := root.Project.Compose(io.Discard, view(t, "readme"))
		if err == nil || !strings.Contains(err.Error(), "include cycle") {
			t.Errorf("want the cycle error, got %v", err)
		}
	})
}
