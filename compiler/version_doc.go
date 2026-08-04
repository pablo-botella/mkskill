package compiler

// Version subsystem — the write path. A cargoxml document with the
// holdings INVERTED from Root's: it claims ONLY what the version
// subsystem owns — <version-spec>'s <version>, <ts> and <label> children,
// and <version-history>'s <release> entries — and adopts EVERY other
// child as a generic item into one ordered list. That list is what
// preserves the user's placement: cargo children would re-emit after the
// claimed ones (cargoxml records no interleaving), so nothing falls to
// the cargo as a child here — the cargo keeps trails and foreign
// attributes only.
//
// The version-owned leaves (<ts>, a mutated <label>) rewrite their text
// node; their foreign INNER content (a comment inside the element) dies
// knowingly with the rewrite — those elements are wholly ours. Untouched
// leaves round-trip with their inner content intact.

import (
	"context"
	"encoding/xml"
	"fmt"
	"iter"
	"os"
	"strconv"
	"strings"

	"github.com/pablo-botella/cargoxml"
)

// --- the document ---

// VersionDoc is the mutators' view of the config file: everything not
// owned by the version subsystem travels opaque and comes out in place.
type VersionDoc struct {
	cargoxml.NullXmlConsumer
	Cargo      *cargoxml.CargoXml
	Items      []cargoxml.XmlTokenProducer // every child of <mkskill>, document order
	Spec       *vSpecNode                  // nil when the config has none
	History    *vHistNode                  // nil when the config has none
	ConfigFile string
}

// VersionDocLoad reads the config for mutation.
func VersionDocLoad(filename string) (*VersionDoc, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	doc := &VersionDoc{ConfigFile: filename}
	d := cargoxml.NewDecoderWithCargo(xml.NewDecoder(f))
	d.Root = doc
	if err := d.Parse(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (c *VersionDoc) GetCargoXml() *cargoxml.CargoXml {
	if c.Cargo == nil {
		c.Cargo = cargoxml.NewCargoXml()
	}
	return c.Cargo
}

func (c *VersionDoc) OnXmlStart(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	if frame.NodeName.Local != "mkskill" {
		return NewParseError(d, "document element must be <mkskill>")
	}
	return nil
}

// OnXmlChildStart adopts every child in document order: the version
// elements as their own nodes, anything else as a generic item — the
// misplaced or duplicated version element included (the lazy pass is the
// one that warns; the write path just must not lose content).
func (c *VersionDoc) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	switch child.NodeName.Local {
	case "version-spec":
		if c.Spec == nil {
			c.Spec = &vSpecNode{}
			child.Consumer = c.Spec
			c.Items = append(c.Items, c.Spec)
			return nil
		}
	case "version-history":
		if c.History == nil {
			c.History = &vHistNode{}
			child.Consumer = c.History
			c.Items = append(c.Items, c.History)
			return nil
		}
	}
	g := cargoxml.NewGenericXmlItem()
	g.Name = child.NodeName
	child.Consumer = g
	c.Items = append(c.Items, g)
	return nil
}

func (c *VersionDoc) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "mkskill"}
}
func (c *VersionDoc) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode
}
func (c *VersionDoc) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return nil }
func (c *VersionDoc) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (c *VersionDoc) XmlDescribeText(ctx context.Context) []string            { return nil }
func (c *VersionDoc) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	return c.Items
}
func (c *VersionDoc) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, c, cargoxml.PreserveMixed)
}

// Save writes the document back; empty means the file it came from.
func (c *VersionDoc) Save(filename string) error {
	if filename == "" {
		filename = c.ConfigFile
	}
	if filename == "" {
		return fmt.Errorf("no destination to save the version document")
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := xml.NewEncoder(f)
	e := cargoxml.NewEncoderWithCargo(enc)
	if err := e.Encode(c); err != nil {
		return err
	}
	return enc.Flush()
}

// stripInner drops an element's Inner trails — the one right a mutator
// has over its own leaves: the old text (and whatever lived with it)
// yields to the new value.
func stripInner(c *cargoxml.CargoXml) {
	if c == nil || c.Trails == nil {
		return
	}
	kept := (*c.Trails)[:0]
	for _, t := range *c.Trails {
		if t.Position != cargoxml.TrailInner {
			kept = append(kept, t)
		}
	}
	*c.Trails = kept
}

// innerText concatenates the Inner text trails of a frame — the captured
// text node of a version-owned leaf.
func vInnerText(trails *cargoxml.Trails) string {
	if trails == nil {
		return ""
	}
	var b strings.Builder
	for _, t := range *trails {
		if t.Position == cargoxml.TrailInner && t.Type&cargoxml.TrailAnyText != 0 {
			b.Write(t.Content)
		}
	}
	return strings.TrimSpace(b.String())
}

// --- <version-spec> ---

type vSpecNode struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml
	Items   []cargoxml.XmlTokenProducer
	Version *vVersionNode
	Labels  []*vLabelNode
}

