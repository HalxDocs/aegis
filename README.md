# Aegis

**Detect. Diagnose. Recover. Verify.**

Aegis is a platform for reasoning about failure in distributed systems. Every recovery action must be justified by
observable evidence, constrained by safety rules, and verified afterwards.

The *failure lab* is a deterministic discrete-event simulator. It models nodes on a virtual clock, exchanges
heartbeats over links with different latencies, and records what every layer knew and when.

Milestone 1 crashes one node and shows that the survivors only *suspect* it after their own heartbeat timeouts.
Milestone 2 cuts one direction of a link and shows the harder case: nobody is broken, and two observers of the same
node still disagree, because trust is made of evidence rather than topology.

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

Milestones 1, 2 and 3 are complete. See [doc_m1.md](doc_m1.md), [doc_m2.md](doc_m2.md) and [doc_m3.md](doc_m3.md)
for every engineering decision, the risks considered, the drift between plan and code, and the verification
evidence behind each claim.

Deliberately absent so far: jitter, correlated loss, recovery, diagnosis, scenario files, counterfactual
comparison, and any real concurrency inside the simulation. A recovery loop that guesses would be worse than no
recovery loop, and a graph that claimed to explain an absence would be worse than one that admits it cannot.

## Releases

A milestone is tagged once it has a written release note; until then it is recorded in its design document and the
row below says so. The notes are aimed at people reading the code: what the run shows, what moved in the API, the one
decision worth arguing about, and what is still not modelled.

| release | what it covers |
| --- | --- |
| Milestone 1 — node crash (untagged) | The foundation: virtual clock, deterministic scheduler, heartbeats over links with different latencies, observer-side timeouts, and the canonical 16-event `node-crash` run. See [doc_m1.md](doc_m1.md). |
| [v0.2.0](https://github.com/HalxDocs/aegis/releases/tag/v0.2.0) — Milestone 2: one-way partitions | A cut as a schedule on the wire rather than a flag: `Engine.PartitionAt`, the `link_partitioned` and `link_restored` events, and the `one-way-partition` scenario with its 75-event golden fixture. Nothing crashes, and a node is still distrusted for 350 ms. |
| [v0.3.0](https://github.com/HalxDocs/aegis/releases/tag/v0.3.0) — Milestone 3: causal history | Every event cites the events that caused it, so the log is also a graph: `Event.Causes`, `Recorder.Record` handing back the reference it assigned, and a new `aegis explain` command that walks either a live run or a JSONL history read from disk. A file edited after the fact is refused by the rule it broke, never explained. The crash explains no belief, and the repair explains nothing. See [doc_m3.md](doc_m3.md). |

## Requirements

- Go 1.25 or newer
- `make` (optional; every target is a plain `go` command)

## Quick start

```bash
make check                                                      # gofmt, vet, build, tests, race detector
make replay                                                     # every scenario twice, byte-for-byte
make demo                                                       # human-readable event log
make explain                                                    # why A distrusted B, and what it rested on

go run ./cmd/aegis run --scenario node-crash --format jsonl      # canonical machine-readable log
go run ./cmd/aegis run --scenario one-way-partition --format table
go run ./cmd/aegis run --scenario node-crash --seed 7            # seed is an input, not a global
go run ./cmd/aegis explain -scenario one-way-partition -ref 61   # the reasons behind one event
go run ./cmd/aegis run --format jsonl > run.jsonl               # also: explain a log from disk
go run ./cmd/aegis explain -log run.jsonl -ref 14
```

The JSONL log is the canonical artifact: one event per line, virtual timestamps only, sequence numbers assigned by
the recorder. The table view is rendered from those events and is never a second source of truth.

## Layout

```
cmd/aegis               command-line entry point and table rendering
internal/domain         core vocabulary: virtual time, ids, node and belief states, events
internal/sim            virtual clock, deterministic scheduler, network, nodes, engine
internal/detector       observer-side suspicion from heartbeat timeouts
internal/observability  append-only event history and JSONL encoding
internal/causal         the history read as a graph of reasons
internal/scenario       scenario specifications, golden fixtures, the runner
docs/                   the milestone plan and the five-minute talk
```

A spoken walkthrough of both runs, built to be delivered in five minutes, is in
[docs/talk-five-minutes.md](docs/talk-five-minutes.md).

The dependency rule is one-way: `cmd` and `scenario` may know every component; the simulator knows the domain; the
detector knows only the domain, so it has no way to read ground truth, and the causal explainer knows only the
events, so it has no way to ask what really happened.

## Licence

MIT. See [LICENSE](LICENSE).
