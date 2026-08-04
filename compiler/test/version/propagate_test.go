package version

// Phase 3 of the version subsystem: the destinations. One gesture writes
// the XML and every update/create from the new state; -vbuild re-renders
// alone; a compute error leaves every destination untouched.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

const propDoc = `<?xml version="1.0" encoding="UTF-8"?>
<mkskill>
  <version-spec format="v%d.%d.%d" params="major:byte,minor:[0-99],build:word">
    <version major="0" minor="5" build="3" lock="major">
      <ts></ts>
    </version>
    <label key="title" volatile="true" default-to-ts="true">seed</label>
    <label key="desc" format="%s%s" params="ver:version,' - ' + label:title" when="label:title"></label>
    <update file="/.build/next-release.json" type="json">
      <entry key="tag">{$$$ version $$$}</entry>
      <entry key="title">{$$$ label:desc $$$}</entry>
    </update>
    <update file="src/app.h">
      <entry>
            #define APP_VERSION "{$$$ version $$$}"
            #define APP_BUILD {$$$ build $$$}
      </entry>
    </update>
    <update file="conf/tool.xml" type="xml">
      <entry key="info" attrib="version">{$$$ version $$$}</entry>
      <entry key="info/notes">{$$$ label:title $$$}</entry>
    </update>
    <create file="__publish.bat" git-ignore="true" overwrite="true">
git tag {$$$ version $$$}
git push origin main --tags
</create>
  </version-spec>
  <project id="main" name="demo" project-type="Go-Module"/>
</mkskill>`

