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
