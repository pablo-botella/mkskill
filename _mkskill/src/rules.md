---
mkskill:
  pos: 90
  in: ai*
---

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
