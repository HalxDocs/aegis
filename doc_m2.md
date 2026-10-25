# Milestone 2 — one-way partitions

**Status:** delivered. [doc_m1.md](doc_m1.md) records the failure lab; this document records the first failure that
produces disagreement without a failure, and every decision that made it measurable.

---

## 1. The three claims, and how each one can fail

**C1 — A cut direction loses messages and nothing else changes.**
Can fail by: evaluating the cut at send time, so a message already on the wire sails through a path that is down; or
treating one directed link as the whole link, so the return path dies too and the disagreement disappears.

**C2 — An observer on the cut side doubts a node that is running.**
Can fail by: letting the observer see ground truth (the M1 rule, still enforced); anchoring the deadline to the cut
instead of to the last arrival; or hiding the cut from the log, which makes a suspicion look like a bug.

**C3 — Repairing the path does not repair the belief.**
Can fail by: clearing a suspicion when a cut heals, which is the simulator reaching into the detector; or by recording
the repair but never the recovery, which leaves the reader unable to tell 350 ms of doubt from a permanent verdict.

## 2. The canonical run, and the exact timeline

`scenario.OneWayPartition()`: A and B heartbeat each other every 100 ms, C watches B, and the B->A direction is cut
from 300 ms to 900 ms. Nothing crashes.

| t | event |
| --- | --- |
| 0 ms | A, B, C start; heartbeats armed at 100 ms |
| 120 / 220 ms | A's evidence from B, last of it at 220 ms |
| **300 ms** | **`link_partitioned` B->A** |
| 320 … 820 ms | six rounds sent by B into the cut, each recorded `message_dropped (partition)` at its arrival instant |
| **570 ms** | **`suspicion_raised` B, by A** — 220 ms of evidence + 350 ms of patience |
| **900 ms** | **`link_restored` B->A** |
| 920 ms | B's next heartbeat arrives; `suspicion_cleared` at the same instant |
| 1150 ms | horizon: three nodes running, one report, no message left on the wire |

Final state: truth is `running` for all three; A believes B healthy (last seen 1120 ms) and holds a report from
570 ms; B and C never doubted anybody.

The fixture is [internal/scenario/testdata/one-way-partition.jsonl](../internal/scenario/testdata/one-way-partition.jsonl),
75 events, compared as bytes by `TestOneWayPartitionMatchesTheCommittedLog`.

## 3. What changed

| File | Change |
| --- | --- |
| `internal/domain/event.go` | `link_partitioned`, `link_restored`, `ReasonPartition`; both new kinds validated as peer-only, no round, no reason |
| `internal/sim/network.go` | `cut`, `cut.crosses`, `wire.cutDuring`, `validateCut`, `addCut`, `arrive`; the send path now resolves the cut and schedules the outcome |
| `internal/sim/run.go` | `Engine.PartitionAt` schedules both ends of an outage and records them |
| `internal/sim/engine.go` | the network gets the engine's `Fail`, so a scheduled write cannot vanish |
| `internal/scenario/spec.go` | `PartitionSpec`, `Spec.Partitions`, name validation, `OneWayPartition()` |
| `internal/scenario/run.go` | cuts are declared before the history opens, so a rejected cut leaves no log |
| `cmd/aegis` | `--scenario one-way-partition`, and table rendering for the two new events |
| `Makefile` | `make replay` covers every scenario in `SCENARIOS`, not just the canonical one |

Nothing in the M1 path changed shape: `--scenario node-crash` still produces the same 16 events, byte for byte, and
`TestCanonicalScenarioReproducesTheExpectedTimeline` was not touched.

## 4. Decisions

**D1 — A cut is a schedule, not a flag on a link.** `wire.cuts` holds intervals; the runtime task only records the
event. The alternative — a boolean that a task flips — makes the answer depend on which task ran first at an instant,
and this milestone exists to remove that class of ambiguity.

**D2 — A message is lost if its transit meets the cut.** The test is the intersection of the closed transit
`[send, arrive]` with the open cut `(from, until)`:

| transit | cut | outcome | why |
| --- | --- | --- | --- |
| 300 → 320 | 300 … 900 | lost | it departs as the path dies |
| 280 → 300 | 300 … 900 | delivered | it arrives as the path dies |
| 890 → 910 | 300 … 900 | lost | it is on the wire when the path dies |
| 900 → 920 | 300 … 900 | delivered | it departs as the path heals |
| 500 → 500 | 300 … 900 | lost | a zero-latency message inside the cut |

The cut is open at both ends precisely so no boundary needs a tie-break. `TestCutCrossesOnlyWhenTheTransitMeetsIt`
pins all ten cases.

**D3 — The loss is recorded at the arrival instant, not the send.** The sender already believes it sent, which is the
M1 principle "sent mail is not recalled". Recording at the send would put a `message_dropped` at 290 ms in a log whose
cut begins at 300 ms, and would claim knowledge the network does not have yet.

**D4 — One direction is one link.** There is no "the link is down" concept, so a scenario that wants both directions
declares two cuts. This is not pedantry: the whole scenario is a run where the two ends of one physical path disagree,
and a symmetric cut would delete the claim.

