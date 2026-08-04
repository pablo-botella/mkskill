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

## The config: `_mkskill/mkskill.config.xml`

One XML file declares the unit tree. Foreign attributes and elements survive every
rewrite (cargoxml): other tools may piggyback on the file safely.

```xml
<mkskill>
  <project id="main" name="myproj" description="…" project-type="Go-Module">
    <meta>
      <module>github.com/me/myproj</module>
      <license>MIT</license>
    </meta>
    <import-miniskin content="./src" fm-gen="extern,preserve"/>
    <preserve>
      <item file="./README.md" method="alias" alias="readme-alt.md"/>
      <item file="./_mkskill/src/*.md"/>
    </preserve>
    <child-project id="cli" name="mytool" path="./cmd/mytool" project-type="Go-CLI">
      <embed filename="mytool_generated_embed.go" module-name="main" embed-parent="*"/>
    </child-project>
  </project>
</mkskill>
```

- `id` — unique unit id, the cross-unit reference name (`include: cli`). Optional:
  a missing one gets "P0", "P1"… with a warning; explicit always wins.
- `name` — the unit's name (default: its folder); the skill's identity.
- `description` — one line, the SKILL.md trigger text (defaults to name).
- `project-type` — Go-Module | Go-CLI | Go | JS-Bundle | Other (unknown = error).
- `artifacts` — the views this unit deploys (`*` or `readme,agents,skill`).
  Default: everything at the root, only readme in a child — one project, one skill.
- `global-artifacts` — child only, explicit list (`agents,skill`): the PARENT's
  views deployed under this unit's base too. Never the readme — that one belongs
  to the unit itself; collisions warn, the first write stands.
- `<meta>` — free unit metadata, any tag inside: macros read it by name, rewrites
  re-emit it whole (comments included).
- `<embed>` — generate a Go file plus a `mkskill_docs/` folder feeding a
  `mkskill.Spec` via `//go:embed`; `embed-parent="*"` embeds the parent's views —
  a CLI binary that documents the whole project.
- `<preserve>` — files mkskill must not overwrite: `method="alias"` writes
  mkskill's version under the alias name, the default (`alt`) sidelines it to the
  `_mkskill/alt/files/` mirror. The original is never touched.
- `<import-miniskin>` — harvest sources marked with `mkskill-*` attributes in
  `*.miniskin.xml` manifests; `fm-gen` controls the front matter materialization
  (`embed` | `extern[,preserve]`).

With a `<version-spec>` in the config, every attribute above accepts
`{$$$ ref $$$}` version macros — EXCEPT `id`, `name`, `path` and
`project-type`, the sources the tree is built from: a macro there is a
load error, never a silent literal.

## Versioning: `<version-spec>`

The config can own the project's release number — one number for the whole
tree, declared in the XML **before any tag exists**; git only consumes it.
Everything validates before anything is written — literally: overflow,
lock, arity, unknown references, and every destination's anchors, files
and templates are computed **before even the config is touched**. A
mutation either lands whole or leaves the world exactly as it was; only
a physical write failure can interrupt it, and `-vbuild` repairs that.
Configured paths can never escape the project tree — `../`, absolute
paths and drive letters are config errors, not writes.

```xml
<version-spec format="v%d.%d.%d" params="major:byte,minor:[0-99],build:word">
  <version major="0" minor="5" build="2" lock="major">
    <ts></ts>
  </version>
  <label key="title" volatile="true" default-to-ts="true"></label>
  <update file="/.build/next-release.json" type="json">
    <entry key="tag">{$$$ version $$$}</entry>
    <entry key="title">{$$$ version $$$}{$$$ ' - ' + label:title $$$}</entry>
  </update>
  <create file="__publish.bat" git-ignore="true" eol="crlf">
git tag {$$$ version $$$}
git push origin main --tags
</create>
</version-spec>
```

- `params` — ordered components with domains (`byte`, `word`, `[lo-hi]`);
  incrementing one resets everything to its right. `lock` refuses `-vinc`
  and `-vset` alike: the conscious gesture is editing the config.
- `format` — optional Go-fmt override of the canonical render; absent it
  is `v` + the components joined by dots. `<ts>` is stamped UTC on every
  mutation.
- **Labels** are the one named-value system, `{$$$ label:key $$$}` to
  consume: value labels (the text node IS the value — and may itself
  carry macros: the document keeps the template, consumers get the
  render; `volatile` resets on every mutation, to `default` or the new
  ts with `default-to-ts`) and computed ones (`format` + `params`,
  guarded by `when` — an empty guard empties the whole render, and an
  empty value swallows its `'connector' +` too). A `<label>` under
  `<mkskill>` is global (`glb:label:key`), under a project it belongs to
  that unit (`prj:id:label:key`). Label chains of any depth are legal;
  a true cycle errors with its whole chain named.
