# Milestone 3 — causal history

**Status:** delivered. [doc_m1.md](doc_m1.md) records the failure lab, [doc_m2.md](doc_m2.md) the one-way partition.
This document records the milestone that made the log explain itself, and the limits of that explanation.

---

## 1. What this milestone adds

The log used to say *what* happened and *when*. It now also says *why*, because every event carries the references
of the events that caused it:

```json
{"seq":14,"at":570000000,"kind":"suspicion_raised","node":"B","peer":"A","reason":"heartbeat_timeout","causes":[12]}
```

Event 14 is A distrusting B at 570 ms. Its cause is event 12, the last heartbeat that arrived, at 220 ms. Follow 12
and it cites event 9, the send at 200 ms; follow 9 and it cites round one; round one cites the node starting; the
node cites the scenario. Nothing in that chain is the crash at event 11 — and that is the whole point of the
milestone, now stated by the artifact rather than by the commentary around it.

The change is one field and one command. It is not cosmetic: the field is written by the component that knows the
reason, at the moment it knows it, and a separate package reconstructs the graph from the finished log and refuses
to build one that points forwards, cites nothing, or ends anywhere other than the scenario.

## 2. The three claims, and how each one can fail

**C1 — Every event but the root cites at least one earlier event.**
Can fail by: a writer forgetting a cause, which leaves an orphan whose explanation silently stops one step short;
or a cause that has not been written yet, which turns the graph into something that can point forwards.

**C2 — No observer's belief is descended from ground truth.**
Can fail by: pointing a receipt at the crash, or at the cut, because they are the "real" reason the evidence
stopped. That would make every explanation agree with the simulator and with nothing the observer could have known.

**C3 — The graph explains what happened, and says nothing about what did not.**
Can fail by: pretending it is complete. A suspicion caused by a cut is descended from the last message that
arrived, not from the cut, because the cut prevented messages rather than producing one. An explanation that hid
that would be more reassuring and less true.

## 3. The canonical explanations

### A belief about a dead node

```
$ go run ./cmd/aegis explain -scenario node-crash -ref 14
14  570ms  suspicion_raised  B distrusted by A (heartbeat_timeout)
<- 12  220ms  heartbeat_received  B->A round 2
    <- 9  200ms  heartbeat_sent  B->A round 2
        <- 5  100ms  heartbeat_sent  B->A round 1
            <- 3  0s  node_started  B started
                <- 1  0s  scenario_started  node-crash
```

The suspicion is explained by a receipt, which is explained by a send, which is explained by the round before it,
which is explained by the node starting. The crash at 215 ms (event 11) is absent, and its absence is not a gap in
the data: nothing was ever delivered from the crash to A, so nothing A did can descend from it.

The arithmetic the milestone is about is visible without subtracting anything by hand: the last evidence arrived at
220 ms, the suspicion is at 570 ms, and the difference is the configured 350 ms timeout. The receipt is the cause;
the timeout is a property of the observer, carried on the report.

### A belief about a node that never died

```
$ go run ./cmd/aegis explain -scenario one-way-partition -ref 61 -depth 4
61  920ms  suspicion_cleared  B trusted again by A
<- 60  920ms  heartbeat_received  B->A round 9
    <- 57  900ms  heartbeat_sent  B->A round 9
        <- 50  800ms  heartbeat_sent  B->A round 8
            <- 44  700ms  heartbeat_sent  B->A round 7
                ...
```

The recovery is 20 ms after the repair and cites the arrival at 920 ms, not the repair at 900 ms. `link_restored`
appears nowhere in this chain, and a test asserts it has no descendants anywhere in the run: repairing a path is
recorded, cites the cut it ends, and explains nothing.

### The branch

A message lost in a cut has two reasons:

```
$ go run ./cmd/aegis explain -scenario one-way-partition -ref 22 -depth 2
22  320ms  message_dropped  B->A round 3 (partition)
<- 17  300ms  link_partitioned  B->A path cut (one way)
    <- 1  0s  scenario_started  one-way-partition
<- 19  300ms  heartbeat_sent  B->A round 3
    <- 12  200ms  heartbeat_sent  B->A round 2
            ...
```

The send alone would describe a message that vanished; the cut alone would describe an outage that lost nothing.
Both are needed, and the refs are printed in ascending order so two renderings of the same run cannot differ.

