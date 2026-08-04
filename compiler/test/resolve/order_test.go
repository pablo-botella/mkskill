package resolve

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// miniProject writes a throwaway single-project tree under test/_out/order
// with the given src files, loads and scans it — ready to resolve.
func miniProject(t *testing.T, name string, files map[string]string) *compiler.Root {
	t.Helper()
	base := filepath.Join("..", "_out", "order", name)
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
	return root
}

// TestOrderWeave checks the full ordering: the weighted flow (pos, 500 for
// the unset) with the anchored sections woven in — a before in front of its
// anchor, an after behind it, and a chain following its root.
func TestOrderWeave(t *testing.T) {
	root := miniProject(t, "weave", map[string]string{
		"a.md": "---\nmkskill:\n  pos: 10\n---\n# A\n",
		"b.md": "---\nmkskill:\n  after: a.md\n---\n# B\n",
		"c.md": "---\nmkskill:\n  before: a.md\n---\n# C\n",
		"d.md": "# D\n",
		"e.md": "---\nmkskill:\n  pos: 20\n---\n# E\n",
		"f.md": "---\nmkskill:\n  after: b.md\n---\n# F\n",
	})
	var buf bytes.Buffer
	if err := root.Resolve(&buf); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sec := range root.Project.Sections {
		got = append(got, sec.Item.DstFileName)
	}
	want := "c.md a.md b.md f.md e.md d.md"
	if strings.Join(got, " ") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
	if !strings.Contains(buf.String(), "[p] order: c.md a.md b.md f.md e.md d.md") {
		t.Errorf("log misses the order line:\n%s", buf.String())
	}
}

// TestOrderErrors checks that every ordering offense stops the phase with
// its own error: a missing anchor, an anchor cycle, a duplicate explicit
// pos, and after+before on the same section.
func TestOrderErrors(t *testing.T) {
	cases := []struct {
		name    string
		wantErr string
		files   map[string]string
	}{
		{
			name:    "missing-anchor",
			wantErr: `anchor "ghost.md" not found`,
			files: map[string]string{
				"x.md": "---\nmkskill:\n  after: ghost.md\n---\n# X\n",
			},
		},
		{
			name:    "cycle",
			wantErr: "anchor cycle",
			files: map[string]string{
				"x.md": "---\nmkskill:\n  after: y.md\n---\n# X\n",
				"y.md": "---\nmkskill:\n  after: x.md\n---\n# Y\n",
			},
		},
		{
			name:    "duplicate-pos",
			wantErr: "duplicate pos 10",
			files: map[string]string{
				"x.md": "---\nmkskill:\n  pos: 10\n---\n# X\n",
				"y.md": "---\nmkskill:\n  pos: 10\n---\n# Y\n",
			},
		},
		{
			name:    "after-and-before",
			wantErr: "after and before together",
			files: map[string]string{
				"x.md": "---\nmkskill:\n  after: y.md\n  before: y.md\n---\n# X\n",
				"y.md": "# Y\n",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := miniProject(t, tc.name, tc.files)
			err := root.Resolve(nil)
			if err == nil {
				t.Fatalf("resolve should fail with %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to carry %q", err, tc.wantErr)
			}
		})
	}
}
