package compiler

import (
	"context"
	"encoding/xml"
	"iter"
	"strings"

	"github.com/pablo-botella/cargoxml"
)

// ItemOriginType tells where a collected Item came from.
type SourceItemOriginType int

const (
	ItemOriginExisting SourceItemOriginType = iota // a file already present under _mkskill/src
	ItemOriginMiniskin                             // materialized by an <import-miniskin>
)

// SourceItem is one collected source, whatever its origin. Content and
// interpretation live in the Section built from it.
type SourceItem struct {
	Parent      SourceItemContainer  // the container that owns this item
	DstPath     string               // _mkskill/src based
	FmExternal  bool                 // true if have an external .fm file
	FmPreserve  bool                 // true if an existing .fm is used as is, not regenerated (fm-gen ",preserve")
	DstFileName string               // the file name of the .md file (without path)
	OriginType  SourceItemOriginType // where this item came from
	OriginPath  string               // the path of the source, if any (e.g. the miniskin file)

	ForeignAttrib []string // "key=value" pairs from the miniskin item or any other foreign source, mkskill- prefix stripped (e.g. "pos=20", "in=*")

	PreserveConflict *PreservedFile // the preservation entry this item's destination collides with; nil = no conflict
}

var _ cargoxml.XmlTokenProducer = (*SourceItem)(nil) // producer face only: debug output, never read back

// XmlTokens emits the item as one <source-item> element with its data as
// attributes — pure debug output (IncludeScanData): a hand-rolled stream,
// producing is free.
func (it *SourceItem) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return func(yield func(xml.Token) bool) {
		var attrs []xml.Attr
		add := func(name, value string) {
			if value != "" {
				attrs = append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
			}
		}
		add("dst-path", it.DstPath)
		add("dst-file", it.DstFileName)
		if it.FmExternal {
			add("fm-external", "true")
		}
		if it.FmPreserve {
			add("fm-preserve", "true")
		}
		switch it.OriginType {
		case ItemOriginExisting:
			add("origin", "existing")
		case ItemOriginMiniskin:
			add("origin", "miniskin")
		}
		add("origin-path", it.OriginPath)
		for _, kv := range it.ForeignAttrib {
			if k, v, ok := strings.Cut(kv, "="); ok {
				add(k, v)
			}
		}
		start := xml.StartElement{Name: xml.Name{Local: "source-item"}, Attr: attrs}
		if !yield(start) {
			return
		}
		yield(xml.EndElement{Name: start.Name})
	}
}
