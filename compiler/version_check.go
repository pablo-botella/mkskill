package compiler

// Version subsystem — the guard. -vcheck verifies without writing: it is
// the EXHAUSTIVE pass — the whole spec resolves, referenced or not (the
// orphan broken label nobody cites is THIS catch) — plus every
// destination computed and compared against the disk, .gitignore promises
// included. The CI leg: CI never bumps; it runs this and fails when the
// config and reality disagree.

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"slices"
)

// VCheckReport is what the guard found. Errors are broken resolutions or
// destination computations; Drift is a destination whose content differs
// from today's render — -vbuild repairs drift, errors need a human.
type VCheckReport struct {
	Errors  []string
	Drift   []string
	Skipped []string // overwrite="warn" creates: informational, never drift
	Warns   []string
}

// Clean reports whether the config and reality agree.
func (r *VCheckReport) Clean() bool { return len(r.Errors) == 0 && len(r.Drift) == 0 }

// VCheck is the -vcheck command.
func VCheck(base string) (*VCheckReport, error) {
	lazy, err := LazyLoad(VersionConfigFile(base))
	if err != nil {
		return nil, err
	}
	if lazy.Spec == nil {
		return nil, fmt.Errorf("no version-spec in the config")
	}
	rep := &VCheckReport{Warns: lazy.Warns}
	fail := func(what string, err error) {
		rep.Errors = append(rep.Errors, what+": "+err.Error())
	}

	// --- exhaustive resolution: everything declared, referenced or not ---
	resolve := lazy.ResolverFor(nil)
	tryRef := func(ref string) {
		r, err := ParseVersionRef(ref)
		if err != nil {
			fail(ref, err)
			return
		}
		if _, err := resolve(r); err != nil {
			fail(ref, err)
		}
	}
	tryRef("version")
	tryRef("ts")
	for i := range lazy.Spec.Components {
		tryRef(lazy.Spec.Components[i].Name)
	}
	for _, k := range lazy.Spec.LabelOrder {
		tryRef("label:" + k)
	}
	for _, k := range slices.Sorted(maps.Keys(lazy.Glb)) {
		tryRef("glb:label:" + k)
	}
	for _, u := range lazy.Units {
		for _, k := range slices.Sorted(maps.Keys(u.Labels)) {
			tryRef("prj:" + u.Id + ":label:" + k)
		}
		if u.EmbedVersion != "" {
			if _, err := ExpandVersionMacros(u.EmbedVersion, true, lazy.ResolverFor(u)); err != nil {
				fail("prj:"+u.Id+": embed-version", err)
			}
		}
	}

	// --- destinations: compute and compare, never write ---
	tmp := &VPropagated{}
	for _, dest := range lazy.Spec.Dests {
		writes, err := vComputeDest(base, resolve, dest, tmp)
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		for _, w := range writes {
			cur, err := os.ReadFile(w.path)
			switch {
			case err != nil:
				rep.Drift = append(rep.Drift, w.rel+": missing")
			case !bytes.Equal(cur, w.content):
				rep.Drift = append(rep.Drift, w.rel+": differs from the render")
			}
		}
	}
	rep.Skipped = tmp.Skipped
	rep.Warns = append(rep.Warns, tmp.Warns...)
	return rep, nil
}
