package scan

import (
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestScan runs the whole scan over the demo fixture and checks what every
// unit collected: own sources, the miniskin harvest, the child's sources.
func TestScan(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if len(root.Warns) != 0 {
		t.Errorf("unexpected load warnings: %q", root.Warns)
	}
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}
	// scan again: Scan cleans up first, so nothing may duplicate
	if err := root.Scan(nil); err != nil {
		t.Fatal(err)
	}

	main := root.Project

	// own sources: overview.md, usage.md (+ its .fm), common/extra.md
	own := main.GetSourceItems()
	if len(own) != 3 {
		t.Fatalf("main has %d own items, want 3", len(own))
	}
	byName := map[string]*compiler.SourceItem{}
	for _, it := range own {
		byName[it.DstFileName] = it
	}
	if it := byName["usage.md"]; it == nil || !it.FmExternal {
		t.Errorf("usage.md missing or without FmExternal: %+v", it)
	}
	if it := byName["overview.md"]; it == nil || it.FmExternal {
		t.Errorf("overview.md missing or with unexpected FmExternal: %+v", it)
	}
	if it := byName["extra.md"]; it == nil || it.DstPath != "common" {
		t.Errorf("extra.md missing or without DstPath=common: %+v", it)
	}

	// the miniskin harvest: exactly 2 items (the decoys carry no mkskill-*)
	all := main.GetAllSourceItems()
	if len(all) != 5 {
		t.Fatalf("main has %d items counting imports, want 5", len(all))
	}
	var imported *compiler.SourceItem
	harvested := 0
	for _, it := range all {
		if it.OriginType == compiler.ItemOriginMiniskin {
			harvested++
			if it.DstFileName == "demo-content.md" {
				imported = it
			}
		}
	}
	if harvested != 2 {
		t.Fatalf("%d miniskin items harvested, want 2", harvested)
	}
	if imported == nil {
		t.Fatal("demo-content.md not harvested")
	}
	if imported.DstFileName != "demo-content.md" || imported.DstPath != "" {
		t.Errorf("miniskin item destination wrong: %+v", imported)
	}
	if !imported.FmExternal || !imported.FmPreserve {
		t.Errorf("fm-gen extern,preserve not reflected: %+v", imported)
	}
	if imported.OriginPath != "src/generated/doc-body.md" {
		t.Errorf("OriginPath = %q, want src/generated/doc-body.md", imported.OriginPath)
	}
	if len(imported.ForeignAttrib) != 2 {
		t.Errorf("foreign attribs = %q, want in and pos", imported.ForeignAttrib)
	}

	// the child collects its own, independently
	child := main.Children[0]
	if items := child.GetSourceItems(); len(items) != 1 || items[0].DstFileName != "cli.md" {
		t.Errorf("child items wrong: %+v", items)
	}

	// cleanup leaves everything empty
	if err := root.CleanUpLastScan(nil); err != nil {
		t.Fatal(err)
	}
	if left := main.GetAllSourceItems(); len(left) != 0 {
		t.Errorf("cleanup left %d items behind", len(left))
	}
}
