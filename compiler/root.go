package compiler

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pablo-botella/cargoxml"
)

// ConfigFileRel is the conventional location of the config file, relative to
// the project base.
const _SkillFolder = "_mkskill"
const _ConfigFileName = "mkskill.config.xml"
const _SrcFolder = "src"

// Root is the config file: it consumes the <mkskill> document element, whose
// only known child is the single <project>. Its cargo keeps the document-level
// unclaimed (foreign attributes/children of <mkskill>, prolog trails).
type Root struct {
	cargoxml.NullXmlConsumer
	Cargo       *cargoxml.CargoXml // the unclaimed (attributes, children, trails), kept for faithful rewriting
	Project     *Project
	ProjectBase string   // base path for the project; empty means the current folder
	ConfigFile  string   // fully qualified config file name; empty means <ProjectBase>/_mkskill/mkskill.config.xml
	Warns       []string // load-time warnings (defaulted ids, …), not serialized

	ProjectMap map[string]*Project // every unit by its (final) id — cross-unit references resolve here

	// Family is the ledger of this load's Expandables: captured raw while
	// parsing, contextualized when the tree links, resolved in one pass
	// against the version universe. Traversable — nothing hides.
	Family ExpandableFamily

	// the version universe of the same file, loaded once on demand — the
	// full parser borrowing the lazy one (the Family resolves there)
	lazyDoc   *LazyDoc
	lazyErr   error
	lazyTried bool
}

// compile-time checks: describe side and producer in place
var (
	_ cargoxml.XmlDescribeWithCargo = (*Root)(nil)
	_ cargoxml.XmlTokenProducer     = (*Root)(nil)
)

// GetProjectList returns the whole tree as a simple flat array: the main
// project first, then the deeper levels — the list is its own worklist, no
// recursion involved.
func (c *Root) GetProjectList() []*Project {
	var list []*Project
	if c.Project != nil {
		list = append(list, c.Project)
	}
	for i := 0; i < len(list); i++ {
		list = append(list, list[i].Children...)
	}
	return list
}

// GetCargoXml wires the preservation: whatever the events below do not claim
// ends up stored here by the decoder.
func (c *Root) GetCargoXml() *cargoxml.CargoXml {
	if c.Cargo == nil {
		c.Cargo = cargoxml.NewCargoXml()
	}
	return c.Cargo
}

// OnXmlStart validates its own element: the document element must be <mkskill>.
func (c *Root) OnXmlStart(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	if frame.NodeName.Local != "mkskill" {
		return NewParseError(d, "document element must be <mkskill>")
	}
	return nil
}

// OnXmlChildStart assigns the single <project> to its consumer; an unknown
// child is left unclaimed, so the decoder parses it generically into the cargo.
func (c *Root) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	if child.NodeName.Local == "project" {
		if c.Project != nil {
			return NewParseError(d, "multiple <project> elements in <mkskill>, only 1 is allowed")
		}
		c.Project = &Project{Root: c}
		child.Consumer = c.Project
	}
	return nil
}

// --- describe side: how the config document presents itself for writing ---

func (c *Root) XmlDescribeNodeName(ctx context.Context) xml.Name { return xml.Name{Local: "mkskill"} }

func (c *Root) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlContainerNode
}

func (c *Root) XmlDescribeAttributes(ctx context.Context) []xml.Attr    { return nil }
func (c *Root) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (c *Root) XmlDescribeText(ctx context.Context) []string            { return nil }

func (c *Root) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	if c.Project == nil {
		return nil
	}
	return []cargoxml.XmlTokenProducer{c.Project}
}

func (c *Root) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, c, cargoxml.PreserveMixed)
}

// CleanUpLastScan removes what the last scan left behind, delegating: each
// project cleans its own ground before collecting again. log receives the
// run's record; nil discards it.
func (c *Root) CleanUpLastScan(log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	for _, p := range c.GetProjectList() {
		if err := p.CleanUpLastScan(log); err != nil {
			return err
		}
	}
	return nil
}

// Scan collects the sources of every project in the tree, pure delegation:
// each project settles its own Base and gathers its own sources — the flat
// list guarantees a parent is scanned before its children. log receives the
// run's record; nil discards it.
func (c *Root) Scan(log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	if err := c.CleanUpLastScan(log); err != nil {
		return err
	}
	for _, p := range c.GetProjectList() {
		if err := p.Scan(log); err != nil {
			return err
		}
	}
	return nil
}

// Prepare materializes what the scan found, pure delegation: each project
// leaves its own _mkskill/src ready for composition. log receives the run's
// record — what was copied, generated, skipped — and where it ends up (a
// prepare.log, stdout, …) is the caller's business; nil discards it.
func (c *Root) Prepare(log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	for _, p := range c.GetProjectList() {
		if err := p.Prepare(log); err != nil {
			return err
		}
	}
	return nil
}

// Resolve builds the sections of every project in the tree, pure delegation:
// each project reads its own materialized items and keeps its own sections.
// log receives the run's record; nil discards it.
func (c *Root) Resolve(log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	for _, p := range c.GetProjectList() {
		if err := p.Resolve(log); err != nil {
			return err
		}
	}
	return nil
}

// Deploy writes the artifacts of every project in the tree, pure
// delegation: each unit composes and writes its own outputs under its own
// base. log receives the run's record; nil discards it.
func (c *Root) Deploy(log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	for _, p := range c.GetProjectList() {
		if err := p.Deploy(log); err != nil {
			return err
		}
	}
	return nil
}

