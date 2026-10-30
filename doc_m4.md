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
restart happens, which is the tempting shortcut — the node is back, so the belief should be.It would make the log say that an observer learned something it was never told.

## 5. The recovery contract

Recovery adds four kinds of event, and each one has exactly one honest cause:

| event | causes | why that is the honest answer |
| --- | --- | --- |
| `recovery_proposed` | one `suspicion_raised` per witness | the decision cites what it weighed, one edge per observer, so a reader can check the count against the threshold |
| `recovery_refused` | the same witnesses | a refusal rests on the same evidence as a grant; its `reason` says what the evidence did not reach |
| `recovery_started` | the proposal | carrying out a grant is caused by the grant. Nothing is re-decided on the way to the world |
| `node_restarted` | the start | the world changed because the action was carried out, and the file says so before it happens |
| `heartbeat_sent`, first round after a restart | `node_restarted` | the resumed chain runs through the reason the node is alive rather than past the crash it survived |
| `suspicion_cleared` | the receipt that arrived | unchanged from Milestone 3: a belief changes on evidence, never on topology or on the world being fixed |

Two consequences are worth stating, because both are claims and both are tested.

**A decision is written before the world changes.** `recovery_proposed` and `recovery_started` are separate events,
100 ms apart in the canonical run, and the proposal is in the file before the simulator is asked to act. A reader
therefore never finds a restart whose reason appears later in the file than the restart itself. The delay is not
decoration: it is the window in which the decision is reviewable as a decision.

**The restart is the first change in ground truth that has descendants — and that is correct.** Milestone 3's rule
was that no observer's belief descends from ground truth, because a crash or a cut is never delivered to anybody.
That rule now needs one qualification, and the qualification is the milestone's own subject. The crash still
produces nothing: nothing cites it, no belief descends from it. But `node_restarted` is cited by the first
heartbeat the revived node sends, so A's clearance at 840 ms does have a restart in its ancestry:

```
$ go run ./cmd/aegis explain -scenario recovered-crash -ref 22 -depth 6
22  840ms  suspicion_cleared  B trusted again by A
<- 21  840ms  heartbeat_received  B->A round 3
    <- 19  820ms  heartbeat_sent  B->A round 3
        <- 18  720ms  node_restarted  B restarted (ground truth)
            <- 17  720ms  recovery_started  action 1 carried out on B
                <- 16  620ms  recovery_proposed  action 1 granted for B
                    <- 14  570ms  suspicion_raised  B distrusted by A (heartbeat_timeout)
                            ...
                    <- 15  575ms  suspicion_raised  B distrusted by C (heartbeat_timeout)
                            ...
```

The two `...` markers are depth, not missing data: below each witness is the receipt that armed its deadline and the
send that receipt answers, all the way back to the scenario.

That is not a leak of truth into a belief. The restart is a change in the world that a decision caused, the
decision cites evidence two observers produced, and the news reaches A the only way news ever reaches an observer:
as a message that crossed a link. What Milestone 3 forbade was an explanation that rested on something nobody could
observe. This one rests on a decision the log contains in full.

The distinction the two events make is worth naming: **an event the world did by itself explains nothing; an event
the world did because a decision said so explains the messages it enabled.** The crash is the first kind. The
restart is the second.

## 6. What changed

| file | change |
| --- | --- |
| `internal/domain/action.go` | new: `ActionID`, the closed `ActionKind` vocabulary (one entry), and `Action` with the instant it takes effect |
| `internal/domain/event.go` | four new kinds, `Event.Action`, and `ReasonInsufficientWitnesses` |
| `internal/domain/node.go` | `crashed -> running` is a legal transition, and repeating a state still is not |
| `internal/recovery` | new: `Policy`, `Witness`, `Decision`, and the pure `Gate` that turns the first two into the third |
| `internal/detector` | `FailureSink`: every written report is handed on with the reference of the line that holds it |
| `internal/sim` | `Engine.Restart`, recording the start before the world moves; a node resumes its round counter and re-arms its timer from the restart instant |
| `internal/verify` | new: the proof query — proposal to start to restart, by identifier and edge, never by timestamp |
| `internal/scenario` | `RecoverySpec`, the loop that holds open cases, `recovered-crash` and `lone-witness`, and their fixtures |
| `cmd/aegis` | the `DECISION` table with an `EVIDENCE` and `PROOF` column, and both scenarios on `-scenario` |
| `Makefile` | `make replay` covers four scenarios |

No existing timeline moved. `node-crash` and `one-way-partition` produce the same bytes they produced for v0.3.0:
their scenarios configure no policy, so no part of recovery runs in them, and a test asserts that not one recovery
event appears in a run that did not ask for one.

