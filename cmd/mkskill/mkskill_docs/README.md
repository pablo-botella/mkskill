## Overview

The problem: every project keeps a `README.md`, an `AGENTS.md` and a `SKILL.md`,
and they all repeat the same core — overview, usage, rules. The same content over
and over, drifting apart.

The cure: write the content **once** as markdown under `_mkskill/`, and let mkskill
compose every view from it.

The same cure now reaches the **release number**: it is just a label — a
number declared once in the config's `<version-spec>` — and everything
renders from there: the tag, a manifest, a C header, the publish script,
the binary's own `version` answer, one `mkskill -vinc-build` away. The
number exists in the XML **before** any tag does, so every destination
agrees by construction.

> mkskill dogfoods the convention: **this very file is generated from
> `_mkskill/src/`**. README, AGENTS.md and SKILL.md all come from the same
> sections; the engine's per-section directives ride the namespaced `mkskill:`
> front matter key and are stripped on compose.

[![Go Reference](https://pkg.go.dev/badge/github.com/pablo-botella/mkskill.svg)](https://pkg.go.dev/github.com/pablo-botella/mkskill)

## Install

```sh
go get github.com/pablo-botella/mkskill    # library
go install github.com/pablo-botella/mkskill/cmd/mkskill@latest    # CLI
```

## The `_mkskill/` folder

Every unit carries a `_mkskill/` folder — the single place its docs are assembled
from. The leading `_` keeps the Go toolchain out (`./...`, `go build`, `go vet`,
`go mod tidy`): the project stays clean and does not depend on mkskill.

```
_mkskill/
  mkskill.config.xml   # the unit tree: projects, meta, embed, preserve
  src/*.md             # the content sections — what compose reads
  alt/
    files/             # preserved destinations: mkskill's version, mirrored
    tips/              # starter recipes (mkskill tips)
    debug/             # scan radiography (-debug)
README.md              # ← generated
AGENTS.md              # ← generated
.claude/skills/<name>/SKILL.md   # ← generated
cmd/<x>/_mkskill/      # a nested unit per thing-with-docs (a CLI, …)
```

A unit is any folder with a `_mkskill/`: the project root and each nested unit the
config declares (`<child-project path="./cmd/x">`). The root's views pull the
children in with `include:` — the parent's document is the global one.

## Using mkskill

Two ways: as a **tool** — the `mkskill` CLI deploys any repo's docs — and as a
**host face** — a binary carries its composed docs and acts in its own name.

The tool:

```sh
mkskill build      # scan → prepare → resolve → deploy: write every artifact
mkskill check      # dry look: warnings, conflicts, section order
mkskill tips       # starter recipes by project-type → _mkskill/alt/tips/
mkskill -C . -generate-claude-skill -global   # this repo's skill → ~/.claude/skills/
```

`-C <dir>` points at another project; with a generate command the pointed repo
speaks for itself, composed on the fly — no binary needed (a JS project installs
its skill the same way).

The host wiring — `<embed>` generates `MkskillSpec`, the main checks it first:

```go
if err, done := MkskillSpec.CheckParams(); done {
    return
} else if err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
}
// …not a mkskill command: the host's own argument handling…
```

Then `mytool -generate-claude-skill -global` installs the skill with no mkskill
around, and `MkskillSpec.Usage(true)` lends the commands to the host's own help —
no import of its own: the generated embed already brings everything.

## The CLI

### mkskill (the CLI)

The `mkskill` binary barely parses the command line and calls the compiler — the
engine lives in `github.com/pablo-botella/mkskill/compiler`. It speaks with two voices:

- **Its own**: the generate commands act on mkskill's embedded docs —
  `mkskill -generate-claude-skill -global` installs mkskill's own skill.
- **Any repo's**: with `-C <dir>` the pointed repo speaks for itself, composed on
  the fly — no binary needed: `mkskill -C . -generate-claude-skill -global`
  installs a pure library's (or a JavaScript project's) skill just the same.

And as the **deployer** it builds the repo it stands in (or the one at `-C`):
every artifact, from the `_mkskill/` sources.

### Commands

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

### Wiring `go generate`

Go can't write source-tree files during `go build`; the hook is `go generate`. A
unit regenerates its docs with the external tool — it never imports mkskill nor
lists it in `go.mod`:

```go
//go:generate go run github.com/pablo-botella/mkskill/cmd/mkskill@latest build
```

`-generate-claude-skill -global` stays a manual, per-machine act — it writes to
your home, never to the repo.

## License

MIT — see [LICENSE](LICENSE).
