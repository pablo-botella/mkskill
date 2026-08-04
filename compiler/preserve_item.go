package compiler

import (
	"context"
	"encoding/xml"
	"iter"

	"github.com/pablo-botella/cargoxml"
)

// PreserveItem is one entry of a Project's <preserve> section: a file mkskill must
// not overwrite blindly.
type PreserveItem struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml // the unclaimed (attributes, children, trails), kept for faithful rewriting
	Project *Project           // owning project — root or child alike; set when linking, not serialized

	File   Expandable // file — exact name or mask/glob
	Method Expandable // method — enum: alt (default) | alias (validated after the render)
	Alias  Expandable // alias — destination name when Method == "alias"
}

// GetCargoXml wires the preservation: whatever OnXmlAttribute does not claim
// ends up stored here by the decoder — it has no known children, so every
// child falls here too.
func (it *PreserveItem) GetCargoXml() *cargoxml.CargoXml {
	if it.Cargo == nil {
		it.Cargo = cargoxml.NewCargoXml()
	}
	return it.Cargo
}

// OnXmlAttribute claims the item's own attributes; anything else falls to
// the cargo.
func (it *PreserveItem) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	switch a.Name.Local {
	case "file":
		it.File.SetRawAt(a.Value, xmlLine(d))
	case "method":
		it.Method.SetRawAt(a.Value, xmlLine(d))
	case "alias":
		it.Alias.SetRawAt(a.Value, xmlLine(d))
	default:
		return false
	}
	return true
}

// OnXmlEnd settles the defaults once the element is complete.
func (it *PreserveItem) OnXmlEnd(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	if it.Method.Empty() {
		it.Method.SetRaw("alt") // default
	}
	return nil
}

// --- describe side + producer: how the item presents itself for writing ---

var (
	_ cargoxml.XmlDescribeWithCargo = (*PreserveItem)(nil)
	_ cargoxml.XmlTokenProducer     = (*PreserveItem)(nil)
)

func (it *PreserveItem) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "item"}
}

func (it *PreserveItem) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlContainerNode // no text wanted here
}

// XmlDescribeAttributes answers the item's own attributes; empty ones are
// omitted (absent in, absent out).
func (it *PreserveItem) XmlDescribeAttributes(ctx context.Context) []xml.Attr {
	var attrs []xml.Attr
	add := func(name, value string) {
		if value != "" {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
		}
	}
	// Expandables ALWAYS emit their raw: a rewrite never freezes a template
	if !it.File.Empty() {
		add("file", it.File.Raw)
	}
	if !it.Method.Empty() {
		add("method", it.Method.Raw)
	}
	if !it.Alias.Empty() {
		add("alias", it.Alias.Raw)
	}
	return attrs
}

func (it *PreserveItem) XmlDescribeInitialComments(ctx context.Context) []string          { return nil }
func (it *PreserveItem) XmlDescribeText(ctx context.Context) []string                     { return nil }
func (it *PreserveItem) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer { return nil }

func (it *PreserveItem) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, it, cargoxml.PreserveMixed)
}
