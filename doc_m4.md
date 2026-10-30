# Milestone 4 — recovery that has to be proved

**Status:** delivered. [doc_m1.md](doc_m1.md) built the failure lab, [doc_m2.md](doc_m2.md) the one-way partition,
[doc_m3.md](doc_m3.md) the causal history. This document records the milestone where the system finally changed the
world, and the machinery that stops the change from being its own witness.

---

## 1. What this milestone adds

For three milestones the simulator observed and never intervened. A crash stayed crashed for the whole run, and the
last word in the log was a belief. Now a decision can put the node back:

```
$ go run ./cmd/aegis run --scenario recovered-crash --format table
SEQ  AT     EVENT               DETAIL
...
14   570ms  suspicion_raised    B distrusted by A (heartbeat_timeout)
15   575ms  suspicion_raised    B distrusted by C (heartbeat_timeout)
16   620ms  recovery_proposed   action 1 granted for B
17   720ms  recovery_started    action 1 carried out on B
18   720ms  node_restarted      B restarted (ground truth)
...
22   840ms  suspicion_cleared   B trusted again by A
```

Three things are new, and they are separable on purpose:

- **A policy** says how much evidence is enough, how long to wait for it, and how long between deciding and acting.
  It is data — two witnesses, 50 ms of patience, 100 ms of delay — and it is the only configuration recovery has.
- **A gate** turns failure reports into a decision. It is a pure function: same witnesses, same policy, same answer,
  no clock and no state.
- **A verification** answers "was this action carried out" by walking the edges of the finished log. It is a query,
  not a stage, because nothing in the history is written by the component being checked.

The timing carries the milestone. The review at 620 ms is 50 ms after A's own suspicion at 570 ms: patience is
measured from the evidence, not from when the loop happened to run. The action lands 100 ms later because the policy
says so. The next 120 ms is not configured anywhere: the restarted node's first message goes out at 820 ms, one
heartbeat interval after it resumed, arrives 20 ms later at A, and only then does A stop suspecting it.

## 2. The two runs

### A crash that gets repaired

```
$ go run ./cmd/aegis run --scenario recovered-crash --format table
```

| t | event |
| --- | --- |
| 0 ms | A, B, C start |
| 100 ms | B sends round 1 to A and C |
| 120 ms / 125 ms | A / C receive it |
| 200 ms | B sends round 2 |
| **215 ms** | **B crashes** |
| 220 ms / 225 ms | round 2 still arrives — it was already on the wire |
| 570 ms / 575 ms | A / C stop trusting B, 350 ms after their last evidence |
| **620 ms** | **the gate reviews: two witnesses, threshold met, action 1 proposed** |
| **720 ms** | **the action is carried out and B restarts** |
| 820 ms | B sends round 3 — the counter did not start over |
| 840 ms / 845 ms | A / C receive it and stop suspecting B |
| 1200 ms | horizon: B running, both observers trust it, one action in the log |

The decision at 620 ms cites events 14 and 15, the two suspicions. Event 14 cites the receipt at 220 ms, which cites
the send at 200 ms. Nothing in that ancestry is the crash at 215 ms, for the third milestone's reason: the crash was
never delivered to anybody. The node comes back because two observers independently stopped trusting it — not
because the simulator noticed its own crash.

### A crash that does not

```
$ go run ./cmd/aegis run --scenario lone-witness --format table
```

| t | event |
| --- | --- |
| 0 ms | A and B start; B heartbeats A |
| 215 ms | B crashes |
| 570 ms | A stops trusting B |
| **620 ms** | **the gate reviews: one witness, threshold not met, action 1 refused** |
| 1200 ms | horizon: B still crashed, A still suspicious, no action in the log |

The refusal cites the one report it declined to act on and names the reason: `insufficient_witnesses`. B stays down,
and the run ends with a written decision not to act rather than with silence.

## 3. The refusal is the milestone

A threshold that is only ever crossed is a preference. `lone-witness` exists so that "two witnesses are required"
can fail, and so that failing is a recorded outcome rather than an absence of one.

Four rules make the refusal as reviewable as the grant, and each is enforced by a test rather than by habit:

- **A refusal must say why.** `Decision.Validate` rejects a refusal with an empty reason and a grant that carries
  one. "We acted" needs no excuse; "we did nothing" needs one.
- **A refusal must cite something.** `Gate.Decide` returns `ErrNoWitnesses` with no evidence at all, because a
  refusal that cites nothing cannot be written into a history where every event but the root has a cause.
- **One observer, one vote.** `ErrRepeatedObserver` refuses the same observer reporting on the same target twice. A
  node that grows suspicious, recovers, and grows suspicious again has proved its own silence twice, not somebody
  else's failure.
- **Two targets are two questions.** `ErrMixedTargets` refuses a set of witnesses that do not name the same node,
  so one node's evidence can never justify acting on another.

The scenario was also the reason a modelling gap got closed. Its first version left C in the cluster with nobody
watching it, so B heartbeated a node that recorded nothing; both messages reached their destination and appeared in
the log as sends with no outcome, indistinguishable from a message still on the wire. A receipt is written by the
observer that accepts it, so a world about one piece of evidence must not manufacture traffic nobody keeps. The
world is now A and B, the fixture is 11 events, and every send in it has a recorded ending.

## 4. The four claims, and how each one can fail

**C1 — A decision cites evidence, and an action cites the decision.**
The proposal's causes are the `suspicion_raised` events; the start cites the proposal; the restart cites the start.
Can fail by: citing the crash or the action itself, which would make the world look justified by something the
operator could not have observed, or by recording the decision and the action as one event, which hides the window
in which the decision existed before the world changed.

**C2 — Nothing in the log says "verified".**
The proof is a walk over the written edges, performed by a package that cannot act. Can fail by: writing a
`recovery_verified` event when the restart succeeds. That event would be the run certifying its own work, and a
reader would have no way to tell a true certification from a confident one.

**C3 — A refusal is a decision, not a silence.**
It has an instant, a reason, and the references it declined. Can fail by: skipping the gate when the threshold
cannot be met, which is exactly the shortcut that makes a single observer's silence look like a dead node.

**C4 — Recovery is not belief recovery.**
B is running again at 720 ms, and A still distrusts it until 840 ms. Can fail by: clearing suspicion when the
restart happens, which is the tempting shortcut — the node is back, so the belief should be. It would make the log
say that an observer learned something it was never told.
