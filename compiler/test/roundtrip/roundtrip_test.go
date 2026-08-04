package roundtrip

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablo-botella/mkskill/compiler"
)

// TestRoundtrip loads the demo fixture, saves it, reloads the copy and checks
// both trees tell the same. Run with -v to see the rewritten XML.
func TestRoundtrip(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.Abs(filepath.Join("../_data/demo", "_mkskill", "mkskill.config.xml")); root.ConfigFile != want {
		t.Errorf("ConfigFile = %q, want %q", root.ConfigFile, want)
	}

	// the rewritten config lands in the shared test/_out, laid out as a project
	// base so it can be reloaded through the same door (Save creates the folders)
	base := "../_out/roundtrip"
	saved := filepath.Join(base, "_mkskill", "mkskill.config.xml")
	if err := root.Save(nil, saved); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rewritten config:\n%s", out)

	root2 := &compiler.Root{ProjectBase: base}
	if err := root2.Load(); err != nil {
		t.Fatal(err)
	}

	compareTrees(t, root.Project, root2.Project)
}

// TestSaveToConfigFile checks the empty-destination rule: Save("") writes back
// to the ConfigFile the document was loaded from.
func TestSaveToConfigFile(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}
	// retarget the ConfigFile to the output area, then save with no destination
	root.ConfigFile = filepath.Join("../_out/roundtrip", "_mkskill", "saved-to-configfile.xml")
	if err := root.Save(nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root.ConfigFile); err != nil {
		t.Fatalf("Save(\"\") did not write to ConfigFile: %v", err)
	}

	// with neither destination nor ConfigFile there is nowhere to write
	empty := &compiler.Root{}
	if err := empty.Save(nil, ""); err == nil {
		t.Error("Save(\"\") with no ConfigFile should fail")
	}
}

// TestClone checks the deep copy: same tree, and mutating the clone leaves
// the original untouched.
func TestClone(t *testing.T) {
	root := &compiler.Root{ProjectBase: "../_data/demo"}
	if err := root.Load(); err != nil {
		t.Fatal(err)
	}

	clone, err := root.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if clone.ProjectBase != root.ProjectBase || clone.ConfigFile != root.ConfigFile {
		t.Error("runtime fields not carried over")
	}
	compareTrees(t, root.Project, clone.Project)

	clone.Project.Name = "mutated"
	clone.Project.Children[0].Path = "./elsewhere"
	if root.Project.Name == "mutated" || root.Project.Children[0].Path == "./elsewhere" {
		t.Error("mutating the clone touched the original")
	}
}

// compareTrees checks that two project trees tell the same.
func compareTrees(t *testing.T, p1, p2 *compiler.Project) {
	t.Helper()
	if p1.Id != p2.Id || p1.Name != p2.Name || p1.ProjectType != p2.ProjectType || p1.Path != p2.Path {
		t.Errorf("project differs: %q/%q/%q/%q vs %q/%q/%q/%q",
			p1.Id, p1.Name, p1.ProjectType, p1.Path, p2.Id, p2.Name, p2.ProjectType, p2.Path)
	}
	if (p1.Embed == nil) != (p2.Embed == nil) {
		t.Error("embed presence differs")
	} else if p1.Embed != nil && p1.Embed.Filename.Raw != p2.Embed.Filename.Raw {
		// the RAW is the document truth a clone must carry; resolution
		// state is the run's, not the document's
		t.Errorf("embed differs: %q vs %q", p1.Embed.Filename.Raw, p2.Embed.Filename.Raw)
	}
	if len(p1.Children) != len(p2.Children) {
		t.Fatalf("children count differs: %d vs %d", len(p1.Children), len(p2.Children))
	}
	for i := range p1.Children {
		compareTrees(t, p1.Children[i], p2.Children[i])
	}
}
