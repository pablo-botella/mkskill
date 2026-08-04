package compiler

// Version subsystem — the propagation: updates and creates rendered from
// the current XML state onto their destination files. Compute EVERYTHING
// first, write after — an error mid-computation leaves every destination
// untouched. The mutators run this as the tail of their gesture; -vbuild
// runs it alone to repair drift; -vcheck (phase 4) will run the compute
// side and compare instead of writing.

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pablo-botella/cargoxml"
)

// VPropagated reports one propagation run.
type VPropagated struct {
	Written []string // project-root-relative destinations written
	Skipped []string // overwrite="warn" hits: left alone, reported
	Warns   []string
}

// VBuild is the -vbuild command: re-propagate every destination from the
// current XML state. No number, no ts, no history — idempotent repair.
func VBuild(base string) (*VPropagated, error) {
	lazy, err := LazyLoad(VersionConfigFile(base))
	if err != nil {
		return nil, err
	}
	if lazy.Spec == nil {
		return nil, fmt.Errorf("no version-spec in the config")
	}
	prop, err := vPropagate(base, lazy)
	if err != nil {
		return nil, err
	}
	prop.Warns = append(lazy.Warns, prop.Warns...)
	return prop, nil
}

// vPendingWrite is one computed destination, ready to flush.
type vPendingWrite struct {
	rel     string // project-root-relative, for the report
	path    string // absolute
	content []byte
}

// vComputeAll renders every destination of the spec against the given
// (possibly freshly mutated) lazy state — reads and decisions included,
// writing NOTHING. The mutators run it before touching even the config:
// a compute error leaves the world exactly as it was.
func vComputeAll(base string, lazy *LazyDoc) ([]vPendingWrite, *VPropagated, error) {
	prop := &VPropagated{}
	resolve := lazy.ResolverFor(nil)
	var pending []vPendingWrite
	for _, dest := range lazy.Spec.Dests {
		writes, err := vComputeDest(base, resolve, dest, prop)
		if err != nil {
			return nil, nil, err
		}
		pending = append(pending, writes...)
	}
	return pending, prop, nil
}

// vFlush writes what vComputeAll prepared — the only writer.
func vFlush(pending []vPendingWrite, prop *VPropagated) error {
	for _, w := range pending {
		if dir := filepath.Dir(w.path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(w.path, w.content, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", w.rel, err)
		}
		prop.Written = append(prop.Written, w.rel)
	}
	return nil
}

// vPropagate is compute + flush in one gesture — what -vbuild runs.
func vPropagate(base string, lazy *LazyDoc) (*VPropagated, error) {
	pending, prop, err := vComputeAll(base, lazy)
	if err != nil {
		return nil, err
	}
	if err := vFlush(pending, prop); err != nil {
		return nil, err
	}
	return prop, nil
}

// vComputeDest renders ONE destination into its pending writes — reads
// and decisions included, writing nothing. -vbuild and the mutators flush
// what it returns; -vcheck compares it against the disk instead.
func vComputeDest(base string, resolve VersionResolver, dest any, prop *VPropagated) ([]vPendingWrite, error) {
	switch d := dest.(type) {
	case *LazyUpdate:
		rel, path, err := vDestPath(base, d.File, resolve)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("update %s: %w", rel, err)
		}
		var out []byte
		switch d.Type {
		case "json":
			out, err = vPatchJSON(data, d.Entries, resolve)
		case "xml":
			out, err = vPatchXML(data, d.Entries, resolve)
		default:
			out, err = vPatchReplace(data, d.Entries, resolve)
		}
		if err != nil {
			return nil, fmt.Errorf("update %s: %w", rel, err)
		}
		return []vPendingWrite{{rel, path, out}}, nil

	case *LazyCreate:
		rel, path, err := vDestPath(base, d.File, resolve)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err == nil {
			switch d.Overwrite {
			case "false":
				return nil, fmt.Errorf("create %s: the file exists and overwrite is false", rel)
			case "warn":
				prop.Skipped = append(prop.Skipped, rel)
				prop.Warns = append(prop.Warns, fmt.Sprintf("create %s: exists, left alone", rel))
				return nil, nil
			}
		}
		tpl := d.Text
		if d.Src != "" {
			_, srcPath, err := vDestPath(base, d.Src, resolve)
			if err != nil {
				return nil, err
			}
			raw, err := os.ReadFile(srcPath)
			if err != nil {
				return nil, fmt.Errorf("create %s: template: %w", rel, err)
			}
			tpl = string(raw)
		} else {
			// the newline right after the opening tag is not output
			tpl = strings.TrimPrefix(strings.TrimPrefix(tpl, "\r\n"), "\n")
		}
		rendered, err := ExpandVersionMacros(tpl, false, resolve)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", rel, err)
		}
		writes := []vPendingWrite{{rel, path, vForceEOL(rendered, d.Eol)}}
		if d.GitIgnore {
			gi, err := vGitignoreWith(base, rel)
			if err != nil {
				return nil, err
			}
			if gi != nil {
				writes = append(writes, *gi)
			}
		}
		return writes, nil
	}
	return nil, nil
}

