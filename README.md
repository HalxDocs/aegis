# Aegis

**Detect. Diagnose. Recover. Verify.**

Aegis is a platform for reasoning about failure in distributed systems. Every recovery action must be justified by
observable evidence, constrained by safety rules, and verified afterwards.

The *failure lab* is a deterministic discrete-event simulator. It models nodes on a virtual clock, exchanges
heartbeats over links with different latencies, and records what every layer knew and when.

Milestone 1 crashes one node and shows that the survivors only *suspect* it after their own heartbeat timeouts.
Milestone 2 cuts one direction of a link and shows the harder case: nobody is broken, and two observers of the same
node still disagree, because trust is made of evidence rather than topology.
Milestone 4 lets that evidence act: two independent observers are enough to restart a node, the decision is written
down before the world changes, and the node does not come back into anybody's trust until a message arrives.

The simulator owns ground truth. Observers never see it.

## What one run shows

```
$ go run ./cmd/aegis run --scenario node-crash --format table
```

| t | event |
| --- | --- |
| 0 ms | A, B, C start |
| 100 ms | B sends heartbeat round 1 to A and C |
| 120 ms / 125 ms | A / C receive it |
| 200 ms | B sends round 2 |
| **215 ms** | **B crashes** |
| 220 ms / 225 ms | round 2 still arrives — it was already on the wire |
| 570 ms / 575 ms | A / C stop trusting B |
| 1200 ms | horizon: B is crashed, both observers still suspect it |

Suspicion latency is 355 ms, not the configured 350 ms, because the last evidence arrived 5 ms after the crash.
That gap is the milestone: a fact and a belief about that fact are different things, and the log records both.

## What the partition run shows

```
$ go run ./cmd/aegis run --scenario one-way-partition --format table
```

| t | event |
| --- | --- |
| 0 ms | A, B, C start; A and B heartbeat each other, C watches B |
| **300 ms** | **the B->A direction is cut; the A->B direction keeps working** |
| 300–800 ms | A never hears B again; B keeps hearing A and C keeps hearing B |
| 570 ms | A stops trusting B |
| **900 ms** | **the path is repaired** |
| 920 ms | A trusts B again, when the first heartbeat after the repair arrives |
| 1150 ms | horizon: every node running, one false report, one observer wrong for 350 ms |

Nothing in this run is down. A one-way cut is the first failure that produces disagreement without a failure:
B is alive and knows it, C can see it, and A is right about everything it can observe.

## What the recovery run shows

```
$ go run ./cmd/aegis run --scenario recovered-crash --format table
```

| t | event |
| --- | --- |
| **215 ms** | **B crashes** |
| 570 ms / 575 ms | A / C stop trusting B |
| **620 ms** | **the gate reviews: two witnesses, threshold met, action 1 proposed** |
| **720 ms** | **action 1 carried out and B restarted** |
| 820 ms | B sends round 3 — the counter resumes where it stopped |
| 840 ms / 845 ms | A / C stop suspecting B, when the message arrives |
| 1200 ms | horizon: B running, both observers trust it |

```
DECISION  AT     OUTCOME        EVIDENCE  PROOF
action 1  620ms  granted for B  14 15     proposed 16, started 17, applied 18
```

The proposal cites the two suspicions; the start cites the proposal; the restart cites the start. `PROOF` is not a
claim by the run — it is the chain read back out of the finished log. The same policy refuses to act when only one
observer reports:

```
$ go run ./cmd/aegis run --scenario lone-witness --format table
DECISION  AT     OUTCOME                                 EVIDENCE  PROOF
action 1  620ms  refused (insufficient_witnesses) for B  12        no complete chain
```

A restart is not a recovery of trust. B is running again at 720 ms and A does not stop suspecting it until 840 ms,
because that is when the first message crosses the link. See [doc_m4.md](doc_m4.md) for the contract, the refusal
rules, and what the milestone cannot do.

## Why it happened

```
$ go run ./cmd/aegis explain -scenario node-crash -ref 14
14  570ms  suspicion_raised  B distrusted by A (heartbeat_timeout)
<- 12  220ms  heartbeat_received  B->A round 2
    <- 9  200ms  heartbeat_sent  B->A round 2
        <- 5  100ms  heartbeat_sent  B->A round 1
            <- 3  0s  node_started  B started
                <- 1  0s  scenario_started  node-crash
```

Every event carries the references of the events that caused it, so the log is a graph as well as a sequence. The
chain above is the whole argument of the project in six lines: A distrusted B because its last evidence arrived at
220 ms, and the evidence was a message B sent at 200 ms. The crash at 215 ms is in the log and in nobody's
reasoning, because the crash was never delivered to anybody.

```
$ go run ./cmd/aegis explain -scenario one-way-partition -ref 61
61  920ms  suspicion_cleared  B trusted again by A
<- 60  920ms  heartbeat_received  B->A round 9
    <- 57  900ms  heartbeat_sent  B->A round 9
```

The recovery cites the arrival at 920 ms, never the repair at 900 ms: a path coming back explains nothing until a
message crosses it. See [doc_m3.md](doc_m3.md) for the contract and its limits.

