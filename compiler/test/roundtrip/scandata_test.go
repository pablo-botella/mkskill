package roundtrip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestRoundtripScanData saves the scanned fixture with and without
// IncludeScanData: the debug output carries the scan elements, the normal one
// stays pure — and reloading the debug output skips them entirely, so a
// re-save leaks nothing.
func TestRoundtripScanData(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}

	out := "../_out/roundtrip"
	normal := filepath.Join(out, "normal.xml")
	debug := filepath.Join(out, "with-scan.xml")
	if err := root.Save(nil, normal); err != nil {
		t.Fatal(err)
	}
	ctx := compiler.WithEncoderParams(nil, &compiler.EncoderParams{IncludeScanData: true})
	if err := root.Save(ctx, debug); err != nil {
		t.Fatal(err)
	}

	plain := mustRead(t, normal)
	if strings.Contains(plain, "<source-item") || strings.Contains(plain, "<import-manifest") {
		t.Error("normal output carries scan data")
	}
	scan := mustRead(t, debug)
	if !strings.Contains(scan, "<source-item") || !strings.Contains(scan, "<import-manifest") {
		t.Error("debug output misses scan data")
	}
	t.Logf("debug output:\n%s", scan)

	// reload the debug output: the scan elements are known and skipped —
	// re-saving without the flag must come out pure again
	back := &compiler.Root{ProjectBase: "../_data/demo", ConfigFile: debug}
	if err := back.Load(); err != nil {
		t.Fatal(err)
	}
	compareTrees(t, root.Project, back.Project)

	resaved := filepath.Join(out, "resaved.xml")
	if err := back.Save(nil, resaved); err != nil {
		t.Fatal(err)
	}
	if again := mustRead(t, resaved); strings.Contains(again, "<source-item") || strings.Contains(again, "<import-manifest") {
		t.Error("scan data leaked through the reload (should have been skipped, not cargo)")
	}
}

// TestRoundtripScanDataPretty saves the scanned fixture with IncludeScanData
// and PrettyOutput: the scan elements come out reindented, one per line, and
// the reload still tells the same tree.
func TestRoundtripScanDataPretty(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}

	pretty := filepath.Join("../_out/roundtrip", "with-scan-pretty.xml")
	ctx := compiler.WithEncoderParams(nil, &compiler.EncoderParams{IncludeScanData: true, PrettyOutput: true})
	if err := root.Save(ctx, pretty); err != nil {
		t.Fatal(err)
	}
	out := mustRead(t, pretty)
	t.Logf("pretty debug output:\n%s", out)

	// the scan elements are there, each reindented on its own line: the own
	// items grouped under <scan>, the harvested one under its manifest
	if !strings.Contains(out, "\n    <scan>") {
		t.Error("want the <scan> group indented under <project>")
	}
	if strings.Count(out, "\n      <source-item") != 3 {
		t.Error("want the 3 own source items indented under <scan>")
	}
	if !strings.Contains(out, "\n        <import-manifest") {
		t.Error("want the manifest indented under <import-miniskin>/<scan>")
	}
	if !strings.Contains(out, "\n          <source-item") {
		t.Error("want the harvested item indented under <import-manifest>")
	}

	// and the reload still skips them into the same tree
	back := &compiler.Root{ProjectBase: "../_data/demo", ConfigFile: pretty}
	if err := back.Load(); err != nil {
		t.Fatal(err)
	}
	compareTrees(t, root.Project, back.Project)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