## 7. Decisions

**D1 — The policy is the whole configuration, and there is no switch to skip the gate.**
A scenario either states the bar for acting or does not act at all. *Rejected:* an `Observe` mode that decides
without acting — it would produce a log that could not be compared with a run that acted, because the decisions
would be missing from the file that is supposed to explain them.

**D2 — A witness is a report plus the reference of the event that recorded it.**
A decision has to cite lines, and a report on its own names none: two observers with identical timings would be
indistinguishable in the file. The reference is what makes the evidence checkable after the run.

**D3 — The review is armed from the report's own instant, not from the clock.**
`ReportFailed` schedules the review at `SuspectedAt + Patience`. Patience therefore measures how long the system
waits for corroboration, not how long the loop took to wake up, and the canonical review at 620 ms is exactly 50 ms
after A's suspicion at 570 ms. Anchoring to the current instant would have made the delay depend on scheduling
order — the one thing a deterministic simulator must not do.

**D4 — A case closes exactly once, and later evidence opens a new one.**
Reports arriving after a review cannot join the decision it already made, because a decision supported by events
later in the file than itself is not a decision a reader can check. The test for this uses a second observer with a
500 ms timeout: A reports at 570 ms, the gate refuses at 620 ms on one witness, C reports at 725 ms, and a second
refusal is written at 775 ms. *Rejected:* waiting for a threshold that may never be reached, which would leave a run
with no decision in it at all.

**D5 — A refusal is an outcome of the gate, not an error path.**
Both outcomes travel the same code: witness validation, canonical ordering, a written event, a decision in the
result. An error is reserved for a decision the history cannot hold — no witnesses, mixed targets, one observer
voting twice. Those are faults in the caller, and a run that produced one would be a run whose log was incomplete.

**D6 — Verification is a query, not an event.**
`verify.Action` walks proposal, start and restart by identifier and causality. Nothing is written when it
succeeds. *Rejected:* a `recovery_verified` event emitted by the component that performed the action, which would
put the actor's word in the log as if it were a finding.

**D7 — The restart resumes the round counter and re-arms the timer from the restart instant.**
A node that counted from one again would send a round number twice, and an observer comparing rounds would read the
second as a reordering rather than as a second life. A timer resumed from the interrupted schedule would backfill
heartbeats the node never sent while it was down. The first resumed send cites `node_restarted`, so the new life is
attached to its reason.

**D8 — The detector is given a sink rather than read by the loop.**
The loop never asks the detector what it has found; it is told, through a one-method interface declared where it is
consumed. This keeps the detector's knowledge at the domain, and it keeps the decision out of hindsight: acting on
detections collected after the run would require the loop to decide at an instant that has already passed.

**D9 — The identifier belongs to the decider, and the simulator checks it.**
`ActionID` is assigned by the component that grants the action, once, and never reused. The simulator refuses a
restart whose identity it cannot match to a crashed node, a missing cause, a past instant, or a kind with no path
through it. Verification then asks "was action 1 carried out", which only has an answer if the name means one
decision for the whole run.

### What changed while building this

Three things moved after the design settled, and all three are worth recording.

**The proof query first matched by kind, not by edge.** `verify.Action` looked for a `recovery_started` with the
right action identifier and a `node_restarted` that followed it. A test with a restart that named the action but
did not cite it caught the difference: two events can share an identifier and an instant without one causing the
other, and the whole point of the query is to read causation. It now requires `started` to cite `proposed` and
`applied` to cite `started`, and a restart that merely sits next to its action is reported as unproven.

**One observer, one vote was a test before it was a rule.** The gate initially counted witnesses, which let a
single node report twice and clear a two-witness bar. The test that reports the same observer twice failed, and the
rule moved into the gate as `ErrRepeatedObserver` rather than into the loop as bookkeeping.

**The refusal world was rebuilt while the document was being written.** Its first version left C in the cluster
with no observer, so B heartbeated a node that recorded nothing: two sends with no outcome, which the log cannot
distinguish from two messages still on the wire. The scenario now has two nodes and one link. The structural graph
test and the one-outcome invariant also gained both recovery worlds, because until then a recovery log could have
cited a report that was never written and nothing would have noticed.

## 8. Failure modes, and what stops them

