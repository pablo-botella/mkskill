package version

// Round-trip fidelity of the version subsystem's two writers. The
// contract: what is not ours comes out intact — comments, foreign
// attributes and elements, templates, the epilog — and repeated trips
// reach a byte fixpoint (corruption would accumulate; stability proves
// it cannot).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

const richDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!-- prolog: announces the document -->
<mkskill data-foreign="keep" другой="attr">
  <!-- a global label -->
  <label key="copyright">Pablo Botella Navarro</label>
  <version-spec format="v%d.%d.%d" params="major:byte,minor:[0-99],build:word">
    <version major="0" minor="5" build="3" lock="major">
      <ts>2026-07-01T00:00:00Z</ts>
    </version>
    <label key="title" volatile="true" default-to-ts="true">seed title</label>
    <label key="desc" format="%s%s" params="ver:version,' - ' + label:title" when="label:title"></label>
    <update file="/.build/next-release.json" type="json">
      <entry key="tag">{$$$ version $$$}</entry>
      <entry key="title">{$$$ label:desc $$$}</entry>
    </update>
    <update file="src/app.h">
      <entry>
        #define APP_VERSION "{$$$ version $$$}"
      </entry>
    </update>
    <create file="__publish.bat" git-ignore="true" eol="crlf">
git tag {$$$ version $$$}
</create>
  </version-spec>
  <custom-tool answer="42">
    <inner>foreign text stays</inner>
  </custom-tool>
  <project id="main" name="demo" project-type="Go-Module">
    <meta>
      <module>github.com/x/demo</module>
      <!-- meta comment -->
      <license>MIT</license>
    </meta>
    <child-project id="cli" name="demo-cli" path="./cmd/demo" project-type="Go-CLI">
      <embed filename="demo_embed.go" module-name="main" embed-parent="*" embed-version="{$$$ ver:version $$$}"/>
    </child-project>
  </project>
  <version-history max="3">
    <release version="v0.5.3" ts="2026-07-30T00:00:00Z" title="prev" method="vinc-build"/>
  </version-history>