- **Destinations** rewrite themselves on every mutation: `<update>`
  patches a file others own (`json`, `xml`, or the default `replace` —
  each template line self-anchors by its static text and rewrites every
  matching line, keeping the file's indentation and newline style);
  `<create>` writes a file wholly owned by the version (`git-ignore`
  promises it to .gitignore, `overwrite="true|warn|false"`,
  `eol="lf|crlf"`).
- `<version-history max="N">` — opt-in mutation log, newest first,
  trimmed FIFO; place the element and the mutators fill it (`version`,
  `ts`, the value labels, `method`).
- `embed-version` on an `<embed>` — a template attribute whose render the
  generated embed carries: the host binary's `version` command answers
  from the config, not from a hand-written constant.

**Macros in the config itself**: every claimed attribute and text value
of `mkskill.config.xml` may carry `{$$$ ref $$$}` — `artifacts`, `embed`
attributes, `preserve` files and aliases, `import-miniskin` content… the
user decides where a macro makes sense. The raw template stays in the
document (a rewrite never freezes it); the render feeds the tool. The
ONLY exceptions are the universe's own sources — `id`, `name`, `path`,
`project-type` — which cannot expand (the tree and the reference space
are built from them) and **fail loudly** when a macro appears: never a
silent literal.

The CI leg never bumps: it runs `-vcheck` (exhaustive — every label of
every domain resolves, referenced or not, and every destination is
compared against today's render) and fails when config and reality
disagree; `-vbuild` repairs drift. The daily gesture is two lines:

```sh
mkskill -vinc-build -label:title "what shipped"
__publish.bat
```

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

## Front matter directives

Everything per-section lives in YAML front matter — general metadata top-level,
the engine's directives namespaced under `mkskill:`. No custom syntax.

```markdown
---
title: Anything          # top-level: this section's own metadata (msk.meta.X)
mkskill:                 # the engine's directives
  pos: 20
  in: ai*
  after: api.md
---
```

| Key | Meaning |
|---|---|
| `pos` | ordering weight: 1 = top … 999 = bottom; unset = 500, the middle |
| `after` / `before` | anchor right after/before that section file (by filename; `path/name` when ambiguous) |
| `in` | target views: `*` (default) \| `readme` \| `ai*`, comma list |
| `include` | nest another unit's composition here, by id — headings demoted |
| `replace-macros` | expand the `<$$$msk.…$$$>` macros in this body (`replace_macros` works too) |

Unknown keys warn and are ignored — the phase never stops for a directive. A file
whose front matter lives in a sibling `.fm` (miniskin extern) reads only that; a
file with no front matter at all is pure body.

## Macros

Two pieces: `replace-macros: true` in a section's front matter says *whether* to
expand — off by default, so a section documenting the tags (like this one) keeps
them literal. `<$$$msk.…$$$>` in the body says *where*. The `$$$` delimiter is
deliberately rare: a plain exact replace — no engine, no parser, no escaping.

A plain key resolves with one precedence: the section's own front matter, then
the project attributes (`name`, `id`, `type`, `description`), then the unit's
`<meta>` tag — whatever is declared answers, no whitelist. Plus two special forms
and four cooked ones:

```
<$$$msk.view$$$>               the file being composed (README.md, …)
<$$$msk.meta.X$$$>             this section's front matter, explicitly
<$$$msk.badge$$$>              Go Reference badge (from module)
<$$$msk.install$$$>            go get + go install lines (module + Go-CLI units)
<$$$msk.skill.usage.short$$$>  the generate commands, compact
<$$$msk.skill.usage.full$$$>   the generate commands, with descriptions
```

A cooked macro yields to a declared value of the same name. An unknown macro
warns and stays in the text — a visible hole beats a silent one.

## Views (`in:`)

Content has two streams; the outputs are three:

```
readme  → README.md            (the human document)
ai*     → AGENTS.md, SKILL.md  (every AI view, present and future)
*       → all of them          (the default)
```

AGENTS.md and SKILL.md drink the same stream — same sections, different wrapper:
SKILL.md gets its front matter (`name` + `description` from the project
attributes, description defaulting to name) and lives in `.claude/skills/<name>/`.
A future AI format joins the `ai*` family without touching a single section.

What each unit writes is the `artifacts` attribute's business — root: everything;
child: its README — plus `global-artifacts` to carry the parent's agents/skill
along. Installing to `~/.claude/skills/` is never the deployer's job: that is
`-generate-claude-skill -global`, an explicit act.

## Composition & order

Sections come out in one resolved order: the weighted flow first — explicit
`pos`, 500 for the unset, scan order among equals — then every `after`/`before`
section woven right next to its anchor (a chain hangs together from its root).
Duplicate explicit pos, a missing or ambiguous anchor, after+before together, and
anchor cycles are errors: the phase stops.

Units compose across the tree: a section with `include: cli` nests that unit's
composition of the same view right after its own body, headings demoted one level
per depth. The parent's document is the global, organized one — that is why
installing the root skill documents the whole project, CLI included.

## Rules & gotchas

- **`_mkskill/` is build-invisible.** The `_` prefix keeps the Go toolchain out —
  never put buildable `.go` files there.
- **Two front matters.** The input one (in a source `.md`) carries directives and
  is stripped on compose; the output SKILL.md gets its own wrapper added by
  mkskill. Don't confuse them.
- **`*` must be quoted in YAML** (`in: "*"`); `ai*` and `readme` need no quotes.
- **Generated files are artifacts.** README.md, AGENTS.md, SKILL.md, the generated
  embed and its `mkskill_docs/` are all rewritten by `mkskill build` — edit the
  sources under `_mkskill/src/`, never the outputs.
- **Preserved files are never touched.** A `<preserve>` match redirects mkskill's
  version — the alias name, or the `_mkskill/alt/files/` mirror — and the log
  tells the story.
- **The config survives foreign content.** Unknown attributes and elements ride
  the cargo (cargoxml) through every rewrite — other tools may piggyback safely.