// Save writes the config document to filename; empty means back to the
// ConfigFile it was loaded from. ctx is the run's context (nil is fine):
// EncoderParams travel in it and reach every describe answer of the tree.
func (c *Root) Save(ctx context.Context, filename string) error {
	if filename == "" {
		filename = c.ConfigFile
	}
	if filename == "" {
		return NewParseError(nil, "no destination: neither filename nor ConfigFile set")
	}
	if dir := filepath.Dir(filename); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := xml.NewEncoder(f)
	e := cargoxml.NewEncoderWithCargo(enc)
	e.Context = ctx // the run's context: every describe question receives it
	if GetEncoderParams(ctx).PrettyOutput {
		// drop the preserved formatting and let the stdlib reindent: comments
		// and real text survive, whitespace is rebuilt clean
		e.SkipWhiteSpace = true
		enc.Indent("", "  ")
	}
	if err := e.Encode(c); err != nil {
		return err
	}
	return enc.Flush()
}

// Clone returns a deep copy of the document: a roundtrip through memory, so
// everything the XML carries (tree, cargos, trails) is copied for real and no
// field-by-field copy code can rot. The runtime fields travel by hand.
func (c *Root) Clone() (*Root, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	if err := cargoxml.NewEncoderWithCargo(enc).Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}

	clone := &Root{}
	d := cargoxml.NewDecoderWithCargo(xml.NewDecoder(&buf))
	d.Root = clone
	if err := d.Parse(); err != nil {
		return nil, err
	}
	clone.ProjectBase = c.ProjectBase
	clone.ConfigFile = c.ConfigFile
	return clone, nil
}

// Load reads the config file from its conventional location under ProjectBase.
func (c *Root) Load() error {

	base := c.ProjectBase
	if base == "" {
		base = "."
	}
	base, err := filepath.Abs(base) // resolves against the cwd and cleans
	if err != nil {
		return err
	}
	c.ProjectBase = base // absolute from here on; every relative path in the config resolves against it
	if c.ConfigFile == "" {
		c.ConfigFile = filepath.Join(base, _SkillFolder, _ConfigFileName) // the project's own
	}
	f, err := os.Open(c.ConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()

	d := cargoxml.NewDecoderWithCargo(xml.NewDecoder(f))
	d.Root = c // Root consumes <mkskill>; the prolog lands on it as Before trails

	if err := d.Parse(); err != nil {
		return err
	}
	if c.Project == nil {
		return NewParseError(nil, "no <project> element found in "+c.ConfigFile)
	}
	if c.Project.Name == "" {
		c.Project.Name = Literal(filepath.Base(base)) // default: the folder the project lives in
	}

	if err := c.automaticallyAssignMissingIdToProjectsAndCheckDupe(); err != nil {
		return err
	}

	// the Expandables' three phases end here: captured raw while parsing
	// (a plain SetRaw), ADOPTED into the family with their context now
	// that ids are final — this walk is the one place that knows every
	// adopter — and resolved in one pass against the version universe:
	// errors surface at load, where law 4 wants them. A family without
	// macros costs nothing. Every attribute-sourced value adopts; the
	// universe's own sources are the Literal type and refused their
	// macros at parse (law 1, in Project.OnXmlEnd, with positions).
	for _, p := range c.GetProjectList() {
		unit := string(p.Id)
		adopt := func(e *Expandable, element, name string) {
			c.Family.Adopt(e, ExpandableContext{UnitID: unit, Element: element, Name: name, Attr: true})
		}
		adopt(&p.Artifacts, "project", "artifacts")
		adopt(&p.GlobalArtifacts, "project", "global-artifacts")
		if e := p.Embed; e != nil {
			adopt(&e.Filename, "embed", "filename")
			adopt(&e.ModuleName, "embed", "module-name")
			adopt(&e.EmbedParent, "embed", "embed-parent")
			adopt(&e.EmbedVersion, "embed", "embed-version")
		}
		for _, im := range p.ImportMiniskin {
			adopt(&im.ContentFolder, "import-miniskin", "content")
			adopt(&im.FrontMatterGen, "import-miniskin", "fm-gen")
		}
		if p.Preserve != nil {
			for _, it := range p.Preserve.Items {
				adopt(&it.File, "item", "file")
				adopt(&it.Method, "item", "method")
				adopt(&it.Alias, "item", "alias")
			}
		}
	}
	if c.Family.NeedsResolution() {
		ld, err := c.versionLazy()
		if err != nil {
			return err
		}
		if err := c.Family.Resolve(ld); err != nil {
			return err
		}
	}
	// grammar attributes validate their RENDER, now that it exists —
	// artifacts, embed, preserve and fm-gen alike
	for _, p := range c.GetProjectList() {
		if err := p.validateRenders(); err != nil {
			return err
		}
	}
	return nil
}

// assignAutoIds walks the tree in document order (root first, then children
// in depth) giving every id-less unit the next free automatic id — with a
// warning: the default is a convenience, not a choice.
func (c *Root) automaticallyAssignMissingIdToProjectsAndCheckDupe() error {
	pl := c.GetProjectList()
	dupes := []string{}

	m := make(map[string]bool)
	for _, p := range pl {
		if id := string(p.Id); id != "" {
			if m[id] {
				dupes = append(dupes, id)
			}
			m[id] = true
		}
	}
	if len(dupes) > 0 {
		return fmt.Errorf("duplicate project ids found: %v", dupes)
	}

	for n, p := range pl {
		if p.Id == "" {
			id := "P" + strconv.Itoa(n)
			for nn := '1'; m[id]; nn++ {
				id = "P" + strconv.Itoa(n) + "_" + string(nn)
			}
			p.Id = Literal(id)
			c.Warns = append(c.Warns, fmt.Sprintf("project %q has no id, assigned %q", p.Name, p.Id))
		}
	}

	// ids are final from here on: index the units for cross-unit lookups
	c.ProjectMap = make(map[string]*Project, len(pl))
	for _, p := range pl {
		c.ProjectMap[string(p.Id)] = p
	}
	return nil
}
