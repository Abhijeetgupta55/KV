# Roadmap

Milestones build strictly in order; each must be fully tested and
documented before the next begins. Details of what shipped live in
[DESIGN.md](DESIGN.md) and the ADRs.

| # | Milestone | Delivers | Status |
|---|-----------|----------|--------|
| 0 | Foundation | Single-node in-memory KV over gRPC; CLI; CI | ✅ done |
| 1 | Durability | WAL (fsync-before-ack, CRC, torn-write recovery), snapshots, crash recovery proven by a kill-the-process test | ✅ done |
| 2 | Raft core | Leader election, log replication, commit rules; one state machine replicated across 3+ nodes, correct under leader failure | next |
| 3 | Production Raft | Log compaction + InstallSnapshot, live membership changes, pre-vote | planned |
| 4 | Linearizable reads | ReadIndex (or leases); demonstrate the naive stale-read violation, then fix it | planned |
| 5 | Sharding | Multiple independent Raft groups, key partitioning, request routing | planned |
| 6 | Correctness verification | Fault injector (crashes, pauses, partitions) + linearizability checker over recorded histories, in CI | planned |
| 7 | Operability | Prometheus metrics, structured logging, admin CLI, honest benchmarks | planned |

Possible extensions beyond the roadmap (deliberately not commitments):
cross-shard transactions, watch/subscribe, MVCC point-in-time reads, TTLs.