Both canonical logs now carry the field. `node-crash` has 16 events and `one-way-partition` 75; in each, exactly one
event cites nothing (the scenario) and every other event cites at least one cause — 15 and 74 respectively. The fixture
[internal/scenario/testdata/one-way-partition.jsonl](internal/scenario/testdata/one-way-partition.jsonl) was
regenerated, which is a change of bytes and not of timeline: every sequence number, timestamp, kind and round is
the same as it was for v0.2.0, and `TestCanonicalScenarioReproducesTheExpectedTimeline` was not edited.

## 4. The causal contract

Every kind of event has exactly one rule about what caused it. The rule is enforced by tests rather than by
convention, because a cause is only as good as the writer's honesty about what it knew.

| event | causes | why that is the honest answer |
| --- | --- | --- |
| `scenario_started` | none | the root; anything that cited something would make the graph unbounded |
| `node_started` | the scenario | a node begins because the run did |
| `heartbeat_sent`, round 1 | the sender's `node_started` | the first round is armed when the node starts |
| `heartbeat_sent`, round N | the previous send on the same direction | the interval timer is re-armed from the moment of sending |
| `heartbeat_received` | the send it answers | a receipt is evidence of one message, not of a sender |
| `message_dropped` (policy) | the send | the configured drop rate discarded this message |
| `message_dropped` (partition) | the send **and** the `link_partitioned` whose interval the transit met | two independent reasons; each alone is half a story |
| `suspicion_raised` | the receipt that armed its deadline, or the scenario if nothing ever arrived | the deadline measures silence from evidence; with no evidence, the scenario is what put the observer on watch |
| `suspicion_cleared` | the receipt that arrived | recovery is caused by evidence, not by elapsed time |
| `node_crashed` | the scenario | exogenous: the script did it, nothing in the world did |
| `link_partitioned` | the scenario | exogenous |
| `link_restored` | the cut it ends | the two ends of one outage belong together |
| `scenario_finished` | the scenario | the run closes because the scenario that opened it reached its horizon |

Two consequences are worth stating plainly, because both are claims and both are tested:

- **Ground truth is causally inert.** `node_crashed` cites the scenario and is cited by nothing. The same is true of
  `link_restored`. A crash and a repair are changes in the world; no observer's belief hangs from either.
- **A cut is not inert.** It is cited by every message it swallowed, which is what makes a lost heartbeat
  explicable. The cut is visible to the *network*, never to an observer, and the graph keeps that distinction: the
  drops descend from it, the beliefs do not.

## 5. What changed

| file | change |
| --- | --- |
| `internal/domain/ref.go` | `Ref`: a reference to a recorded event, valid only above zero |
| `internal/domain/event.go` | `Event.Causes`, with rules for zero references, increasing order and a root that cites nothing |
| `internal/observability/recorder.go` | `Record` returns the reference it assigned and refuses a cause that names the future; `Event(ref)` looks one up; `ReadJSONL` reads the artifact back |
| `internal/sim/engine.go` | `Open`, `Close`, `Root`; the receiver is handed the reference of the delivery |
| `internal/sim/node.go` | the start and the crash cite the scenario; each heartbeat cites the previous send on its direction |
| `internal/sim/network.go` | sends carry a cause and return a reference; a cut remembers the event that announced it; a drop cites the send and, inside a cut, the cut |
| `internal/sim/run.go` | a partition records its two ends as a chain rather than as two unrelated events |
| `internal/detector` | a monitor remembers the reference that armed its deadline; suspicion and recovery cite it |
| `internal/causal` | new: validates a whole history as one rooted, backwards-pointing graph and explains any event |
| `internal/scenario` | `Result.Causal()`, the runner opens the history before building observers, and the fixture is regenerated |
| `cmd/aegis` | `explain -ref N [-depth N] [-log path]` |
| `Makefile` | `make explain` |

The one thing that did not change is the timeline. Adding a field to every line changes the bytes of the fixture and
nothing else: the events, their order, their timestamps and their reasons are the ones v0.2.0 produced.

## 6. Decisions

**D1 — A cause is the recorder's sequence number, not a second identity.**
The recorder already gave every event a number, densely and in virtual-time order. Reusing it means a reference is
also an ordering fact, so "recorded before" and "caused before" cannot disagree. *Rejected:* a separate event id
allocated by the writers. Two counters have to be kept in step by hand, and the failure mode — a citation that
resolves to the wrong event — is silent.

**D2 — Causes are recorded when the event is written, not reconstructed afterwards.**
The detector knows which receipt armed the deadline at the instant it arms it. A post-hoc reconstruction from
timestamps ("the last receipt before this") would produce the same answer in the canonical runs and a different one
in every interesting case, because the reason a deadline was set is not always the most recent event.

