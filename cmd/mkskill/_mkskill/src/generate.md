---
mkskill:
  pos: 30
---

## Wiring `go generate`

Go can't write source-tree files during `go build`; the hook is `go generate`. A
unit regenerates its docs with the external tool — it never imports mkskill nor
lists it in `go.mod`:

```go
//go:generate go run github.com/pablo-botella/mkskill/cmd/mkskill@latest build
```

`-generate-claude-skill -global` stays a manual, per-machine act — it writes to
your home, never to the repo.
