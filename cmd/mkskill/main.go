// mkskill.exe barely parses the command line and calls the compiler: the
// engine lives in github.com/pablo-botella/mkskill/compiler and the self-doc
// machinery in the root package. The -C flag decides whose voice speaks:
//
//	mkskill -generate-claude-skill -global        mkskill's own embedded docs
//	mkskill -C . -generate-claude-skill -global   the pointed repo's docs, composed on the fly
//	mkskill build | check | scan | prepare        the deployer over the repo (-C or the cwd)
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/pablo-botella/mkskill"
	"github.com/pablo-botella/mkskill/compiler"
)

// version comes from the module's own build info, so a binary from
// `go install ...@v0.5.1` reports that tag and can never drift from it.
// Local `go build` has no tag to report and says "dev".
var version = buildVersion()

func buildVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return "dev"
	}
	return bi.Main.Version
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dir, logDest         string
		debug, pretty, quiet bool
		rest                 []string
	)
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-C", "--C":
			if i++; i >= len(args) {
				return fmt.Errorf("-C needs a directory")
			}
			dir = args[i]
		case "-log", "--log":
			if i++; i >= len(args) {
				return fmt.Errorf("-log needs a destination file (or - for stdout)")
			}
			logDest = args[i]
		case "-debug", "--debug":
			debug = true
		case "-pretty", "--pretty":
			pretty = true
		case "-q", "--q":
			quiet = true
		case "-v", "--v":
			quiet = false
		default:
			rest = append(rest, args[i])
		}
	}

	log, closeLog, err := openLog(logDest, quiet)
	if err != nil {
		return err
	}
	defer closeLog()

	if len(rest) == 0 {
		printHelp()
		return nil
	}
	cmd, cmdArgs := rest[0], rest[1:]

	// the deployer first — its commands never compose ahead of time: the
	// current folder, or another project folder with -C
	switch cmd {
	case "build":
		return pipeline(log, dir, debug, pretty, "scan", "prepare", "resolve", "deploy")
	case "check":
		return pipeline(log, dir, debug, pretty, "scan", "resolve")
	case "scan":
		return pipeline(log, dir, debug, pretty, "scan")
	case "prepare":
		return pipeline(log, dir, debug, pretty, "scan", "prepare")
	case "tips":
		root, err := load(dir)
		if err != nil {
			return err
		}
		return root.WriteTips(log)
	case "version":
		// two truths, both printed when both exist: the embed says which
		// release this claims to be (from the config's version-spec); the
		// build info says what was actually compiled.
		if MkskillSpec.Version != "" {
			fmt.Println("mkskill " + MkskillSpec.Version + " (" + version + ")")
		} else {
			fmt.Println("mkskill " + version)
		}
		return nil

	// the version subsystem — thin shell over the compiler verbs: parse
	// the argument, call, print. Any .build can do the same through the
	// compiler API.
	case "-vout":
		if len(cmdArgs) != 1 {
			return fmt.Errorf("-vout needs exactly one reference")
		}
		out, err := compiler.VOut(dir, cmdArgs[0])
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	case "-tag":
		out, err := compiler.VTag(dir)
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	case "-vbuild":
		prop, err := compiler.VBuild(dir)
		if err != nil {
			return err
		}
		for _, w := range prop.Warns {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		for _, w := range prop.Written {
			fmt.Fprintln(log, "[vbuild]", w)
		}
		return nil
	case "-vtrace":
		switch len(cmdArgs) {
		case 0: // the whole radiography: one-by-one is a chore
			lines, err := compiler.VTraceAll(dir)
			if err != nil {
				return err
			}
			for _, l := range lines {
				fmt.Println(l)
			}
			return nil
		case 1:
			_, lines, err := compiler.VTrace(dir, cmdArgs[0])
			for _, l := range lines {
				fmt.Println(l)
			}
			return err // the trace already printed WHERE it died
		}
		return fmt.Errorf("-vtrace takes one reference, or none for everything")
	case "-vlabels":
		lines, err := compiler.VLabels(dir)
		if err != nil {
			return err
		}
		for _, l := range lines {
			fmt.Println(l)
		}
		return nil
	case "-vcheck":
		rep, err := compiler.VCheck(dir)
		if err != nil {
			return err
		}
		for _, w := range rep.Warns {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		for _, s := range rep.Errors {
			fmt.Fprintln(log, "[vcheck] ERROR:", s)
		}
		for _, s := range rep.Drift {
			fmt.Fprintln(log, "[vcheck] drift:", s)
		}
		for _, s := range rep.Skipped {
			fmt.Fprintln(log, "[vcheck] skipped:", s)
		}
		if !rep.Clean() {
			return fmt.Errorf("vcheck: %d error(s), %d drifted destination(s)", len(rep.Errors), len(rep.Drift))
		}
		fmt.Fprintln(log, "[vcheck] clean")
		return nil
	}
	if strings.HasPrefix(cmd, "-vinc-") || cmd == "-vset" {
		return runVMutate(dir, cmd, cmdArgs)
	}

	// the generate family: -C decides whose voice speaks — without it,
	// mkskill's own embedded docs; with it, the pointed repo composed on
	// the fly
	spec := MkskillSpec
	if dir != "" {
		repoSpec, err := composeRepo(log, dir)
		if err != nil {
			return err
		}
		spec = repoSpec
	}
	if handled, err := spec.Dispatch(cmd, cmdArgs); handled {
		return err
	}
	printHelp()
	return fmt.Errorf("unknown command %q", cmd)
}

// load reads the config of the repo at dir (empty = the cwd) and voices its
// load warnings on stderr.
func load(dir string) (*compiler.Root, error) {
	root := &compiler.Root{ProjectBase: dir}
	if err := root.Load(); err != nil {
		return nil, err
	}
	for _, w := range root.Warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return root, nil
}

// pipeline runs the named phases in order over the repo — the current
// folder, or the one -C points at — plus the -debug radiography when asked.
func pipeline(log io.Writer, dir string, debug, pretty bool, phases ...string) error {
	root, err := load(dir)
	if err != nil {
		return err
	}
	for _, phase := range phases {
		var err error
		switch phase {
		case "scan":
			err = root.Scan(log)
		case "prepare":
			err = root.Prepare(log)
		case "resolve":
			err = root.Resolve(log)
		case "deploy":
			err = root.Deploy(log)
		}
		if err != nil {
			return err
		}
	}
	if debug {
		return saveDebug(log, root, pretty)
	}
	return nil
}

// composeRepo makes the repo speak for itself: the full read side of the
// pipeline (writes only _mkskill/src materializations, never artifacts),
// then a Spec filled with its root unit's composed views.
func composeRepo(log io.Writer, dir string) (mkskill.Spec, error) {
	spec := mkskill.Spec{}
	root, err := load(dir)
	if err != nil {
		return spec, err
	}
	for _, phase := range []func(io.Writer) error{root.Scan, root.Prepare, root.Resolve} {
		if err := phase(log); err != nil {
			return spec, err
		}
	}
	p := root.Project
	spec.Name, spec.Description = string(p.Name), p.MetaValue("description")
	for i := range compiler.Views {
		v := &compiler.Views[i]
		doc, err := p.Compose(log, v)
		if err != nil {
			return spec, err
		}
		switch v.Name {
		case "readme":
			spec.Readme = doc
		case "agents":
			spec.Agents = doc
		case "skill":
			spec.Skill = doc
		}
	}
	return spec, nil
}

// saveDebug writes the IncludeScanData radiography to its conventional home
// under _mkskill/alt/debug/.
func saveDebug(log io.Writer, root *compiler.Root, pretty bool) error {
	dst := filepath.Join(root.ProjectBase, "_mkskill", "alt", "debug", "mkskill.config.xml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	ctx := compiler.WithEncoderParams(context.Background(), &compiler.EncoderParams{IncludeScanData: true, PrettyOutput: pretty})
	fmt.Fprintf(log, "[debug] radiography -> %s\n", dst)
	return root.Save(ctx, dst)
}

// openLog resolves where the run's record goes: stdout by default, a file
// with -log, nowhere with -q.
func openLog(dest string, quiet bool) (io.Writer, func(), error) {
	if quiet {
		return io.Discard, func() {}, nil
	}
	if dest == "" || dest == "-" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(dest)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { f.Close() }, nil
}

func printHelp() {
	fmt.Println(`mkskill ` + version + ` — one source docset, every view

In mkskill's own name (its embedded docs), or a repo's with -C:
` + indent(MkskillSpec.Usage(true)) + `

Deployer commands (the repo at -C, or the current folder):
  build      scan + prepare + resolve + deploy: write every artifact
  check      scan + resolve without writing: warnings, conflicts, order
  scan       collect the sources only
  prepare    materialize the collected sources into _mkskill/src
  tips       write the starter recipes to _mkskill/alt/tips/ (by project-type)
  version    print the version

Version subsystem (the config's <version-spec> is the source of truth):
  -vinc-<component>   increment it, reset everything right, propagate
  -vset "0.7.4"       set the number outright — the same atomic gesture
  -label:<key> "..."  set a label inline; rides on either mutator
  -vbuild             re-propagate the destinations, mutating nothing
  -vcheck             verify config vs reality without writing (the CI leg)
  -vout <ref>         print one value: version, ts, a component, label:key
  -tag                alias of -vout version — git tag (mkskill -tag)
  -vlabels            every label with its render, one line each
  -vtrace [ref]       how a reference resolves, whole chain — no ref: all

Flags (anywhere on the line):
  -C <dir>   act on that repo; with a generate command, the repo speaks
             for itself — composed on the fly, no binary needed
  -log <f>   the run's record to a file (- for stdout, the default)
  -debug     save the scan radiography to _mkskill/alt/debug/
  -pretty    reformat the saved config (with -debug)
  -q         silence the record`)
}

// runVMutate is the thin shell over the two version mutators: collect
// the -label:<key> pairs riding the gesture, call the compiler, print
// the new tag. Any .build can do the same through the compiler API.
func runVMutate(dir, cmd string, args []string) error {
	labels := map[string]string{}
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-label:") {
			key := strings.TrimPrefix(args[i], "-label:")
			if key == "" {
				return fmt.Errorf("-label: needs a key")
			}
			if i++; i >= len(args) {
				return fmt.Errorf("-label:%s needs a value", key)
			}
			labels[key] = args[i]
			continue
		}
		positional = append(positional, args[i])
	}

	var res *compiler.VMutated
	var err error
	if cmd == "-vset" {
		if len(positional) != 1 {
			return fmt.Errorf("-vset needs exactly one value")
		}
		res, err = compiler.VSet(dir, positional[0], labels)
	} else {
		if len(positional) != 0 {
			return fmt.Errorf("unexpected argument %q", positional[0])
		}
		res, err = compiler.VInc(dir, strings.TrimPrefix(cmd, "-vinc-"), labels)
	}
	if err != nil {
		return err
	}
	for _, w := range res.Warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	fmt.Println(res.Tag)
	return nil
}

// indent shifts the shared usage block under its header.
func indent(s string) string {
	var out strings.Builder
	out.Grow(len(s) + 2)
	out.WriteString("  ")
	for _, r := range s {
		out.WriteRune(r)
		if r == '\n' {
			out.WriteString("  ")
		}
	}
	return out.String()
}
