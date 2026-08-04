package version

// Phase 2 of the version subsystem: the mutators. The critical contract
// is preservation — what the mutator does not own comes out where it was,
// byte-meaningful: comments, foreign attributes, the user's placement of
// <version-history> (here BETWEEN spec and project, on purpose).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pablo-botella/mkskill/compiler"
)

const mutDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!-- prolog comment survives -->
<mkskill data-foreign="keep">
  <label key="copyright">PBN</label>
  <version-spec format="v%d.%d.%d" params="major:byte,minor:[0-99],build:word">
    <version major="0" minor="5" build="3" lock="major">
      <ts>2026-07-01T00:00:00Z</ts>
    </version>
    <label key="title" volatile="true" default-to-ts="true">old title</label>
    <label key="note">stays untouched</label>
    <update file="/x.json" type="json">
      <entry key="tag">{$$$ version $$$}</entry>
    </update>
  </version-spec>
  <version-history max="2">
    <release version="v0.5.3" ts="2026-07-30T00:00:00Z" title="prev" method="vinc-build"/>
    <release version="v0.5.2" ts="2026-07-29T00:00:00Z" title="prev2" method="vinc-build"/>
  </version-history>
  <project id="main" name="demo" project-type="Go-Module">
    <child-project id="cli" name="cli" path="cmd/x" project-type="Go-CLI"/>
  </project>
</mkskill>`

// writeConfig plants a config under a fresh base and returns the base —
// plus the /x.json destination the phase-2 doc declares, so its gestures
// can propagate (the mutators run the destinations since phase 3).
func writeConfig(t *testing.T, doc string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "_mkskill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mkskill.config.xml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "x.json"), []byte("{\n  \"tag\": \"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return base
}

func readConfig(t *testing.T, base string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(base, "_mkskill", "mkskill.config.xml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestVIncGesture(t *testing.T) {
	base := writeConfig(t, mutDoc)

	res, err := compiler.VInc(base, "build", map[string]string{"title": "Nueva Entrega"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Tag != "v0.5.4" {
		t.Errorf("tag = %q, want v0.5.4", res.Tag)
	}
	if _, err := time.Parse(time.RFC3339, res.Ts); err != nil {
		t.Errorf("ts %q not RFC-3339: %v", res.Ts, err)
	}

	// the brain re-reads its own writing
	doc, err := compiler.LazyLoad(compiler.VersionConfigFile(base))
	if err != nil {
		t.Fatal(err)
	}
	if c := doc.Spec.Component("build"); c == nil || c.Value != 4 {
		t.Errorf("build = %+v", c)
	}
	if doc.Spec.Ts != res.Ts {
		t.Errorf("ts in config %q != reported %q", doc.Spec.Ts, res.Ts)
	}
	if doc.Spec.Labels["title"].Value != "Nueva Entrega" {
		t.Errorf("title = %q", doc.Spec.Labels["title"].Value)
	}
	if doc.Spec.Labels["note"].Value != "stays untouched" {
		t.Errorf("note = %q", doc.Spec.Labels["note"].Value)
	}

	// preservation: everything not ours, exactly where it was
	out := readConfig(t, base)
	for _, want := range []string{
		"<!-- prolog comment survives -->",
		`data-foreign="keep"`,
		`lock="major"`,
		`format="v%d.%d.%d"`,
		"{$$$ version $$$}",
		"stays untouched",
		`<child-project id="cli"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lost on rewrite: %q", want)
		}
	}
	// the user placed the history BETWEEN spec and project: it stays there
	iSpec := strings.Index(out, "<version-spec")
	iHist := strings.Index(out, "<version-history")
	iProj := strings.Index(out, "<project")
	if !(iSpec < iHist && iHist < iProj) {
		t.Errorf("document order lost: spec@%d hist@%d proj@%d", iSpec, iHist, iProj)
	}

	// history: the new entry AT THE TOP, max=2 trimmed the oldest FIFO
	iNew := strings.Index(out, `version="v0.5.4"`)
	iPrev := strings.Index(out, `version="v0.5.3"`)
	if iNew < 0 || iPrev < 0 || iNew > iPrev {
		t.Errorf("new release not at the top: new@%d prev@%d", iNew, iPrev)
	}
	if strings.Contains(out, `version="v0.5.2"`) {
		t.Error("max=2 did not trim the oldest entry")
	}
	if !strings.Contains(out, `method="vinc-build"`) || !strings.Contains(out, `title="Nueva Entrega"`) {
		t.Error("release entry incomplete")
	}
}

