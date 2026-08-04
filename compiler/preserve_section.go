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

// PreserveSection models the <preserve> group: the items to protect, plus its
// own cargo so anything foreign inside the wrapper survives in place.
type PreserveSection struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml // the unclaimed (attributes, children, trails), kept for faithful rewriting
	Project *Project           // owning project — root or child alike; set when linking, not serialized

	Items []*PreserveItem // item

	Files []*PreservedFile // the preservation map, resolved by Scan; not serialized
}

// PreservedFile is one resolved entry of the preservation map: a concrete
// file the rules protect, and whether it is on disk right now.
type PreservedFile struct {
	Path   string        // base-relative, slashed (e.g. "README.md", "doc/x.md")
	Exists bool          // whether it exists on disk at scan time
	Item   *PreserveItem // the rule that produced this entry (method/alias)
}

// CleanUpLastScan empties the resolved map: the section starts the next
// scan clean.
func (s *PreserveSection) CleanUpLastScan(log io.Writer) error {
	s.Files = nil
	return nil
}

// Scan resolves the rules against the owning project's Base into the
// preservation map. A wildcard pattern can only match files that exist
// (Glob walks the disk); an exact pattern enters the map even when the file
// is missing, with Exists false — which is exactly what one wants to know.
func (s *PreserveSection) Scan(log io.Writer) error {
	base := s.Project.Base
	for _, item := range s.Items {
		file := item.File.Get() // the render: expandables resolve at load
		pattern := filepath.FromSlash(file)
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(base, pattern)
		}
		if strings.ContainsAny(file, "*?[") {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return err
			}
			for _, match := range matches {
				s.addFile(base, match, true, item)
			}
			continue
		}
		_, err := os.Stat(pattern)
		s.addFile(base, pattern, err == nil, item)
	}
	fmt.Fprintf(log, "[%s] preserve map: %d file(s)\n", s.Project.Id, len(s.Files))
	return nil
}

// addFile appends one resolved entry, its path expressed base-relative.
func (s *PreserveSection) addFile(base, path string, exists bool, item *PreserveItem) {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		rel = path // outside the base: keep it as is
	}
	s.Files = append(s.Files, &PreservedFile{
		Path:   filepath.ToSlash(rel),
		Exists: exists,
		Item:   item,
	})
}

// GetCargoXml wires the preservation: whatever the events below do not claim
// ends up stored here by the decoder.
func (s *PreserveSection) GetCargoXml() *cargoxml.CargoXml {
	if s.Cargo == nil {
		s.Cargo = cargoxml.NewCargoXml()
	}
	return s.Cargo
}

// OnXmlChildStart assigns each <item> to a PreserveItem; an unknown child is
// left unclaimed, so the decoder parses it generically into the cargo.
func (s *PreserveSection) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	if child.NodeName.Local == "item" {
		it := &PreserveItem{Project: s.Project}
		s.Items = append(s.Items, it)
		child.Consumer = it
	}
	return nil
}

func (s *PreserveSection) OnXmlEnd(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	// probably nothing to validate here as just a container for the items
	return nil
}

// --- describe side + producer: how the section presents itself for writing ---

var (
	_ cargoxml.XmlDescribeWithCargo = (*PreserveSection)(nil)
	_ cargoxml.XmlTokenProducer     = (*PreserveSection)(nil)
)

func (s *PreserveSection) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "preserve"}
}

func (s *PreserveSection) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlContainerNode // no text wanted here
}

func (s *PreserveSection) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return nil }
func (s *PreserveSection) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (s *PreserveSection) XmlDescribeText(ctx context.Context) []string            { return nil }

// XmlDescribeItems answers the section's items, in order — Items adapts the
// typed slice as one producer.
func (s *PreserveSection) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	if len(s.Items) == 0 {
		return nil
	}
	return []cargoxml.XmlTokenProducer{cargoxml.Items(s.Items)}
}

func (s *PreserveSection) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, s, cargoxml.PreserveMixed)
}
