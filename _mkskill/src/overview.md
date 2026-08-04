---
mkskill:
  pos: 10
---

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
