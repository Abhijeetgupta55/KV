# raftkv

A distributed key-value store built from first principles in Go: gRPC API,
hand-rolled write-ahead log, Raft consensus implemented from the paper (no
consensus libraries), and consistent-hash sharding across multiple Raft
groups.

This is backend infrastructure — there is no web UI. Clients talk to the
cluster over gRPC (`PUT` / `GET` / `DELETE`) via a CLI or any generated
client. The project's focus is **correctness under failure**: crash
recovery, leader failover, and network partitions, verified by a
fault-injection test harness.

## Status

| Milestone | State |
|---|---|
| 0 — Single-node KV over gRPC (server, CLI, tests, CI) | ✅ done |
| 1 — Durability: WAL + snapshots + crash recovery | ✅ done |
| 2 — Raft consensus: election, replication, failover | next |
| 3 — Production Raft: compaction, membership changes | planned |
| 4 — Linearizable reads (ReadIndex) | planned |
| 5 — Sharding: multiple Raft groups | planned |
| 6 — Fault injection + linearizability checking | planned |
| 7 — Metrics, admin CLI, benchmarks | planned |

The store is currently **single-node but durable**: every acknowledged
write is fsynced to a write-ahead log before the client hears "OK", state
is periodically snapshotted, and recovery replays the log tail — proven by
an integration test that hard-kills the server mid-write-storm and audits
every acknowledged write after restart. Replication is Milestone 2's job.
Full roadmap: [docs/ROADMAP.md](docs/ROADMAP.md).

## Target architecture

```
        clients (CLI / generated stubs)
                 │  gRPC: Put / Get / Delete
                 ▼
        ┌─────────────────────────────┐
        │   Router / shard resolver   │  consistent hashing → shard
        └─────────────────────────────┘
                 │
     ┌───────────┼───────────────────┐
     ▼           ▼                   ▼
 ┌────────┐  ┌────────┐          ┌────────┐
 │Shard A │  │Shard B │   ...    │Shard N │   each shard = one Raft
 │(Raft   │  │(Raft   │          │(Raft   │   group of 3+ replicas
 │ group) │  │ group) │          │ group) │
 └────────┘  └────────┘          └────────┘
     │  within a shard:
     ▼
 leader ──replicates log──▶ followers
     │ apply committed entries
     ▼
 KV state machine → WAL + snapshots on disk
```

Design rationale, trade-offs, and known limitations live in
[docs/DESIGN.md](docs/DESIGN.md); individual decisions are recorded in
[docs/DECISIONS/](docs/DECISIONS/).

## Quickstart

Requires Go 1.26+. (`make proto` additionally needs `protoc` with the Go
plugins, but generated code is committed, so regular builds don't.)

```sh
make build          # builds bin/kvserver and bin/kvcli
make test           # unit + integration tests
make race           # same, under the race detector

bin/kvserver                          # serves on 127.0.0.1:5001, data in ./data
bin/kvcli put greeting "hello"        # → OK
bin/kvcli get greeting                # → hello
bin/kvcli delete greeting             # → deleted
bin/kvcli get greeting                # → exit code 1, "key not found"
```

On Windows, run `make` from Git Bash.

## Command reference

```
kvcli [-addr host:port] put <key> <value>    store value under key
kvcli [-addr host:port] get <key>            print value; exit 1 if absent
kvcli [-addr host:port] delete <key>         remove key (idempotent)

kvserver [-listen host:port]                 run a node (default 127.0.0.1:5001)
         [-data-dir dir]                     WAL + snapshot directory (default ./data)
         [-snapshot-threshold-bytes n]       WAL size that triggers a snapshot
```

Keys are UTF-8 strings up to 4 KiB; values are opaque bytes up to 1 MiB.

## Repository layout

```
proto/       gRPC schema + committed generated code
cmd/server   node entrypoint
cmd/cli      kvcli client
internal/
  storage/   state machine + WAL + snapshots + crash recovery
  server/    gRPC service: validation + response mapping
test/crash   crash-recovery integration test (hard-kills a real server)
docs/        DESIGN.md, ROADMAP.md, and ADRs
```

## Testing

`go test -race ./...` runs everything CI runs: unit tests for the storage
engine (including a test that cuts a write-ahead log at every byte offset
of its final record to prove torn-write recovery), service-layer tests
over a real durable store, an end-to-end test over an in-memory gRPC
connection, and a crash test that repeatedly kill-9s a real server process
mid-write and audits every acknowledged write after restart. CI (GitHub
Actions) enforces gofmt, `go vet`, the build, and the race-detector test
run on every push.
