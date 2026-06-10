# Clean up RE-MVP-ORAM populatePath

This ExecPlan is a living document. It follows `.agents/PLANS.md` and must be updated as the cleanup proceeds.

## Purpose / Big Picture

The user rewrote `RE-MVP-ORAM/re_mvp_oram.go` function `populatePath` and wants obvious implementation mistakes fixed without changing algorithmic choices that may be intentional. After this work, the repository should compile, the updated `populatePath` should have comments that divide the long function into clear processing phases, and short `normal` runs should not fail immediately from list/map/index mistakes.

## Progress

- [x] (2026-06-09 Asia/Tokyo) Read `.agents/PLANS.md`, current diff, and the rewritten `populatePath`.
- [x] (2026-06-09 Asia/Tokyo) Identified obvious implementation mistakes: iterating over map values as addresses, comparing a signature map key to a slot position, and indexing `prioritylist` without checking the index.
- [x] (2026-06-09 Asia/Tokyo) Fixed compile-time and obvious runtime list/map/index errors in `RE-MVP-ORAM/re_mvp_oram.go`.
- [x] (2026-06-09 Asia/Tokyo) Added concise phase comments inside `populatePath`.
- [x] (2026-06-09 Asia/Tokyo) Ran `GOCACHE=/tmp/go-build-cache GO111MODULE=off go test ./RE-MVP-ORAM`; it passed.
- [x] (2026-06-09 Asia/Tokyo) Ran short `normal` executions with one and ten clients; no panic appeared after the fixes.

## Surprises & Discoveries

- Observation: The rewritten `populatePath` has a loop `for _, addr := range evaluationResult`, but `evaluationResult` is `map[int]int`; the blank identifier receives the key and `addr` receives the score value. This is an implementation mistake because later code indexes `virtualPositionMap[addr]`.
  Evidence: `evaluationPathpattern` returns `map[int]int`, and the loop then uses `virtualPositionMap[addr]`.
- Observation: The loop over `c.PositionMap[addr]` names the map key `slot` and compares it to `Slot.slot`; the key is actually a signature.
  Evidence: `PositionMap` is `map[int]map[int]MvpPositionMapEntry`, where the inner key is sig.
- Observation: The rewritten function stored target and priority blocks with `PositionMap` timestamp `S=c.seq` but sometimes left the physical block `Version.S` unchanged or omitted a stash position update.
  Evidence: Short normal runs failed with `Not target block in working set`; `mergePathStashes` requires exact `sameDataVersion`, including `S`.
- Observation: After cleanup, short normal runs no longer panic, but stash can still grow later in multi-client runs.
  Evidence: A 10-client short run reached `seq=400` without panic and logged `stash_out=209`, `stash_total=1210`.

## Decision Log

- Decision: Treat obvious type/collection misuse as fixable without asking the user.
  Rationale: The user explicitly asked to fix list/map implementation mistakes.
  Date/Author: 2026-06-09 / Codex.
- Decision: Avoid changing high-level algorithmic choices such as the new priority/drain split unless needed for compilation or clear index safety.
  Rationale: The user asked to ask before changing behavior when intent is unclear.
  Date/Author: 2026-06-09 / Codex.
- Decision: Update physical block `Version.S` whenever the function emits a position-map update with `S=c.seq`.
  Rationale: The merge path requires physical block versions and position-map timestamps to match exactly.
  Date/Author: 2026-06-09 / Codex.
- Decision: Leave the `c.Z-1` per-bucket placement loop unchanged.
  Rationale: It may be an intentional reservation of one slot per bucket; changing it would alter algorithm behavior and should be confirmed by the user.
  Date/Author: 2026-06-09 / Codex.

## Outcomes & Retrospective

The cleanup is complete for obvious implementation issues. `RE-MVP-ORAM` compiles, unused old populate helpers were removed, `populatePath` has phase comments, and short normal runs no longer show target-miss panics. A remaining algorithmic question is whether placing only `c.Z-1` slots per bucket is intentional.

## Context and Orientation

The relevant file is `RE-MVP-ORAM/re_mvp_oram.go`. `populatePath` builds three outputs for an ORAM client eviction: `populatedPath`, a map from physical path slots to blocks; `populatedStash`, blocks left in stash; and `populatedPathMap`, location updates for the client's position map. In this repository, a block address can have multiple signatures, represented as the inner key of `PositionMap[addr][sig]`.

## Plan of Work

First, fix compile-time and obvious runtime bugs in the rewritten `populatePath`: map key/value confusion, signature-vs-slot confusion, and slices accessed without bounds checks. Second, add short comments before each phase of the function: setup, target handling, priority/drain split, priority placement, drain copy placement, drain deletion, and return. Third, run the Go tests and a short normal run to verify the cleanup.

## Concrete Steps

Work from `/mnt/c/Users/gento/Desktop/TERM2026`.

Run:

    GOCACHE=/tmp/go-build-cache GO111MODULE=off go test ./RE-MVP-ORAM

Then run:

    timeout 10s env RE_MVP_CLIENT_COUNT=1 RE_MVP_ACCESS_LOG=0 GOCACHE=/tmp/go-build-cache GO111MODULE=off go run ./RE-MVP-ORAM -experiment normal

## Validation and Acceptance

Acceptance is that `go test ./RE-MVP-ORAM` succeeds and the short normal run does not immediately fail from compile errors, index out of range, `Not target block in working set`, or `no position map entry`.

## Idempotence and Recovery

The edits are source-only and can be rerun. If a behavior choice is ambiguous, stop and ask the user rather than encoding an assumption.

## Artifacts and Notes

No final validation transcript yet.

## Interfaces and Dependencies

No new dependencies are introduced. The main touched interface is `func (c *MvpClient) populatePath(W map[int][]MvpDataBlock, op OramOP, targetSig int) (map[MvpPosition]MvpSlot, []MvpDataBlock, []path)`.
