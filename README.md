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

## Status

Milestones 1 and 2 are complete. See [doc_m1.md](doc_m1.md) and [doc_m2.md](doc_m2.md) for every engineering
decision, the risks considered, the drift between plan and code, and the verification evidence behind each claim.

Deliberately absent so far: jitter, correlated loss, causal history, recovery, diagnosis, scenario files, and any
real concurrency inside the simulation. A recovery loop that guesses would be worse than no recovery loop.

## Requirements

- Go 1.25 or newer
- `make` (optional; every target is a plain `go` command)

## Quick start

```bash
make check                                                      # gofmt, vet, build, tests, race detector
make replay                                                     # every scenario twice, byte-for-byte
make demo                                                       # human-readable event log

go run ./cmd/aegis run --scenario node-crash --format jsonl      # canonical machine-readable log
go run ./cmd/aegis run --scenario one-way-partition --format table
go run ./cmd/aegis run --scenario node-crash --seed 7            # seed is an input, not a global
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
internal/scenario       scenario specifications, golden fixtures, the runner
docs/                   the milestone plans
```

The dependency rule is one-way: `cmd` and `scenario` may know every component; the simulator knows the domain; the
detector knows only the domain, so it has no way to read ground truth.

## Licence

MIT. See [LICENSE](LICENSE).
