package compiler

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/cargoxml"
)

// MiniskinNode pulls the mkskill-declared sources from the *.miniskin.xml
// manifests found under ContentFolder. A project may have several (usually 0 or 1).
type MiniskinNode struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml // the unclaimed (attributes, children, trails), kept for faithful rewriting
	Project *Project           // owning project — root or child alike; set when linking, not serialized

	ContentFolder      Expandable                // content — folder to scan for manifests, relative to the content base
	FrontMatterGen     Expandable                // fm-gen — embed (default) | extern[,preserve] (validated after the render)
	ImportManifestList []*MiniskinImportManifest // the manifests found under ContentFolder, if any

}

// GetCargoXml wires the preservation: whatever OnXmlAttribute does not claim
// ends up stored here by the decoder — it has no known children, so every
// child falls here too.
func (node *MiniskinNode) GetCargoXml() *cargoxml.CargoXml {
	if node.Cargo == nil {
		node.Cargo = cargoxml.NewCargoXml()
	}
	return node.Cargo
}

// OnXmlChildStart declines the debug output of a previous IncludeScanData
// run: regenerable scan data, not config — skipped entirely, not even cargo.
func (node *MiniskinNode) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	if child.NodeName.Local == "scan" {
		child.SkipUnknownChildren = true
	}
	return nil
}

// OnXmlAttribute claims the node's own attributes; anything else falls to
// the cargo.
func (node *MiniskinNode) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	switch a.Name.Local {
	case "content":
		node.ContentFolder.SetRawAt(a.Value, xmlLine(d))
	case "fm-gen":
		node.FrontMatterGen.SetRawAt(a.Value, xmlLine(d))
	default:
		return false
	}
	return true
}

func (node *MiniskinNode) OnXmlEnd(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	// validations
	return nil
}

// CleanUpLastScan drops the manifests the last scan found (and with them
// their harvested items): the import starts the next one clean.
func (node *MiniskinNode) CleanUpLastScan(log io.Writer) error {
	node.ImportManifestList = nil
	return nil
}

// Scan walks the content folder recursively for *.miniskin.xml manifests:
// each one found becomes a MiniskinImportManifest that scans itself. The
// content folder resolves against the owning project's Base.
func (node *MiniskinNode) Scan(log io.Writer) error {
	contentDir := filepath.Join(node.Project.Base, filepath.FromSlash(node.ContentFolder.Get()))
	return filepath.WalkDir(contentDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".miniskin.xml") {
			return nil
		}
		manifest := &MiniskinImportManifest{Path: path, Node: node}
		if err := manifest.Scan(log); err != nil {
			return err
		}
		node.ImportManifestList = append(node.ImportManifestList, manifest)
		fmt.Fprintf(log, "[%s] manifest %s: %d item(s)\n", node.Project.Id, path, len(manifest.SourceItems))
		return nil
	})
}

// --- describe side + producer: how the import presents itself for writing ---

var (
	_ SourceItemContainer           = (*MiniskinNode)(nil)
	_ cargoxml.XmlDescribeWithCargo = (*MiniskinNode)(nil)
	_ cargoxml.XmlTokenProducer     = (*MiniskinNode)(nil)
)

func (node *MiniskinNode) GetSourceItems() []*SourceItem {
	items := []*SourceItem(nil)
	for _, manifest := range node.ImportManifestList {
		items = append(items, manifest.GetSourceItems()...)
	}
	return items
}
func (node *MiniskinNode) GetAllSourceItems() []*SourceItem {
	return node.GetSourceItems()
}
func (node *MiniskinNode) GetCurrentProject() *Project {
	return node.Project
}

func (node *MiniskinNode) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "import-miniskin"}
}

func (node *MiniskinNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlContainerNode // no text wanted here
}

// XmlDescribeAttributes answers the import's own attributes; empty ones are
// omitted (absent in, absent out).
func (node *MiniskinNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr {
	var attrs []xml.Attr
	add := func(name, value string) {
		if value != "" {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
		}
	}
	add("content", node.ContentFolder.Raw) // ALWAYS the raw: a rewrite never freezes a template
	add("fm-gen", node.FrontMatterGen.Raw) // ALWAYS the raw: a rewrite never freezes a template
	return attrs
}

func (node *MiniskinNode) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (node *MiniskinNode) XmlDescribeText(ctx context.Context) []string            { return nil }

// XmlDescribeItems has no config children; under IncludeScanData the found
// manifests come out grouped in a debug <scan>, each as an <import-manifest>
// with its items.
func (node *MiniskinNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	if !GetEncoderParams(ctx).IncludeScanData || len(node.ImportManifestList) == 0 {
		return nil
	}
	group := make(scanGroup, 0, len(node.ImportManifestList))
	for _, m := range node.ImportManifestList {
		group = append(group, m)
	}
	return []cargoxml.XmlTokenProducer{group}
}

func (node *MiniskinNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, node, cargoxml.PreserveMixed)
}