**D5 — Cuts that overlap or touch are refused.** Two cuts meeting at an instant describe one continuous outage while
writing `link_restored` and `link_partitioned` at the same tick. Rejecting the case is cheaper than explaining the
output.

**D6 — A report outlives the suspicion.** A's `failure_report` from 570 ms stays in `Result.Reports` after A recovers at
920 ms. A report is evidence about what an observer concluded, not a claim about the present; deleting it on recovery
would destroy the only record of the mistake this milestone is about.

**D7 — The cut is in the log.** `link_partitioned` and `link_restored` explain the drops. At 300 ms the partition event
precedes the heartbeats because it was scheduled during configuration, before any timer was armed — a stable ordering
by construction rather than by luck.

## 5. Failure modes, and what stops them

| Failure | What stops it |
| --- | --- |
| A cut in flight is ignored because it is evaluated at send | `cutDuring` tests the whole transit; `loss in flight` case in `TestPartitionLosesOnlyMessagesThatCrossIt` |
| A message survives a cut because the heal ran first | the predicate is pure configuration, so task order cannot change it |
| The return path dies too, silently | `TestPartitionLeavesTheReturnPathWorking` counts A->B sends against B's receipts and requires equality |
| The detector learns about the cut | the detector still imports only `domain`; the network never calls a receiver except on delivery |
| The belief follows the topology | `TestRepairingThePathDoesNotRepairTheBelief` requires the clear at 920, one link latency after the repair |
| A cut is announced but never repaired in the log, or vice versa | `TestPartitionBeyondTheHorizonIsNeverRepaired` |
| A rejected cut leaves a half-written log | cuts are declared before `scenario_started`, and `PartitionAt` validates before it schedules |
| A failed write inside a scheduled task disappears | the network holds the engine's `Fail`, and the test network fails the test |

## 6. Known limitations

1. **A message in flight during a cut is lost, never delivered late.** There is no queue that drains after the repair.
   Real networks do both; modelling the choice would need a per-link buffer policy and a new observation ("arrived
   late") that no milestone has asked for.
2. **No asymmetry within a direction.** A cut disables the path for every message equally; there is no per-message
   policy, no jitter and no correlated loss. Those remain the rest of M2.
3. **Overlapping cuts are rejected rather than merged.** A scenario that wants 300–500 and 450–700 must write 300–700.
4. **No causal history.** The log says what happened, not what depended on what; M2's dependency graph is still open.
5. **The CLI still exposes a fixed list of scenarios.** Scenario files remain a later milestone.
6. **Coverage is 93.5% of statements.** The dark branches are unreachable error paths: a recorder that fails only on
   the drop write, and scheduler calls that cannot fail after the time check.

## 7. Verification

All commands below were run on the delivered tree.

```
$ gofmt -l .                      # no output
$ go vet ./...                    # no output
$ go build ./...                  # no output
$ go test ./... -count=1
ok  github.com/HalxDocs/aegis/cmd/aegis              0.018s
ok  github.com/HalxDocs/aegis/internal/detector      0.010s
ok  github.com/HalxDocs/aegis/internal/domain        0.010s
ok  github.com/HalxDocs/aegis/internal/observability 0.008s
ok  github.com/HalxDocs/aegis/internal/scenario      0.076s
ok  github.com/HalxDocs/aegis/internal/sim           0.010s

$ go test -race ./...             # all packages ok
$ make replay
replay node-crash: identical
replay one-way-partition: identical
```

Coverage (`make cover`): 93.5% total — `cmd/aegis` 93.5%, `detector` 91.2%, `domain` 93.2%, `observability` 100%,
`scenario` 89.3%, `sim` 95.8%. Size: 6,641 lines of Go, 4,071 of them tests.

| Claim | Evidence |
| --- | --- |
| A cut loses only the crossing messages | `TestPartitionLosesOnlyMessagesThatCrossIt`, five cut placements including in-flight loss |
| Every boundary instant is decided, not tie-broken | `TestCutCrossesOnlyWhenTheTransitMeetsIt`, ten cases |
| The cut is announced at both ends | `TestPartitionAnnouncesBothEnds` |
| Nobody failed, yet somebody is wrong | `TestOneWayPartitionBreaksNobodyAndStillFoolsSomebody`: truth running; A reports at 570; B and C report nothing |
| The repair is not the recovery | `TestRepairingThePathDoesNotRepairTheBelief`: restore 900, clear 920, arrivals `[120 220 920 1020 1120]` |
| Only one direction is affected | `TestPartitionLeavesTheReturnPathWorking`: 11 sends, 11 receipts, 6 drops, all B->A |
| The history is reproducible | `TestOneWayPartitionMatchesTheCommittedLog` (bytes), `make replay` |
| Invalid worlds fail before any log exists | `TestPartitionNamesMustResolve`, `TestCutIntervalRulesStayWithTheSimulator`, `TestPartitionRejectsACutInThePast` |
