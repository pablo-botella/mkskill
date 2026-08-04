package scan

import (
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestPreserveScan checks round 0 of the scan: the preservation map gets
// resolved against the disk, and every collected item whose destination
// collides with it comes out marked.
func TestPreserveScan(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}

	main := root.Project
	if main.Preserve == nil {
		t.Fatal("fixture lost its <preserve> section")
	}

	// the map: README.md (exact, missing, alias rule) + the glob over the
	// existing sources (overview.md, usage.md) + demo-content.md (exact,
	// missing: the import's destination)
	files := map[string]*compiler.PreservedFile{}
	for _, f := range main.Preserve.Files {
		files[f.Path] = f
	}
	if len(files) != 4 {
		t.Fatalf("preservation map has %d entries, want 4: %v", len(files), files)
	}
	if f := files["README.md"]; f == nil || f.Exists || f.Item.Method.Get() != "alias" || f.Item.Alias.Get() != "readme-alt.md" {
		t.Errorf("README.md entry wrong: %+v", f)
	}
	if f := files["_mkskill/src/overview.md"]; f == nil || !f.Exists || f.Item.Method.Get() != "alt" {
		t.Errorf("overview.md entry wrong: %+v", f)
	}
	if f := files["_mkskill/src/usage.md"]; f == nil || !f.Exists {
		t.Errorf("usage.md entry wrong: %+v", f)
	}
	if f := files["_mkskill/src/demo-content.md"]; f == nil || f.Exists {
		t.Errorf("demo-content.md entry wrong (must map even when missing): %+v", f)
	}

	// the marks: both existing sources and the import's destination conflict;
	// the subfolder one does not
	conflicts := map[string]bool{}
	for _, it := range main.GetAllSourceItems() {
		conflicts[it.DstFileName] = it.PreserveConflict != nil
		if it.DstFileName == "demo-content.md" && it.PreserveConflict != nil && it.PreserveConflict.Exists {
			t.Error("import conflict should point at a missing file")
		}
	}
	for name, want := range map[string]bool{
		"overview.md":     true,
		"usage.md":        true,
		"demo-content.md": true,
		"extra.md":        false,
	} {
		if conflicts[name] != want {
			t.Errorf("%s: conflict=%v, want %v", name, conflicts[name], want)
		}
	}

	// the child has no preserve section: its item stays unmarked
	child := main.Children[0]
	if items := child.GetSourceItems(); len(items) != 1 || items[0].PreserveConflict != nil {
		t.Errorf("child item should carry no conflict: %+v", items)
	}

	// cleanup empties the map too
	if err := root.CleanUpLastScan(nil); err != nil {
		t.Fatal(err)
	}
	if len(main.Preserve.Files) != 0 {
		t.Error("cleanup left the preservation map behind")
	}
}
