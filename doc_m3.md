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
| `cmd/aegis` | `explain -ref N [-depth N]` |
| `Makefile` | `make explain` |

The one thing that did not change is the timeline. Adding a field to every line changes the bytes of the fixture and
nothing else: the events, their order, their timestamps and their reasons are the ones v0.2.0 produced.
