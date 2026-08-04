---
mkskill:
  pos: 70
  in: ai*
---

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
