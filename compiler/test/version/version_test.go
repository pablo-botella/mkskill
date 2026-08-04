package version

// Phase 1 of the version subsystem: the reference grammar, the {$$$ $$$}
// config skin, the positional formats, and the lazy first-pass parser
// resolving a prototype-shaped document over itself.

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// --- reference grammar ---

func TestParseVersionRef(t *testing.T) {
	good := map[string]string{
		"major":               "major",
		"label:title":         "label:title",
		"prj:child1:base":     "prj:child1:base",
		"prj:child1:label:x":  "prj:child1:label:x",
		"ver:version":         "ver:version",
		"ver:label:vlong":     "ver:label:vlong",
		"glb:label:copyright": "glb:label:copyright",
	}
	for in, want := range good {
		ref, err := compiler.ParseVersionRef(in)
		if err != nil {
			t.Fatalf("ParseVersionRef(%q): %v", in, err)
		}
		if ref.String() != want {
			t.Errorf("ParseVersionRef(%q) = %q, want %q", in, ref.String(), want)
		}
	}
	bad := []string{
		"", "prj:", "prj:base", "prj:id:", "glb:x", "glb:label:",
		"label:", "label:a:b", "ver:", "foo:bar", "a b", "prj:id:meta:x",
	}
	for _, in := range bad {
		if _, err := compiler.ParseVersionRef(in); err == nil {
			t.Errorf("ParseVersionRef(%q): want error, got none", in)
		}
	}
}

// testResolver is a hand map: title empty or set, components numeric.
func testResolver(title string) compiler.VersionResolver {
	return func(ref compiler.VersionRef) (compiler.VersionValue, error) {
		switch ref.String() {
		case "version":
			return compiler.TextValue("v0.5.0"), nil
		case "ts":
			return compiler.TextValue(""), nil
		case "major":
			return compiler.NumValue(0), nil
		case "build":
			return compiler.NumValue(7), nil
		case "label:title":
			return compiler.TextValue(title), nil
		}
		return compiler.VersionValue{}, fmt.Errorf("unresolvable reference %q", ref.String())
	}
}

// --- the {$$$ $$$} skin ---

