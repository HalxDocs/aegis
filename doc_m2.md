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
