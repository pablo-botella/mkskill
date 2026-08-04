package version

// Phase 4 of the version subsystem: the guard. -vcheck resolves the WHOLE
// spec (the orphan broken label is its catch, not the fast verbs') and
// compares every destination against today's render without writing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

func TestVCheckCleanAfterGesture(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	if _, err := compiler.VInc(base, "build", map[string]string{"title": "x"}); err != nil {
		t.Fatal(err)
	}
	rep, err := compiler.VCheck(base)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Errorf("not clean after a full gesture: errors=%v drift=%v", rep.Errors, rep.Drift)
	}
}

func TestVCheckSeesDriftAndVBuildClears(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	if _, err := compiler.VInc(base, "build", nil); err != nil {
		t.Fatal(err)
	}

	// hand-drift two kinds: a destination and the .gitignore promise
	hPath := filepath.Join(base, "src", "app.h")
	broken := strings.Replace(readFile(t, base, "src/app.h"), "v0.5.4", "v9.9.9", 1)
	if err := os.WriteFile(hPath, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(base, ".gitignore")); err != nil {
		t.Fatal(err)
	}

	rep, err := compiler.VCheck(base)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() {
		t.Fatal("drift not seen")
	}
	found := strings.Join(rep.Drift, "\n")
	if !strings.Contains(found, "src/app.h") || !strings.Contains(found, ".gitignore") {
		t.Errorf("drift misses a case: %v", rep.Drift)
	}
	if len(rep.Errors) != 0 {
		t.Errorf("drift reported as errors: %v", rep.Errors)
	}

	// -vbuild repairs, -vcheck agrees
	if _, err := compiler.VBuild(base); err != nil {
		t.Fatal(err)
	}
	rep, err = compiler.VCheck(base)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Errorf("still dirty after vbuild: errors=%v drift=%v", rep.Errors, rep.Drift)
	}
}

func TestOrphanLabelFailsEveryVerb(t *testing.T) {
	// a broken computed label NOBODY references: every label resolves at
	// load, so EVERY verb refuses it — the orphan is nobody's blind spot
	doc := strings.Replace(propDoc,
		`<label key="title" volatile="true" default-to-ts="true">seed</label>`,
		`<label key="title" volatile="true" default-to-ts="true">seed</label>
    <label key="orphan" format="%s" params="label:nope"></label>`, 1)
	base := writeConfig(t, doc)
	plantDest(t, base)
	before := readConfig(t, base)

	if _, err := compiler.VInc(base, "build", nil); err == nil {
		t.Error("vinc accepted the orphan")
	}
	if readConfig(t, base) != before {
		t.Error("a failed load still wrote the config")
	}
	if _, err := compiler.VCheck(base); err == nil {
		t.Error("vcheck accepted the orphan")
	}
	if _, err := compiler.VOut(base, "version"); err == nil {
		t.Error("vout accepted the orphan")
	}
}

func TestVCheckEmbedVersion(t *testing.T) {
	// good: resolves in the unit's scope; bad: unknown label — an error
	good := strings.Replace(propDoc,
		`<project id="main" name="demo" project-type="Go-Module"/>`,
		`<project id="main" name="demo" project-type="Go-Module">
    <embed filename="x_embed.go" embed-version="{$$$ ver:version $$$}"/>
  </project>`, 1)
	base := writeConfig(t, good)
	plantDest(t, base)
	rep, err := compiler.VCheck(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) != 0 {
		t.Errorf("good embed-version errored: %v", rep.Errors)
	}

	bad := strings.Replace(good, `embed-version="{$$$ ver:version $$$}"`,
		`embed-version="{$$$ ver:label:nope $$$}"`, 1)
	base2 := writeConfig(t, bad)
	plantDest(t, base2)
	rep, err = compiler.VCheck(base2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Errors, "\n"), "embed-version") {
		t.Errorf("bad embed-version not caught: %v", rep.Errors)
	}
}

// TestTraceShowsTheChain locks the debugger: -vtrace answers "how did
// this label resolve?" with the whole tree — and on a broken config
// (unchecked load) the trace marks the exact node that died.
func TestTraceShowsTheChain(t *testing.T) {
	base := writeConfig(t, propDoc)
	value, lines, err := compiler.VTrace(base, "label:desc")
	if err != nil {
		t.Fatal(err)
	}
	if value != "v0.5.3 - seed" {
		t.Errorf("value = %q", value)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		`label:desc = "v0.5.3 - seed"`, // the root, filled in place
		"  label:title",                // the when guard's lookup, indented
		"  ver:version",                // the params, one level in
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("trace misses %q:\n%s", want, joined)
		}
	}

	// broken config: the load does not refuse the debugger, and the
	// trace shows WHERE it died
	bad := strings.Replace(propDoc,
		`<label key="desc" format="%s%s" params="ver:version,' - ' + label:title" when="label:title"></label>`,
		`<label key="desc" format="%s" params="label:nope"></label>`, 1)
	base2 := writeConfig(t, bad)
	_, lines, err = compiler.VTrace(base2, "label:desc")
	if err == nil {
		t.Fatal("broken trace did not error")
	}
	joined = strings.Join(lines, "\n")
	if !strings.Contains(joined, "label:desc !!") || !strings.Contains(joined, "  label:nope !!") {
		t.Errorf("failure not located in the trace:\n%s", joined)
	}
}

// TestVLabelsOverview locks the one-line-per-label overview — broken
// ones marked, working ones rendered, every domain listed.
func TestVLabelsOverview(t *testing.T) {
	doc := strings.Replace(propDoc,
		`<label key="title" volatile="true" default-to-ts="true">seed</label>`,
		`<label key="title" volatile="true" default-to-ts="true">seed</label>
    <label key="broken" format="%s" params="label:nope"></label>`, 1)
	base := writeConfig(t, doc)
	lines, err := compiler.VLabels(base)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	// every line carries its label's document line — the overview points
	// at the config, not at a loose name
	if !strings.Contains(joined, "ver:label:title (line ") || !strings.Contains(joined, `= "seed"`) {
		t.Errorf("title not rendered with its line:\n%s", joined)
	}
	if !strings.Contains(joined, "ver:label:broken (line ") || !strings.Contains(joined, "!!") {
		t.Errorf("broken not marked with its line:\n%s", joined)
	}
}

func TestVCheckMissingAnchorIsError(t *testing.T) {
	base := writeConfig(t, propDoc)
	plantDest(t, base)
	hPath := filepath.Join(base, "src", "app.h")
	if err := os.WriteFile(hPath, []byte("nothing here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := compiler.VCheck(base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Errors, "\n"), "anchor") {
		t.Errorf("missing anchor not an error: %v", rep.Errors)
	}
}
