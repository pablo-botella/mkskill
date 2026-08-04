## mkskill (the CLI)

The `mkskill` binary barely parses the command line and calls the compiler — the
engine lives in `github.com/pablo-botella/mkskill/compiler`. It speaks with two voices:

- **Its own**: the generate commands act on mkskill's embedded docs —
  `mkskill -generate-claude-skill -global` installs mkskill's own skill.
- **Any repo's**: with `-C <dir>` the pointed repo speaks for itself, composed on
  the fly — no binary needed: `mkskill -C . -generate-claude-skill -global`
  installs a pure library's (or a JavaScript project's) skill just the same.

And as the **deployer** it builds the repo it stands in (or the one at `-C`):
every artifact, from the `_mkskill/` sources.

## Commands

Deployer commands — the repo at `-C`, or the current folder:

```
build      scan + prepare + resolve + deploy: write every artifact
check      scan + resolve without writing: warnings, conflicts, order
scan       collect the sources only
prepare    materialize the collected sources into _mkskill/src
tips       write the starter recipes to _mkskill/alt/tips/ (by project-type)
version    print the version
```

Version subsystem — the config's `<version-spec>` is the single source of
truth: the number exists there **before** any tag does, and mkskill only
produces output — it never tags, never pushes, never touches git:

```
-vinc-<comp>      increment a component, reset everything to its right,
                  propagate every destination — one atomic gesture
-vset "0.7.4"     set the number outright — the same gesture
-label:<key> "…"  set a label inline; rides on either mutator
-vbuild           re-propagate the destinations from the current state
-vcheck           verify config vs reality without writing — the CI leg
-vout <ref>       print one value (version, ts, a component, label:key)
-tag              alias of -vout version — git tag (mkskill -tag)
-vlabels          every label with its render, one line each
-vtrace [ref]     how a reference resolves, the whole chain shown —
                  without a ref, everything (the debugging radiography;
                  both work on broken configs: that is what they are for)
```

The generate family is **not hand-written**: this table is the macro
`msk.skill.usage.full`, expanded at compose time — in sync by construction:

```
-generate-claude-skill  [-dst f] [-global] [-force]
    write the Claude Code SKILL.md (-global installs ~/.claude/skills/<name>/SKILL.md)
-generate-agent-docs    [-dst f] [-force]
    write the agent-agnostic AGENTS.md
-generate-readme        [-dst f] [-force]
    write the README.md
```

Flags, anywhere on the line:

```
-C <dir>   act on that repo; with a generate command the repo speaks for itself
-log <f>   the run's record to a file (- for stdout, the default)
-debug     save the scan radiography to _mkskill/alt/debug/
-pretty    reformat the saved config (with -debug)
-q         silence the record
```

## Wiring `go generate`

Go can't write source-tree files during `go build`; the hook is `go generate`. A
unit regenerates its docs with the external tool — it never imports mkskill nor
lists it in `go.mod`:

```go
//go:generate go run github.com/pablo-botella/mkskill/cmd/mkskill@latest build
```

`-generate-claude-skill -global` stays a manual, per-machine act — it writes to
your home, never to the repo.
