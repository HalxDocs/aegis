# Aegis — Milestone 1 Plan: the Failure Lab

**Status:** plan only. No Go code exists yet; the workspace has this document and nothing else.
**Source:** `aegis_full_architecture_blueprint.pdf` (v1, 16 pages).
**Governing principle (verbatim from the blueprint):** every recovery action must be justified by observable
evidence, constrained by safety rules, and verified afterward. M1 builds the *evidence* half of that sentence and
stops there.

---

## 0. What the blueprint actually commits us to

| Concern | Decision (from blueprint §1, §9) |
| --- | --- |
| Language | Go, standard library first |
| Environment | Custom deterministic discrete-event simulator |
| Execution | One process modelling several nodes; no real concurrency needed yet |
| Time | Explicit virtual clock; **no real sleeps, ever**, including in tests |
| Interface | CLI + structured event output |
| Persistence / config / AI | None in M1; scenarios are Go tests, YAML comes later |

The blueprint's own scope fence for M1 (§6, §12): reproduce a node crash and show **separate observers** detecting
it, prove ground truth and observer belief are separately inspectable, emit a deterministic event log, and make
`go test ./...`, `go test -race ./...`, `go vet ./...` pass. **No recovery.** No diagnosis. No YAML. No dashboard.

---

## 1. Understanding: the three things M1 must prove

M1 is not "write a heartbeat detector". It is three claims, each of which can fail:

1. **Facts and knowledge are different objects.** A crashed node can still be believed healthy (the window before a
   timeout). A running node can be suspected (a slow or blocked link). If the code has one `Node.Status` field that
   mixes "what happened" with "what A thinks happened", the milestone has already failed — no test can rescue it.
2. **Time is an input, not a resource.** The scenario is a list of scheduled events. Advancing an hour of simulated
   time costs nothing, and the exact ordering at a boundary timestamp is a *specified* outcome, not an accident.
3. **The log is the product.** M1's artifact is an event history that a stranger can read and replay to the same
   final state, byte for byte. Every later milestone (diagnosis, policy, verification, audit) consumes this log.

### The semantic model (this is the whole milestone in one table)

| Layer | Owns | May read |
| --- | --- | --- |
| Simulator (ground truth) | `NodeState`: Running / Crashed, per node | everything |
| Network | in-flight messages, latency, drops | sender/receiver IDs only |
| Detector (belief) | `Observation` per (observer, target): last-seen, Healthy / Suspected | messages actually received |
| Recorder | append-only ordered events | both, tagged as truth or belief |

The detector **must never** read `NodeState`. That is an architectural boundary, not a convention, and §7 gives it
an enforcing test.

### The vocabulary trap

"Suspected" is a belief about a node, not a state of it. It lives on `Observation`, keyed by observer — A and C
suspect B *independently*, at possibly different virtual times (different link latencies). A single global
`IsSuspected` flag would silently destroy the property M1 exists to demonstrate.

---

## 2. Scope fence

**In M1**

- Domain types: `NodeID`, `NodeState` (+ validated transitions), `Observation`, `Event`, `FailureReport`.
- Virtual clock + deterministic event scheduler with an explicit tie-break rule.
- Three simulated nodes A, B, C; periodic heartbeats to each peer.
- Network with per-link latency and message drop.
- Heartbeat-timeout detector producing `FailureReport` with evidence.
- Append-only recorder with structured (JSONL) output.
- Scenario runner for tests + a thin CLI that prints one scenario's log.
- The seven test families in §7.