**D3 — A cause list is sorted, unique, and may hold more than one entry.**
A lost message has two independent reasons and flattening them loses information; leaving the order free would make
two renderings of the same run differ by the order of two simultaneous causes. *Rejected:* a single parent with the
other reason in the `reason` field, which is what the drop already had — it put prose where a reference belongs.

**D4 — Exogenous events cite the scenario.**
A crash and a cut are not caused by anything in the world; the script caused them. Pointing them at the scenario
keeps them out of every observer's ancestry while keeping every event reachable from the root, so the two goals —
"ground truth explains no belief" and "everything traces back to the run" — hold at the same time.

**D5 — The deadline remembers the reference that armed it.**
The monitor stores `evidence`, set to the receipt on every arrival and to the scenario when it is first armed. A
suspicion cites that. The alternative — citing `LastSeen` by finding the receipt at that timestamp — would tempt a
reader to think the graph was derived, and would break the moment two receipts shared a tick.

**D6 — The repair cites the cut, and nothing cites the repair.**
An outage is one object with two ends, so `link_restored` hangs off `link_partitioned`. This has a consequence
worth stating: the repair is the second event in the history with no descendants at all, which is exactly the claim
that repairing a path does not repair a belief.

**D7 — The explainer lives in its own package and reads only the log.**
`internal/causal` imports `domain` and the standard library. It cannot ask the simulator what happened, so it
cannot produce an explanation that a reader of the JSONL could not produce. That is the same rule the detector
follows for the same reason: knowledge has to come from the artifact.

**D8 — The invariants of the graph are checked by the index, not by the recorder.**
The recorder enforces the rules that are true event by event: a cause precedes its effect, references are real. The
index enforces the rules that are only true of a whole history: one root, every event cited, every walk ending
there. Putting the second set in the recorder would have meant asking it to know the future. *Rejected:* skipping
the checks at write time entirely; every scenario run is validated with `Result.Causal()` in tests, so a writer
that forgets a cause fails the build rather than the demonstration.

### What changed while building this

Three things moved after the design settled, and all three are worth recording.

**A `deadline_armed` event was designed and then dropped.** The first sketch recorded every arming of a deadline, so
a suspicion could cite the arm and the arm could cite the receipt, with the timeout visible in between. It was cut
because it doubles the size of the partition log (every arrival re-arms) to express something the receipt plus the
report's own `Timeout` field already say. The monitor keeps the reference instead, and the arithmetic stays a
property of the report rather than of the graph.

**The detector and the runner had to change together.** A detector cannot be built without the root of the history,
and only the runner knows it, so arming a deadline and opening the history became one commit. That commit also moved
`engine.Open` ahead of the observers, which trades the old "a configuration error writes nothing" property for "a
configuration error writes one event that is then discarded with the run".

**The gate itself was broken for a while.** The commit script checked `go test ./... | tail`, which reports tail's
exit status, and one commit went in with a failing test in `internal/causal`. The test was fixed, the commit
corrected, and the script now sets `pipefail`; the failure is recorded here because a verification story that hides
its own failure is not one.

## 7. Failure modes, and what stops them

| failure | what it would look like | what stops it |
| --- | --- | --- |
| a cause that has not happened yet | a graph that points forwards, and an explanation that quotes the future | `Recorder.Record` returns `ErrFutureCause`; the index refuses the reference too |
| an event that cites nothing | an ancestry that stops one step short of the root, with no sign that it did | `causal.New` returns `ErrUncaused`, and every scenario run is indexed in tests |
| a cause outside the history | a walk that ends at a number that resolves to nothing | `causal.New` returns `ErrDanglingCause` |
| a cause datestamped after its effect | an explanation that reads backwards in time while looking correct in sequence | `causal.New` returns `ErrFutureCause` when the cited event's tick is later |
| a phantom suspicion | a belief that cites a message the observer never received | the detector only sets `evidence` from a receipt it recorded itself |
| a crash in a belief's ancestry | an explanation that is right for the wrong reason, and impossible to falsify | a test asserts the crash has no descendants and appears in no suspicion's ancestry |
| a repair in a belief's ancestry | the comfortable story: the path came back, so trust came back | a test asserts `link_restored` has no descendants, and the recovery cites the arrival |
| two renderings of one run differing | non-deterministic output from a deterministic simulator | causes are sorted by reference; the JSONL is compared byte for byte by `make replay` |
| a fixture regenerated over a regression | a committed log that hides a changed timeline behind a legitimate schema change | the timeline tests do not read the fixture; they assert the events, and were not edited |

