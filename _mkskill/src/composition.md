---
mkskill:
  pos: 80
  in: ai*
---

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
