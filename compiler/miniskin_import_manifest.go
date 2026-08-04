package compiler

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/cargoxml"
)

// MiniskinImportManifest is one *.miniskin.xml manifest found under an
// <import-miniskin> content folder: where it lives and what it declared for
// mkskill (the items carrying mkskill-* attributes).
type MiniskinImportManifest struct {
	Path        string        // full path of the manifest file
	Node        *MiniskinNode // the <import-miniskin> this manifest was found for
	SourceItems []*SourceItem // the items harvested from it
}

var (
	_ = SourceItemContainer(&MiniskinImportManifest{})
)

func (m *MiniskinImportManifest) GetSourceItems() []*SourceItem {
	return m.SourceItems
}

func (m *MiniskinImportManifest) GetAllSourceItems() []*SourceItem {
	return m.SourceItems
}

func (m *MiniskinImportManifest) GetCurrentProject() *Project {
	if m.Node != nil {
		return m.Node.Project
	}
	return nil
}

// XmlTokens emits the manifest as one <import-manifest> element containing
// its harvested items — pure debug output (IncludeScanData): a hand-rolled
// stream, producing is free.
func (m *MiniskinImportManifest) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return func(yield func(xml.Token) bool) {
		start := xml.StartElement{
			Name: xml.Name{Local: "import-manifest"},
			Attr: []xml.Attr{{Name: xml.Name{Local: "path"}, Value: m.Path}},
		}
		if !yield(start) {
			return
		}
		for _, item := range m.SourceItems {
			for tok := range item.XmlTokens(ctx) {
				if !yield(tok) {
					return
				}
			}
		}
		yield(xml.EndElement{Name: start.Name})
	}
}

// Scan parses the manifest generically (cargoxml, no miniskin dependency)
// and harvests its items: every <resource-list> at any level, its direct
// <item> children, keeping those that carry mkskill-* attributes.
func (m *MiniskinImportManifest) Scan(log io.Writer) error {
	f, err := os.Open(m.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	d := cargoxml.NewDecoderWithCargo(xml.NewDecoder(f))
	if err := d.Parse(); err != nil {
		return fmt.Errorf("%s: %w", m.Path, err)
	}
	if d.RootFrame == nil {
		return nil // empty document, nothing to harvest
	}
	root, ok := d.RootFrame.Consumer.(*cargoxml.GenericXmlItem)
	if !ok {
		return fmt.Errorf("%s: unexpected root consumer", m.Path)
	}

	var walk func(n *cargoxml.GenericXmlItem) error
	walk = func(n *cargoxml.GenericXmlItem) error {
		if n.Name.Local == "resource-list" {
			for _, child := range n.Children {
				if child.Name.Local != "item" {
					continue
				}
				if err := m.harvestItem(log, child); err != nil {
					return err
				}
			}
			return nil
		}
		for _, child := range n.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

// harvestItem reads one manifest <item>: nothing happens if it carries no
// mkskill-* attributes (not ours). file= is the source; mkskill-copy (or the
// file's basename) is the destination, contained in _mkskill/src; every other
// mkskill-* travels as a "key=value" foreign attribute.
func (m *MiniskinImportManifest) harvestItem(log io.Writer, node *cargoxml.GenericXmlItem) error {
	var file, copyDst string
	var foreign []string
	ours := false
	for _, a := range node.Attributes {
		switch {
		case a.Name.Local == "file":
			file = a.Value
		case strings.HasPrefix(a.Name.Local, "mkskill-"):
			ours = true
			key := strings.TrimPrefix(a.Name.Local, "mkskill-")
			if key == "copy" {
				copyDst = a.Value
			} else {
				foreign = append(foreign, key+"="+a.Value)
			}
		}
	}
	if !ours {
		return nil
	}
	if file == "" {
		return fmt.Errorf("%s: <item> carries mkskill-* attributes but no file=", m.Path)
	}

	// destination: mkskill-copy, or the file's basename; always contained
	dst := copyDst
	if dst == "" {
		dst = filepath.Base(filepath.FromSlash(file))
	}
	dst = filepath.ToSlash(filepath.Clean(filepath.FromSlash(dst)))
	if filepath.IsAbs(dst) || dst == ".." || strings.HasPrefix(dst, "../") {
		return fmt.Errorf("%s: mkskill-copy %q escapes _mkskill/src", m.Path, copyDst)
	}

	// origin: file= resolved against the manifest's folder, relative to the
	// owning project's base
	src := filepath.Join(filepath.Dir(m.Path), filepath.FromSlash(file))
	origin, err := filepath.Rel(m.Node.Project.Base, src)
	if err != nil {
		origin = src // outside the base: keep it as is
	}

	fmGen := m.Node.FrontMatterGen.Get()
	item := &SourceItem{
		Parent:        m,
		DstFileName:   filepath.Base(dst),
		FmExternal:    strings.Contains(fmGen, "extern"),
		FmPreserve:    strings.Contains(fmGen, "preserve"),
		OriginType:    ItemOriginMiniskin,
		OriginPath:    filepath.ToSlash(origin),
		ForeignAttrib: foreign,
	}
	if dir := filepath.Dir(dst); dir != "." {
		item.DstPath = filepath.ToSlash(dir)
	}
	m.SourceItems = append(m.SourceItems, item)
	fmt.Fprintf(log, "[%s] harvest %s -> %s\n", m.Node.Project.Id, item.OriginPath, dst)
	return nil
}
