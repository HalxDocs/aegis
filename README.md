# Aegis

**Detect. Diagnose. Recover. Verify.**

Aegis is a platform for reasoning about failure in distributed systems. Every recovery action must be justified by
observable evidence, constrained by safety rules, and verified afterwards.

Milestone 1 — the *failure lab* — is a deterministic discrete-event simulator. It models three nodes on a virtual
clock, exchanges heartbeats over a networked message path, crashes one node, and shows that the surviving observers
only *suspect* the failure after their own heartbeat timeouts. It does not recover anything yet.

The simulator owns ground truth. Observers never see it.

## Status

Milestone 1 is under construction. See [doc_m1.md](doc_m1.md) for the engineering decisions, the failure modes that
were considered, and the verification evidence.

## Requirements

- Go 1.25 or newer
- `make` (optional; every target is a plain `go` command)

## Quick start

```bash
make check                                                      # gofmt, vet, build, tests, race detector
go run ./cmd/aegis run --scenario node-crash --format table      # human-readable event log
go run ./cmd/aegis run --scenario node-crash --format jsonl      # canonical machine-readable log
```

## Layout

```
cmd/aegis           command-line entry point
internal/domain     core vocabulary: virtual time, ids, node and belief states, events
internal/sim        virtual clock, deterministic scheduler, network, nodes, engine
internal/detector   observer-side failure detection from heartbeat timeouts
internal/observability  append-only event history and JSONL encoding
internal/scenario   scenario specifications, the canonical crash fixture, the runner
internal/cli        command-line parsing and rendering
docs/               architecture notes and the milestone plan
```

## Licence

MIT. See [LICENSE](LICENSE).
