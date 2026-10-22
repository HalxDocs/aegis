# Milestone 1 — the failure lab

This document records how Milestone 1 of Aegis was built: what it claims, why each decision was made, what could
break, what actually broke while building it, and the evidence that the claims hold. It is meant to be read
alongside the code, not instead of it.

- **Scope:** reproduce a node crash in a deterministic simulation and show two independent observers detecting it
  from their own timeouts, with ground truth and belief separately inspectable.
- **Source:** `aegis_full_architecture_blueprint.pdf` (v1) and [docs/plan-m1-failure-lab.md](docs/plan-m1-failure-lab.md).
- **Not in scope:** recovery, diagnosis, verification, network partitions, scenario files, dashboards, real
  processes. Those are later milestones, and stubbing them now would only create packages with no responsibility.

---

## 1. The three claims, and how each one can fail

Milestone 1 is not "write a heartbeat detector". It is three claims:

1. **Facts and knowledge are different objects.** A crashed node can still be believed healthy; a running node can
   be suspected. This fails the moment one struct field means both "what happened" and "what A thinks happened".
2. **Time is an input, not a resource.** The run is a list of scheduled events on a virtual clock. Ordering at a
   boundary is a specified outcome, not a coincidence of scheduling luck.
3. **The log is the product.** A stranger can read the JSONL history and replay it to the same bytes. Every later
   milestone consumes this log.

Note on claim 2: virtual time is a resource in the sense that the queue is bounded, but advancing 1.2 simulated
seconds costs microseconds of wall time and no synchronisation.

---

## 2. Layout and dependency direction

```
cmd/aegis                 flags, scenario lookup, table rendering
internal/scenario         the only composition root: builds a world, runs it, returns both layers
internal/sim              clock, scheduler, network, nodes, engine (ground truth)
internal/detector         per-observer belief from received heartbeats
internal/observability    append-only recorder and JSONL rendering
internal/domain           virtual time, node ids and links, node state, belief, events, failure reports
```

Dependencies point inward:

```
cmd ──► scenario ──► sim, detector, observability, domain
                         │        │
                         │        └──► domain only
                         └──► domain
```

Two rules are enforced by tests rather than by good intentions:

- `internal/detector` may import the standard library and `internal/domain`, nothing else
  (`TestDetectorImportsOnlyTheDomain`).
- Only `internal/scenario` may import both `sim` and `detector`, so no component can quietly start reading the
  other half (`TestOnlyTheCompositionRootWiresBothHalves`).

The detector therefore has no compile-time path to `Engine.Truth`. That is the architectural point of the
milestone, not a convention.

---

## 3. The canonical scenario and the exact timeline

Three nodes. B sends heartbeats to A and C. A and C watch B with independent deadlines. B crashes at 215 ms.

| Parameter | Value |
| --- | --- |
| heartbeat interval, B → A and B → C | 100 ms |
| link latency B → A / B → C | 20 ms / 25 ms |
| observer timeout A / C | 350 ms / 350 ms |
| crash of B | 215 ms |
| horizon | 1200 ms |

The recorded history (seq, virtual time, event):

```
1      0ms  scenario_started    node-crash
2      0ms  node_started        A
3      0ms  node_started        B
4      0ms  node_started        C
5    100ms  heartbeat_sent      B->A round 1
6    100ms  heartbeat_sent      B->C round 1
7    120ms  heartbeat_received  B->A round 1
8    125ms  heartbeat_received  B->C round 1
9    200ms  heartbeat_sent      B->A round 2
10   200ms  heartbeat_sent      B->C round 2
11   215ms  node_crashed        B
12   220ms  heartbeat_received  B->A round 2
13   225ms  heartbeat_received  B->C round 2
14   570ms  suspicion_raised    B distrusted by A
15   575ms  suspicion_raised    B distrusted by C
16  1200ms  scenario_finished   node-crash
```

Three consequences, each of which breaks a naive test:

