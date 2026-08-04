package version

// The Expandable type over EVERY claimed value attribute: all of them
// expand, the renders feed the consumers, and a rewrite emits every raw
// — no template ever freezes. The universe's own sources (id, name,
// path, project-type) are the one exception, and they fail LOUDLY.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

const expandableDoc = `<?xml version="1.0" encoding="UTF-8"?>
<mkskill>
  <label key="stem">demo</label>
  <label key="views">readme,agents,skill</label>
  <version-spec format="v%d.%d.%d" params="major:byte,minor:[0-99],build:word">
    <version major="0" minor="5" build="3" lock="major">
      <ts></ts>
    </version>
    <label key="title" volatile="true" default-to-ts="true">seed</label>
  </version-spec>
  <project id="main" name="demo" project-type="Go-Module" artifacts="{$$$ glb:label:views $$$}">
    <import-miniskin content="{$$$ glb:label:stem $$$}-src" fm-gen="extern"/>
    <preserve>
      <item file="{$$$ glb:label:stem $$$}-README.md" method="alias" alias="{$$$ ver:version $$$}-readme.md"/>
    </preserve>
    <child-project id="cli" name="demo-cli" path="./cmd/demo" project-type="Go-CLI">
      <embed filename="{$$$ glb:label:stem $$$}_embed.go" module-name="{$$$ glb:label:stem $$$}main" embed-parent="*" embed-version="{$$$ ver:version $$$}"/>
    </child-project>
  </project>
</mkskill>`