func (n *vSpecNode) GetCargoXml() *cargoxml.CargoXml {
	if n.Cargo == nil {
		n.Cargo = cargoxml.NewCargoXml()
	}
	return n.Cargo
}

func (n *vSpecNode) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	switch child.NodeName.Local {
	case "version":
		if n.Version == nil {
			n.Version = &vVersionNode{}
			child.Consumer = n.Version
			n.Items = append(n.Items, n.Version)
			return nil
		}
	case "label":
		l := &vLabelNode{}
		child.Consumer = l
		n.Labels = append(n.Labels, l)
		n.Items = append(n.Items, l)
		return nil
	}
	g := cargoxml.NewGenericXmlItem()
	g.Name = child.NodeName
	child.Consumer = g
	n.Items = append(n.Items, g)
	return nil
}

// Label finds a claimed label by its key; nil when absent.
func (n *vSpecNode) Label(key string) *vLabelNode {
	for _, l := range n.Labels {
		if l.Key == key {
			return l
		}
	}
	return nil
}

func (n *vSpecNode) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "version-spec"}
}
func (n *vSpecNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode
}
func (n *vSpecNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return nil } // format/params live in the cargo
func (n *vSpecNode) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (n *vSpecNode) XmlDescribeText(ctx context.Context) []string            { return nil }
func (n *vSpecNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	return n.Items
}
func (n *vSpecNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, n, cargoxml.PreserveMixed)
}

// --- <version> ---

type vVersionNode struct {
	cargoxml.NullXmlConsumer
	Cargo *cargoxml.CargoXml
	Attrs []xml.Attr // claimed whole, document order — components and lock re-emit in place
	Items []cargoxml.XmlTokenProducer
	Ts    *vTsNode
}

func (n *vVersionNode) GetCargoXml() *cargoxml.CargoXml {
	if n.Cargo == nil {
		n.Cargo = cargoxml.NewCargoXml()
	}
	return n.Cargo
}

func (n *vVersionNode) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	n.Attrs = append(n.Attrs, *a)
	return true
}

func (n *vVersionNode) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	if child.NodeName.Local == "ts" && n.Ts == nil {
		n.Ts = &vTsNode{}
		child.Consumer = n.Ts
		n.Items = append(n.Items, n.Ts)
		return nil
	}
	g := cargoxml.NewGenericXmlItem()
	g.Name = child.NodeName
	child.Consumer = g
	n.Items = append(n.Items, g)
	return nil
}

// SetComponent updates one component attribute in place, appending it
// when the element never declared it.
func (n *vVersionNode) SetComponent(name string, value int) {
	text := strconv.Itoa(value)
	for i := range n.Attrs {
		if n.Attrs[i].Name.Local == name {
			n.Attrs[i].Value = text
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: text})
}

// SetTs stamps the mutation instant, creating the <ts> child if the
// element never had one.
func (n *vVersionNode) SetTs(ts string) {
	if n.Ts == nil {
		n.Ts = &vTsNode{}
		n.Items = append([]cargoxml.XmlTokenProducer{n.Ts}, n.Items...)
	}
	n.Ts.Set(ts)
}

func (n *vVersionNode) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "version"}
}
func (n *vVersionNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode
}
func (n *vVersionNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return n.Attrs }
func (n *vVersionNode) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (n *vVersionNode) XmlDescribeText(ctx context.Context) []string            { return nil }
func (n *vVersionNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	return n.Items
}
func (n *vVersionNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, n, cargoxml.PreserveMixed)
}

// --- <ts> ---

type vTsNode struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml
	Value   string
	mutated bool
}

func (n *vTsNode) GetCargoXml() *cargoxml.CargoXml {
	if n.Cargo == nil {
		n.Cargo = cargoxml.NewCargoXml()
	}
	return n.Cargo
}

func (n *vTsNode) OnXmlEnd(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	n.Value = vInnerText(frame.Trails)
	return nil
}

func (n *vTsNode) Set(ts string) {
	n.Value = ts
	n.mutated = true
	stripInner(n.Cargo)
}

func (n *vTsNode) XmlDescribeNodeName(ctx context.Context) xml.Name { return xml.Name{Local: "ts"} }
func (n *vTsNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode
}
func (n *vTsNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return nil }
func (n *vTsNode) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (n *vTsNode) XmlDescribeText(ctx context.Context) []string {
	if n.mutated {
		return []string{n.Value}
	}
	return nil // the original text still lives in the cargo's Inner trails
}
func (n *vTsNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer { return nil }
func (n *vTsNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, n, cargoxml.PreserveMixed)
}