1. **Suspicion latency is 355 ms and 350 ms, not 350 ms for both.** A's last evidence arrived at 220 — after the
   crash at 215 — so its deadline fires at 570. C's last evidence arrived at 225, so its deadline fires at 575.
   Asserting `suspectedAt == crashAt + timeout` would be wrong for both.
2. **In-flight messages survive the crash.** The network already accepted round two; retracting it would be a
   simulator cheat. This is what makes `lastSeen > crashAt` possible, and it is why the suspicion window is real.
3. **At 216 ms, truth says `crashed` while both observers still believe `healthy`.** That single instant is the
   milestone. The test runs the scenario to exactly 216 ms and asserts all three values.

---

## 4. Engineering decisions

Each entry states the decision, the alternative that was rejected, and the defect the decision prevents.

### Time and ordering

**D1. Virtual time is `domain.Tick`, an `int64` count of nanoseconds.** Configuration uses `time.Duration` and is
converted once via `TickFrom`, which is exact because a duration is already a nanosecond count. Rejected: float
seconds (boundary comparisons become inexact) and wall-clock time (replay becomes impossible).
Guarded by `TestSimulationNeverReadsTheWallClock`, which parses every non-test file under `internal/` and fails on
`time.Now`, `Sleep`, `After`, `AfterFunc`, `Tick`, `NewTicker`, `NewTimer`, `Since` or `Until`.

**D2. The scheduler is a heap ordered by `(at, class, seq)`.** `class` is new relative to the plan: ordinary work is
class 0 and deadline expiry is class 1, so a message that lands on the deadline instant is processed first.
`seq` is assigned when work is *scheduled*, never when it runs, so work queued during execution at the same instant
goes to the back of that instant's queue.
Rejected: pure `(at, seq)`, where a deadline-versus-delivery tie is resolved by whichever timer happened to be
armed first. That is deterministic, but it is a statement about configuration order rather than about evidence. The
class key was added while writing the detector, before the end-to-end fixture existed, because without it a
heartbeat landing exactly on its deadline would raise a suspicion and clear it again at the same tick.

**D3. A run has a hard step limit of 1,000,000 tasks.** A timer that re-arms itself at the current instant turns a
hang into `ErrStepLimit`. Zero or negative limits fall back to the default rather than disabling the guard.

**D4. Cancellation is a tombstone in the queue plus an epoch in the detector.** `task.Cancel` marks the task dead;
the queue discards it when it reaches the head, so cancellation never searches the heap. The detector additionally
increments a per-monitor epoch on every re-arm and ignores any callback whose epoch is stale.
The plan specified epochs alone. Tombstones are cheaper and sufficient for a queue that never runs cancelled work;
the epoch is kept as the second lock because a phantom timeout is *the* failure mode of a heartbeat detector, and
it looks exactly like a real failure. `TestPhantomDeadlineIsIgnoredByTheEpochGuard` deliberately runs cancelled
work to prove the second lock holds.

**D5. Randomness is a local splitmix64, not `math/rand`.** `math/rand` makes no cross-release guarantee that a seed
produces the same stream, and this project's central claim is replay. The generator is verified against an
independent Python implementation of the same algorithm, and a source guard rejects `math/rand` imports.
Degenerate probabilities consume no draw: `DropPPM = 0` returns false without touching the stream, so adding a
drop-free link cannot shift another link's decisions.

**D6. No goroutines inside the simulation.** One clock, one queue, one thread. This is why `-race` is not enough on
its own, and why `TestConcurrentRunsShareNoState` runs eight scenarios concurrently around the engine instead.

### Truth and belief

**D7. `NodeState` and `Belief` are different types, and belief is keyed by `(observer, target)`.** `Engine.Truth`
returns ground truth for assertions and the recorder; the detector is never given the engine as anything but a
`Clock` and a `Scheduler`. Rejected: a single `Node.Status`, and a global `IsSuspected` flag. The flag would have
destroyed the property the milestone exists to show: A and C suspect B at different times.

