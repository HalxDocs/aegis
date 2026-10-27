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
