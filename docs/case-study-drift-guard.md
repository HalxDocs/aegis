# Two implementations, one truth

**A case study in guarding a Go simulator against its TypeScript port — and in closing the five weaknesses that
guard turned out to have.**

**Status:** complete; both repositories are green, protected against history rewriting, and gated at push time.
**Repositories:** [`aegis`](https://github.com/HalxDocs/aegis) (Go) and `aegis-lab` (Vite + React + TypeScript).
**Commits under discussion:** `5190108`, `81c211d` (first pass) through `b85c16a`, `3812479`, `362c7be`, `c3566d6`,
`a927602` (second pass).

---

## 1. The system, briefly

`aegis` is a deterministic distributed-systems simulator in Go. It runs one of four scenarios (`node-crash`,
`one-way-partition`, `recovered-crash`, `lone-witness`) on virtual time, and emits a JSONL log: one event per line,
sequence numbers assigned by the recorder, no wall clock anywhere. Two runs of the same scenario with the same seed
must produce identical bytes. That determinism is the product, not a nicety — it is what lets a claim like "the
suspicion arrived 355 ms after the crash, not the configured 350" be checked instead of asserted.

`aegis-lab` is the explainer: the same engine ported to TypeScript, replayed in a browser, with a "why did this
happen?" panel that unwinds one event back to its causes. It is the thing people actually look at.

Two implementations, one truth. Which is the entire problem.

## 2. The failure mode this had to survive

A port and an original drift in a specific, nasty way: each side keeps working. A constant changes on one side —
`350` becomes `351` — and nothing throws. The Go program still produces a log. The page still renders a timeline.
The two simply describe different worlds, and the only thing that notices is a person reading both carefully enough
to spot a millisecond.

The obvious response — keep a copy of the expected output and compare — creates a second problem. A committed copy
of the truth is itself a third implementation: it can be updated to match the bug. Copies are how the drift gets
preserved rather than caught.

So the design rule was:

> If a check needs to know something, it asks the source. A number that appears twice is a number that will one day
> disagree with itself.

## 3. The guard, in layers

**L1 — CI gates both ways.** The Go workflow checks out the page and runs `make check-all`, which runs the Go checks
and then, with `LAB` pointed at the checkout, the page's own proof. A page-only change is therefore gated without a
commit to the implementation. Verified in run `41607820726` (`check` and `lab` both green).

**L2 — Fixture parity, byte for byte.** The page replays four committed fixture logs captured from Go: `node-crash`
(16 events), `one-way-partition` (75), `recovered-crash` (37), `lone-witness` (11) — 139 events total. `npm run
parity` re-runs the port and compares serialised bytes. Comparing parsed events would miss reordering, and reordering
is exactly the class of defect a virtual-time simulator produces, so the comparison is on text.

**L3 — Diagnosing the right side.** A byte diff that says "these differ" is a bug report the author cannot act on.
Parity classifies:

- the fixture is stale (the Go program no longer produces it), or
- the port drifted from a fixture that still matches Go,

and prints the divergent event with both sides aligned:

```
✗ parity failed

node-crash: the port differs from the fixture at event 14

    fixture: {"seq":14,"at":570000000,"kind":"suspicion_raised",…}
    engine:  {"seq":14,"at":571000000,"kind":"suspicion_raised",…}

  The fixture matches the Go program, so the divergence is in the port:
  fix src/engine/ until it reproduces src/engine/fixtures/node-crash.jsonl.
```

Absent fields render as `(absent)` rather than `undefined`, because `undefined` in a diff between two languages
reads as a value. Drift across a whole log is summarised with field tallies instead of 70 lines of diff. A renamed
event kind is called out as a rename caveat rather than presented as fifteen unrelated diffs.

**L4 — Coverage, not just agreement.** Agreement is satisfiable by covering less. If a fifth scenario exists in Go
and the page has no world for it, every committed fixture still matches — the page is simply describing a smaller
universe. Parity therefore compares the *sets*, and fails by name:

```
the implementation runs 5 scenarios; the page covers 4.

uncovered: cluster-split
```

**L5 — Naming the Go source.** When a fixture is stale, "this file is stale" is a weaker sentence than "this file is
stale, and the spec that produces it is `internal/scenario/x.go:41`". `scenarioSource` walks the Go tree and reports
the file and line of the declaration, handling both `Name: "x"` and `spec.Name = "x"`, ranking bare mentions behind
declarations so the flag parser in `cmd/` cannot answer for a scenario it merely recognises.

**L6 — Measurements are measured.** The page's proof card shows 10 packages, 91.7% coverage, and 4 scenarios
replayed. None of those numbers is typed into the page: `npm run gate` measures them in the Go repository and writes
`src/engine/gate.json`, and parity re-measures and fails with `run: npm run gate` when the committed card has drifted.
A card that is copied goes stale exactly the way a fixture does.

## 4. The part this case study is really about

With those six layers green, the guard had five weaknesses. They were found by reading it adversarially, and the
useful thing about them is that four were *not* logic errors — they were checks that could not fail, or failed for
the wrong reason.

### 4.1 A test that proved nothing

`TestUnknownScenarioNamesEveryRunnableScenario` asserted that the error message for an unknown scenario contains the
right scenario names. But the message is generated from the same `runnable` table the test reads. The test compared
a string to itself modulo wording and separators; it could not detect a scenario the binary refuses to run, because
the list it checked against came from the list that produced the message.

Deleting the copy is the actual fix, and after deletion there is nothing left for that test to catch *about
content* — that is the residue, and it should be named rather than counted. What was left worth testing is that the
advice works:

```go
// The message is generated from the same table this test reads, so this cannot
// catch a stale copy — there is no copy left. What it can catch is advice that
// does not run: every name the error offers must be accepted by lookup.
```

The test now parses the `available:` list back out of the message, requires set *and* order equality with
`scenarioNames()`, and then feeds every offered name through `lookup`. The assertion changed from "the sentence is
spelled correctly" to "the sentence is executable".

That reframing also caught a real defect introduced while writing it: the parser left a trailing `)` on the last
name, so `go test` failed and — because the gate card reads test results — the page reported `go test: card says true,
repo says false`, which reads like a stale card and was actually a stale parser.

### 4.2 A copy that had become the interface

Two parsers read Go source as text: the `runnable` table in `cmd/aegis/main.go`, and the `Name:` declaration behind
each fixture. Both failed loudly when the shape moved, which is why they were tolerable for a while — but a regex
over another repository's source is not an interface. It is a copy that happens to be regenerated.

The interface now exists: `aegis scenarios` prints one name per line and rejects arguments with usage.

```
$ go run ./cmd/aegis scenarios
node-crash
one-way-partition
recovered-crash
lone-witness
```

This deliberately fixed **one** of the two parse sites. The second — locating the file and line of a declaration —
stayed a regex, because its job is different: it must answer "roughly where" and degrade to `null` when the
answer is unknown. A locator that throws is worse than a locator that shrugs. Turning that into a contract would
mean the interface had to survive a scenario migrating to a new file, which is the case it exists to tolerate.

Proof that the new contract is load-bearing came from mutation, not confidence: a fifth entry added to the table
made parity print `the implementation runs 5 scenarios; the page covers 4.` / `uncovered: cluster-split`, then the
entry was removed.

### 4.3 A hardcoded path that failed loudly and wrongly

`scripts/go-scenarios.ts` located the table by path: `cmd/aegis/main.go`. If that file moved, the check failed — with
the right loudness and the wrong diagnosis ("no runnable scenarios found", implying the table was empty rather than
misplaced). With the subcommand, the reader shells out:

```ts
execFileSync("go", ["run", "./cmd/aegis", "scenarios"], {
  cwd: goRepo, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"],
});
```

and returns `null` on failure, where the failure message now names the command and the repository:

```
could not run `go run ./cmd/aegis scenarios` in /tmp/notgo.

  That command prints the scenarios the implementation can run, one per line.
  Without it the fixtures cannot be regenerated for the right set.
```

That message was verified by pointing `AEGIS_GO_REPO` at a directory containing no Go at all, rather than reasoned
about. The same stale path survived in `sync-fixture.ts`, which still said it could not read `main.go (func lookup)` —
found by grepping for the path after the refactor, which is the only reliable way to find the last copy.

### 4.4 Forty-three errors nobody could see

`scripts/` sat outside every `tsconfig`. `tsconfig.json` covered `src`; the build ran `tsc -b` over the app. So the
scripts that *enforce* the guard were the only untyped code in the repository, and `scripts/parity.ts` carried 43
strict errors.

All 43 had one cause:

```ts
const fail = (message: string): never => { console.error(message); process.exit(1); };
```

TypeScript narrows control flow on a `never` return only when the annotation is on the **binding**, not the
expression. As a parameter/return type on an arrow function assigned to a `const`, the call was not treated as
never-returning, so every guard written as `if (!x) fail("…")` left the compiler believing control continued — and
everything below it was unreachable, unreported dead code.

```ts
const fail: (message: string) => never = (message) => { … };
```

One declaration change; 43 errors to 0. Then, because a compiler that reports zero is indistinguishable from one
that reports nothing, a deliberate `const x: number = "s"` was added, seen to fail, and removed.

`typecheck:scripts` was added to `npm run check`, which now reads:

```
typecheck:scripts → lint → gate → parity → build
```

Type checking precedes everything that runs code, and lint precedes everything that measures anything.

### 4.5 A bus error explained by a byte count

`oxlint` died with SIGBUS, and it had been filed mentally as "pre-existing, unrelated breakage, not part of the
pipeline". That framing was a hypothesis, and it was wrong in an instructive way.

The native binding on disk was 5,691,165 bytes; the published platform package is 15,265,392. A truncated
`node_modules` entry — a partial download, not a version conflict — makes `dlopen` map pages that are not there, and
that is SIGBUS rather than a clean error. No configuration change could have fixed it; no lockfile change was needed
either. Extracting the published tarball restored it: `oxlint --version` → `1.89.0`, `npm run lint` → 4 warnings, 0
errors.

The lesson is not "reinstall dependencies". It is that "unrelated pre-existing breakage" is a claim about a
mechanism, and a size comparison tests the claim in four seconds. Since then lint has been step two of the pipeline,
so it is not merely working — it is load-bearing.

## 5. Branch protection, and what the platform cannot do

Both `main` branches were unprotected. Turning on the sensible settings is one command; the interesting part is
what "require status checks" means for a repository whose workflow pushes directly to `main`.

It means the branch becomes un-pushable. Enabled and tested:

```
remote: error: GH006: Protected branch update failed for refs/heads/main.
remote: - 2 of 2 required status checks are expected.
To https://github.com/HalxDocs/aegis.git
 ! [remote rejected] main -> main (protected branch hook declined)
```

The mechanism is circular by design: the checks run on the commit being pushed, so a new commit has none, so requiring
them rejects the push that would create them. With `enforce_admins: true` there is no bypass for anyone. The setting
is only usable with a branch-and-pull-request flow, which trades a 90-second wait for a workflow change nobody asked
for.

The setting was reverted rather than left in place, and the gate moved to where it can actually fire: a committed
pre-push hook.

```sh
# hooks/pre-push (aegis)
if [ -d "${LAB:-../aegis-lab}" ]; then
	exec make check-all
fi
exec make check
```

Installed by `make hooks` / `npm run hooks`, which point `core.hooksPath` at `hooks/`. Installing it surfaced a
second, smaller finding: `make hooks` reported *"up to date"*, because a directory named `hooks/` shadows a target of
the same name. Adding it to `.PHONY` fixed it — and only a real installation would have revealed it.

**The hook was then proven to block**, which is the only evidence that matters for a guard. The detector window was
changed `350 → 351`, committed, and pushed: parity failed at event 14, the push died locally, and the commit was
removed with `git reset --hard HEAD~1` (line 31 back to `350`, tree clean). The subsequent real pushes both ran
through the hook and landed with the full suite green on the way out.

Final state of both repositories, read back from the API rather than from the intent:

| Repository | `required_status_checks` | `enforce_admins` | `allow_force_pushes` | `allow_deletions` |
| --- | --- | --- | --- | --- |
| `HalxDocs/aegis` | none | true | false | false |
| `HalxDocs/aegis-lab` | none | true | false | false |

## 6. Evidence log

Every guard here was exercised by mutation. A passing check is not a working check; the table is the argument.

| Mutation | Guard | Observed |
| --- | --- | --- |
| Fifth scenario added to the table | `aegis scenarios` → parity | `the implementation runs 5 scenarios; the page covers 4.` / `uncovered: cluster-split` |
| Usage line drops a scenario | `TestUsageAnnouncesEveryRunnableScenario` | `lookup runs "lone-witness" but usage does not announce it` |
| Heartbeat `100 → 101 ms` (Go) | replay determinism / fixture parity | fixtures no longer reproduce; parity classifies as stale fixture |
| `MinWitnesses: 2 → 3` | fixture parity | world diverges at the grant decision |
| `Attempt int` added to `domain.Event` | fixture parity | schema drift reported by field tally, `(absent)` on the old side |
| `lone-witness` case cloned and renamed | coverage set | offered name does not run; test fails on the advice, not the string |
| `350 → 351 ms` in the port | page pre-push hook | `✗ parity failed … the port differs from the fixture at event 14`, push rejected |
| Deliberate `const x: number = "s"` | `tsc -p tsconfig.scripts.json` | two errors reported; zero after revert |
| Wrong `AEGIS_GO_REPO` | `sync-fixture` error path | names `go run ./cmd/aegis scenarios` and the missing repo |

Numbers measured, not estimated: 4 scenarios, 139 fixture events (16 / 75 / 37 / 11), 10 packages, 91.7% coverage, 4
scenarios replayed, local `npm run check` exit 0 with `built in 3.02s`.

## 7. What remains, honestly

- **Required status checks are off.** Not an oversight: they cannot coexist with direct pushes (see §5).
- **The hook is per-clone.** `core.hooksPath` is local git configuration, so a fresh clone needs `make hooks` /
  `npm run hooks` once. Both READMEs say so. `git push --no-verify` bypasses it by design; CI remains the backstop.
- **The error-message test guards shape, not content.** After deleting the copy there is nothing in it to drift. Its
  real assertion is that every offered name runs. Calling it coverage would be the original mistake wearing a new
  hat.
- **`scenarioSource` is still a regex.** It is a locator, not a contract, and it degrades to `null`.
- **One untracked file exists** in `aegis`: `docs/lab-causal-chain.png`, a browser capture of the lab page taken
  mid-load. Nothing references it and the talk promises no architecture diagram, so it was left on disk rather than
  committed.

## 8. Lessons worth carrying

1. **A test that reads the same source as the thing it tests is a formatting check wearing a test's clothing.** The
   fix is not a better assertion; it is removing the copy.
2. **An interface beats a stronger assertion.** `aegis scenarios` replaced a regex with a contract, and made the
   failing message name the command rather than a path.
3. **Diagnose which side moved.** "These differ" wastes an afternoon. "The fixture matches Go, so the port is wrong,
   fix `src/engine/`" does not.
4. **A guard that has never failed is an assumption.** Every entry in §6 is a deliberate break; the pre-push hook in
   particular was proven by trying to push a broken tree, not by reading the script.
5. **"Pre-existing and unrelated" is a hypothesis.** SIGBUS had a byte count. The 43 TypeScript errors had one
   annotation. Both were one measurement away from being fixable.
6. **Platform settings have mechanisms; test them and the mechanism tells you what it cannot do.** `GH006` is not a
   misconfiguration — it is the reason a pre-push hook exists here.

The through-line, if there is one: every layer in §3 is a comparison of two things, and every weakness in §4 was a
comparison of a thing with itself. Guards fail less often by being wrong than by being satisfied for the wrong
reason.