**D8. The detector imports only `internal/domain`.** Enforced by an import-graph test, not by review.

**D9. There is no crash listener.** The first implementation had an `OnCrash` hook "so a component can mirror the
lifecycle". Nothing used it, and it was a one-field path for ground truth to reach belief without any test
noticing. It was removed rather than documented as unused. The detector's only inputs are deliveries, a clock, a
scheduler and a recorder.

**D10. A `FailureReport` is typed evidence, not prose.** Fields are `Observer`, `Target`, `SuspectedAt`,
`LastSeenAt`, `Timeout`, `Silence`. Construction rejects a healthy observation, a non-positive timeout, and a
silence shorter than the timeout — so "suspicion never precedes the deadline" is enforced at the type boundary
even if a caller tries to construct a report by hand.
The plan mentioned a "missed count". `Silence` replaced it deliberately: the detector knows how long it has been
silent, not how often the sender intended to speak. An observer that knows the sender's interval is an observer
with privileged information.

**D11. A target that was never heard from is still suspectable.** Watching arms a deadline immediately; otherwise a
node that dies before its first heartbeat would be believed healthy forever. The cost: a report for a never-heard
target measures silence from the start of the run, because there is no arrival to measure from. Milestone 1 watches
from tick zero, where that is exact; see §8.

### Network

**D12. Every link is declared; an undeclared path aborts the run.** No implicit zero-latency default, because that
would make delivery semantics depend on what someone forgot to configure.

**D13. Loss is decided at send time and recorded.** A dropped message never enters the network and produces
`message_dropped{reason:"drop_policy"}`. Deciding at delivery time would conflate "lost on the wire" with "arrived
late" — a distinction the next milestone's partition work depends on.

**D14. A crash stops the node's own timers but not the messages it already sent.** The network cannot retract what
it accepted; a node that crashes after sending is still heard from for one link latency. This split is what makes
"crashed but believed healthy" observable at 216 ms instead of being a one-instant artifact.

### The log

**D15. The recorder assigns every sequence number and refuses three things:** an event that is already invalid, an
event that carries its own sequence number, and an event whose virtual time precedes its predecessor.
Rejected: trusting callers to number events, and sorting at the end. Rejecting backwards time turns a latent
ordering bug into an error at the moment it happens, instead of a log that looks fine until someone diffs it.

**D16. JSONL is the canonical artifact; the table is derived from the same events.** Field order comes from the
event struct, never from a map. `WriteJSONL` reports encoder failures, so a closed pipe cannot look like a
successful run.

**D17. `Node` on an event is the node the event is about; `Peer` is the other party.** For heartbeat events that is
`from`/`to`; for suspicion it is `target`/`observer`, so filtering a log by one node yields the whole story of that
node. The convention is stated here because the domain validator can only check presence, not intent.

### Wiring

**D18. `internal/scenario` is the only composition root.** It validates names, builds the engine, links and
observers, records `scenario_started`, starts the world, schedules the crash, runs to the horizon, then records
`scenario_finished`. Nothing above that point writes an event, so a configuration error cannot leave a
half-written log.

**D19. Validating a scenario checks only what the components cannot check themselves** — that observer and target
are configured nodes, that names are well formed, that a horizon exists. Interval, link and transition rules stay
with the simulator, so each rule has one authority.

**D20. The engine delivers to every registered receiver, in registration order.** The first version had one
receiver slot; with two observers, whichever detector registered last would have silently seen all the traffic.
Ordering by registration keeps two observers' receipts in a fixed sequence.

**D21. Components that schedule work report failures through a `FaultSink`, which the engine implements.** A
deadline callback has no caller to return to; without a sink, a recorder failure inside a timeout would disappear
and the run would claim success. The engine keeps the first fault and returns it from `Run`.

**D22. Each observer has its own timeout.** Sharing a timeout per target would erase exactly the difference the
milestone measures.

