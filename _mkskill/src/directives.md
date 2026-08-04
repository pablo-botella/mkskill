---
mkskill:
  pos: 50
  in: ai*
---

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
