---
mkskill:
  pos: 20
---

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