**D23. The CLI has one scenario and two flags.** `--scenario` is looked up in an explicit switch; `--format` is
`jsonl` or `table`; `--seed` is the only input that affects random decisions. Unknown values are errors, not
defaults. Scenario files are a later milestone.

**D24. A run snapshots the spec it executed.** `Result.Spec` is a deep copy (including peers inside node configs),
so reusing and editing a `Spec` after a run cannot rewrite the record of that run.

**D25. The repository carries no attribution trailers and no agent state.** Commits are authored by the repository
owner; `.freebuff/` is ignored so tool state never enters the tree.

---

## 5. Failure modes, and what stops them

| # | Failure mode | Why it is dangerous | Guard |
| --- | --- | --- | --- |
| R1 | One field meaning both truth and belief | Every test can pass while the milestone's claim is false | Separate types, `Engine.Truth` never passed to the detector, `TestCrashedNodeIsBelievedHealthyUntilEachTimeout` |
| R2 | Stale timeout fires after a fresh heartbeat | Phantom suspicion that looks like a real failure | Tombstone cancel + epoch guard; `TestPhantomDeadlineIsIgnoredByTheEpochGuard`, `TestCancelledDeadlineNeverFires` |
| R3 | Map iteration reaches an event-producing path | Passes locally, reorders on another run | Sorted peer lists and node order; replay test compares raw bytes 100× |
| R4 | `suspectedAt == crashAt + timeout` | Wrong whenever a message is in flight over the crash | Fixture pins 570/575; property test bounds suspicion by `crash ± interval + latency` |
| R5 | Comparing parsed events instead of bytes | Hides ordering drift | `TestCanonicalScenarioIsByteIdenticalAcrossRuns`, `make replay` |
| R6 | A message at the deadline counted as silence | One-tick false suspicion, the hardest to notice | Scheduler class ordering; `TestMessageAtTheDeadlineInstantsWinsTheTie`, `TestHeartbeatOnTheDeadlineTickPreventsSuspicion` |
| R7 | Float or wall-clock time | Boundary comparisons become inexact, replay impossible | Integer ticks; source guard |
| R8 | `-race` green because there are no goroutines | False confidence in concurrency safety | Eight concurrent runs with byte comparison |
| R9 | Lost error in a scheduled callback | Run reports success while the log is incomplete | `FaultSink`; `TestRecorderFailureInsideADeadlineAbortsTheRun` |
| R10 | Delivery with no observer | Traffic silently falls into nowhere | `ErrNoReceiver` at the first delivery; `TestTrafficWithNobodyWatchingFailsTheRun` |
| R11 | Undeclared link | Latency becomes accidental | `ErrUnknownLink`; `TestHeartbeatToAPeerWithoutALinkStopsTheRun` |
| R12 | Self-rescheduling timer | Infinite loop, hung CI | Step limit; `TestSchedulerEnforcesStepLimit` |
| R13 | Observer watching a non-node | Suspects forever, test "passes" | Scenario validation; `TestSpecValidationRejectsIncoherentWorlds` |
| R14 | Spec aliasing after a run | Result describes a world that never ran | Deep copy; `TestRunSnapshotsTheSpecItExecuted` |
| R15 | Go release changes the PRNG stream | Replay breaks on a toolchain upgrade | Local splitmix64 + no `math/rand` |
| R16 | Scope creep into recovery | A milestone that cannot be verified as a unit | No `recovery/`, `diagnosis/`, `verification/` packages exist |

---

## 6. What actually broke while building this

Recorded because the failures are more instructive than the final state.

1. **An assertion that contradicted its own message.** `TestTruthReportsUnknownNodes` asked for an unconfigured
   node and then failed when it got exactly that. The test was wrong, the code was right; the assertion was
   corrected rather than the behaviour.
2. **One receiver slot, two observers.** Found while wiring the scenario, before it could produce a passing but
   wrong run. Replaced with registration-ordered fan-out.
