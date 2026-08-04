package compose

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill"
)

// TestMacros expands the whole dictionary in a replace-macros section —
// name, id, type, view, a meta key of the section's own — leaves the
// unknown one in place with its warning, and never touches a section that
// did not ask.
func TestMacros(t *testing.T) {
	root := miniProject(t, "macros", map[string]string{
		"raw.md": "---\nmkskill:\n  pos: 10\n---\nLiteral <$$$msk.name$$$>.\n",
		"m.md": "---\ntitle: \"T\"\nmkskill:\n  pos: 20\n  replace-macros: true\n---\n" +
			"Unit <$$$msk.name$$$> (<$$$msk.id$$$>, <$$$msk.type$$$>) in <$$$msk.view$$$>: <$$$msk.meta.title$$$> and <$$$msk.ghost$$$>.\n",
	})

	var buf bytes.Buffer
	got, err := root.Project.Compose(&buf, view(t, "readme"))
	if err != nil {
		t.Fatal(err)
	}
	want := "Literal <$$$msk.name$$$>.\n\n" +
		"Unit mini (p, Other) in README.md: T and <$$$msk.ghost$$$>.\n"
	if got != want {
		t.Errorf("README.md = %q, want %q", got, want)
	}
	if !strings.Contains(buf.String(), `[p] WARN: m.md: unknown macro "<$$$msk.ghost$$$>" left as is`) {
		t.Errorf("log misses the unknown-macro warning:\n%s", buf.String())
	}

	// the view macro follows the view being composed
	agents, err := root.Project.Compose(&buf, view(t, "agents"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(agents, "in AGENTS.md:") {
		t.Errorf("AGENTS.md should expand its own view macro:\n%s", agents)
	}
}

// TestMetaTagMacros checks the fall-through: any key that is not a project
// attribute or front matter asks the unit's <meta> tag.
func TestMetaTagMacros(t *testing.T) {
	root := miniTree(t, "meta-macros",
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
			"<mkskill>\n"+
			"  <project id=\"p\" name=\"mini\" project-type=\"Other\">\n"+
			"    <meta><module>github.com/pablo-botella/mini</module><license>MIT</license></meta>\n"+
			"  </project>\n"+
			"</mkskill>\n",
		map[string]string{
			"_mkskill/src/a.md": "---\nmkskill:\n  pos: 10\n  replace-macros: true\n---\n" +
				"Module <$$$msk.module$$$>, license <$$$msk.license$$$>.\n",
			"_mkskill/src/b.md": "---\nlicense: Apache-2.0\nmkskill:\n  pos: 20\n  replace-macros: true\n---\n" +
				"Own front matter wins: <$$$msk.license$$$>.\n",
		})
	got, err := root.Project.Compose(io.Discard, view(t, "readme"))
	if err != nil {
		t.Fatal(err)
	}
	want := "Module github.com/pablo-botella/mini, license MIT.\n\n" +
		"Own front matter wins: Apache-2.0.\n"
	if got != want {
		t.Errorf("README.md = %q, want %q", got, want)
	}
}

// TestDerivedMacros checks the cooked ones: badge and install from the
// module (a Go-CLI child rides in, its module derived from path), and the
// usage straight from the root package.
func TestDerivedMacros(t *testing.T) {
	root := miniTree(t, "derived-macros",
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
			"<mkskill>\n"+
			"  <project id=\"p\" name=\"mini\" project-type=\"Go-Module\">\n"+
			"    <meta><module>github.com/pablo-botella/mini</module></meta>\n"+
			"    <child-project id=\"c\" name=\"tool\" path=\"./cmd/tool\" project-type=\"Go-CLI\"/>\n"+
			"  </project>\n"+
			"</mkskill>\n",
		map[string]string{
			"_mkskill/src/a.md": "---\nmkskill:\n  replace-macros: true\n---\n" +
				"<$$$msk.badge$$$>\n\n<$$$msk.install$$$>\n\n<$$$msk.skill.usage.short$$$>\n",
			"cmd/tool/_mkskill/src/b.md": "# Tool\n",
		})
	got, err := root.Project.Compose(io.Discard, view(t, "readme"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[![Go Reference](https://pkg.go.dev/badge/github.com/pablo-botella/mini.svg)](https://pkg.go.dev/github.com/pablo-botella/mini)",
		"```sh\ngo get github.com/pablo-botella/mini    # library\ngo install github.com/pablo-botella/mini/cmd/tool@latest    # CLI\n```",
		(&mkskill.Spec{}).Usage(false),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("README.md misses %q:\n%s", want, got)
		}
	}
}