## 8. Known limitations

**Absence is not an event.** In the partition run, A suspects B because no heartbeat arrived for 350 ms, and the cut
is why none arrived. The graph does not say that. A's suspicion is descended from the last receipt at 220 ms and
nothing else, because the cut produced no arrival to cite; it prevented one. The log explains why A acted and not
why the world was quiet, and no amount of citation can close that gap without recording absences as events.

**The graph records what a writer believed, not what is true.** A cause is checked for existence and order, never
for correctness of judgement. A detector could cite an earlier receipt that did not arm its deadline and the graph
would accept it; only the detector's own tests catch that class of mistake.

**No counterfactuals.** The graph can say what happened and why. It cannot answer "would A have suspected B if the
cut had not happened", which needs a second run and a comparison between runs that no part of this project does yet.

**One run at a time.** References are per history. Two runs cannot be compared by reference, only by scenario, seed
and virtual time, and nothing enforces that they are the same shape.

**The explanation is a tree, not a proof.** It shows the reasons an event rests on. It does not show that those
reasons were sufficient, and the same chain would be produced whether or not the writer's reasoning was sound.

## 9. How to read a chain

```
aegis explain -ref <seq> [-scenario node-crash|one-way-partition] [-seed N] [-depth N]
aegis explain -ref <seq> -log <path-to-jsonl>
```

`-ref` is required and is a sequence number, not a timestamp: the log is numbered, and the number is the identity the
causes cite. `-depth` limits how far back to walk; a branch that is cut off prints `...` rather than stopping
silently. `-seed` matters only for scenarios with random loss, since a different draw is a different history and a
different numbering.

`-log` explains a history from disk instead of running one. `-scenario` and `-seed` are refused with it rather
than ignored: a seed only changes how a run draws its random decisions, so accepting one beside a recorded history
would let a reader believe it had done something.

Reading a file is also where the graph's rules earn their keep, because a file can be edited after it was written.
A refused log is refused as a file, with the rule that broke named:

```
$ aegis explain -log edited.jsonl -ref 61
aegis: log edited.jsonl is not a causal history: cause is not recorded before its effect: suspicion_cleared cites 63
```

The reader cannot catch this on its own: the edited line is still dense, well-formed JSONL, and the damage only
appears when the references are resolved. An unreadable file is reported separately, as `not a readable history`,
so a truncated artifact and a rewired graph do not share one message.

Reading tips that hold for every chain:

- **Read upwards.** The top line is the event; everything below it is a reason for the line above, indented under it.
- **A branch is a conjunction.** Two causes at the same level both contributed; neither is the "real" one.
- **The bottom of every chain is the scenario.** An event that stops earlier than that is a defect, and the index
  refuses it.
- **A missing event is information.** If the crash is not in a suspicion's ancestry, that is not a hole in the log:
  it is the statement that the crash never reached the observer.

## 10. Verification

```
gofmt -l .          # empty
go vet ./...        # empty
go test ./...       # 7 packages, 228 test functions
go test -race ./... # clean
make replay         # node-crash: identical / one-way-partition: identical
make check          # fmt, vet, build, test, race
```

Coverage is 93.1% overall: `causal` 96.4, `observability` 100, `sim` 94.9, `domain` 93.7, `detector` 91.7,
`scenario` 89.4, `cmd` 88.8. The project is 9,292 lines of Go, 6,025 of them tests.

What the tests establish, in the order the claims are made:

- every run of every scenario, over seven seeds and with the lossy network on, indexes without error;
- the crash and the repair have no descendants, and no suspicion or recovery is descended from either;
- every suspicion of a dead node cites a receipt, and that receipt cites a send;
- every drop inside the cut cites the cut and the send, in that order;
- the committed fixture, read back from disk, explains event 61 as the receipt at 920 ms;
- the same fixture, corrupted in a temporary copy so event 61 cites a later event, is accepted by the
  reader and refused by the index with `ErrCauseOrder`, and a reference to an event that does not exist
  is refused with `ErrDanglingCause`;
- the CLI renders the crash run's suspicion chain, the partition run's recovery, and a two-parent drop, and the
  first of those contains no crash.;
- the CLI reads a history from disk and explains it exactly as it explains a live run, and refuses one that a hand
  edit gave a future cause or a missing one, naming the file and the rule while printing no chain at all.

The fixture was regenerated once, in the commit that first wrote a cause. The timeline tests were not touched, and
the 16-event canonical run still has the same 16 events in the same order at the same instants.