func loadRoot(t *testing.T, base string) *compiler.Root {
	t.Helper()
	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEveryAttributeExpands(t *testing.T) {
	base := writeConfig(t, expandableDoc)
	root := loadRoot(t, base)
	main, cli := root.Project, root.Project.Children[0]

	cases := []struct{ name, got, want string }{
		{"artifacts", main.Artifacts.Get(), "readme,agents,skill"},
		{"import content", main.ImportMiniskin[0].ContentFolder.Get(), "demo-src"},
		{"fm-gen", main.ImportMiniskin[0].FrontMatterGen.Get(), "extern"},
		{"preserve file", main.Preserve.Items[0].File.Get(), "demo-README.md"},
		{"preserve method", main.Preserve.Items[0].Method.Get(), "alias"},
		{"preserve alias", main.Preserve.Items[0].Alias.Get(), "v0.5.3-readme.md"},
		{"embed filename", cli.Embed.Filename.Get(), "demo_embed.go"},
		{"embed module-name", cli.Embed.ModuleName.Get(), "demomain"},
		{"embed embed-parent", cli.Embed.EmbedParent.Get(), "*"},
		{"embed embed-version", cli.Embed.EmbedVersion.Get(), "v0.5.3"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	// the family holds every non-empty adopter — traversable, nothing hides
	if len(root.Family.All) != len(cases) {
		t.Errorf("family size = %d, want %d", len(root.Family.All), len(cases))
	}

	// the value carries its explanation: a macro-bearing Expandable keeps
	// the TraceInfo of its own resolution, with its node named
	tr := strings.Join(main.Artifacts.TraceInfo, "\n")
	if !strings.Contains(tr, `glb:label:views = "readme,agents,skill"`) {
		t.Errorf("artifacts trace incomplete:\n%s", tr)
	}
	if len(cli.Embed.EmbedVersion.TraceInfo) == 0 {
		t.Error("embed-version resolved without keeping its trace")
	}
	// Where names the node AND its line — the value points at the document
	if got := cli.Embed.EmbedVersion.Where(); !strings.HasPrefix(got, "prj:cli <embed embed-version> (line ") {
		t.Errorf("Where() = %q", got)
	}

	// and the family reports everything the load resolved, in one read
	rep := strings.Join(root.Family.Report(), "\n")
	for _, want := range []string{
		"prj:main <project artifacts> (line ",
		`= "{$$$ glb:label:views $$$}" -> "readme,agents,skill"`,
		"prj:cli <embed embed-version> (line ",
		`= "{$$$ ver:version $$$}" -> "v0.5.3"`,
		`  ver:version = "v0.5.3"`,
	} {
		if !strings.Contains(rep, want) {
			t.Errorf("family report misses %q:\n%s", want, rep)
		}
	}
}

func TestRewriteFreezesNothing(t *testing.T) {
	base := writeConfig(t, expandableDoc)
	root := loadRoot(t, base)

	out := filepath.Join(base, "rewritten.xml")
	if err := root.Save(nil, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	// every template survives as declared…
	for _, raw := range []string{
		`artifacts="{$$$ glb:label:views $$$}"`,
		`content="{$$$ glb:label:stem $$$}-src"`,
		`file="{$$$ glb:label:stem $$$}-README.md"`,
		`alias="{$$$ ver:version $$$}-readme.md"`,
		`filename="{$$$ glb:label:stem $$$}_embed.go"`,
		`module-name="{$$$ glb:label:stem $$$}main"`,
		`embed-version="{$$$ ver:version $$$}"`,
	} {
		if !strings.Contains(doc, raw) {
			t.Errorf("rewrite lost the template %q", raw)
		}
	}
	// …and no render leaked into the document
	for _, frozen := range []string{
		`artifacts="readme,agents,skill"`, `content="demo-src"`,
		`filename="demo_embed.go"`, `embed-version="v0.5.3"`,
	} {
		if strings.Contains(doc, frozen) {
			t.Errorf("rewrite froze a render: %q", frozen)
		}
	}
}

func TestSourcesRefuseMacrosLoudly(t *testing.T) {
	// law 1: a macro in a universe source is an ERROR — silence would
	// confuse (the user typed a macro and nothing happened)
	for _, bad := range []struct{ name, doc string }{
		{"name", strings.Replace(expandableDoc, `name="demo-cli"`, `name="{$$$ glb:label:stem $$$}-cli"`, 1)},
		{"path", strings.Replace(expandableDoc, `path="./cmd/demo"`, `path="./cmd/{$$$ glb:label:stem $$$}"`, 1)},
		{"project-type", strings.Replace(expandableDoc, `project-type="Go-CLI"`, `project-type="{$$$ glb:label:stem $$$}"`, 1)},
	} {
		base := writeConfig(t, bad.doc)
		root := &compiler.Root{ProjectBase: base}
		err := root.Load()
		if err == nil {
			t.Errorf("%s with a macro did not error", bad.name)
			continue
		}
		if !strings.Contains(err.Error(), "law 1") {
			t.Errorf("%s error does not cite law 1: %v", bad.name, err)
		}
	}
}

// TestRenderValidations locks the promised post-expansion checks: the
// grammar attributes validate their RENDER, paths stay inside the tree,
// and a typo is an error — never a silent different behavior.
func TestRenderValidations(t *testing.T) {
	for _, c := range []struct{ name, from, to string }{
		{"embed-parent typo", `embed-parent="*"`, `embed-parent="agent"`},
		{"embed-parent mixed list", `embed-parent="*"`, `embed-parent="readme,typo"`},
		{"embed filename escape", `filename="{$$$ glb:label:stem $$$}_embed.go"`, `filename="../x_embed.go"`},
		{"embed filename not go", `filename="{$$$ glb:label:stem $$$}_embed.go"`, `filename="x.txt"`},
		{"embed without filename", `filename="{$$$ glb:label:stem $$$}_embed.go" module-name`, `module-name`},
		{"embed-parent on the root", `<import-miniskin content="{$$$ glb:label:stem $$$}-src" fm-gen="extern"/>`,
			`<import-miniskin content="{$$$ glb:label:stem $$$}-src" fm-gen="extern"/>
    <embed filename="root_embed.go" embed-parent="readme"/>`},
		{"module-name invalid", `module-name="{$$$ glb:label:stem $$$}main"`, `module-name="1bad"`},
		{"preserve method typo", `method="alias"`, `method="alis"`},
		{"preserve alias with path", `alias="{$$$ ver:version $$$}-readme.md"`, `alias="../readme.md"`},
		{"fm-gen typo", `fm-gen="extern"`, `fm-gen="external"`},
		{"import content escape", `content="{$$$ glb:label:stem $$$}-src"`, `content="../src"`},
	} {
		doc := strings.Replace(expandableDoc, c.from, c.to, 1)
		if doc == expandableDoc {
			t.Fatalf("%s: replacement %q not found", c.name, c.from)
		}
		base := writeConfig(t, doc)
		root := &compiler.Root{ProjectBase: base}
		if err := root.Load(); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}

	// and the child path is refused at the parse itself
	doc := strings.Replace(expandableDoc, `path="./cmd/demo"`, `path="../outside"`, 1)
	base := writeConfig(t, doc)
	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err == nil || !strings.Contains(err.Error(), "child path") {
		t.Errorf("escaping child path not refused at parse: %v", err)
	}
}

func TestExpandableUnresolvedFailsAtLoad(t *testing.T) {
	bad := strings.Replace(expandableDoc, "{$$$ ver:version $$$}", "{$$$ ver:label:nope $$$}", 1)
	base := writeConfig(t, bad)
	root := &compiler.Root{ProjectBase: base}
	if err := root.Load(); err == nil {
		t.Error("unresolvable expandable did not fail the load")
	}
}

func TestExpandableWithoutMacroCostsNothing(t *testing.T) {
	// a config with no macros at all never touches the version universe
	plain := `<?xml version="1.0" encoding="UTF-8"?>
<mkskill>
  <project id="main" name="demo" project-type="Go-Module">
    <child-project id="cli" name="demo-cli" path="./cmd/demo" project-type="Go-CLI">
      <embed filename="demo_embed.go" module-name="main" embed-parent="*"/>
    </child-project>
  </project>
</mkskill>`
	base := writeConfig(t, plain)
	root := loadRoot(t, base) // no version-spec, and no error: nothing to resolve
	if got := root.Project.Children[0].Embed.Filename.Get(); got != "demo_embed.go" {
		t.Errorf("plain value = %q", got)
	}
}