3. **An unused crash listener.** It compiled, passed, and existed only as a shortcut from truth to belief. Removed.
4. **The first end-to-end tie test never tested a tie.** With a 350 ms timeout and a 350 ms link latency, the
   first heartbeat arrives at 450 ms while the initial deadline is 350 ms, so the observer legitimately suspected
   first. The scenario was corrected to a 250 ms latency so that the arrival and the deadline actually coincide —
   at which point class ordering is what turns "suspicion, then recovery at the same tick" into "no suspicion".
5. **The test double's clock did not advance.** The fake scheduler only moved time when a task ran, so a run with an
   empty queue left the clock at zero, and a watch started mid-run armed its deadline in the past relative to the
   simulated horizon. Fixed in the double; the production scheduler already parked the clock at the horizon.
6. **`heartbeat_received` was expected from a stub receiver.** The event belongs to the detector (it is the only
   component that knows a message arrived), so the simulator test had to assert deliveries on the receiver stub
   instead of on the log.
7. **An accounting assertion ignored in-flight messages.** `received == sent - dropped` is false when the horizon
   cuts the last round; the invariant was restated as an upper bound plus an exact per-message outcome check.
8. **A placeholder name (`slicesSort`) survived into the first compile.** Caught immediately by `go vet`, which is
   why the gate starts there.

---

## 7. Drift from the plan, and why

| Plan | Delivered | Reason |
| --- | --- | --- |
| Epoch-only cancellation | Tombstones + epoch | Tombstones make cancellation O(1) and correct with a queue that never runs dead work; the epoch stays as the guard against the one bug that looks like a real failure |
| `(at, seq)` ordering | `(at, class, seq)` | Needed so a delivery at the deadline instant is evidence rather than a tie decided by arming order |
| `FailureReport.evidence` as a list, with a missed count | Typed fields with `Silence` | An observer must not know the sender's heartbeat interval; silence is what it actually measured |
| `internal/cli` package | `cmd/aegis` with `main.go` and `table.go` | There is one command and no reusable CLI logic; a package would exist only to hold a `main` |
| `go 1.26` in `go.mod` | `go 1.25.0`, built with `go1.26.0` | Nothing in the code needs 1.26; the module declares the version the code is written against |
| One receiver | Registration-ordered receivers | Two observers, one sender |
| A crash listener for components | Removed | Unused, and a path from truth to belief |
| Test suite 500–700 lines | 3,500+ lines | The fixture, guards, property tests and concurrency test are all cheap to run and each pins a distinct claim |
| Eight build steps | Atomic commits, one concept each | Requested granularity; every commit builds and passes the gate |

---

## 8. Known limitations

These are deliberate and belong to later milestones, or to a decision that would have expanded the domain types.

1. **A late watcher's silence is measured from the start of the run.** If an observer starts watching at 500 ms and
   never hears anything, its first report at 850 ms says `Silence = 850ms`, because `LastSeen = 0` means "no
   evidence ever". Fixing it exactly needs a "watching since" field; Milestone 1 watches from tick zero, where the
   two coincide. Documented in `detector.Watch` and pinned by a test.
2. **Latency is fixed per link and loss is independent per message.** No jitter, no partitions, no correlation.
   Partitions are M2; the delivery path is already a per-link decision, so the hook exists.
3. **No persistence.** The log lives in memory and is written once at the end.
4. **No recovery, diagnosis or verification.** A suspicion produces a report and nothing acts on it.
5. **Single process, no real sockets.** The engine is the only component that knows the world is simulated.
6. **`golangci-lint` and `staticcheck` are not installed on the development machine.** The gate is `gofmt`, `vet`,
   `go test`, `-race` and `make replay`, exactly what the blueprint asks for. Adding a linter is a separate
   decision with its own configuration surface.
7. **Coverage is 93.4% of statements, not 100%.** The dark branches are mostly defensive early returns that cannot
   be reached without deleting a component mid-run; the coverage target exists so a reviewer can ask about them.