| failure | what it would look like | what stops it |
| --- | --- | --- |
| acting on ground truth | a decision that cites the crash, so the system looks right for a reason it could not have had | the gate only ever receives reports and their references; a test asserts the proposal cites the two suspicions and nothing else |
| restarting a node that never crashed | an action granted from a stale report, applied to a running node | `Engine.Restart` returns `ErrNotCrashed`; the state machine rejects running -> running |
| a decision supported by later events | a reader who finds the reason for a decision further down the file | the proposal is recorded before the restart is scheduled, and a case closes before later evidence can join it |
| a refusal nobody can justify | a decision to do nothing, with no evidence and no reason | `ErrNoWitnesses` and `Decision.Validate`; a refusal with no reason cannot be written |
| one paranoid node acting alone | one observer's silence treated as a majority | `ErrRepeatedObserver` and `MinWitnesses`; the `lone-witness` fixture pins the refusal |
| evidence about two different nodes | one node's report justifying action on another | `ErrMixedTargets`, checked before anything is written |
| an action with no proof | a table claiming a restart that the log does not connect | `verify.Action` returns `ok = false`, and the table prints `no complete chain` |
| a belief cleared by the repair instead of by evidence | trust returning at 720 ms, when the node came back | the detector clears only on a receipt; a test asserts the clearance is at 840 ms and after the restart |
| a restarted node repeating a round | two messages with one identity, and an observer reading the second as a reordering | the round counter resumes; the one-outcome invariant now runs on `recovered-crash` |
| recovery sneaking into a scenario that did not ask | a policy applied to a world that states no threshold | a test asserts no recovery event appears without a `RecoverySpec` |
| a fixture regenerated over a regression | a committed log hiding a changed timeline behind a legitimate new event | the fixtures pin bytes, and separate tests assert the instants (570, 575, 620, 720, 840) from the run itself |
| a graph that cannot explain a decision | a recovery log whose proposal cites an event that does not exist | the rooted-graph test now runs both recovery worlds over three seeds |

## 9. Known limitations

**One action, one direction.** The only thing recovery can do is restart a crashed node. There is no failover, no
drain, no rollback, and no way to act on a node that is running but suspected — which is exactly the situation the
partition milestone produces. A policy in that world would either do nothing or restart a healthy node, and the
project does not yet have a vocabulary for saying which.

**Nothing stops a crash loop.** A node that crashes again after restart produces a new report, a new case, and a
new grant. The policy has no memory of what it has already done and no backoff; a world that crashed repeatedly
would restart repeatedly, and the log would be a faithful record of a bad decision repeated.

**The gate counts observers, not truth.** Two observers behind the same cut both stop hearing the same node and both
are right about what they heard. The action is justified by the evidence, which is all a decision can ever be
justified by — but the log should not be read as a claim that the restart was necessary.

**The evidence is not checked for staleness.** A report produced long ago and delivered late still counts at its own
instant, and patience is measured from `SuspectedAt`, so a report can arrive after its review was already due. The
loop decides at that deadline with whatever it has, which is the honest behaviour but not a sophisticated one.

**Success is not verified, only occurrence.** Verification proves the proposal, the start and the restart exist in
one chain. It does not prove the node is healthy afterwards, that its peers agree, or that the action fixed
anything. The `recovered-crash` run ends with everybody trusting B, but that is a consequence of the heartbeat
schedule, not a check the system performs.

**A refusal is final for its case.** The only thing that reopens a question is new evidence. A node refused at
620 ms is not reconsidered at 1200 ms on the same evidence, even though the world may have changed in between.

## 10. How to read a decision

```
$ go run ./cmd/aegis run --scenario recovered-crash --format table
...
DECISION  AT     OUTCOME        EVIDENCE  PROOF
action 1  620ms  granted for B  14 15     proposed 16, started 17, applied 18
```

- **`EVIDENCE` lists references, not observers.** `14 15` are the two suspicions the decision weighed. They are
  sorted, so two renderings of one run cannot differ, and they are the proposal's causes, so `explain -ref 16`
  prints exactly the branches that produced them.
- **`PROOF` is a walk, not a claim.** The column is filled from `verify.Action` over the finished log: the start
  cites the proposal, the restart cites the start. A granted action whose chain is broken prints
  `no complete chain`, and the table does not pretend otherwise.
- **A refusal has no proof, and needs none.** `lone-witness` prints `refused (insufficient_witnesses)` with one
  reference and `no complete chain`, because refusing is not carrying something out.
- **The decision comes before the world change in the file.** A restart above its own reason is a defect; the file
  is written so that a reader following the sequence never sees a consequence before its cause.

To check a decision by hand, explain the proposal and then the restart:

```
$ go run ./cmd/aegis explain -scenario recovered-crash -ref 16
$ go run ./cmd/aegis explain -scenario recovered-crash -ref 18
```

The first prints the two reports and, under them, the receipts those reports rest on. The second prints the
proposal, then the reports, then the receipts — the whole argument for why a node that never told anybody it was
alive came back.