**Explicitly out of M1** (do not create these packages, even as stubs — blueprint §3: "avoid creating packages
before they have a clear responsibility")

- `diagnosis/`, `recovery/`, `verification/`, `faults/scenarios.go` (YAML), `testdata/`, `docs/decisions/` machinery.
- Network **partitions** (M2 — but the network is shaped so a partition is a per-link delivery predicate).
- Randomised schedules, property-based schedules, seeds-as-files (M4).
- Any wall-clock integration, Docker, Kubernetes, metrics HTTP endpoint, AI.

---

## 3. The scenario, precisely

Canonical M1 scenario — `node-crash`. Timings are virtual milliseconds, chosen so the interesting edge appears.

| Parameter | Value |
| --- | --- |
| Heartbeat interval (B → A, B → C) | 100 ms |
| Link latency | A←B 20 ms, C←B 25 ms |
| Detector timeout (A, C watching B) | 350 ms |
| Crash of B | t = 215 ms |
| Run horizon | t = 1200 ms |

Expected timeline (this is the acceptance fixture, not an illustration):

| t (ms) | Event |
| --- | --- |
| 0 | A, B, C start Running |
| 100 | B sends heartbeat #1 → A; #2 → C |
| 120 | A receives #1 → lastSeen(A,B)=120 |
| 125 | C receives #2 → lastSeen(C,B)=125 |
| 200 | B sends heartbeat #3 → A; #4 → C |
| **215** | **B crashes** (truth changes) |
| 220 | A receives #3 → lastSeen(A,B)=220 *(in flight when B died)* |
| 225 | C receives #4 → lastSeen(C,B)=225 |
| 570 | A's timeout expires → A suspects B |
| 575 | C's timeout expires → C suspects B |
| 1200 | horizon reached; both still suspect, truth still Crashed |

Three consequences worth stating up front, because each one breaks a naive test:

1. **Suspicion latency is not the timeout.** A's latency is 570 − 215 = 355 ms, because the last heartbeat arrived
   5 ms *after* the crash. Assert `suspectedAt ∈ [timeout, timeout + maxLatency]` or the exact logged value — never
   `crashAt + timeout`.
2. **In-flight messages survive the crash.** The network has no idea the sender died; retracting mail would be a
   simulator cheat. Consequence: `lastSeen > crashAt`, which is exactly the realism the lab needs. A node's own
   timers, by contrast, do stop — see D4.
3. **Truth and belief are simultaneously visible at t = 216 ms.** That instant (`Crashed` + `Healthy` + `Healthy`) is
   the single most important assertion in the milestone.

---

## 4. Design decisions, with the trap each one avoids

**D1 — Time is `Tick = int64` nanoseconds of virtual time.**
Config expressed as `time.Duration` and converted once at the boundary. Integer arithmetic only: float time makes
boundary comparisons inexact, and wall-clock time makes replay impossible. A source-scan test (§7, T7) fails the
build if `time.Now`, `time.Sleep`, `time.After`, or `time.Ticker` appear anywhere under `internal/`.

**D2 — Scheduler: one priority queue ordered by `(at, seq)`, FIFO within a timestamp.**
`seq` is assigned from a single monotonic counter when the event is *scheduled*, which also becomes the log's
sequence number. Documented rule: **at equal virtual time, the event scheduled first runs first.** Deterministic,
cheap, and it makes "a heartbeat and a timeout at the same tick" a specified outcome (T5 locks it). Events
scheduled *during* processing at the current tick get a later `seq`, so they run after everything already queued —
no starvation, no reordering surprises. A hard step cap (e.g. 1e6) turns a runaway self-rescheduling timer into a
test failure instead of a hang.

**D3 — Cancellation is lazy: epochs, not heap removal.**
Removing arbitrary entries from the heap is error-prone and makes ordering subtle. Instead each node and each
monitor carries an `epoch`; every scheduled timer captures the epoch it was created under and is ignored on fire if
stale. Cancelling = increment the epoch. This is the classic fix for the "stale timeout fires after a fresh
heartbeat" bug, which in a heartbeat detector is *the* bug: it produces phantom suspicion that looks like a real
failure. T5/T6 cover both directions.

**D4 — A crashed node stops its own timers; the network keeps delivering what it already holds.**
Implemented as: crash bumps the node epoch (kills future heartbeats) and flips truth to `Crashed`. In-flight
messages are untouched. This split is what makes consequence (2) above observable.

**D5 — Our own seeded PRNG (splitmix64, ~15 lines), not `math/rand`.**
`math/rand`'s algorithm is not a compatibility guarantee across Go releases, and a determinism project cannot have
its replay break on a toolchain upgrade. Draws happen only inside deterministic event processing (drops, jitter
later), so the draw order is itself an event order.

**D6 — Drops are decided at send time and recorded.**
A dropped message never enters the network, and the recorder gets `message_dropped{from,to,reason:"policy"}`.
Deciding at delivery time instead would conflate "lost on the wire" with "arrived late" — a distinction M2's
partitions and latency modelling need kept apart.

**D7 — One append path into the recorder; JSONL with struct field order.**
No `map[string]any` payloads, no wall-clock fields, no goroutine writing the log. Replay equality (T6) compares raw
bytes, which is a much stronger check than comparing parsed structs and catches ordering drift that "looks equal".

**D8 — The detector depends on `domain` only.**
Its input is a received-message callback plus the clock. If `detector` ever imports `simulator`, ground truth is one
struct field away, and the boundary dies quietly. Enforced by an import-graph test (T8) rather than by good
intentions.

**D9 — Typed states, one transition table, errors on violation.**
`NodeState` transitions: Running→Crashed only (self-transition rejected as a no-op error; Crashed→anything
rejected). `Belief`: Healthy→Suspected→Healthy. Both validated in one place; strings never appear in conditionals.

**D10 — No goroutines inside the simulation.**
Determinism first; the blueprint says the same. `-race` is therefore green trivially, so T9 adds real concurrency
*outside* the engine (N goroutines each running the same scenario) to prove the engine shares no mutable state and
to give `-race` something meaningful to check.

**D11 — Iteration order is never a map.**
Peer lists, monitor tables, and anything walked while producing events are sorted slices. Map iteration order is
randomised per iteration in Go, and a single passing run hides it; T6 therefore repeats the scenario 100× in-process
so map order actually gets a chance to vary.

---

## 5. File plan for M1

Only these packages. Nothing else gets created "because the blueprint diagram has it".

| Path | Responsibility | Rough size |
| --- | --- | --- |
| `go.mod` | module `github.com/HalxDocs/aegis`, `go 1.26` | 3 lines |
| `internal/domain/id.go` | `NodeID`, link key, deterministic ordering helpers | 40 |
| `internal/domain/node.go` | `NodeState`, transition validation | 60 |
| `internal/domain/event.go` | `Event` kinds, JSON shape, `Tick` | 80 |
| `internal/domain/observation.go` | `Belief`, `Observation`, `FailureReport` + evidence | 90 |
| `internal/sim/clock.go` | `Clock` interface, tick constants/helpers | 30 |
| `internal/sim/scheduler.go` | heap, `(at,seq)` ordering, step cap | 110 |
| `internal/sim/network.go` | links, latency, send/drop/deliver, in-flight bookkeeping | 120 |
| `internal/sim/node.go` | node behavior: heartbeat scheduling, crash, epoch | 100 |
| `internal/sim/engine.go` | wiring, ground-truth accessor, run loop, horizon | 140 |
| `internal/sim/rand.go` | splitmix64 | 25 |
| `internal/detector/detector.go` | monitors, timeout scheduling, suspicion + restore, reports | 130 |
| `internal/observability/recorder.go` | append-only log, JSONL encode, trace accessor | 90 |
| `internal/scenario/nodecrash.go` | the canonical scenario as a Go struct + builder | 80 |
| `cmd/aegis/main.go` | `run --scenario node-crash --seed N --format jsonl\|table` | 110 |
| `internal/*/\*_test.go` | test families T1–T9 | 500–700 total |
| `Makefile` | `test`, `race`, `vet`, `check`, `demo` | 20 |
| `README.md` | what M1 is, how to run it, what it deliberately does not do | 80 |

Total ≈ 1,900 lines including tests. If step 1 is not under 100 lines, the design is wrong.

---

## 6. Build order (test first, one concept per step)

| # | Goal | Files | Failing test written first | Done when |
| --- | --- | --- | --- | --- |
| 1 | Domain types + transitions | `domain/*` | `TestNodeStateTransitions`, `TestBeliefTransitions` | `go test ./internal/domain` green; invalid transitions error |
| 2 | Clock + scheduler | `sim/clock.go`, `sim/scheduler.go` | `TestSchedulerOrdersByTimeThenSeq`, `TestSchedulerStepCap` | order asserted across ~20 events incl. ties |
| 3 | Recorder + JSONL | `observability/recorder.go` | `TestRecorderAppendsInSeqOrder` | bytes stable across two encodes |
| 4 | Network + PRNG | `sim/network.go`, `sim/rand.go` | `TestLatencyDelaysDelivery`, `TestDropIsDeterministicForSeed` | same seed → same delivery set, 100× |
| 5 | Nodes + engine | `sim/node.go`, `sim/engine.go` | `TestHeartbeatsArriveAtInterval`, `TestTruthAccessorIsSeparate` | A/C see B every 100 ms, no suspicion yet |
| 6 | Detector | `detector/detector.go` | `TestTimeoutCreatesSuspicion`, `TestStaleTimerIgnored` | suspicion exactly at lastSeen+timeout; epoch bug killed |
| 7 | The milestone scenario | `scenario/nodecrash.go` + T1–T6 | `TestCrashedButBelievedHealthy` | table in §3 reproduced exactly |
| 8 | CLI + Makefile + README | `cmd/aegis/main.go`, `Makefile`, `README.md` | none (manual demo) | two runs diff clean; `make check` green |

Steps 1–7 are the milestone. Step 8 is the thinnest possible wrapper so the lab is inspectable by hand; it stays
under ~110 lines and adds no features beyond printing the log.

---

## 7. Test matrix

| ID | Test | Scenario | Asserts |
| --- | --- | --- | --- |
| T1 | `TestNormalHeartbeatsNoSuspicion` | B healthy for 1200 ms | zero `FailureReport`s; all beliefs Healthy; truth Running |
| T2 | `TestCrashedNodeIsSuspectedByEachObserver` | canonical | A suspects at 570, C at 575 (from the log, not recomputed); both reports carry evidence (lastSeen, timeout, missed count); truth Crashed |
| T3 | `TestCrashedButBelievedHealthy` | canonical, inspected at 216 ms | truth(B)=Crashed **and** belief(A,B)=belief(C,B)=Healthy — the milestone's core claim |
| T4 | `TestDelayedLinkCausesFalseSuspicion` | B→A latency 400 ms > timeout | A suspects a **Running** node; report exists; later heartbeat restores belief to Healthy (locks the "suspicion ≠ proof" invariant in both directions) |
| T5 | `TestTieBreakHeartbeatReachesAtTimeoutTick` | heartbeat delivered at exactly lastSeen+timeout | documented FIFO rule decides the outcome; no suspicion, and the ordering is asserted explicitly |
| T6 | `TestDeterministicReplay` | canonical + a drop scenario, run 100× | JSONL byte-identical across runs; final truth and beliefs identical; different seed → different sequence |
| T7 | `TestNoWallClockInSimulation` | source scan of `internal/` | fails on `time.Now|Sleep|After|Ticker|NewTicker` |
| T8 | `TestDetectorDoesNotImportSimulator` | `go list`/parser import graph | `detector` imports `domain` (+stdlib) only |
| T9 | `TestConcurrentScenariosAreIsolated` | 8 goroutines, same scenario | each goroutine's log equals the reference log; meaningful under `-race` |
| T10 | `TestInvalidNodeTransitionRejected` | domain unit | Running→Running and Crashed→Running return errors |

Blueprint §8's partition test is deliberately absent: partitions are an M2 fault. T4 covers the same *detector*
lesson (unreachable ≠ dead) using link delay, which M1 owns.

---

## 8. Definition of done, and exactly how each item is checked

| Blueprint DoD item | Check |
| --- | --- |
| Test advances virtual time, no real sleeps | T1 runs 1.2 s of virtual time in microseconds of wall time; T7 guards the source |
| B crashes at a scheduled virtual timestamp | T2 asserts `node_crashed` event at t=215 with `seq` ordering vs. the in-flight deliveries at 220/225 |
| A and C suspect B only after their timeouts | T2 asserts exact suspicion ticks and that no report exists before |
| Ground truth inspectable separately from belief | T3 at t=216, plus `engine.Truth(id)` used only by assertions/recorder |
| Deterministic event log emitted | T6 byte comparison; CLI writes JSONL to stdout |
| Same scenario twice → same sequence and final state | T6 (100 iterations), plus a shell check in §8.1 |
| `go test ./...`, `-race`, `vet` pass | `make check` |

### 8.1 Commands

```bash
go build ./... && go vet ./... && go test ./... && go test -race ./...
go run ./cmd/aegis run --scenario node-crash --format jsonl > /tmp/a.jsonl
go run ./cmd/aegis run --scenario node-crash --format jsonl > /tmp/b.jsonl
diff /tmp/a.jsonl /tmp/b.jsonl        # must be empty
grep -rnE 'time\.(Now|Sleep|After|Ticker)' internal/ | grep -v _test.go   # must be empty
```

`golangci-lint`/`staticcheck` are not installed on this machine, so the gate is `vet` + tests + `-race` only, as the
blueprint specifies. Adding a linter is a separate, later decision.

---

## 9. How this plan can fail (watched risks)

1. **One `Node.Status` mixing truth and belief** — the milestone's most likely failure. Guard: T3 and T8 exist before
   any detector code.
2. **Stale timeout timers** — phantom suspicion after a fresh heartbeat. Guard: D3 epochs + `TestStaleTimerIgnored`
   in step 6, written before the detector.
3. **Map iteration in event-producing paths** — passes locally, breaks replay on a different run. Guard: D11 + T6's
   100 in-process repetitions (one run can hide random order).
4. **Asserting `suspectedAt == crashAt + timeout`** — wrong whenever a message is in flight over the crash (§3,
   consequence 1). Guard: assert against the logged fixture, and document the ±latency bound.
5. **Log comparison that is too loose** (comparing structs) hiding ordering drift — Guard: byte comparison, D7.
6. **Scope creep into recovery** — the blueprint's own rule. Guard: §2 fence; `recovery/` does not exist in M1.
7. **`-race` giving false confidence** with no goroutines — Guard: T9 puts real concurrency around the engine.
8. **Premature configurability** (YAML, flags for everything) — Guard: Go structs only, one scenario, three flags.

---

## 10. Deferred, with the hook left in place

| Deferred | Milestone | Hook M1 must leave |
| --- | --- | --- |
| Partitions | M2 | delivery is already a per-link predicate |
| Jitter / latency distributions | M2 | PRNG + per-link params exist |
| Dependency graph, causal history | M2 | events carry subject + correlation ID |
| Recovery planning, action IDs, safety gate | M3 | `FailureReport.evidence` is already a list, not a string |
| Generated schedules, seeded replay files | M4 | seed is an explicit engine input, not a global |
| Real processes / Docker / Kubernetes | M5 | engine is the only thing that knows the world is simulated |

---

## 11. Defaults I have chosen (change any of these before step 1 costs nothing)

- Module path `github.com/HalxDocs/aegis`, `go 1.26` (blueprint's suggested path).
- Virtual time in **milliseconds** for the canonical scenario, stored as nanoseconds; timings from §3.
- CLI: one scenario, `--seed`, `--format jsonl|table`; no config file.
- JSONL is the canonical log format; the `table` view is derived from it, never a second source of truth.
- Repository initialised as a git repo at the first commit, with `docs/` kept.
