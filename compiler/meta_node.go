package compiler

import (
	"context"
	"encoding/xml"
	"iter"
	"strings"

	"github.com/pablo-botella/cargoxml"
)

// MetaNode is the <meta> element of a project: free metadata, any tag
// inside — <module>, <license>, whatever the user means. mkskill never
// models the keys: the children stay unclaimed and travel whole in the
// cargo (comments and formatting included, faithful rewriting for free);
// consumers read a value by name when they need one (macros, wrappers…).
type MetaNode struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml // where every child lands, generically — the storage itself
	Project *Project           // owning project — root or child alike; set when linking, not serialized
}

// compile-time checks: describe side and producer in place
var (
	_ cargoxml.XmlDescribeWithCargo = (*MetaNode)(nil)
	_ cargoxml.XmlTokenProducer     = (*MetaNode)(nil)
)

// GetCargoXml wires the whole mechanism: with a cargo present and no
// claimed children, the decoder parses every tag generically into it.
func (m *MetaNode) GetCargoXml() *cargoxml.CargoXml {
	if m.Cargo == nil {
		m.Cargo = cargoxml.NewCargoXml()
	}
	return m.Cargo
}

// Get returns the trimmed inner text of the first <name> child; empty when
// the tag is not there — absence and emptiness read the same.
func (m *MetaNode) Get(name string) string {
	if m == nil || m.Cargo == nil {
		return ""
	}
	for _, child := range m.Cargo.MoreChildren {
		if child.Name != nil && child.Name.Local == name {
			return genericInnerText(child)
		}
	}
	return ""
}

// genericInnerText concatenates a generic item's Inner text trails.
func genericInnerText(node *cargoxml.GenericXmlItem) string {
	var b strings.Builder
	if node.Trails != nil {
		for _, trail := range *node.Trails {
			if trail.Position == cargoxml.TrailInner && trail.Type&cargoxml.TrailAnyText != 0 {
				b.Write(trail.Content)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// --- describe side + producer: the element is its cargo ---

func (m *MetaNode) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "meta"}
}

func (m *MetaNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode // whatever the cargo holds, it emits
}

func (m *MetaNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr             { return nil }
func (m *MetaNode) XmlDescribeInitialComments(ctx context.Context) []string          { return nil }
func (m *MetaNode) XmlDescribeText(ctx context.Context) []string                     { return nil }
func (m *MetaNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer { return nil }

func (m *MetaNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, m, cargoxml.PreserveMixed)
}
