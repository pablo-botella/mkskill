---
mkskill:
  pos: 60
  in: ai*
  # NOTE: no replace-macros here on purpose — this section documents the
  # macros, so its tags must stay literal. That is the whole escape mechanism.
---

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
