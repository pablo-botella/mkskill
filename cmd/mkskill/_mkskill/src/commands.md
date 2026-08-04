---
mkskill:
  pos: 20
  replace-macros: true
---

## Commands

Deployer commands — the repo at `-C`, or the current folder:

```
build      scan + prepare + resolve + deploy: write every artifact
check      scan + resolve without writing: warnings, conflicts, order
scan       collect the sources only
prepare    materialize the collected sources into _mkskill/src
tips       write the starter recipes to _mkskill/alt/tips/ (by project-type)
version    print the version
```

Version subsystem — the config's `<version-spec>` is the single source of
truth: the number exists there **before** any tag does, and mkskill only
produces output — it never tags, never pushes, never touches git:

```
-vinc-<comp>      increment a component, reset everything to its right,
                  propagate every destination — one atomic gesture
-vset "0.7.4"     set the number outright — the same gesture
-label:<key> "…"  set a label inline; rides on either mutator
-vbuild           re-propagate the destinations from the current state
-vcheck           verify config vs reality without writing — the CI leg
-vout <ref>       print one value (version, ts, a component, label:key)
-tag              alias of -vout version — git tag (mkskill -tag)
-vlabels          every label with its render, one line each
-vtrace [ref]     how a reference resolves, the whole chain shown —
                  without a ref, everything (the debugging radiography;
                  both work on broken configs: that is what they are for)
```

The generate family is **not hand-written**: this table is the macro
`msk.skill.usage.full`, expanded at compose time — in sync by construction:

```
<$$$msk.skill.usage.full$$$>
```

Flags, anywhere on the line:

```
-C <dir>   act on that repo; with a generate command the repo speaks for itself
-log <f>   the run's record to a file (- for stdout, the default)
-debug     save the scan radiography to _mkskill/alt/debug/
-pretty    reformat the saved config (with -debug)
-q         silence the record
```
