package meta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

const config = `<?xml version="1.0" encoding="UTF-8"?>
<mkskill>
  <project id="m" name="demo" project-type="Other">
    <meta>
      <!-- anything goes in here; mkskill models none of it -->
      <module>github.com/pablo-botella/demo</module>
      <license>MIT</license>
      <homepage>https://example.org</homepage>
    </meta>
  </project>
</mkskill>
`

// load writes the config into a throwaway tree and loads it.
func load(t *testing.T, name, cfg string) *compiler.Root {
	t.Helper()
	base := filepath.Join("..", "_out", "meta", name)
	if err := os.RemoveAll(base); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "_mkskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "_mkskill", "mkskill.config.xml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestMetaValues reads the free tags by name — and absence, at every level,
// reads as the empty string.
func TestMetaValues(t *testing.T) {
	root := load(t, "values", config)
	p := root.Project
	if got := p.MetaValue("module"); got != "github.com/pablo-botella/demo" {
		t.Errorf("module = %q", got)
	}
	if got := p.MetaValue("license"); got != "MIT" {
		t.Errorf("license = %q", got)
	}
	if got := p.MetaValue("nothing"); got != "" {
		t.Errorf("unknown tag should read empty, got %q", got)
	}
	noMeta := load(t, "no-meta",
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<mkskill><project id=\"m\" name=\"demo\" project-type=\"Other\"/></mkskill>\n")
	if got := noMeta.Project.MetaValue("module"); got != "" {
		t.Errorf("no <meta> at all should read empty, got %q", got)
	}
}

// TestMetaRoundtrip saves the config back and checks the whole <meta> block
// survives — tags, values and the comment mkskill never modeled.
func TestMetaRoundtrip(t *testing.T) {
	root := load(t, "roundtrip", config)
	dst := filepath.Join("..", "_out", "meta", "roundtrip", "saved.xml")
	if err := root.Save(nil, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	saved := string(data)
	for _, want := range []string{
		"<meta>",
		"<module>github.com/pablo-botella/demo</module>",
		"<license>MIT</license>",
		"<homepage>https://example.org</homepage>",
		"<!-- anything goes in here; mkskill models none of it -->",
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved config misses %q:\n%s", want, saved)
		}
	}
}