// safeRel validates a base-relative path: slashes normalized and
// cleaned, never absolute, never a drive, never escaping upward. The
// paths a config declares stay inside the tree they belong to — a
// "../x", a "C:/x" or a "/tmp/x" is a config error, not a write. The
// verdict is the SAME on every host: a config path is portable text,
// so the checks cannot lean on the platform's filepath rules — GOOS
// decides nothing here.
func safeRel(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.ContainsRune(s, ':') {
		return "", fmt.Errorf("path %q: a drive letter or scheme cannot live in a project tree", raw)
	}
	s = strings.ReplaceAll(s, `\`, "/")
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("path %q: absolute paths cannot live in a project tree", raw)
	}
	clean := path.Clean(s)
	if clean == "." {
		return "", fmt.Errorf("path %q resolves empty", raw)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the project tree", raw)
	}
	return clean, nil
}

// vDestPath expands the bare-ref macros of a file attribute (law 5) and
// resolves it project-root-relative — safeRel guarding the result: what
// the macros assembled must still live inside the tree.
func vDestPath(base, file string, resolve VersionResolver) (rel, abs string, err error) {
	expanded, err := ExpandVersionMacros(file, true, resolve)
	if err != nil {
		return "", "", fmt.Errorf("file %q: %w", file, err)
	}
	rel, err = safeRel(strings.TrimPrefix(strings.TrimPrefix(expanded, "/"), "./"))
	if err != nil {
		return "", "", fmt.Errorf("file %q: %w", file, err)
	}
	if base == "" {
		base = "."
	}
	return rel, filepath.Join(base, filepath.FromSlash(rel)), nil
}

// vEntryValue renders an entry's template: trim per mode, blank XML
// formatting lines dropped, macros expanded.
func vEntryValue(e *LazyEntry, resolve VersionResolver) (string, error) {
	var lines []string
	for ln := range strings.SplitSeq(strings.ReplaceAll(e.Text, "\r\n", "\n"), "\n") {
		trimmed := vTrimLine(ln, e.Trim)
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return ExpandVersionMacros(strings.Join(lines, "\n"), false, resolve)
}

func vTrimLine(s, mode string) string {
	switch mode {
	case "left":
		return strings.TrimLeft(s, " \t")
	case "right":
		return strings.TrimRight(s, " \t")
	case "none":
		return s
	}
	return strings.TrimSpace(s) // all, the default
}

// vForceEOL applies a create's declared eol: normalize everything to LF
// first (so a CRLF src template cannot double up), then to CRLF when
// asked. Empty means "as the template arrives" — inline is LF by XML
// §2.11, src is verbatim.
func vForceEOL(s, eol string) []byte {
	if eol == "" {
		return []byte(s)
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if eol == "crlf" {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return []byte(s)
}

// vMatchEOL gives the rendered output the destination's own newline
// style — the update rule, for every type alike: the file's EOLs are the
// file's business. out arrives LF-only (Marshal and the xml encoder emit
// LF); when the original used CRLF, so does the result.
func vMatchEOL(original, out []byte) []byte {
	if bytes.Contains(original, []byte("\r\n")) {
		return bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	return out
}

// --- json ---

// vPatchJSON sets each entry's slash path in the document. Formatting is
// normalized (two-space indent, sorted keys) — JSON is structure, not
// prose; intermediate objects are created as needed.
func vPatchJSON(data []byte, entries []*LazyEntry, resolve VersionResolver) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	for _, e := range entries {
		value, err := vEntryValue(e, resolve)
		if err != nil {
			return nil, err
		}
		segs := strings.Split(e.Key, "/")
		cur := root
		for _, seg := range segs[:len(segs)-1] {
			next, ok := cur[seg].(map[string]any)
			if !ok {
				next = map[string]any{}
				cur[seg] = next
			}
			cur = next
		}
		cur[segs[len(segs)-1]] = value
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return vMatchEOL(data, append(out, '\n')), nil
}

// --- xml (through cargoxml: foreign content survives untouched) ---

func vPatchXML(data []byte, entries []*LazyEntry, resolve VersionResolver) ([]byte, error) {
	d := cargoxml.NewDecoderWithCargo(xml.NewDecoder(bytes.NewReader(data)))
	if err := d.Parse(); err != nil {
		return nil, err
	}
	root, ok := d.RootFrame.Consumer.(*cargoxml.GenericXmlItem)
	if !ok {
		return nil, fmt.Errorf("xml document did not parse generically")
	}
	for _, e := range entries {
		value, err := vEntryValue(e, resolve)
		if err != nil {
			return nil, err
		}
		target := root
		for seg := range strings.SplitSeq(e.Key, "/") {
			var next *cargoxml.GenericXmlItem
			for _, ch := range target.Children {
				if ch.Name != nil && ch.Name.Local == seg {
					next = ch
					break
				}
			}
			if next == nil {
				return nil, fmt.Errorf("key %q: element %q not found", e.Key, seg)
			}
			target = next
		}
		if e.Attrib != "" {
			set := false
			for i := range target.Attributes {
				if target.Attributes[i].Name.Local == e.Attrib {
					target.Attributes[i].Value = value
					set = true
					break
				}
			}
			if !set {
				target.Attributes = append(target.Attributes,
					xml.Attr{Name: xml.Name{Local: e.Attrib}, Value: value})
			}
			continue
		}
		// replace the element's text content, keeping its non-text trails
		if target.Trails != nil {
			kept := (*target.Trails)[:0]
			for _, t := range *target.Trails {
				if !(t.Position == cargoxml.TrailInner && t.Type&cargoxml.TrailAnyText != 0) {
					kept = append(kept, t)
				}
			}
			*target.Trails = kept
		} else {
			target.Trails = cargoxml.NewTrails()
		}
		target.Trails.AddText(value).Position = cargoxml.TrailInner
	}

	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	if err := cargoxml.NewEncoderWithCargo(enc).Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return vMatchEOL(data, buf.Bytes()), nil
}

// --- replace (the default: assumes nothing about the file) ---

// vPatchReplace anchors each template line by its static skeleton and
// rewrites every matching file line whole. Zero matches: error. Several:
// all rewritten — coordinating the number everywhere is the point. The
// file's EOL style is preserved.
func vPatchReplace(data []byte, entries []*LazyEntry, resolve VersionResolver) ([]byte, error) {
	content := string(data)
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	for _, e := range entries {
		for tplLine := range strings.SplitSeq(strings.ReplaceAll(e.Text, "\r\n", "\n"), "\n") {
			tpl := vTrimLine(tplLine, e.Trim)
			if strings.TrimSpace(tpl) == "" {
				continue // XML formatting, not an anchor
			}
			segs, err := vSkeleton(tpl)
			if err != nil {
				return nil, err
			}
			render, err := ExpandVersionMacros(tpl, false, resolve)
			if err != nil {
				return nil, err
			}
			matches := 0
			for i, fl := range lines {
				if !vSkeletonMatch(strings.TrimSpace(fl), segs) {
					continue
				}
				indent := fl[:len(fl)-len(strings.TrimLeft(fl, " \t"))]
				if e.Indent != "" {
					n, _ := strconv.Atoi(e.Indent)
					indent = strings.Repeat(" ", n)
				}
				lines[i] = indent + strings.TrimSpace(render)
				matches++
			}
			if matches == 0 {
				return nil, fmt.Errorf("anchor not found: %q", tpl)
			}
		}
	}
	return []byte(strings.Join(lines, eol)), nil
}

// vSkeleton splits a template line into its static segments — the text
// outside the macros, which is what anchors the line in the file.
func vSkeleton(tpl string) ([]string, error) {
	var segs []string
	static := ""
	rest := tpl
	for {
		i := strings.Index(rest, vMacroOpen)
		if i < 0 {
			break
		}
		j := strings.Index(rest[i+len(vMacroOpen):], vMacroClose)
		if j < 0 {
			return nil, fmt.Errorf("unterminated macro in %q", tpl)
		}
		static += rest[:i]
		segs = append(segs, static)
		static = ""
		rest = rest[i+len(vMacroOpen)+j+len(vMacroClose):]
	}
	segs = append(segs, static+rest)
	anchored := false
	for _, s := range segs {
		if strings.TrimSpace(s) != "" {
			anchored = true
			break
		}
	}
	if !anchored {
		return nil, fmt.Errorf("a replace entry needs static text to anchor on: %q", tpl)
	}
	return segs, nil
}

// vSkeletonMatch reports whether a (trimmed) file line fits the skeleton:
// starts with the first segment, ends with the last, and carries the
// middle ones in order between them.
func vSkeletonMatch(line string, segs []string) bool {
	first, last := strings.TrimLeft(segs[0], " \t"), strings.TrimRight(segs[len(segs)-1], " \t")
	if !strings.HasPrefix(line, first) || !strings.HasSuffix(line, last) {
		return false
	}
	pos := len(first)
	end := len(line) - len(last)
	if pos > end {
		return false
	}
	for _, mid := range segs[1 : len(segs)-1] {
		i := strings.Index(line[pos:end], mid)
		if i < 0 {
			return false
		}
		pos += i + len(mid)
	}
	return true
}

// --- .gitignore (create git-ignore="true") ---

// vGitignoreWith returns the pending write that guarantees rel is listed
// in the project's .gitignore — nil when it already is.
func vGitignoreWith(base, rel string) (*vPendingWrite, error) {
	if base == "" {
		base = "."
	}
	path := filepath.Join(base, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	content := string(data)
	for line := range strings.SplitSeq(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == rel {
			return nil, nil // already promised
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += rel + "\n"
	return &vPendingWrite{rel: ".gitignore", path: path, content: []byte(content)}, nil
}
