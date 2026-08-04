---
mkskill:
  pos: 35
  in: ai*
---

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