8. **The CLI exposes one scenario.** Others exist only as test values, on purpose: scenario files arrive later.

---

## 9. Verification

All commands below were run on the delivered tree. They are the definition of done.

```
$ gofmt -l .                      # no output
$ go vet ./...                    # no output
$ go build ./...                  # no output
$ go test ./... -count=1
ok  github.com/HalxDocs/aegis/cmd/aegis              0.004s
ok  github.com/HalxDocs/aegis/internal/detector      0.004s
ok  github.com/HalxDocs/aegis/internal/domain        0.005s
ok  github.com/HalxDocs/aegis/internal/observability 0.003s
ok  github.com/HalxDocs/aegis/internal/scenario      0.033s
ok  github.com/HalxDocs/aegis/internal/sim           0.008s

$ go test -race ./...             # all packages ok
$ make replay
replay: identical
```

Coverage (`make cover`):

| Package | Statements covered |
| --- | --- |
| `cmd/aegis` | 93.2% |
| `internal/detector` | 91.2% |
| `internal/domain` | 92.9% |
| `internal/observability` | 100.0% |
| `internal/scenario` | 88.9% |
| `internal/sim` | 96.3% |
| **total** | **93.4%** |

Size: 5,833 lines of Go, of which 3,534 are tests.

Claim-by-claim:

| Claim | Evidence |
| --- | --- |
| A crash at a scheduled virtual instant, with no real sleeps | `TestCanonicalScenarioReproducesTheExpectedTimeline` asserts `node_crashed` at seq 11, t=215ms, between in-flight deliveries at 220 and 225 |
| Each observer suspects only after its own timeout | `TestEachObserverSuspendsAtItsOwnDeadline` asserts 570 and 575 exactly; `TestSuspicionAlwaysWaitsForTheFullTimeout` checks twelve timeout/latency combinations |
| Truth and belief are separately inspectable | `TestCrashedNodeIsBelievedHealthyUntilEachTimeout`: at 216 ms truth is `crashed` and both beliefs are `healthy` |
| Suspicion is not proof | `TestSlowLinkMakesARunningNodeSuspectAndThenRecovers`: a running node is suspected once and then trusted again |
| The log is deterministic | 100 in-process runs compared as bytes; `make replay` diffs two CLI runs; a lossy run reproduces byte-for-byte across 50 runs |
| Different seeds actually matter | `TestDifferentSeedsProduceDifferentHistories` on a 40% loss network |
| The detector cannot read ground truth | `TestDetectorImportsOnlyTheDomain`, plus no exported route from `sim` to belief |
| No wall clock or `math/rand` | `TestSimulationNeverReadsTheWallClock`, `TestSimulationDoesNotUseMathRand` |
| Concurrency is safe around the engine | `TestConcurrentRunsShareNoState` under `-race` |

---

## 10. How to read the history

The commit history is the build order, one concept per commit: licence and module, domain vocabulary, scheduler,
PRNG, network, nodes, engine, detector, recorder, scenario, CLI, guards, documentation. Each commit builds and
passes the gate on its own. Reading them in order is the fastest way to understand the code, because every
abstraction is introduced by the commit that needed it.

Useful entry points:

```bash
git log --oneline --reverse         # the build order
go test ./internal/scenario -v      # the milestone's own fixture
make demo                           # the table view
```

---

## 11. What the next milestone inherits

- **Delivery is already a per-link predicate**, so a partition is a rule on that predicate, not a rewrite.
- **The PRNG and per-link parameters exist**, so jitter and latency distributions are additions to configuration.
- **Events carry subject and correlation fields** (`node`, `peer`, `round`), so causal history has a starting
  point.
- **Failure reports are typed evidence**, so a policy layer can decide what to do without parsing text.
- **The seed is an explicit input to `Run`**, so generated schedules and replay files are a caller concern.
- **The engine is the only thing that knows the world is simulated**, so a later milestone can replace it without
  touching the detector's contract.
