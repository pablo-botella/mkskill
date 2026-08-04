package compose

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestSkillWrapper checks the skill view's front matter: the name from the
// project attribute, the description from its <meta> home — and only
// SKILL.md gets wrapped.
func TestSkillWrapper(t *testing.T) {
	root := miniTree(t, "wrapper",
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
			"<mkskill><project id=\"p\" name=\"mini\" project-type=\"Other\">\n"+
			"  <meta><description>Does the demo thing.</description></meta>\n"+
			"</project></mkskill>\n",
		map[string]string{
			"_mkskill/src/a.md": "# A\n\nBody A.\n",
		})

	var buf bytes.Buffer
	got, err := root.Project.Compose(&buf, view(t, "skill"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nname: mini\ndescription: \"Does the demo thing.\"\n---\n\n# A\n\nBody A.\n"
	if got != want {
		t.Errorf("SKILL.md = %q, want %q", got, want)
	}
	if !strings.Contains(buf.String(), "[p] wrap SKILL.md: skill front matter") {
		t.Errorf("log misses the wrap line:\n%s", buf.String())
	}

	agents, err := root.Project.Compose(&buf, view(t, "agents"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(agents, "---") {
		t.Errorf("AGENTS.md must not be wrapped:\n%s", agents)
	}
}

// TestSkillWrapperMetaDescription checks the middle step of the chain: no
// attribute, but a <meta><description> tag — legality by readability.
func TestSkillWrapperMetaDescription(t *testing.T) {
	root := miniTree(t, "wrapper-meta",
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
			"<mkskill><project id=\"p\" name=\"mini\" project-type=\"Other\">\n"+
			"  <meta><description>From the meta tag.</description></meta>\n"+
			"</project></mkskill>\n",
		map[string]string{
			"_mkskill/src/a.md": "# A\n",
		})
	got, err := root.Project.Compose(io.Discard, view(t, "skill"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "---\nname: mini\ndescription: \"From the meta tag.\"\n---\n\n") {
		t.Errorf("SKILL.md wrapper misses the meta description:\n%q", got)
	}
}

// TestSkillWrapperDefaultDescription checks the fallback: no description
// attribute in the config, the name steps in — never an empty hole.
func TestSkillWrapperDefaultDescription(t *testing.T) {
	root := miniProject(t, "wrapper-default", map[string]string{
		"a.md": "# A\n",
	})
	got, err := root.Project.Compose(io.Discard, view(t, "skill"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "---\nname: mini\ndescription: mini\n---\n\n") {
		t.Errorf("SKILL.md wrapper wrong:\n%q", got)
	}
}