func TestExpandVersionMacros(t *testing.T) {
	r := testResolver("Patata Frita")
	cases := []struct{ in, want string }{
		{"git tag {$$$ version $$$}", "git tag v0.5.0"},
		{"{$$$ version $$$}{$$$ ' - ' + label:title $$$}", "v0.5.0 - Patata Frita"},
		{"{$$$ %02d build $$$}", "07"},
		{"{$$$ %x build $$$}", "7"},
		{"{$$$ '-' + %03d build $$$}", "-007"},
		{"no macros at all", "no macros at all"},
	}
	for _, c := range cases {
		got, err := compiler.ExpandVersionMacros(c.in, false, r)
		if err != nil {
			t.Fatalf("Expand(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// empty value: the piece vanishes, connector included
	empty := testResolver("")
	got, err := compiler.ExpandVersionMacros("{$$$ version $$$}{$$$ ' - ' + label:title $$$}", false, empty)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.5.0" {
		t.Errorf("empty connector: got %q, want %q", got, "v0.5.0")
	}

	// law 5: attributes take bare references only
	if _, err := compiler.ExpandVersionMacros("{$$$ %02d build $$$}", true, r); err == nil {
		t.Error("attr mode accepted a verb")
	}
	if _, err := compiler.ExpandVersionMacros("{$$$ ' - ' + label:title $$$}", true, r); err == nil {
		t.Error("attr mode accepted a connector")
	}
	// literal text around bare holes is fine in attributes
	got, err = compiler.ExpandVersionMacros("out/{$$$ version $$$}/file.txt", true, r)
	if err != nil || got != "out/v0.5.0/file.txt" {
		t.Errorf("attr path assembly: got %q, %v", got, err)
	}

	// the config universe errors on the unknown and the unterminated
	if _, err := compiler.ExpandVersionMacros("{$$$ nope $$$}", false, r); err == nil {
		t.Error("unresolvable reference did not error")
	}
	if _, err := compiler.ExpandVersionMacros("{$$$ version", false, r); err == nil {
		t.Error("unterminated macro did not error")
	}
}

// --- positional formats ---

func TestRenderVersionFormat(t *testing.T) {
	r := testResolver("Patata Frita")
	got, err := compiler.RenderVersionFormat("%s - v%d build %03.3d", "label:title,major,build", r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Patata Frita - v0 build 007" {
		t.Errorf("got %q", got)
	}

	// connector inside a params item, with and without value
	got, err = compiler.RenderVersionFormat("v%d%s", "major,' - ' + label:title", r)
	if err != nil || got != "v0 - Patata Frita" {
		t.Errorf("connector params: got %q, %v", got, err)
	}
	got, err = compiler.RenderVersionFormat("v%d%s", "major,' - ' + label:title", testResolver(""))
	if err != nil || got != "v0" {
		t.Errorf("empty connector params: got %q, %v", got, err)
	}

	// discipline: arity, kinds, no verbs in params
	if _, err := compiler.RenderVersionFormat("v%d.%d", "major", r); err == nil {
		t.Error("arity mismatch did not error")
	}
	if _, err := compiler.RenderVersionFormat("%d", "label:title", r); err == nil {
		t.Error("numeric verb on a label did not error")
	}
	if _, err := compiler.RenderVersionFormat("%s", "%02d major", r); err == nil {
		t.Error("verb inside params did not error")
	}
}

// --- the lazy first pass over a prototype-shaped document ---

const sampleDoc = `<?xml version="1.0" encoding="UTF-8"?>
<mkskill>
  <label key="copyright">Pablo Botella Navarro</label>
  <label key="owner">PBN Global</label>
  <label key="stamp" format="(c) %s" params="label:owner"></label>
  <version-spec format="v%d.%d.%d" params="major:byte,minor:[0-99],build:word">
    <version major="0" minor="5" build="0" lock="major">
      <ts></ts>
    </version>
    <label key="title" volatile="true" default-to-ts="true">Patata Frita</label>
    <label key="vlong" format="version: %d.%d build: %03.3d" params="major,minor,build"></label>
    <label key="desc" format="%s - version %d.%d build: %03.3d" params="label:title,major,minor,build" when="label:title"></label>
    <label key="owner">Spec Owner</label>
    <update file="/.build/next-release.json" type="json">
      <entry key="tag">{$$$ version $$$}</entry>
    </update>
    <create file="__publish.bat" git-ignore="true">git tag {$$$ version $$$}</create>
  </version-spec>
  <project id="main" name="demo" project-type="Go-Module">
    <label key="banner">the demo banner</label>
    <version-spec format="nope"></version-spec>
    <child-project id="cli" name="demo-cli" path="cmd/demo" project-type="Go-CLI">
      <label key="full" format="%s at %s" params="name,ver:version"></label>
    </child-project>
  </project>
  <version-history max="10">
    <release version="v0.4.0" ts="2026-07-30T00:00:00Z" title="old" method="vinc-minor"/>
  </version-history>
</mkskill>`

func lazyParse(t *testing.T, doc string) *compiler.LazyDoc {
	t.Helper()
	d, err := compiler.LazyParse(xml.NewDecoder(strings.NewReader(doc)))
	if err != nil {
		t.Fatalf("LazyParse: %v", err)
	}
	return d
}

func TestLazyParseResolves(t *testing.T) {
	doc := lazyParse(t, sampleDoc)

	// the misplaced spec warned and was ignored
	if len(doc.Warns) != 1 || !strings.Contains(doc.Warns[0], "version-spec inside a project") {
		t.Errorf("warns = %v", doc.Warns)
	}
	if doc.Spec.Format != "v%d.%d.%d" {
		t.Errorf("spec format = %q — the misplaced one won?", doc.Spec.Format)
	}

	// skeleton: bases computed, ids indexed
	if doc.ByID["cli"] == nil || doc.ByID["cli"].Base != "cmd/demo" {
		t.Errorf("cli base = %+v", doc.ByID["cli"])
	}
	if doc.ByID["main"].Base != "" {
		t.Errorf("main base = %q", doc.ByID["main"].Base)
	}

	r := doc.ResolverFor(nil) // the v-verbs' context: the spec is local
	expand := func(s string) string {
		t.Helper()
		got, err := compiler.ExpandVersionMacros(s, false, r)
		if err != nil {
			t.Fatalf("expand %q: %v", s, err)
		}
		return got
	}

	cases := []struct{ in, want string }{
		{"{$$$ version $$$}", "v0.5.0"},
		{"{$$$ major $$$}", "0"},
		{"{$$$ ts $$$}", ""},
		{"{$$$ label:vlong $$$}", "version: 0.5 build: 000"},
		{"{$$$ label:desc $$$}", "Patata Frita - version 0.5 build: 000"},
		{"{$$$ glb:label:copyright $$$}", "Pablo Botella Navarro"},
		{"{$$$ prj:cli:base $$$}", "cmd/demo"},
		{"{$$$ prj:main:name $$$}", "demo"},
		{"{$$$ prj:main:label:banner $$$}", "the demo banner"},
	}
	for _, c := range cases {
		if got := expand(c.in); got != c.want {
			t.Errorf("%s = %q, want %q", c.in, got, c.want)
		}
	}

	// a unit as local context: bare structural, ver: to cross
	ru := doc.ResolverFor(doc.ByID["cli"])
	got, err := compiler.ExpandVersionMacros("{$$$ name $$$}_{$$$ ver:version $$$}", false, ru)
	if err != nil || got != "demo-cli_v0.5.0" {
		t.Errorf("unit context: %q, %v", got, err)
	}

	// lock captured
	if c := doc.Spec.Component("major"); c == nil || !c.Locked {
		t.Error("major not locked")
	}
}

func TestLazyParseWhenGuard(t *testing.T) {
	// empty title: desc guards to empty, and a connector on it vanishes
	empty := strings.Replace(sampleDoc,
		`<label key="title" volatile="true" default-to-ts="true">Patata Frita</label>`,
		`<label key="title" volatile="true" default-to-ts="true"></label>`, 1)
	doc := lazyParse(t, empty)
	r := doc.ResolverFor(nil)
	got, err := compiler.ExpandVersionMacros("{$$$ version $$$}{$$$ ', ' + label:desc $$$}", false, r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v0.5.0" {
		t.Errorf("guarded desc leaked: %q", got)
	}
}

func TestLazyParseErrors(t *testing.T) {
	bad := []struct{ name, doc string }{
		{"law 1 macro in id",
			`<mkskill><project id="{$$$ x $$$}"/></mkskill>`},
		{"law 1 macro in name",
			`<mkskill><project name="{$$$ x $$$}"/></mkskill>`},
		{"reserved component name",
			`<mkskill><version-spec params="version:byte"/></mkskill>`},
		{"label without key",
			`<mkskill><label>x</label></mkskill>`},
		{"computed and value traits mixed",
			`<mkskill><version-spec params="a:byte"><label key="x" volatile="true" format="%d" params="a"/></version-spec></mkskill>`},
		{"volatile outside the spec",
			`<mkskill><label key="x" volatile="true">v</label></mkskill>`},
		{"when on a value label",
			`<mkskill><version-spec params="a:byte"><label key="x" when="a">v</label></version-spec></mkskill>`},
		{"both defaults",
			`<mkskill><version-spec params="a:byte"><label key="x" volatile="true" default="d" default-to-ts="true"></label></version-spec></mkskill>`},
		{"component out of domain",
			`<mkskill><version-spec params="a:[0-9]"><version a="10"/></version-spec></mkskill>`},
		{"lock on unknown component",
			`<mkskill><version-spec params="a:byte"><version a="1" lock="b"/></version-spec></mkskill>`},
		{"duplicate ids",
			`<mkskill><project id="x"><child-project id="x" path="a"/></project></mkskill>`},
	}
	for _, c := range bad {
		if _, err := compiler.LazyParse(xml.NewDecoder(strings.NewReader(c.doc))); err == nil {
			t.Errorf("%s: want error, got none", c.name)
		}
	}
}

// TestScopeSeparation locks the domain model: a computed label's params
// resolve in the label's OWN domain, never the caller's. The spec has a
// decoy "owner" — if scopes leaked, the global stamp would pick it up.
func TestScopeSeparation(t *testing.T) {
	doc := lazyParse(t, sampleDoc)
	r := doc.ResolverFor(nil) // resolving FROM the spec context

	got, err := compiler.ExpandVersionMacros("{$$$ glb:label:stamp $$$}", false, r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "(c) PBN Global" {
		t.Errorf("glb scope leaked: %q", got)
	}

	// a unit label: bare name is the unit's structural, ver: crosses
	got, err = compiler.ExpandVersionMacros("{$$$ prj:cli:label:full $$$}", false, r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "demo-cli at v0.5.0" {
		t.Errorf("unit scope: %q", got)
	}
}

// --- the verbs as compiler API, over a real config on disk ---

func TestVOutAndVTag(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "_mkskill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mkskill.config.xml"), []byte(sampleDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ ref, want string }{
		{"version", "v0.5.0"},
		{"major", "0"},
		{"label:vlong", "version: 0.5 build: 000"},
		{"glb:label:copyright", "Pablo Botella Navarro"},
		{"prj:cli:base", "cmd/demo"},
	}
	for _, c := range cases {
		got, err := compiler.VOut(base, c.ref)
		if err != nil {
			t.Fatalf("VOut(%q): %v", c.ref, err)
		}
		if got != c.want {
			t.Errorf("VOut(%q) = %q, want %q", c.ref, got, c.want)
		}
	}

	tag, err := compiler.VTag(base)
	if err != nil || tag != "v0.5.0" {
		t.Errorf("VTag = %q, %v", tag, err)
	}

	// bare references only, and the unknown errors
	if _, err := compiler.VOut(base, "nope"); err == nil {
		t.Error("unknown reference did not error")
	}

	// without a version-spec every v-command errors
	noSpec := t.TempDir()
	if err := os.MkdirAll(filepath.Join(noSpec, "_mkskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noSpec, "_mkskill", "mkskill.config.xml"),
		[]byte(`<mkskill><project id="x"/></mkskill>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.VTag(noSpec); err == nil {
		t.Error("missing version-spec did not error")
	}
}

// TestValueLabelWithMacros locks the universal rule on the label's own
// text node: a VALUE label may carry macros — the document keeps the
// raw, consumers get the render — and a cycle through value texts is
// caught by the same guard with its chain.
func TestValueLabelWithMacros(t *testing.T) {
	doc := lazyParse(t, `<mkskill><version-spec format="v%d" params="major:byte">
		<version major="7"/>
		<label key="motto">release {$$$ version $$$} rocks</label>
		<label key="wrap">[{$$$ label:motto $$$}]</label>
	</version-spec></mkskill>`)
	r := doc.ResolverFor(nil)
	got, err := compiler.ExpandVersionMacros("{$$$ label:wrap $$$}", false, r)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[release v7 rocks]" {
		t.Errorf("value-text chain: %q", got)
	}

	// a cycle through value TEXTS is a cycle like any other — and since
	// every label resolves at load, the PARSE itself refuses it
	_, err = compiler.LazyParse(xml.NewDecoder(strings.NewReader(`<mkskill><version-spec params="a:byte">
		<label key="p">x{$$$ label:q $$$}</label>
		<label key="q">y{$$$ label:p $$$}</label>
	</version-spec></mkskill>`)))
	if err == nil || !strings.Contains(err.Error(), "label cycle") {
		t.Errorf("value-text cycle not caught at load: %v", err)
	}
}

// TestLabelChains locks computed→computed dependency: a linear chain
// renders through every level, and a diamond (two paths to the same
// label) is legal — only a true cycle errors.
func TestLabelChains(t *testing.T) {
	doc := lazyParse(t, `<mkskill><version-spec params="n:byte">
		<label key="c">end</label>
		<label key="b" format="B%s" params="label:c"/>
		<label key="a" format="A%s" params="label:b"/>
		<label key="diamond" format="%s|%s" params="label:a,label:b"/>
	</version-spec></mkskill>`)
	r := doc.ResolverFor(nil)

	got, err := compiler.ExpandVersionMacros("{$$$ label:a $$$}", false, r)
	if err != nil || got != "ABend" {
		t.Errorf("chain: %q, %v", got, err)
	}
	got, err = compiler.ExpandVersionMacros("{$$$ label:diamond $$$}", false, r)
	if err != nil || got != "ABend|Bend" {
		t.Errorf("diamond: %q, %v", got, err)
	}
}

func TestLazyCycleGuard(t *testing.T) {
	// every label resolves at load: the cycle fails the PARSE, and the
	// error must SHOW the chain — in a long config "unresolvable" alone
	// helps nobody
	_, err := compiler.LazyParse(xml.NewDecoder(strings.NewReader(`<mkskill><version-spec params="a:byte">
		<label key="p" format="%s" params="label:q"/>
		<label key="q" format="%s" params="label:p"/>
	</version-spec></mkskill>`)))
	if err == nil {
		t.Fatal("cycle loaded instead of erroring")
	}
	if !strings.Contains(err.Error(), "label cycle") ||
		!strings.Contains(err.Error(), "ver:label:p -> ver:label:q -> ver:label:p") {
		t.Errorf("cycle chain not reported: %v", err)
	}
}