// --- <label> (write side) ---

type vLabelNode struct {
	cargoxml.NullXmlConsumer
	Cargo   *cargoxml.CargoXml
	Attrs   []xml.Attr
	Key     string
	Value   string
	mutated bool
}

func (n *vLabelNode) GetCargoXml() *cargoxml.CargoXml {
	if n.Cargo == nil {
		n.Cargo = cargoxml.NewCargoXml()
	}
	return n.Cargo
}

func (n *vLabelNode) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	if a.Name.Local == "key" {
		n.Key = a.Value
	}
	n.Attrs = append(n.Attrs, *a)
	return true
}

func (n *vLabelNode) OnXmlEnd(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	n.Value = vInnerText(frame.Trails)
	return nil
}

func (n *vLabelNode) SetValue(v string) {
	n.Value = v
	n.mutated = true
	stripInner(n.Cargo)
}

func (n *vLabelNode) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "label"}
}
func (n *vLabelNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode
}
func (n *vLabelNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return n.Attrs }
func (n *vLabelNode) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (n *vLabelNode) XmlDescribeText(ctx context.Context) []string {
	if n.mutated {
		if n.Value == "" {
			return nil
		}
		return []string{n.Value}
	}
	return nil
}
func (n *vLabelNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer { return nil }
func (n *vLabelNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, n, cargoxml.PreserveMixed)
}

// --- <version-history> ---

type vHistNode struct {
	cargoxml.NullXmlConsumer
	Cargo *cargoxml.CargoXml
	Items []cargoxml.XmlTokenProducer
	Max   int // the max attribute; 0 = unlimited
}

func (n *vHistNode) GetCargoXml() *cargoxml.CargoXml {
	if n.Cargo == nil {
		n.Cargo = cargoxml.NewCargoXml()
	}
	return n.Cargo
}

// OnXmlAttribute reads max but leaves it in the cargo — the mutator
// consults it, never rewrites it.
func (n *vHistNode) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	if a.Name.Local == "max" {
		if v, err := strconv.Atoi(a.Value); err == nil {
			n.Max = v
		}
	}
	return false
}

// Existing releases are adopted generic — they re-emit untouched; the
// mutator only inserts fresh ones and trims from the bottom.
func (n *vHistNode) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	g := cargoxml.NewGenericXmlItem()
	g.Name = child.NodeName
	child.Consumer = g
	n.Items = append(n.Items, g)
	return nil
}

// isRelease recognizes a history entry among the items.
func isRelease(p cargoxml.XmlTokenProducer) bool {
	switch it := p.(type) {
	case *cargoxml.GenericXmlItem:
		return it.Name != nil && it.Name.Local == "release"
	case *vFreshRelease:
		return true
	}
	return false
}

// InsertRelease puts a fresh entry AT THE TOP — before the first existing
// release — and trims FIFO at the bottom when max says so.
func (n *vHistNode) InsertRelease(attrs []xml.Attr) {
	fresh := &vFreshRelease{Attrs: attrs}
	at := len(n.Items)
	for i, p := range n.Items {
		if isRelease(p) {
			at = i
			break
		}
	}
	n.Items = append(n.Items[:at], append([]cargoxml.XmlTokenProducer{fresh}, n.Items[at:]...)...)

	if n.Max <= 0 {
		return
	}
	count := 0
	for _, p := range n.Items {
		if isRelease(p) {
			count++
		}
	}
	for count > n.Max {
		for i := len(n.Items) - 1; i >= 0; i-- {
			if isRelease(n.Items[i]) {
				n.Items = append(n.Items[:i], n.Items[i+1:]...)
				count--
				break
			}
		}
	}
}

func (n *vHistNode) XmlDescribeNodeName(ctx context.Context) xml.Name {
	return xml.Name{Local: "version-history"}
}
func (n *vHistNode) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlMixedNode
}
func (n *vHistNode) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return nil } // max lives in the cargo
func (n *vHistNode) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (n *vHistNode) XmlDescribeText(ctx context.Context) []string            { return nil }
func (n *vHistNode) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	return n.Items
}
func (n *vHistNode) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, n, cargoxml.PreserveMixed)
}

// vFreshRelease is a hand-authored history entry: a pure producer — an
// indent, the element, done.
type vFreshRelease struct {
	Attrs []xml.Attr
}

func (r *vFreshRelease) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return func(yield func(xml.Token) bool) {
		if !yield(xml.CharData("\n    ")) {
			return
		}
		start := xml.StartElement{Name: xml.Name{Local: "release"}, Attr: r.Attrs}
		if !yield(start) {
			return
		}
		yield(xml.EndElement{Name: start.Name})
	}
}