</mkskill>
<!-- epilog: after the root -->`

// everything the writers must never lose, claimed and foreign alike
var richBody = []string{
	"<!-- prolog: announces the document -->",
	`data-foreign="keep"`, `другой="attr"`,
	"<!-- a global label -->", ">Pablo Botella Navarro<",
	`format="v%d.%d.%d"`, `params="major:byte,minor:[0-99],build:word"`,
	`lock="major"`, ">2026-07-01T00:00:00Z<",
	"seed title", `when="label:title"`,
	"{$$$ version $$$}", "{$$$ label:desc $$$}", "#define APP_VERSION",
	`git-ignore="true"`, `eol="crlf"`,
	`answer="42"`, "foreign text stays",
	"<!-- meta comment -->", ">MIT<",
	`embed-version="{$$$ ver:version $$$}"`,
	`max="3"`, `version="v0.5.3"`, `method="vinc-build"`,
	"<!-- epilog: after the root -->",
}

func assertBody(t *testing.T, label, doc string) {
	t.Helper()
	for _, want := range richBody {
		if !strings.Contains(doc, want) {
			t.Errorf("%s lost %q", label, want)
		}
	}
}

func plantRichDest(t *testing.T, base string) {
	t.Helper()
	for rel, content := range map[string]string{
		".build/next-release.json": "{\n  \"tag\": \"v0.5.3\",\n  \"title\": \"x\"\n}\n",
		"src/app.h":                "#define APP_VERSION \"v0.5.3\"\n",
	} {
		path := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestVersionDocRoundTrip: the mutation path, loading and saving WITHOUT
// mutating. Trip 1 may normalize (stdlib forms); trips 2 and 3 must be
// byte-identical — the fixpoint — and nothing of the body may be lost,
// document order included.
func TestVersionDocRoundTrip(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "config.xml")
	if err := os.WriteFile(cfg, []byte(richDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	trip := func(in, out string) string {
		t.Helper()
		doc, err := compiler.VersionDocLoad(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := doc.Save(out); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	one := trip(cfg, filepath.Join(base, "t1.xml"))
	two := trip(filepath.Join(base, "t1.xml"), filepath.Join(base, "t2.xml"))
	three := trip(filepath.Join(base, "t2.xml"), filepath.Join(base, "t3.xml"))

	assertBody(t, "trip 1", one)
	if two != three {
		t.Error("no fixpoint: trips 2 and 3 differ — corruption accumulates")
	}

	// the user's placement survives: spec, foreign, project, history
	iSpec := strings.Index(one, "<version-spec")
	iTool := strings.Index(one, "<custom-tool")
	iProj := strings.Index(one, "<project")
	iHist := strings.Index(one, "<version-history")
	if !(iSpec < iTool && iTool < iProj && iProj < iHist) {
		t.Errorf("document order lost: spec@%d tool@%d proj@%d hist@%d", iSpec, iTool, iProj, iHist)
	}
}

// TestRootRoundTrip: the full parser's rewrite (the debug radiography
// path). Known property: unclaimed children re-emit after the claimed
// <project> — order changes, content must not, and the template must
// never freeze (the Expandable's whole point). Fixpoint from trip 2.
func TestRootRoundTrip(t *testing.T) {
	base := writeConfig(t, richDoc)

	load := func(b string) *compiler.Root {
		t.Helper()
		root := &compiler.Root{ProjectBase: b}
		if err := root.Load(); err != nil {
			t.Fatal(err)
		}
		return root
	}
	root := load(base)

	out1 := filepath.Join(base, "t1.xml")
	if err := root.Save(nil, out1); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(out1)
	assertBody(t, "root trip 1", string(b1))
	if strings.Contains(string(b1), `embed-version="v0.5.3"`) {
		t.Error("the rewrite froze the template")
	}

	// the output is a valid config again: reload from a copied tree
	base2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base2, "_mkskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base2, "_mkskill", "mkskill.config.xml"), b1, 0o644); err != nil {
		t.Fatal(err)
	}
	root2 := load(base2)
	if got := root2.Project.Children[0].Embed.EmbedVersion.Get(); got != "v0.5.3" {
		t.Errorf("reloaded embed-version = %q", got)
	}
	out2 := filepath.Join(base2, "t2.xml")
	if err := root2.Save(nil, out2); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(out2)

	// fixpoint: a third trip reproduces the second byte for byte
	base3 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base3, "_mkskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base3, "_mkskill", "mkskill.config.xml"), b2, 0o644); err != nil {
		t.Fatal(err)
	}
	out3 := filepath.Join(base3, "t3.xml")
	if err := load(base3).Save(nil, out3); err != nil {
		t.Fatal(err)
	}
	b3, _ := os.ReadFile(out3)
	if string(b2) != string(b3) {
		t.Error("no fixpoint on the Root path")
	}
}

// TestMutationHammering: 30 consecutive gestures over the same config —
// alternating vinc and vset, escaping-hostile titles included. Every
// intermediate state must parse; the final one must keep every foreign
// body, honor max, agree across parsers, and pass vcheck.
func TestMutationHammering(t *testing.T) {
	base := writeConfig(t, richDoc)
	plantRichDest(t, base)

	last := ""
	for i := 0; i < 30; i++ {
		var err error
		var res *compiler.VMutated
		switch i % 3 {
		case 0:
			res, err = compiler.VInc(base, "build", map[string]string{"title": fmt.Sprintf("round %d", i)})
		case 1:
			res, err = compiler.VInc(base, "minor", nil)
		default:
			res, err = compiler.VInc(base, "build", map[string]string{"title": `a<b&"c" 'd'`})
		}
		if err != nil {
			t.Fatalf("gesture %d: %v", i, err)
		}
		last = res.Tag
		if _, err := compiler.LazyLoad(compiler.VersionConfigFile(base)); err != nil {
			t.Fatalf("gesture %d corrupted the config: %v", i, err)
		}
	}

	out := readConfig(t, base)
	for _, want := range []string{
		"<!-- prolog: announces the document -->", `другой="attr"`,
		"foreign text stays", "<!-- meta comment -->",
		"{$$$ version $$$}", `embed-version="{$$$ ver:version $$$}"`,
		"<!-- epilog: after the root -->",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("hammering lost %q", want)
		}
	}
	if n := strings.Count(out, "<release "); n != 3 {
		t.Errorf("history holds %d releases, max is 3", n)
	}

	// the escaping-hostile title survived the trip exactly
	doc, err := compiler.LazyLoad(compiler.VersionConfigFile(base))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Spec.Labels["title"].Value; got != `a<b&"c" 'd'` {
		t.Errorf("hostile title corrupted: %q", got)
	}

	// parsers agree and the guard is happy
	if got, err := compiler.VOut(base, "version"); err != nil || got != last {
		t.Errorf("VOut = %q, %v; want %q", got, err, last)
	}
	rep, err := compiler.VCheck(base)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Errorf("vcheck dirty after hammering: errors=%v drift=%v", rep.Errors, rep.Drift)
	}
}
