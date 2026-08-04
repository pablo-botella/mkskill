---
mkskill:
  pos: 40
---

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