## Status

Milestones 1, 2, 3 and 4 are complete. See [doc_m1.md](doc_m1.md), [doc_m2.md](doc_m2.md), [doc_m3.md](doc_m3.md)
and [doc_m4.md](doc_m4.md) for every engineering decision, the risks considered, the drift between plan and code,
and the verification evidence behind each claim.

Deliberately absent so far: jitter, correlated loss, any action other than a restart, backoff or crash-loop
detection, diagnosis of *why* a node failed, counterfactual comparison, and any real concurrency inside the
simulation. A recovery loop that guesses would be worse than no recovery loop, a graph that claimed to
explain an absence would be worse than one that admits it cannot, and an action that certified its own success
would be worse than one that leaves the proof to a reader.

## Releases

Each milestone is tagged, and its release note is written for people reading the code: what the run shows, what moved
in the API, the one decision worth arguing about, and what is still not modelled. A guard in `make check` compares the
table below with the tags that exist and with the files the rows link, so a row claiming a release nobody published,
or pointing at a document that was never written, fails the build.

| release | what it covers |
| --- | --- |
| [v0.1.0](https://github.com/HalxDocs/aegis/releases/tag/v0.1.0) — Milestone 1: node crash | The foundation: virtual clock, deterministic scheduler, heartbeats over links with different latencies, observer-side timeouts, and the canonical 16-event `node-crash` run. See [doc_m1.md](doc_m1.md). |
| [v0.2.0](https://github.com/HalxDocs/aegis/releases/tag/v0.2.0) — Milestone 2: one-way partitions | A cut as a schedule on the wire rather than a flag: `Engine.PartitionAt`, the `link_partitioned` and `link_restored` events, and the `one-way-partition` scenario with its 75-event golden fixture. Nothing crashes, and a node is still distrusted for 350 ms. |
| [v0.3.0](https://github.com/HalxDocs/aegis/releases/tag/v0.3.0) — Milestone 3: causal history | Every event cites the events that caused it, so the log is also a graph: `Event.Causes`, `Recorder.Record` handing back the reference it assigned, and a new `aegis explain` command that walks either a live run or a JSONL history read from disk. A file edited after the fact is refused by the rule it broke, never explained. The crash explains no belief, and the repair explains nothing. See [doc_m3.md](doc_m3.md). |
| [v0.4.0](https://github.com/HalxDocs/aegis/releases/tag/v0.4.0) — Milestone 4: recovery that has to be proved | The first milestone that changes the world. A policy of two witnesses, 50 ms of patience and 100 ms of delay; a pure gate that writes a proposal or a refusal before anything happens; `Engine.Restart` resuming a node at the round it stopped on; and `internal/verify`, a query that reads proposal, start and restart back out of the log because nothing in a run may certify itself. `recovered-crash` brings B back at 720 ms and A does not trust it until 840 ms; `lone-witness` refuses, with a reason, and leaves it down. See [doc_m4.md](doc_m4.md). |

## Requirements

- Go 1.25 or newer
- `make` (optional; every target is a plain `go` command)
- Node.js and npm, only for `make check-all`, which also checks the explainer page if it is checked out beside this
  repository (`LAB=...` points the target elsewhere)

## Quick start

```bash
make check                                                      # gofmt, vet, build, tests, race detector
make check-all                                                  # the above, then the page's own proof (needs ../aegis-lab)
make replay                                                     # every scenario twice, byte-for-byte
make demo                                                       # human-readable event log
make explain                                                    # why A distrusted B, and what it rested on
go run ./cmd/aegis run --scenario recovered-crash --format table  # a decision, and the proof it happened
go run ./cmd/aegis run --scenario lone-witness --format table     # the same policy declining to act

go run ./cmd/aegis run --scenario node-crash --format jsonl      # canonical machine-readable log
go run ./cmd/aegis run --scenario one-way-partition --format table
go run ./cmd/aegis run --scenario node-crash --seed 7            # seed is an input, not a global
go run ./cmd/aegis explain -scenario one-way-partition -ref 61   # the reasons behind one event
go run ./cmd/aegis run --format jsonl > run.jsonl               # also: explain a log from disk
go run ./cmd/aegis explain -log run.jsonl -ref 14
go run ./cmd/aegis scenarios                                   # what can run, one name per line
go run ./cmd/aegis version                                     # the build, and the format it reads
go run ./cmd/aegis run --file world.json --format table         # a scenario written as a file
```

The JSONL log is the canonical artifact: one event per line, virtual timestamps only, sequence numbers assigned by
the recorder. The table view is rendered from those events and is never a second source of truth.

The first line of a log says what the file is: `"schema": "aegis.eventlog/v1"` on the event that opens the history,
and on no other event. A version repeated on every line could disagree with itself, and a reader would have no rule
for which line wins. `aegis version` prints the same string beside the build that writes it, and a log naming any
other format is refused outright rather than decoded with the wrong rules.

`make hooks` points `core.hooksPath` at `hooks/`, where a pre-push hook runs `make check-all` (or `make check` if
the page is not checked out) before anything leaves the machine. Branch protection cannot do this: requiring status
checks rejects a direct push to `main` with `GH006`, because the checks only exist after the push they reject.
`git push --no-verify` skips it deliberately.

`aegis scenarios` is that same idea applied to the list itself: it prints the scenarios this binary can run, so a
tool that needs to know — the explainer page's parity check, a script, a person — asks the binary instead of
parsing `main.go`. It is an interface, not decoration; adding a scenario means it shows up there too.

## Scenario files

A scenario can be a file instead of a name in the binary. It is the same world either way: the format expresses every
setting the four built-in scenarios use, and a file describing `recovered-crash` produces that scenario's log byte for
byte.

A whole world is small enough to read:

```json
{
  "name": "watched-heartbeat",
  "horizon": "400ms",
  "nodes": [
    {"id": "A"},
    {"id": "B", "heartbeat_every": "100ms", "peers": ["A"]}
  ],
  "links": [{"from": "B", "to": "A", "latency": "20ms"}],
  "observers": [{"observer": "A", "target": "B", "timeout": "350ms"}]
}
```

[docs/scenario-example.json](docs/scenario-example.json) is the full case: `recovered-crash` written out as a file,
crash, policy and all. It is not a copy that can drift — a test loads it, runs it, and compares the log with the
built-in scenario's.

```
$ go run ./cmd/aegis run --file docs/scenario-example.json --format table
```

Times are Go duration strings, because a scenario is read by people first. Every key is required to be a key the
format knows: a misspelled `observor` is an error, not an empty list, since a file that quietly observes nothing is
worse than one that says why it cannot run. `crash`, `partitions` and `recovery` may be omitted; `-file` and
`-scenario` cannot both be passed, because they answer the same question and there is no precedence to invent.

Reading from files does not make the built-in scenarios redundant. They are the fixtures the page's parity check and
the committed logs in `internal/scenario/testdata` are pinned to, and a test writes each of them out in this format,
reads it back and compares the two logs — so the format cannot forget a field the simulator accepts.

## Using it from Go

`github.com/HalxDocs/aegis` is an importable package, and it is deliberately narrow: the event vocabulary, the history
read as a graph of reasons, the gate that weighs evidence, and the verifier that reads a proof back out of a finished
log. It is not the simulator. Running a scenario is the command's job; the exported parts are the ones that can be
trusted without knowing what really happened.

```go
events, err := aegis.ReadLog(file)      // refuses a log naming another schema
idx, err := aegis.NewIndex(events)     // the same events, as a graph
proofs, err := aegis.VerifyActions(idx) // which actions the log actually proves

gate, err := aegis.NewGate(aegis.Policy{
	Kind:         aegis.ActionRestartNode,
	MinWitnesses: 2,
	Patience:     aegis.TickFrom(50 * time.Millisecond),
	Delay:        aegis.TickFrom(100 * time.Millisecond),
})
decision, err := gate.Decide(witnesses) // granted, or refused with a reason
```

Every exported name is an alias of the type that implements it, so there is no second definition to drift. What is
behind it — `domain`, `sim`, `detector`, `scenario`, `causal`, `recovery`, `verify` — stays `internal`, because those
shapes still move with each milestone and a promise not to move them would be a promise the project cannot keep.

`examples/consumer` is a separate module that imports the package, and `make api-check` runs it. It has to be a
separate module to be a proof: code inside this module can reach `internal` whatever the public package claims, while
code outside it cannot, so the target fails to build the moment an exported name is only reachable from inside.

## Layout

```
aegis.go                the importable surface: aliases, ReadLog, the gate, the verifier
cmd/aegis               command-line entry point and table rendering
examples/consumer       a separate module proving the surface is importable from outside
internal/domain         core vocabulary: virtual time, ids, node and belief states, events
internal/sim            virtual clock, deterministic scheduler, network, nodes, engine
internal/detector       observer-side suspicion from heartbeat timeouts
internal/observability  append-only event history and JSONL encoding
internal/causal         the history read as a graph of reasons
internal/recovery       the policy, the witnesses, and the gate that decides between them
internal/verify         the proof that a granted action was carried out, read from the log
internal/scenario       scenario specifications, the file format, golden fixtures, the runner
docs/                   the milestone plan and the five-minute talk
```

A spoken walkthrough of both runs, built to be delivered in five minutes, is in
[docs/talk-five-minutes.md](docs/talk-five-minutes.md).

The live explainer page is its own repository, [HalxDocs/aegis-lab](https://github.com/HalxDocs/aegis-lab).
`make check-all` runs it from a checkout beside this one; CI is the only place the two are checked out together, as a
subdirectory of this workspace, and it runs the same command with `LAB` and `AEGIS_GO_REPO` set to say where each side
is.

The dependency rule is one-way: `cmd` and `scenario` may know every component; the simulator knows the domain; the
detector knows only the domain, so it has no way to read ground truth; the gate knows the domain and the reports
handed to it, so it cannot ask the simulator anything; the causal explainer and the verifier know only the events,
so they have no way to ask what really happened.

## Licence

MIT. See [LICENSE](LICENSE).
