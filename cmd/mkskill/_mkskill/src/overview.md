---
mkskill:
  pos: 10
---

## mkskill (the CLI)

The `mkskill` binary barely parses the command line and calls the compiler — the
engine lives in `github.com/pablo-botella/mkskill/compiler`. It speaks with two voices:

- **Its own**: the generate commands act on mkskill's embedded docs —
  `mkskill -generate-claude-skill -global` installs mkskill's own skill.
- **Any repo's**: with `-C <dir>` the pointed repo speaks for itself, composed on
  the fly — no binary needed: `mkskill -C . -generate-claude-skill -global`
  installs a pure library's (or a JavaScript project's) skill just the same.

And as the **deployer** it builds the repo it stands in (or the one at `-C`):
every artifact, from the `_mkskill/` sources.
