package compiler

import (
	"context"
	"encoding/xml"
	"iter"

	"github.com/pablo-botella/cargoxml"
)

// MkEmbed describes the Go embed a project exposes: which file holds the embedded
// content and under which module it lives. On a child, EmbedParent selects which of
// the parent's views the child inherits.
type MkEmbed struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml // the unclaimed (attributes, children, trails), kept for faithful rewriting
	Project *Project           // owning project — root or child alike; set when linking, not serialized

	// the first three validate AFTER the render (validateRenders): a macro
	// is legal in the attribute, what it assembles must honor the grammar
	Filename     Expandable // filename — path to the embed file, relative to the base project; a .go inside the tree
	ModuleName   Expandable // module-name — a Go identifier; empty = "main"
	EmbedParent  Expandable // embed-parent — child only: * | readme,agents,skill
	EmbedVersion Expandable // embed-version — free text: Raw in the document, its render is what the embed carries
}

// compile-time checks: describe side and producer in place
var (
	_ cargoxml.XmlDescribeWithCargo = (*MkEmbed)(nil)
	_ cargoxml.XmlTokenProducer     = (*MkEmbed)(nil)
)

// GetCargoXml wires the preservation: whatever OnXmlAttribute does not claim
// ends up stored here by the decoder — it has no known children, so every
// child falls here too.
func (e *MkEmbed) GetCargoXml() *cargoxml.CargoXml {
	if e.Cargo == nil {
		e.Cargo = cargoxml.NewCargoXml()
	}
	return e.Cargo
}

// OnXmlAttribute claims the embed's own attributes; anything else falls to
// the cargo.
func (e *MkEmbed) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	switch a.Name.Local {
	case "filename":
		e.Filename.SetRawAt(a.Value, xmlLine(d))
	case "module-name":
		e.ModuleName.SetRawAt(a.Value, xmlLine(d))
	case "embed-parent":
		e.EmbedParent.SetRawAt(a.Value, xmlLine(d))
	case "embed-version":
		e.EmbedVersion.SetRawAt(a.Value, xmlLine(d))
	default:
		return false
	}
	return true
}

// No OnXmlEnd: there is nothing an embed can validate at the parse. Its
// four attributes are Expandable, so what the document holds here is a
// raw that may still be a hole — `module-name="{$$$ glb:label:stem $$$}"`
// is not a Go identifier YET, and refusing it here would refuse a legal
// config. The rules live where the render exists: validateRenders.

// --- producer side: describe yourself, DescribedTokens assembles the stream ---

func (e *MkEmbed) XmlDescribeNodeName(ctx context.Context) xml.Name { return xml.Name{Local: "embed"} }

func (e *MkEmbed) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlContainerNode // no text wanted here
}

// XmlDescribeAttributes answers the embed's own attributes; empty ones are
// omitted (absent in, absent out).
func (e *MkEmbed) XmlDescribeAttributes(ctx context.Context) []xml.Attr {
	var attrs []xml.Attr
	add := func(name, value string) {
		if value != "" {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
		}
	}
	// Expandables ALWAYS emit their raw: a rewrite never freezes a template
	if !e.Filename.Empty() {
		add("filename", e.Filename.Raw)
	}
	if !e.ModuleName.Empty() {
		add("module-name", e.ModuleName.Raw)
	}
	if !e.EmbedParent.Empty() {
		add("embed-parent", e.EmbedParent.Raw)
	}
	if !e.EmbedVersion.Empty() {
		add("embed-version", e.EmbedVersion.Raw)
	}
	return attrs
}

func (e *MkEmbed) XmlDescribeInitialComments(ctx context.Context) []string          { return nil }
func (e *MkEmbed) XmlDescribeText(ctx context.Context) []string                     { return nil }
func (e *MkEmbed) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer { return nil }

func (e *MkEmbed) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, e, cargoxml.PreserveMixed)
}
