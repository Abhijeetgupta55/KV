# ADR 0003: WAL and snapshot on-disk format

**Status:** accepted (2026-07-05)

## Context

Milestone 1 makes writes durable. The on-disk format is the hardest
thing in the system to change later (it must be readable forever), so
it gets an ADR.

## Decision

**WAL**: segment files of records framed as
`uint32 payloadLen | uint32 crc32c(payload) | payload`, where the
payload is `uint64 seq` followed by a self-describing command encoding
(op, key, value). Hand-rolled binary, little-endian, no protobuf.

**Snapshots**: full checksummed dumps written tmp-file-then-rename,
named by the last sequence number they cover. After a snapshot the WAL
rotates to a fresh segment and covered segments are deleted whole.

**Torn-write policy**: the first invalid record in the *newest* segment
ends the log (truncated — it can only be an unacknowledged half-write,
because appends fsync before acking); an invalid record in a *finished*
segment, or any sequence gap, refuses startup.

## Rationale

- **Hand-rolled over protobuf**: the framing (lengths, checksums,
  torn-write semantics) is the systems substance this milestone exists
  to build; protobuf would outsource exactly the interesting part. The
  storage layer also stays free of generated-code dependencies.
- **CRC32-C (Castagnoli)**: hardware-accelerated on modern CPUs and the
  checksum used by ext4, iSCSI, and most storage systems.
- **Sequence numbers in every record**: gives replay a corruption check
  that checksums alone can't provide (a missing-but-valid record), and
  maps directly onto the Raft log index in Milestone 2, so the format
  survives the consensus milestone unchanged.
- **Whole-segment deletion over in-file truncation**: deleting a file
  is atomic on every filesystem; punching bytes out of a live log is
  not.

## Consequences

- Every write costs an fsync; throughput is bounded by disk sync
  latency until group commit is added (documented known limitation).
- Within the newest segment, real corruption before the tail is
  indistinguishable from a torn write, so replay stops there and
  discards whatever follows. Detecting this case would require a
  segment-level footer; accepted for now and pinned by a test.
- Snapshots are written under the write lock, pausing writes for the
  duration of the dump. Fine at current scale; copy-on-write iteration
  is the known fix if it ever isn't.
