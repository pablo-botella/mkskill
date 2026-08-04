package compiler

// Version subsystem — the mutators. One atomic gesture each: validate
// EVERYTHING first (unknown component, arity, domain, lock, labels),
// then numbers, ts, volatile labels and the history entry, then one
// Save. The lazy pass is the brain (domains, classes, rendering); the
// VersionDoc is the hand (writes without touching what is not ours).

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VMutated is what a mutation reports back.
type VMutated struct {
	Tag     string   // the new canonical render
	Ts      string   // the stamped instant, UTC RFC-3339
	Written []string // destinations propagated by the gesture
	Skipped []string // overwrite="warn" creates left alone
	Warns   []string
}

// VInc is the -vinc-<component> command: increment one declared
// component, reset everything to its right, propagate the gesture.
func VInc(base, component string, labels map[string]string) (*VMutated, error) {
	return vMutate(base, component, "", labels)
}

// VSet is the -vset command: set the whole number outright —
// dot-separated, full arity, positional over params.
func VSet(base, value string, labels map[string]string) (*VMutated, error) {
	return vMutate(base, "", value, labels)
}

func vMutate(base, incComponent, setValue string, labels map[string]string) (*VMutated, error) {
	cfg := VersionConfigFile(base)
	lazy, err := LazyLoad(cfg)
	if err != nil {
		return nil, err
	}
	spec := lazy.Spec
	if spec == nil {
		return nil, fmt.Errorf("no version-spec in the config")
	}

	// --- validate and compute the new numbers, writing nothing ---
	method := ""
	switch {
	case incComponent != "":
		method = "vinc-" + incComponent
		at := -1
		for i := range spec.Components {
			if spec.Components[i].Name == incComponent {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, fmt.Errorf("unknown component %q", incComponent)
		}
		c := &spec.Components[at]
		if c.Locked {
			return nil, fmt.Errorf("component %q is locked — the conscious gesture is editing the config", c.Name)
		}
		if c.Value+1 > c.Max {
			return nil, fmt.Errorf("component %q overflows its domain [%d-%d] — no carry, no saturation", c.Name, c.Min, c.Max)
		}
		c.Value++
		for i := at + 1; i < len(spec.Components); i++ {
			spec.Components[i].Value = spec.Components[i].Min // reset to the right
		}

	case setValue != "":
		method = "vset"
		parts := strings.Split(setValue, ".")
		if len(parts) != len(spec.Components) {
			return nil, fmt.Errorf("-vset %q: %d values for %d declared components", setValue, len(parts), len(spec.Components))
		}
		for i, p := range parts {
			c := &spec.Components[i]
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("-vset %q: %q is not a number", setValue, p)
			}
			if n < c.Min || n > c.Max {
				return nil, fmt.Errorf("-vset %q: %s=%d out of its domain [%d-%d]", setValue, c.Name, n, c.Min, c.Max)
			}
			if c.Locked && n != c.Value {
				return nil, fmt.Errorf("component %q is locked — the conscious gesture is editing the config", c.Name)
			}
			c.Value = n
		}

	default:
		return nil, fmt.Errorf("nothing to mutate")
	}

	// --- validate the riding labels ---
	for k := range labels {
		l := spec.Labels[k]
		if l == nil {
			return nil, fmt.Errorf("unknown label %q", k)
		}
		if l.Computed() {
			return nil, fmt.Errorf("label %q is computed — it has no value to set", k)
		}
	}

	// --- the new state, in the brain first ---
	ts := time.Now().UTC().Format(time.RFC3339)
	spec.Ts = ts
	for _, key := range spec.LabelOrder {
		l := spec.Labels[key]
		if l.Computed() {
			continue
		}
		if v, given := labels[key]; given {
			l.Value = v
			continue
		}
		if l.Volatile { // a mutation without -label:<key> resets it
			switch {
			case l.DefaultToTs:
				l.Value = ts
			case l.Default != "":
				l.Value = l.Default
			default:
				l.Value = ""
			}
		}
	}
	tag, err := spec.renderCanonical()
	if err != nil {
		return nil, err
	}

	// --- every destination computes BEFORE anything is written: a bad
	// anchor, a missing file, an unresolvable template — and the world,
	// config included, stays exactly as it was. The promise is literal.
	pending, prop, err := vComputeAll(base, lazy)
	if err != nil {
		return nil, err
	}

	// --- and only now, the hand ---
	doc, err := VersionDocLoad(cfg)
	if err != nil {
		return nil, err
	}
	if doc.Spec == nil || doc.Spec.Version == nil {
		return nil, fmt.Errorf("version-spec has no <version> element to write")
	}
	for i := range spec.Components {
		doc.Spec.Version.SetComponent(spec.Components[i].Name, spec.Components[i].Value)
	}
	doc.Spec.Version.SetTs(ts)
	for _, key := range spec.LabelOrder {
		l := spec.Labels[key]
		if l.Computed() {
			continue
		}
		_, given := labels[key]
		if !given && !l.Volatile {
			continue // untouched: round-trips with its inner content intact
		}
		if node := doc.Spec.Label(key); node != nil {
			node.SetValue(l.Value)
		}
	}
	if doc.History != nil {
		attrs, err := vReleaseAttrs(lazy, tag, ts, method)
		if err != nil {
			return nil, err
		}
		doc.History.InsertRelease(attrs)
	}
	if err := doc.Save(""); err != nil {
		return nil, err
	}

	// the gesture's tail: flush what was computed and validated up front.
	// Only a PHYSICAL write failure (permissions, disk) can land here —
	// and -vbuild repairs it once the cause is fixed.
	if err := vFlush(pending, prop); err != nil {
		return nil, fmt.Errorf("version written (%s), destination write failed — fix and run -vbuild: %w", tag, err)
	}
	return &VMutated{Tag: tag, Ts: ts, Written: prop.Written, Skipped: prop.Skipped,
		Warns: append(lazy.Warns, prop.Warns...)}, nil
}

// vReleaseAttrs builds one history entry: version, ts, every VALUE label
// by its key in declaration order (computed ones are derivable), method.
// The labels are archived RENDERED: the history is a log — output, like a
// destination — and a log freezes what was true at that instant; a raw
// template there would re-render differently later, which is exactly what
// a record must not do.
func vReleaseAttrs(lazy *LazyDoc, tag, ts, method string) ([]xml.Attr, error) {
	attr := func(k, v string) xml.Attr {
		return xml.Attr{Name: xml.Name{Local: k}, Value: v}
	}
	attrs := []xml.Attr{attr("version", tag), attr("ts", ts)}
	resolve := lazy.ResolverFor(nil)
	for _, key := range lazy.Spec.LabelOrder {
		l := lazy.Spec.Labels[key]
		if l == nil || l.Computed() {
			continue
		}
		ref, err := ParseVersionRef("label:" + key)
		if err != nil {
			return nil, err
		}
		v, err := resolve(ref)
		if err != nil {
			return nil, fmt.Errorf("archiving label %q: %w", key, err)
		}
		attrs = append(attrs, attr(key, v.Text))
	}
	return append(attrs, attr("method", method)), nil
}
