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
