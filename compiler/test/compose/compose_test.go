package compose

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// miniProject writes a throwaway single-project tree under test/_out/compose
// with the given src files, loads, scans and resolves it — ready to compose.
func miniProject(t *testing.T, name string, files map[string]string) *compiler.Root {
	t.Helper()
	base := filepath.Join("..", "_out", "compose", name)
	if err := os.RemoveAll(base); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(base, "_mkskill", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<mkskill><project id=\"p\" name=\"mini\" project-type=\"Other\"/></mkskill>\n"
	if err := os.WriteFile(filepath.Join(base, "_mkskill", "mkskill.config.xml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(srcDir, file), []byte(content), 0o644); err != nil {
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

// view finds one of the canonical views by name.
func view(t *testing.T, name string) *compiler.View {
	t.Helper()
	for i := range compiler.Views {
		if compiler.Views[i].Name == name {
			return &compiler.Views[i]
		}
	}
	t.Fatalf("no view named %s", name)
	return nil
}

// TestCompose renders the three canonical views of a project whose sections
// speak every selection dialect: the * default, one stream, the other, and
// both by comma — checking each document verbatim, order included.
func TestCompose(t *testing.T) {
	root := miniProject(t, "views", map[string]string{
		"a.md": "# A\n\nBody A.\n",
		"b.md": "---\nmkskill:\n  pos: 10\n  in: readme\n---\n# B\n",
		"c.md": "---\nmkskill:\n  pos: 20\n  in: ai*\n---\n# C\n",
		"d.md": "---\nmkskill:\n  pos: 5\n  in: readme,ai*\n---\n# D\n",
	})
	p := root.Project

	var buf bytes.Buffer
	for _, tc := range []struct{ view, want string }{
		{"readme", "# D\n\n# B\n\n# A\n\nBody A.\n"},
		{"agents", "# D\n\n# C\n\n# A\n\nBody A.\n"},
		{"skill", "---\nname: mini\ndescription: mini\n---\n\n# D\n\n# C\n\n# A\n\nBody A.\n"},
	} {
		got, err := p.Compose(&buf, view(t, tc.view))
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.view, got, tc.want)
		}
	}
	if !strings.Contains(buf.String(), "[p] compose README.md: 3 sections") {
		t.Errorf("log misses the compose line:\n%s", buf.String())
	}
}

// TestComposeEmpty checks that a view nobody targets composes to nothing —
// and says so in the log.
func TestComposeEmpty(t *testing.T) {
	root := miniProject(t, "empty", map[string]string{
		"a.md": "---\nmkskill:\n  in: readme\n---\n# A\n",
	})
	var buf bytes.Buffer
	got, err := root.Project.Compose(&buf, view(t, "agents"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("AGENTS.md should be empty, got %q", got)
	}
	if !strings.Contains(buf.String(), "[p] compose AGENTS.md: empty") {
		t.Errorf("log misses the empty line:\n%s", buf.String())
	}
}
