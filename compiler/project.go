package compiler

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/pablo-botella/cargoxml"
	"github.com/pablo-botella/fmlines"
	"github.com/pablo-botella/linereader"
)

// projectTypes are the valid project-type values, in canonical casing.
var projectTypes = []string{"Go-Module", "Go-CLI", "Go", "JS-Bundle", "Other"}

// Project is one unit of documentation. The root <project> and every nested
// <child-project> share this single type; a child only differs by carrying a Path
// and a Parent. The tree is doubly linked: Children hold the nested units and Parent
// points back up (nil at the root).
type Project struct {
	cargoxml.NullXmlConsumer
	Cargo *cargoxml.CargoXml // the unclaimed (attributes, children, trails), kept for faithful rewriting

	Id              Literal    // id — mandatory: unique within the whole project tree; the prefix for cross-unit references
	Name            Literal    // name — a source of the version universe: a macro here is a law-1 error
	Artifacts       Expandable // artifacts — the views this unit deploys ("*" or "readme,agents,skill"); empty = everything at the root, only readme in a child
	GlobalArtifacts Expandable // global-artifacts — the PARENT's views this unit also deploys under its own base; explicit list ("agents,skill"), empty = none
	ProjectType     Literal    // project-type: Go-Module | Go-CLI | Go | JS-Bundle | Other
	Path            Literal    // path — child only: location relative to the parent's root
	Parent          *Project   // nil at the root; set when linking, not serialized
	Root            *Root      // the root of the whole tree; set when linking, not serialized

	Meta           *MetaNode        // meta — free unit metadata (module, license, …), any tag inside
	Embed          *MkEmbed         // embed
	ImportMiniskin []*MiniskinNode  // import-miniskin — 0..N (usually 0 or 1)
	Preserve       *PreserveSection // preserve sections — 0..N (usually 0 or 1)
	Children       []*Project       // child-project — nested units of the same type
	SourceItems    []*SourceItem    // the source items collected under this project
	Sections       []*Section       // the resolved sections, in their final order; each project keeps its own

	Base string // absolute base of this unit on disk; set when scanning, not serialized
}

// MetaValue reads one <meta> tag's text by name; empty when there is no
// meta section or no such tag — absence always reads as "".
func (p *Project) MetaValue(name string) string {
	return p.Meta.Get(name) // Get is nil-safe
}

// IsChildProject tells whether this unit is nested in another: it has a
// parent. The root is the one without.
func (p *Project) IsChildProject() bool {
	return p.Parent != nil
}

// GetCargoXml wires the preservation: whatever the events below do not claim
// ends up stored here by the decoder.
func (p *Project) GetCargoXml() *cargoxml.CargoXml {
	if p.Cargo == nil {
		p.Cargo = cargoxml.NewCargoXml()
	}
	return p.Cargo
}

// validateArtifacts checks the RENDERED artifact lists — it runs after
// the Expandables resolve, from Root.Load: a macro is legal in the
// attribute, the render must name known views.
func (p *Project) validateArtifacts() error {
	for tok := range strings.SplitSeq(p.Artifacts.Get(), ",") {
		switch strings.TrimSpace(tok) {
		case "", "*", "readme", "agents", "skill":
		default:
			return fmt.Errorf("[%s] unknown artifact %q (want *|readme|agents|skill)", p.Id, strings.TrimSpace(tok))
		}
	}
	for tok := range strings.SplitSeq(p.GlobalArtifacts.Get(), ",") {
		switch strings.TrimSpace(tok) {
		case "", "agents", "skill": // explicit only: name what comes in, no wildcard
		case "readme":
			return fmt.Errorf("[%s] global-artifacts cannot take the parent's readme: the README belongs to the unit itself", p.Id)
		default:
			return fmt.Errorf("[%s] unknown global artifact %q (want agents|skill)", p.Id, strings.TrimSpace(tok))
		}
	}
	if !p.GlobalArtifacts.Empty() && !p.IsChildProject() {
		return fmt.Errorf("[%s] global-artifacts on the root unit: there is no parent to take them from", p.Id)
	}
	return nil
}