// plantDest writes the destination files the updates expect.
func plantDest(t *testing.T, base string) {
	t.Helper()
	files := map[string]string{
		".build/next-release.json": "{\n  \"tag\": \"v0.5.3\",\n  \"title\": \"old\"\n}\n",
		"src/app.h": "// header comment\r\n" +
			"  #define APP_VERSION \"v0.5.3\"\r\n" +
			"#define APP_BUILD 3\r\n" +
			"#define OTHER 1\r\n",
		"conf/tool.xml": `<?xml version="1.0"?><tool><!-- keep --><info version="v0.5.3" extra="yes"><notes>old</notes></info></tool>`,
	}
	for rel, content := range files {
		path := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, base, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGestureWithDestinations(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)

	res, err := compiler.VInc(base, "build", map[string]string{"title": "Patata Frita"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Tag != "v0.5.4" {
		t.Fatalf("tag = %q", res.Tag)
	}
	if len(res.Written) == 0 {
		t.Fatal("nothing propagated")
	}

	// json: patched, normalized, values from the new state
	var man struct{ Tag, Title string }
	if err := json.Unmarshal([]byte(readFile(t, base, ".build/next-release.json")), &man); err != nil {
		t.Fatal(err)
	}
	if man.Tag != "v0.5.4" || man.Title != "v0.5.4 - Patata Frita" {
		t.Errorf("manifest = %+v", man)
	}

	// replace: both lines rewritten, indent kept, EOL style (CRLF) kept,
	// untouched lines untouched
	h := readFile(t, base, "src/app.h")
	if !strings.Contains(h, "  #define APP_VERSION \"v0.5.4\"\r\n") {
		t.Errorf("version line: %q", h)
	}
	if !strings.Contains(h, "#define APP_BUILD 4\r\n") {
		t.Errorf("build line: %q", h)
	}
	if !strings.Contains(h, "// header comment") || !strings.Contains(h, "#define OTHER 1") {
		t.Error("foreign lines lost")
	}

	// xml: attribute set, text replaced, foreign attr and comment intact
	x := readFile(t, base, "conf/tool.xml")
	if !strings.Contains(x, `version="v0.5.4"`) || !strings.Contains(x, `extra="yes"`) {
		t.Errorf("xml attrs: %q", x)
	}
	if !strings.Contains(x, "<notes>Patata Frita</notes>") {
		t.Errorf("xml text: %q", x)
	}
	if !strings.Contains(x, "<!-- keep -->") {
		t.Error("xml comment lost")
	}

	// create: the bat with the tag baked in, and .gitignore promised
	bat := readFile(t, base, "__publish.bat")
	if !strings.HasPrefix(bat, "git tag v0.5.4\n") {
		t.Errorf("bat = %q", bat)
	}
	gi := readFile(t, base, ".gitignore")
	if !strings.Contains(gi, "__publish.bat") {
		t.Errorf(".gitignore = %q", gi)
	}
}

func TestVBuildRepairsDrift(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	if _, err := compiler.VInc(base, "build", map[string]string{"title": "x"}); err != nil {
		t.Fatal(err)
	}

	// somebody hand-breaks a destination
	hPath := filepath.Join(base, "src", "app.h")
	broken := strings.Replace(readFile(t, base, "src/app.h"), "v0.5.4", "v9.9.9", 1)
	if err := os.WriteFile(hPath, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	prop, err := compiler.VBuild(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Written) == 0 {
		t.Fatal("vbuild wrote nothing")
	}
	if !strings.Contains(readFile(t, base, "src/app.h"), `"v0.5.4"`) {
		t.Error("drift not repaired")
	}

	// and idempotent: a second run produces identical content
	before := readFile(t, base, ".build/next-release.json")
	if _, err := compiler.VBuild(base); err != nil {
		t.Fatal(err)
	}
	if after := readFile(t, base, ".build/next-release.json"); after != before {
		t.Error("vbuild is not idempotent")
	}
}

func TestPropagationComputeFirst(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	// break the LAST destination's anchor: the header loses its line
	hPath := filepath.Join(base, "src", "app.h")
	if err := os.WriteFile(hPath, []byte("nothing to anchor on\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	jsonBefore := readFile(t, base, ".build/next-release.json")
	cfgBefore := readConfig(t, base)

	_, err := compiler.VInc(base, "build", nil)
	if err == nil {
		t.Fatal("missing anchor did not error")
	}
	// the promise is LITERAL: everything computes before anything is
	// written — the json was not touched, and neither was the CONFIG:
	// the world stays exactly as it was
	if readFile(t, base, ".build/next-release.json") != jsonBefore {
		t.Error("a destination was written despite the compute error")
	}
	if readConfig(t, base) != cfgBefore {
		t.Error("the config was written despite the compute error")
	}
}

// TestDestPathRejectsEscapes locks safeRel on the destinations: a config
// cannot write outside its own tree, whatever the macros assemble.
func TestDestPathRejectsEscapes(t *testing.T) {
	for _, bad := range []string{"../outside.txt", "C:/x.txt", "/../x", `\x`, "..", `sub\..\..\x`} {
		doc := strings.Replace(propDoc, `<create file="__publish.bat" git-ignore="true" overwrite="true">`,
			`<create file="`+bad+`" git-ignore="true" overwrite="true">`, 1)
		base := writeConfig(t, doc)
		plantDest(t, base)
		cfgBefore := readConfig(t, base)
		if _, err := compiler.VInc(base, "build", nil); err == nil {
			t.Errorf("file=%q accepted", bad)
		}
		if readConfig(t, base) != cfgBefore {
			t.Errorf("file=%q: the config was written despite the error", bad)
		}
	}
}

func TestCreateOverwriteModes(t *testing.T) {
	warnDoc := strings.Replace(propDoc, `overwrite="true"`, `overwrite="warn"`, 1)
	base := writeConfig(t, warnDoc)
	plantDest(t, base)
	batPath := filepath.Join(base, "__publish.bat")
	if err := os.WriteFile(batPath, []byte("mine, do not touch"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := compiler.VInc(base, "build", nil)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, base, "__publish.bat") != "mine, do not touch" {
		t.Error("warn overwrote the existing file")
	}
	if len(res.Skipped) != 1 || len(res.Warns) == 0 {
		t.Errorf("skip not reported: skipped=%v warns=%v", res.Skipped, res.Warns)
	}

	falseDoc := strings.Replace(propDoc, `overwrite="true"`, `overwrite="false"`, 1)
	base2 := writeConfig(t, falseDoc)
	plantDest(t, base2)
	if err := os.WriteFile(filepath.Join(base2, "__publish.bat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.VInc(base2, "build", nil); err == nil {
		t.Error("overwrite=false with an existing file did not error")
	}
}

func TestCreateFromSrc(t *testing.T) {
	doc := strings.Replace(propDoc,
		`<create file="__publish.bat" git-ignore="true" overwrite="true">
git tag {$$$ version $$$}
git push origin main --tags
</create>`,
		`<create file="__publish.bat" src="_mkskill/publish.tpl"/>`, 1)
	base := writeConfig(t, doc)
	plantDest(t, base)
	tpl := filepath.Join(base, "_mkskill", "publish.tpl")
	if err := os.WriteFile(tpl, []byte("tag={$$$ version $$$}\r\nrem CRLF verbatim\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.VInc(base, "build", nil); err != nil {
		t.Fatal(err)
	}
	bat := readFile(t, base, "__publish.bat")
	if bat != "tag=v0.5.4\r\nrem CRLF verbatim\r\n" {
		t.Errorf("src template not verbatim: %q", bat)
	}
}

// TestUpdateKeepsEOLStyle locks the update rule for every type: the
// file's newline style is the file's business — a CRLF json stays CRLF.
func TestUpdateKeepsEOLStyle(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	// rewrite the manifest in CRLF before the gesture
	jPath := filepath.Join(base, ".build", "next-release.json")
	crlf := strings.ReplaceAll(readFile(t, base, ".build/next-release.json"), "\n", "\r\n")
	if err := os.WriteFile(jPath, []byte(crlf), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.VInc(base, "build", map[string]string{"title": "x"}); err != nil {
		t.Fatal(err)
	}
	out := readFile(t, base, ".build/next-release.json")
	if !strings.Contains(out, "\r\n") {
		t.Error("CRLF json flattened to LF")
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("mixed EOLs in the patched json")
	}
}

// TestCreateEOLAttribute locks the eol declaration: crlf forces CRLF on
// an inline template (LF by XML law), lf flattens a CRLF src template.
func TestCreateEOLAttribute(t *testing.T) {
	doc := strings.Replace(propDoc, `<create file="__publish.bat" git-ignore="true" overwrite="true">`,
		`<create file="__publish.bat" git-ignore="true" overwrite="true" eol="crlf">`, 1)
	base := writeConfig(t, doc)
	plantDest(t, base)
	if _, err := compiler.VInc(base, "build", nil); err != nil {
		t.Fatal(err)
	}
	bat := readFile(t, base, "__publish.bat")
	if !strings.HasPrefix(bat, "git tag v0.5.4\r\n") {
		t.Errorf("eol=crlf not applied: %q", bat)
	}

	doc2 := strings.Replace(propDoc,
		`<create file="__publish.bat" git-ignore="true" overwrite="true">
git tag {$$$ version $$$}
git push origin main --tags
</create>`,
		`<create file="__publish.bat" src="_mkskill/p.tpl" eol="lf"/>`, 1)
	base2 := writeConfig(t, doc2)
	plantDest(t, base2)
	tpl := filepath.Join(base2, "_mkskill", "p.tpl")
	if err := os.WriteFile(tpl, []byte("tag={$$$ version $$$}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.VInc(base2, "build", nil); err != nil {
		t.Fatal(err)
	}
	if bat := readFile(t, base2, "__publish.bat"); bat != "tag=v0.5.4\n" {
		t.Errorf("eol=lf not applied to src: %q", bat)
	}
}

func TestUpdateMissingFileErrors(t *testing.T) {
	base := writeConfig(t, propDoc) // destinations never planted
	if _, err := compiler.VInc(base, "build", nil); err == nil {
		t.Error("missing update destination did not error")
	}
}

func TestGitignoreIdempotent(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	if _, err := compiler.VInc(base, "build", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.VBuild(base); err != nil {
		t.Fatal(err)
	}
	gi := readFile(t, base, ".gitignore")
	if strings.Count(gi, "__publish.bat") != 1 {
		t.Errorf(".gitignore accumulated duplicates: %q", gi)
	}
}
