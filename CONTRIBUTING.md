# Contributing

Thanks for taking a look. Aegis is a small project with unusually strict habits,
and the habits are the point: most of the code here exists to catch a claim that
stopped being true. Those rules apply to contributions too, so this file says
what they are before you spend an evening on a pull request.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## What the project is, in one paragraph

A deterministic discrete-event simulator for distributed failures, plus the
machinery around it: observers that suspect without seeing ground truth, a gate
that will not act without enough independent evidence, and a verifier that reads
the proof back out of the finished log instead of trusting the run that produced
it. The JSONL log is the artifact. Everything else — the table view, the
explanation walk, the explainer page — is derived from it.

## Setup

- Go 1.25 or newer
- `make` (optional; every target is a plain `go` command)
- Node.js and npm, only for `make check-all`, which also checks the explainer
  page if [`aegis-lab`](https://github.com/HalxDocs/aegis-lab) is checked out
  beside this repository (`LAB=/path/to/aegis-lab` points elsewhere)

```bash
make check       # gofmt, vet, build, test, race, and the public-surface check
make check-all   # the above, then the page's own proof (needs ../aegis-lab)
make replay      # every scenario twice, byte for byte
make hooks       # run the gate before a push
```

`make hooks` points `core.hooksPath` at `hooks/`, so `make check-all` runs before
anything leaves the machine. `git push --no-verify` skips it deliberately, and CI
still runs both halves on the branch.

## The four rules

**One source of truth.** If a fact exists in two places, one of them is a copy
and will eventually be wrong. A scenario's name lives in its spec; the list of
runnable scenarios is derived from that table; the versions in the release table
are checked against the tags that exist; the page's proof card is measured, never
typed. If your change needs the same fact twice, derive it instead.

**Every guard must be seen to fail.** A test that has never failed has not been
shown to test anything. Break the thing it protects, watch it fail, restore it,
and say so in the pull request. That is how the fixtures, the parity check and
the hook were verified, and it is the quickest review a change can get.

**Bytes before prose.** The canonical artifact is the JSONL log, and its
determinism is compared byte for byte, not event by event: comparing parsed
events would miss a reordering, which is exactly the defect the guard exists to
catch. If your change alters a log, the diff in `internal/scenario/testdata/` is
the change.

**Refuse rather than guess.** A log naming a format this build does not read is
refused whole. A scenario file with an unknown key is an error, not an empty
list. A decision with no evidence is an error, not a refusal. The pattern is
consistent: an input the program cannot honour should say so, because a plausible
guess is harder to find later than a failure.

## Adding a scenario

Three places, and the guards will tell you if you miss one:

1. `internal/scenario/spec.go` — a `func Name() Spec` beside the others, with a
   comment saying what the run shows and why the numbers are what they are.
2. `cmd/aegis/main.go` — one entry in `runnable`. Without it the scenario exists
   but cannot be run; the announcement test fails by name.
3. `internal/scenario/testdata/` — a committed log, if the scenario is one the
   project pins. Generate it with the command, never by hand:
   `go run ./cmd/aegis run --scenario name --format jsonl > internal/scenario/testdata/name.jsonl`.

Then sync the page, from `../aegis-lab`: `npm run sync-fixture`. Its parity check
fails when the implementation runs a scenario the page does not cover, so an
unsynced scenario is a red build rather than a gap nobody notices.

## Changing the log format

The format is a contract with every consumer, including the page's TypeScript
port. A change that a reader would have to know about bumps
`domain.EventLogSchema` in `internal/domain/event.go`, which is written on the
event that opens a history and on no other event.

1. Bump the constant and say why in the comment.
2. Regenerate `internal/scenario/testdata/` and the page's fixtures.
3. Change the port in `aegis-lab/src/engine/` in the same breath — parity
   compares bytes, so it will not let a half-migration through.
4. Update the field list in `internal/domain/event.go` and `aegis-lab`'s README if
   the shape changed.

Adding a scenario never bumps the schema. Adding a field a reader must interpret
does.

## Changing the scenario file format

`internal/scenario/file.go` holds the on-disk form as a separate type from the
`Spec` a run uses, on purpose: the file can be described and refused on its own
terms. Add a field in three places — the file struct, `Load`, and `Marshal` — and
extend the round-trip guard in `internal/scenario/file_test.go`. That test writes
every built-in scenario out, reads it back and compares the two logs, so a field
that `Marshal` forgets fails as a different world. If your field is zero in every
built-in scenario, add a world where it is not, the way the lossy network covers
`drop_ppm`.

## Changing the public package

`aegis.go` at the module root is the surface other programs are allowed to use.
Every name in it is an alias of the type that implements it — no wrappers that
re-implement, no copies that can drift. Adding a name is a promise that it will
keep working; running a scenario is deliberately not in that set while its shapes
still move with each milestone.

`make api-check` builds and runs `examples/consumer`, which is a separate module
for the only reason that matters: code inside this module can reach `internal`
whatever the public package claims, and code outside it cannot. If you export
something that only resolves internally, that target stops building.

## Style

- `gofmt` decides formatting; `make fmt` applies it, `make fmt-check` enforces it.
- Comments explain why, not what. Where a rule looks arbitrary, the comment says
  which failure it prevents — that is the documentation this project actually
  has.
- Errors are wrapped with `%w` and name what they were doing and what was wrong:
  `scenario recovered-crash: recovery B: record refusal: ...`.
- Tests are table-driven where the cases are data, and named for the rule they
  pin (`TestPartitionLosesOnlyMessagesThatCrossIt`), not for the function they
  call.
- Commits: an imperative subject with a scope where it helps
  (`feat(cmd/aegis): ...`, `docs: ...`, `build: ...`), and a body that says why
  the change was needed. The project does not keep a changelog file; the release
  notes and the README's release table are the history.

## Pull requests

Small and focused. If a change touches the format, the fixtures come with it. If
it fixes a bug, say how you reproduced it. If it adds a guard, say what you broke
to prove the guard works — a reviewer will ask.

If you are unsure whether something belongs, open an issue first. A good idea
that does not fit the current milestone is still worth writing down.