// validateRenders runs every promised post-expansion validation — from
// Root.Load, once the Expandables resolved: a macro is legal in the
// attribute, the RENDER must honor the grammar it lands in.
func (p *Project) validateRenders() error {
	if err := p.validateArtifacts(); err != nil {
		return err
	}
	if e := p.Embed; e != nil {
		// the structural rules come first — same family, same moment: an
		// embed generates a file, so it needs a name; embed-parent selects
		// among a PARENT's views, so the root has nothing to select from
		if e.Filename.Empty() {
			return fmt.Errorf("[%s] embed: no filename", p.Id)
		}
		if !e.EmbedParent.Empty() && p.Parent == nil {
			return fmt.Errorf("[%s] embed-parent on a unit with no parent", p.Id)
		}
		if !e.EmbedParent.Empty() {
			for tok := range strings.SplitSeq(e.EmbedParent.Get(), ",") {
				switch strings.TrimSpace(tok) {
				case "*", "readme", "agents", "skill":
				default:
					return fmt.Errorf("[%s] unknown embed-parent %q (want *|readme|agents|skill)", p.Id, strings.TrimSpace(tok))
				}
			}
		}
		fname := e.Filename.Get()
		if _, err := safeRel(fname); err != nil {
			return fmt.Errorf("[%s] embed filename: %w", p.Id, err)
		}
		if !strings.HasSuffix(fname, ".go") {
			return fmt.Errorf("[%s] embed filename %q: want a .go file", p.Id, fname)
		}
		if module := e.ModuleName.Get(); module != "" && !isGoIdent(module) {
			return fmt.Errorf("[%s] embed module-name %q is not a valid Go identifier", p.Id, module)
		}
	}
	if p.Preserve != nil {
		for _, it := range p.Preserve.Items {
			switch it.Method.Get() {
			case "alt", "alias":
			default:
				return fmt.Errorf("[%s] unknown preserve method %q (want alt|alias)", p.Id, it.Method.Get())
			}
			if it.Method.Get() == "alias" {
				alias := it.Alias.Get()
				if alias == "" || strings.ContainsAny(alias, `/\`) {
					return fmt.Errorf("[%s] preserve alias %q: a plain file name only — the alias lives next to the original", p.Id, alias)
				}
			}
		}
	}
	for _, im := range p.ImportMiniskin {
		switch im.FrontMatterGen.Get() {
		case "", "embed", "extern", "extern,preserve":
		default:
			return fmt.Errorf("[%s] unknown fm-gen %q (want embed|extern|extern,preserve)", p.Id, im.FrontMatterGen.Get())
		}
		if !im.ContentFolder.Empty() {
			if _, err := safeRel(im.ContentFolder.Get()); err != nil {
				return fmt.Errorf("[%s] import-miniskin content: %w", p.Id, err)
			}
		}
	}
	return nil
}

// isGoIdent reports a valid Go identifier — the embed's package name.
func isGoIdent(s string) bool {
	for i, r := range s {
		switch {
		case unicode.IsLetter(r) || r == '_':
		case i > 0 && unicode.IsDigit(r):
		default:
			return false
		}
	}
	return s != ""
}

// OnXmlAttribute claims the project's own attributes; anything else falls to
// the cargo.
func (p *Project) OnXmlAttribute(d *cargoxml.DecoderWithCargo, a *xml.Attr) bool {
	switch a.Name.Local {
	case "id":
		p.Id = Literal(a.Value)
	case "name":
		p.Name = Literal(a.Value)
	case "artifacts":
		p.Artifacts.SetRawAt(a.Value, xmlLine(d))
	case "global-artifacts":
		p.GlobalArtifacts.SetRawAt(a.Value, xmlLine(d))
	case "project-type":
		p.ProjectType = Literal(a.Value)
	case "path":
		p.Path = Literal(a.Value)
	default:
		return false
	}
	return true
}

// OnXmlChildStart assigns a consumer to each known child; an unknown child is
// left unclaimed, so the decoder parses it generically into the cargo.
func (p *Project) OnXmlChildStart(d *cargoxml.DecoderWithCargo, child *cargoxml.DecoderStackFrame) error {
	switch child.NodeName.Local {
	case "meta":
		if p.Meta != nil {
			return NewParseError(d, "multiple <meta> elements in <project>, only 1 is allowed")
		}
		p.Meta = &MetaNode{Project: p}
		child.Consumer = p.Meta
	case "embed":
		if p.Embed != nil {
			return NewParseError(d, "multiple <embed> elements in <project>, only 1 is allowed")
		}
		p.Embed = &MkEmbed{Project: p}
		child.Consumer = p.Embed
	case "import-miniskin":
		im := &MiniskinNode{Project: p}
		p.ImportMiniskin = append(p.ImportMiniskin, im)
		child.Consumer = im
	case "preserve":
		if p.Preserve != nil {
			return NewParseError(d, "multiple <preserve> elements in <project>, only 1 is allowed")
		}
		p.Preserve = &PreserveSection{Project: p}
		child.Consumer = p.Preserve

	case "child-project", "project":
		c := &Project{Parent: p, Root: p.Root}
		p.Children = append(p.Children, c)
		child.Consumer = c

	case "scan":
		// debug output of a previous IncludeScanData run: regenerable scan
		// data, not config — skipped entirely, not even cargo
		child.SkipUnknownChildren = true
	}
	return nil
}

// scanGroup wraps any producers as one debug <scan> element — a hand-rolled
// stream, producing is free.
type scanGroup []cargoxml.XmlTokenProducer

func (g scanGroup) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return func(yield func(xml.Token) bool) {
		start := xml.StartElement{Name: xml.Name{Local: "scan"}}
		if !yield(start) {
			return
		}
		for _, producer := range g {
			for tok := range producer.XmlTokens(ctx) {
				if !yield(tok) {
					return
				}
			}
		}
		yield(xml.EndElement{Name: start.Name})
	}
}

func (p *Project) OnXmlEnd(d *cargoxml.DecoderWithCargo, frame *cargoxml.DecoderStackFrame) error {
	// The parse validates what the parse can know: the tree's structure
	// and the Literal sources (law 1) — everything render-dependent waits
	// for validateRenders, where the Expandables have resolved.
	// The id (for cross-unit references) is not demanded here: a missing one
	// gets an automatic "P0", "P1", … once the whole tree is loaded.
	if p.IsChildProject() && p.Path == "" {
		return NewParseError(d, "child project needs a path (relative to the parent's root)")
	}
	if p.Name == "" && p.Path != "" {
		p.Name = Literal(filepath.Base(string(p.Path))) // default: the folder the project lives in
	}
	// the artifact lists validate AFTER the Expandables resolve (a macro
	// is legal in the attribute; the RENDER must name known views) — see
	// validateArtifacts, called by Root.Load.
	// law 1, loudly and at the parse (with its position): the four
	// sources refuse macros before anything else looks at them — BEFORE
	// any shape check, so a macro'd path dies citing the law, not the
	// colon its macro happens to carry
	for _, src := range [...]struct {
		name  string
		value Literal
	}{{"id", p.Id}, {"name", p.Name}, {"path", p.Path}, {"project-type", p.ProjectType}} {
		if err := CheckLiteral(src.name, src.value); err != nil {
			return NewParseError(d, err.Error())
		}
	}
	if p.Path != "" {
		// a child's path stays inside the parent's tree — "../x" or an
		// absolute path is a config error, at the parse, with position
		if _, err := safeRel(string(p.Path)); err != nil {
			return NewParseError(d, "child path: "+err.Error())
		}
	}
	if p.ProjectType == "" {
		p.ProjectType = "Other" // default
	} else {
		known := false
		for _, t := range projectTypes {
			if strings.EqualFold(string(p.ProjectType), t) {
				p.ProjectType = Literal(t) // normalize to canonical casing
				known = true
				break
			}
		}
		if !known {
			return NewParseError(d, "unknown project-type \""+string(p.ProjectType)+"\" (want "+strings.Join(projectTypes, "|")+")")
		}
	}
	return nil
}

// --- describe side + producer: how the project presents itself for writing ---

var (
	_ SourceItemContainer           = (*Project)(nil)
	_ cargoxml.XmlDescribeWithCargo = (*Project)(nil)
	_ cargoxml.XmlTokenProducer     = (*Project)(nil)
)

// CleanUpLastScan empties what the last scan collected — own items first,
// then delegating to each import and to the preserve section: the project
// starts the next one clean.
func (p *Project) CleanUpLastScan(log io.Writer) error {
	fmt.Fprintf(log, "[%s] cleanup\n", p.Id)
	p.SourceItems = nil
	for _, node := range p.ImportMiniskin {
		if err := node.CleanUpLastScan(log); err != nil {
			return err
		}
	}
	if p.Preserve != nil {
		if err := p.Preserve.CleanUpLastScan(log); err != nil {
			return err
		}
	}
	return nil
}

// Scan collects this project's sources. First thing, it settles its own Base:
// the main project sits at the Root's ProjectBase, a child under its parent —
// with parents always scanned first, the parent's Base is already good.
func (p *Project) Scan(log io.Writer) error {

	func() {
		if p.IsChildProject() {
			p.Base = filepath.Join(p.Parent.Base, filepath.FromSlash(string(p.Path)))
		} else {
			p.Base = p.Root.ProjectBase
		}
	}() // settle the Base first, so everything below can use it

	// round 0: resolve the preservation map, so conflicts can be marked
	if p.Preserve != nil {
		if err := p.Preserve.Scan(log); err != nil {
			return err
		}
	}

	srcRoot := filepath.Join(p.Base, _SkillFolder, _SrcFolder)
	exist := func() bool {
		if _, err := os.Stat(srcRoot); os.IsNotExist(err) {
			return false
		}
		return true
	}()
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}

		item := &SourceItem{Parent: p, OriginType: ItemOriginExisting}
		item.DstFileName = entry.Name()
		if _, err := os.Stat(strings.TrimSuffix(path, ".md") + ".fm"); err == nil {
			item.FmExternal = true
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if dir := filepath.Dir(rel); dir != "." {
			item.DstPath = filepath.ToSlash(dir)
		}
		p.SourceItems = append(p.SourceItems, item)
		fmt.Fprintf(log, "[%s] source %s\n", p.Id, rel)
		return nil
	}
	if exist {
		if err := filepath.WalkDir(srcRoot, walk); err != nil {
			return err
		}
	}

	for _, node := range p.ImportMiniskin {
		if err := node.Scan(log); err != nil {
			return err
		}
	}

	p.dropSupersededNatives(log)
	p.markPreserveConflicts(log)
	return nil
}

// dropSupersededNatives removes the native items that are really last run's
// materializations: a file living in _mkskill/src at the same destination a
// harvested item will write is yesterday's copy, not a source — the harvest
// is the truth, and keeping both would double the section (and break the
// build's idempotence with a duplicate pos).
func (p *Project) dropSupersededNatives(log io.Writer) {
	harvested := make(map[string]bool)
	for _, node := range p.ImportMiniskin {
		for _, manifest := range node.ImportManifestList {
			for _, item := range manifest.SourceItems {
				harvested[item.DstPath+"/"+item.DstFileName] = true
			}
		}
	}
	if len(harvested) == 0 {
		return
	}
	kept := p.SourceItems[:0]
	for _, item := range p.SourceItems {
		if harvested[item.DstPath+"/"+item.DstFileName] {
			fmt.Fprintf(log, "[%s] source %s superseded by the harvest, dropped\n", p.Id, item.DstFileName)
			continue
		}
		kept = append(kept, item)
	}
	p.SourceItems = kept
}

// Prepare materializes this project's collected items into _mkskill/src:
// copies for the imported ones, frontmatter per fm-gen, preserve conflicts
// respected — everything on the record through log.
func (p *Project) Prepare(log io.Writer) error {
	for _, item := range p.GetAllSourceItems() {
		if item.OriginType == ItemOriginExisting {
			continue // already living in src: nothing to materialize
		}
		if err := p.prepareItem(log, item); err != nil {
			return err
		}
	}
	return nil
}

// prepareItem materializes one imported item: the copy is an artifact and is
// rewritten every run; a preserved destination is never touched; a missing
// source warns — and skips, or keeps an existing copy.
func (p *Project) prepareItem(log io.Writer, item *SourceItem) error {
	dstRel := _SkillFolder + "/" + _SrcFolder + "/" + item.DstFileName
	if item.DstPath != "" {
		dstRel = _SkillFolder + "/" + _SrcFolder + "/" + item.DstPath + "/" + item.DstFileName
	}
	dstAbs := filepath.Join(p.Base, filepath.FromSlash(dstRel))

	if item.PreserveConflict != nil {
		fmt.Fprintf(log, "[%s] preserve: %s is protected (%s), not written\n",
			p.Id, dstRel, item.PreserveConflict.Item.Method)
		return nil
	}

	src := filepath.FromSlash(item.OriginPath)
	if !filepath.IsAbs(src) {
		src = filepath.Join(p.Base, src)
	}
	content, err := os.ReadFile(src)
	if err != nil {
		if _, statErr := os.Stat(dstAbs); statErr == nil {
			fmt.Fprintf(log, "[%s] WARN: source %s missing, using the existing copy %s\n",
				p.Id, item.OriginPath, dstRel)
		} else {
			fmt.Fprintf(log, "[%s] WARN: source %s missing, item %s skipped\n",
				p.Id, item.OriginPath, dstRel)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return err
	}

	fm := renderForeignFm(item.ForeignAttrib)
	if item.FmExternal {
		if err := os.WriteFile(dstAbs, content, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(log, "[%s] copy %s -> %s\n", p.Id, item.OriginPath, dstRel)

		fmAbs := strings.TrimSuffix(dstAbs, ".md") + ".fm"
		fmRel := strings.TrimSuffix(dstRel, ".md") + ".fm"
		if item.FmPreserve {
			if _, err := os.Stat(fmAbs); err == nil {
				fmt.Fprintf(log, "[%s] fm: existing %s preserved\n", p.Id, fmRel)
				return nil
			}
		}
		if fm == "" {
			return nil // nothing declared, nothing to generate
		}
		if err := os.WriteFile(fmAbs, []byte(fm), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(log, "[%s] fm: generated %s\n", p.Id, fmRel)
		return nil
	}

	// embed: our block goes on top; if the source carries a frontmatter of
	// its own both blocks stay — resolving that mix is the output's business
	if fm != "" {
		content = append([]byte("---\n"+fm+"---\n"), content...)
	}
	if err := os.WriteFile(dstAbs, content, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(log, "[%s] copy %s -> %s (fm embedded)\n", p.Id, item.OriginPath, dstRel)
	return nil
}

// Resolve builds this project's sections from its collected items: every
// materialized file is opened, its front matter extracted with fmlines and
// the body kept verbatim — from here on the content has meaning. The
// sections come out in their final order.
func (p *Project) Resolve(log io.Writer) error {
	p.Sections = nil
	for _, item := range p.GetAllSourceItems() {
		sec, err := p.resolveItem(log, item)
		if err != nil {
			return err
		}
		if sec != nil {
			p.Sections = append(p.Sections, sec)
		}
	}
	ordered, err := orderSections(log, string(p.Id), p.Sections)
	if err != nil {
		return err
	}
	p.Sections = ordered
	return nil
}

// resolveItem reads one item's materialized file into a Section: the front
// matter lines come from the internal fenced header — or from the sibling
// .fm when the item is external — and the body stays verbatim. A file that
// never got materialized warns and yields no section.
func (p *Project) resolveItem(log io.Writer, item *SourceItem) (*Section, error) {
	dstRel := _SkillFolder + "/" + _SrcFolder + "/" + item.DstFileName
	if item.DstPath != "" {
		dstRel = _SkillFolder + "/" + _SrcFolder + "/" + item.DstPath + "/" + item.DstFileName
	}
	dstAbs := filepath.Join(p.Base, filepath.FromSlash(dstRel))

	f, err := os.Open(dstAbs)
	if err != nil {
		fmt.Fprintf(log, "[%s] WARN: %s not materialized, no section\n", p.Id, dstRel)
		return nil, nil
	}
	defer f.Close()

	sec := &Section{Item: item, In: "*"}
	lines := &fmlines.FmLines{}

	if item.FmExternal {
		// the front matter lives in the sibling .fm; the .md is all body
		fmAbs := strings.TrimSuffix(dstAbs, ".md") + ".fm"
		if fmFile, err := os.Open(fmAbs); err == nil {
			err = lines.ReadAll(linereader.NewLineReader(fmFile, 0, 0))
			fmFile.Close()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", strings.TrimSuffix(dstRel, ".md")+".fm", err)
			}
		}
		body, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dstRel, err)
		}
		sec.Body = string(body)
	} else {
		lr := linereader.NewLineReader(f, 0, 0)
		if err := lines.ReadHeader(lr); err != nil {
			return nil, fmt.Errorf("%s: %w", dstRel, err)
		}
		body, err := io.ReadAll(lr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dstRel, err)
		}
		if len(*lines) > 0 && (*lines)[0].Type != fmlines.FmLineStartBlock {
			// no front matter: the consumed first line is body, put it back
			// in front with the terminator it came with
			var eol string
			switch lr.LastEolType {
			case linereader.EolLf:
				eol = "\n"
			case linereader.EolCr:
				eol = "\r"
			case linereader.EolCrLf:
				eol = "\r\n"
			}
			body = append([]byte((*lines)[0].RawLine+eol), body...)
			lines = &fmlines.FmLines{}
		}
		sec.Body = string(body)
	}

	fmt.Fprintf(log, "[%s] resolve %s: %d fm lines, %d body bytes\n", p.Id, dstRel, len(*lines), len(sec.Body))
	sec.applyFm(log, string(p.Id), dstRel, lines)
	return sec, nil
}

// renderForeignFm renders the foreign attributes as the mkskill: frontmatter
// block, raw YAML without fences (the .fm form; embed wraps it in ---).
func renderForeignFm(attribs []string) string {
	if len(attribs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("mkskill:\n")
	for _, kv := range attribs {
		key, value, _ := strings.Cut(kv, "=")
		b.WriteString("  " + key + ": " + yamlScalar(value) + "\n")
	}
	return b.String()
}

// yamlScalar quotes a value when a plain YAML scalar would misread it.
func yamlScalar(value string) string {
	if value == "" || strings.ContainsAny(value, "*:#{}[]&!|>'\"%@`, \t") {
		return strconv.Quote(value)
	}
	return value
}

// markPreserveConflicts stamps every collected item whose destination —
// expressed base-relative, the map's anchor — collides with a preserved file.
func (p *Project) markPreserveConflicts(log io.Writer) {
	if p.Preserve == nil || len(p.Preserve.Files) == 0 {
		return
	}
	byPath := map[string]*PreservedFile{}
	for _, f := range p.Preserve.Files {
		byPath[f.Path] = f
	}
	for _, item := range p.GetAllSourceItems() {
		dst := _SkillFolder + "/" + _SrcFolder + "/" + item.DstFileName
		if item.DstPath != "" {
			dst = _SkillFolder + "/" + _SrcFolder + "/" + item.DstPath + "/" + item.DstFileName
		}
		if f, ok := byPath[dst]; ok {
			item.PreserveConflict = f
			fmt.Fprintf(log, "[%s] conflict: %s is preserved (%s)\n", p.Id, dst, f.Item.Method)
		}
	}
}

// GetSourceItems hands over the project's items in a fresh array: the caller
// may reorder or filter its copy freely; the elements are the shared ones.
func (p *Project) GetSourceItems() []*SourceItem {
	return append([]*SourceItem(nil), p.SourceItems...)
}

func (p *Project) GetAllSourceItems() []*SourceItem {
	items := append([]*SourceItem(nil), p.SourceItems...)
	for _, c := range p.ImportMiniskin {
		items = append(items, c.GetAllSourceItems()...)
	}

	return items
}

func (p *Project) GetCurrentProject() *Project {
	return p
}

// XmlDescribeNodeName: the root writes as <project>, a nested unit as
// <child-project> — same type, the Parent pointer decides the tag.
func (p *Project) XmlDescribeNodeName(ctx context.Context) xml.Name {
	if p.IsChildProject() {
		return xml.Name{Local: "child-project"}
	}
	return xml.Name{Local: "project"}
}

func (p *Project) XmlDescribeNodeType(ctx context.Context, policy cargoxml.MixedNodePolicy) cargoxml.XmlNodeType {
	return cargoxml.XmlContainerNode // no text wanted here
}

// XmlDescribeAttributes answers the project's own attributes; empty ones are
// omitted (absent in, absent out).
func (p *Project) XmlDescribeAttributes(ctx context.Context) []xml.Attr {
	var attrs []xml.Attr
	add := func(name, value string) {
		if value != "" {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
		}
	}
	add("id", string(p.Id))
	add("name", string(p.Name))
	add("artifacts", p.Artifacts.Raw)              // ALWAYS the raw: a rewrite
	add("global-artifacts", p.GlobalArtifacts.Raw) // never freezes a template
	add("project-type", string(p.ProjectType))
	add("path", string(p.Path))
	return attrs
}

func (p *Project) XmlDescribeInitialComments(ctx context.Context) []string { return nil }
func (p *Project) XmlDescribeText(ctx context.Context) []string            { return nil }

// XmlDescribeItems answers the known children in prototype order: embed,
// import-miniskin, preserve, then the nested projects. Under IncludeScanData
// the project's own source items come out too, as debug <source-item>s.
func (p *Project) XmlDescribeItems(ctx context.Context) []cargoxml.XmlTokenProducer {
	var items []cargoxml.XmlTokenProducer
	if p.Meta != nil {
		items = append(items, p.Meta)
	}
	if p.Embed != nil {
		items = append(items, p.Embed)
	}
	for _, im := range p.ImportMiniskin {
		items = append(items, im)
	}
	if p.Preserve != nil {
		items = append(items, p.Preserve)
	}
	if GetEncoderParams(ctx).IncludeScanData && len(p.SourceItems) > 0 {
		group := make(scanGroup, 0, len(p.SourceItems))
		for _, it := range p.SourceItems {
			group = append(group, it)
		}
		items = append(items, group)
	}
	for _, c := range p.Children {
		items = append(items, c)
	}
	return items
}

func (p *Project) XmlTokens(ctx context.Context) iter.Seq[xml.Token] {
	return cargoxml.DescribedTokens(ctx, p, cargoxml.PreserveMixed)
}
