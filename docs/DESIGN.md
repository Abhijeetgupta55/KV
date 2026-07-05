# raftkv design

This document records the system's requirements, the position it takes on
the classic distributed-systems trade-offs, and the honest list of what it
does not yet do. It grows with each milestone; sections marked *(planned)*
describe committed design direction, not implemented behavior.

## Requirements

- Store key → value pairs; keys are UTF-8 strings ≤ 4 KiB, values are
  opaque bytes ≤ 1 MiB.
- Serve `Put` / `Get` / `Delete` over gRPC.
- **Durability** *(done, M1)*: an acknowledged write survives a process
  crash — fsynced write-ahead log, snapshots, verified by a
  kill-the-process test.
- **Fault tolerance** *(planned, M2–M3)*: a shard of N replicas keeps
  serving writes while a minority ⌊(N−1)/2⌋ of its nodes are down, with no
  acknowledged write lost and no split-brain.
- **Linearizable reads** *(planned, M4)*: reads that can never observe
  stale state, via ReadIndex or leader leases.
- **Horizontal scale** *(planned, M5)*: keys partition across shards;
  each shard is an independent Raft group.
- **Verified correctness** *(planned, M6)*: fault injection plus a
  linearizability checker over recorded histories.

### Non-goals

- No web UI, auth, or multi-tenancy — this is infrastructure, and every
  line of that kind of surface area would dilute the systems focus.
- No SQL, secondary indexes, or query language. It is a KV store.
- No raw-throughput contest with Redis. The engineering target is
  correctness under failure, measured by the fault-injection harness (M6),
  not by benchmark marketing.
- No third-party consensus library. Raft is implemented from the paper —
  that is the point of the project.

## Position on the CAP spectrum

raftkv chooses **CP**: linearizable writes through Raft, at the price of a
minority partition refusing writes (they cannot reach quorum). For a
system of record, refusing service beats silently diverging; AP-style
eventual consistency pushes conflict resolution onto every client, which
is the wrong default for a general KV store. Read consistency gets its own
treatment in M4 (ReadIndex), because naive leader reads are *not*
automatically linearizable — a deposed leader can serve stale data without
knowing it has been replaced.

## Current architecture (through Milestone 1)

Layers, each independently testable, with deliberately boring seams:

1. **State machine** (`internal/storage.MemStore`) — a mutex-guarded map.
   This is what Raft will later replicate, so it knows nothing about
   networking, persistence, or consensus. `Put`/`Get` copy value slices so
   no caller can alias the store's internal data.
2. **Storage engine** (`internal/storage.DurableStore`) — wraps the state
   machine with durability:
   - Every mutation is encoded as a self-contained binary *command*,
     appended to a **write-ahead log**, and **fsynced before the client
     sees an acknowledgment**. The in-memory apply happens only after the
     disk write succeeds, so an error always means "not stored", never
     "maybe stored".
   - Records carry strictly increasing sequence numbers and CRC32-C
     checksums. The sequence number becomes the Raft log index in M2.
   - **Snapshots** (checksummed full dumps, written tmp-then-rename for
     atomicity) bound WAL growth and recovery time; after a snapshot the
     WAL rotates and covered segments are deleted whole.
   - **Recovery** = newest snapshot + WAL tail replay. Replay and live
     writes funnel through the same `applyCommand`, so they cannot
     diverge. Format details and rationale: ADR 0003.
3. **Service** (`internal/server`) — implements the `kv.v1` gRPC service.
   Owns request validation (empty keys, size limits) and the mapping from
   storage results to wire responses; storage errors surface as gRPC
   `INTERNAL`.
4. **Entrypoints** (`cmd/server`, `cmd/cli`) — flag parsing, wiring, and
   graceful shutdown.

### The torn-write problem (why WAL recovery is subtle)

A crash can interrupt an append anywhere, leaving a half-written record at
the log's tail. Recovery must answer: is an unreadable record a torn write
(harmless — it was never acknowledged, because acknowledgment follows
fsync) or corruption of acknowledged data (must not be ignored)? The
policy: an invalid record at the tail of the **newest** segment is torn —
truncate and continue; an invalid record in any **finished** segment, or a
gap in sequence numbers, refuses startup. Within the newest segment,
corruption *before* the tail is indistinguishable from a torn write; that
ambiguity is a documented limitation, pinned by a test.

## Failure model

- **Crash-stop of the single node (tolerated, M1):** acknowledged writes
  survive any process death — kill -9 mid-write included — via
  fsync-before-ack and snapshot + WAL-tail recovery. Verified by an
  integration test that repeatedly hard-kills a real server process
  during a write storm (crossing snapshot and rotation boundaries) and
  audits every acknowledged write after restart.
- **Node loss / partitions (planned, M2):** Raft leader election with
  randomized timeouts, heartbeat failure detection, quorum commit;
  split-brain prevented by election safety (at most one leader per term).
- **Out of scope at every milestone:** Byzantine failures (nodes lying,
  disks returning plausible-but-wrong data — CRCs catch bit rot, not
  adversaries).

## Known limitations (through Milestone 1)

- **Single node.** No replication, no failover. Durability protects
  against process death, not disk death.
- **One fsync per write.** Throughput is bounded by disk sync latency
  (hundreds of writes/sec locally). Group commit — batching concurrent
  appends into one fsync — is the standard fix, deferred until after Raft
  lands to avoid tuning the same code twice.
- **Snapshots pause writes** for the duration of the dump (the write lock
  is held). Copy-on-write iteration is the known fix if state grows.
- **Newest-segment ambiguity**: corruption before the tail of the active
  WAL segment is treated as a torn write, silently dropping whatever
  followed it (see above).
- **Directory fsync is a no-op on Windows** (unsupported by the OS); file
  creations/renames rely on the NTFS metadata journal. On Linux — where
  CI runs and any real deployment would live — directories are fsynced.
- **Plaintext gRPC.** No TLS; the server binds to loopback by default and
  must not be exposed beyond a trusted network.
- **No backpressure or rate limiting.** A hostile client can fill memory
  with 1 MiB values; there is no eviction and no total-size cap.
- **CI cannot detect stale generated proto code** (accepted in ADR 0001).
