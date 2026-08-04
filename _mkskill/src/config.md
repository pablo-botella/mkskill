---
mkskill:
  pos: 30
  in: ai*
---

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