func TestVIncResetsRight(t *testing.T) {
	base := writeConfig(t, mutDoc)
	res, err := compiler.VInc(base, "minor", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tag != "v0.6.0" {
		t.Errorf("tag = %q, want v0.6.0 (build reset to its minimum)", res.Tag)
	}
}

func TestVIncVolatileReset(t *testing.T) {
	base := writeConfig(t, mutDoc)
	res, err := compiler.VInc(base, "build", nil) // no -label:title
	if err != nil {
		t.Fatal(err)
	}
	doc, err := compiler.LazyLoad(compiler.VersionConfigFile(base))
	if err != nil {
		t.Fatal(err)
	}
	// volatile + default-to-ts: the title materialized the new ts
	if doc.Spec.Labels["title"].Value != res.Ts {
		t.Errorf("title = %q, want the ts %q", doc.Spec.Labels["title"].Value, res.Ts)
	}
}

func TestVSet(t *testing.T) {
	base := writeConfig(t, mutDoc)
	res, err := compiler.VSet(base, "0.7.4", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tag != "v0.7.4" {
		t.Errorf("tag = %q", res.Tag)
	}
	// method recorded as vset
	if out := readConfig(t, base); !strings.Contains(out, `method="vset"`) {
		t.Error("method=vset not archived")
	}
}

func TestMutationErrors(t *testing.T) {
	cases := []struct {
		name string
		run  func(base string) error
	}{
		{"vinc unknown component", func(b string) error { _, err := compiler.VInc(b, "nope", nil); return err }},
		{"vinc locked", func(b string) error { _, err := compiler.VInc(b, "major", nil); return err }},
		{"vset changing locked", func(b string) error { _, err := compiler.VSet(b, "1.0.0", nil); return err }},
		{"vset arity", func(b string) error { _, err := compiler.VSet(b, "0.7", nil); return err }},
		{"vset domain", func(b string) error { _, err := compiler.VSet(b, "0.700.4", nil); return err }},
		{"vset not a number", func(b string) error { _, err := compiler.VSet(b, "0.x.4", nil); return err }},
		{"unknown label", func(b string) error {
			_, err := compiler.VInc(b, "build", map[string]string{"nope": "x"})
			return err
		}},
	}
	for _, c := range cases {
		base := writeConfig(t, mutDoc)
		before := readConfig(t, base)
		if err := c.run(base); err == nil {
			t.Errorf("%s: want error, got none", c.name)
		}
		// nothing was written: the file is untouched
		if after := readConfig(t, base); after != before {
			t.Errorf("%s: the file changed on a failed mutation", c.name)
		}
	}
}

func TestVSetKeepingLockedIsFine(t *testing.T) {
	base := writeConfig(t, mutDoc)
	// major stays 0: the locked component does not change — legal
	if _, err := compiler.VSet(base, "0.9.9", nil); err != nil {
		t.Fatalf("vset keeping the locked value errored: %v", err)
	}
}

func TestComputedLabelRefused(t *testing.T) {
	doc := strings.Replace(mutDoc,
		`<label key="note">stays untouched</label>`,
		`<label key="note">stays untouched</label>
    <label key="vlong" format="v%d" params="major"></label>`, 1)
	base := writeConfig(t, doc)
	if _, err := compiler.VInc(base, "build", map[string]string{"vlong": "x"}); err == nil {
		t.Error("setting a computed label did not error")
	}
}

func TestNoHistoryNoEntry(t *testing.T) {
	// strip the whole version-history element: opt-in means the mutator
	// neither inserts nor creates it
	start := strings.Index(mutDoc, "<version-history")
	end := strings.Index(mutDoc, "</version-history>") + len("</version-history>")
	doc := mutDoc[:start] + mutDoc[end:]
	base := writeConfig(t, doc)
	if _, err := compiler.VInc(base, "build", nil); err != nil {
		t.Fatal(err)
	}
	if out := readConfig(t, base); strings.Contains(out, "version-history") {
		t.Error("the mutator created a version-history out of thin air")
	}
}
