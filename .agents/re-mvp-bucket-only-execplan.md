# Move RE-MVP-ORAM position tracking from slot-level to bucket-level

This ExecPlan is a living document. It follows `.agents/PLANS.md` and must be updated as the work proceeds.

## Purpose / Big Picture

The current RE-MVP-ORAM code keeps a full physical slot position in `PositionMap`, so the client remembers both the bucket and the exact slot inside that bucket. The user wants to stop doing that and manage placement only at the bucket level. After this change, the client should still be able to run the normal experiment, but every `PositionMap` lookup and update should treat a block as being "in bucket `X`" rather than "in bucket `X`, slot `Y`". This makes the access logic less fragile and makes it easier to reason about bucket movement without slot bookkeeping getting in the way.

The observable result will be that `RE-MVP-ORAM/re_mvp_oram.go` compiles with bucket-only position tracking, `RE-MVP-ORAM/normal.go` and `RE-MVP-ORAM/experiment3.go` still run, and short normal runs complete without the old slot-specific position assumptions causing panics.

## Progress

- [x] (2026-06-10 Asia/Tokyo) Read `.agents/PLANS.md` and inspected the current slot-aware `PositionMap` code paths.
- [x] (2026-06-10 Asia/Tokyo) Identified the main places that still depend on slot-level position: `MvpPositionMapEntry`, `choosePositionMapEntry`, `consolidatePathMaps`, `mergePathStashes`, `populatePath`, and `experiment3.go`.
- [x] (2026-06-10 Asia/Tokyo) Changed `MvpPositionMapEntry` to store a bucket only and updated initialization, path consolidation, and working-set recovery to use the bucket field.
- [x] (2026-06-10 Asia/Tokyo) Updated client access, populate, and experiment helper code so they no longer read `.Slot` from the position map.
- [x] (2026-06-10 Asia/Tokyo) Ran `gofmt` and `go test ./...` in `RE-MVP-ORAM`; both passed.
- [x] (2026-06-10 Asia/Tokyo) Ran short normal runs with 1 client and 50 clients; both progressed without panic, and the 50-client run still showed stash growth.
- [x] (2026-06-10 Asia/Tokyo) Changed priority placement so priority blocks are packed into empty slots first, then drain blocks fill any remaining empty slots, and leftover priority blocks go to stash.
- [x] (2026-06-10 Asia/Tokyo) Ran the updated 50-client experiment with the new placement order and measured stash totals at `seq=100`, `seq=150`, and `seq=200`.
- [x] (2026-06-10 Asia/Tokyo) Changed leftover priority placement to use the same pattern-based fill logic as drain placement before falling back to stash, then reran the 50-client experiment.

## Surprises & Discoveries

- Observation: `GetPS` had been returning shared bucket maps, which caused concurrent map writes when multiple clients ran at once.
  Evidence: a 50-client run previously failed with `fatal error: concurrent map iteration and map write` until `GetPS` cloned each bucket before returning it.
- Observation: `PositionMap` is used in two different ways right now: as a logical identity map during access selection and as a physical placement map during eviction.
  Evidence: `choosePositionMapEntry` only needs to know whether a block is deleted or not, while `populatePath` still compares exact slot positions for bucket placement.
- Observation: After switching `PositionMap` to bucket-only records, the 50-client run still did not panic, but stash growth remained visible.
  Evidence: the short run reached `stash_total=2249` by `seq=100` and later logged `stash_total=4451` by `seq=150`.
- Observation: Packing priority blocks into empty slots before drain placement dramatically reduced stash growth on the same 50-client workload.
  Evidence: on the updated run, `stash_total` was `6` at `seq=100`, then `0` at `seq=150` and `seq=200`.
- Observation: After switching leftover priority placement to pattern-based fill, stash growth stayed lower than the earlier bucket-only run, and the `no position map entry` panic disappeared.
  Evidence: on the latest 50-client run, `stash_total` was `685` at `seq=100`, `1311` at `seq=150`, and `1708` at `seq=200`; the run stopped only because of the timeout, not a panic.

## Decision Log

- Decision: Keep `MvpPosition` as the physical path position type, but change `MvpPositionMapEntry` to store bucket identity only.
  Rationale: The eviction path still needs slot positions internally, but the user explicitly wants the persisted position map to stop remembering slot numbers.
  Date/Author: 2026-06-10 / Codex.
- Decision: Update experiment helpers to use the same bucket-only access selection logic as the client instead of reaching into `.Slot`.
  Rationale: This keeps the experiment aligned with the new data model and avoids duplicating old slot-based assumptions.
  Date/Author: 2026-06-10 / Codex.
- Decision: Preserve the path-level slot layout inside eviction logic for now, even though the persisted position map no longer stores it.
  Rationale: The ORAM tree still needs concrete slots for physical placement, but that detail does not need to leak into the logical position map.
  Date/Author: 2026-06-10 / Codex.
- Decision: Keep bucket-level changes local to the position map and experiment helpers instead of redesigning the physical tree representation in the same step.
  Rationale: The user asked specifically to stop remembering slots in `PositionMap`; broadening the refactor to the tree structure would add risk without helping that goal.
  Date/Author: 2026-06-10 / Codex.
- Decision: Place priority blocks into empty slots before using drain blocks, then stash only any leftover priority blocks.
  Rationale: This matches the user’s requested order of operations and reduces stash pressure by letting priority blocks consume open space first.
  Date/Author: 2026-06-10 / Codex.
- Decision: Reuse the same pattern-based fill helper for leftover priority blocks before drain placement.
  Rationale: The user asked for leftover priority to be scheduled the same way drain blocks are scheduled, based on the virtual path-map pattern score.
  Date/Author: 2026-06-10 / Codex.

## Outcomes & Retrospective

The bucket-only position-map refactor is in place. The client now stores `Bucket` instead of a full slot position in `PositionMap`, the experiment helper no longer reads `.Slot`, and compilation plus short runtime checks pass. After the latest placement change, the 50-client run no longer panics on missing position-map entries. The measured stash totals were `685` at `seq=100`, `1311` at `seq=150`, and `1708` at `seq=200`, which is lower than the earlier bucket-only baseline but still not flat over longer runs.

## Context and Orientation

The main implementation lives in `RE-MVP-ORAM/re_mvp_oram.go`.

`MvpPosition` is the physical location type used inside the tree. It currently has a `bucket` and a `slot`. `MvpPositionMapEntry` is the client-side record of where a block lives. Today it points to a full `MvpPosition`, but the requested change is to make that record bucket-only.

The main client flow is `func (c *MvpClient) Access(op OramOP) error`, which calls `GetPM`, updates `PositionMap`, then calls `GetPS`, `mergePathStashes`, `populatePath`, and `Evict`. `populatePath` still needs physical slot positions while it constructs the outgoing path, but the stored position map should no longer depend on them.

`RE-MVP-ORAM/experiment3.go` contains experiment helpers that still assume `PositionMap[addr].Slot` exists. Those helpers must be updated to work with the bucket-only record.

## Plan of Work

First, change `MvpPositionMapEntry` in `RE-MVP-ORAM/re_mvp_oram.go` so it stores a bucket-only field, and update the constructors and clone helpers that create or copy it. Then update `InitializeRandomData`, `consolidatePathMaps`, `choosePositionMapEntry`, `mergePathStashes`, and `populatePath` so they read and write bucket identity rather than full slot positions when they are updating the logical map.

Second, update any code that currently compares `entry.Slot.bucket` or `entry.Slot.slot` to use the bucket field only. Where physical slots are still required for placement inside `populatePath`, keep using the temporary `MvpPosition` values there, but do not write those exact slot values back into `PositionMap`.

Third, update `RE-MVP-ORAM/experiment3.go` so it no longer expects `PositionMap[addr].Slot` and instead uses the same bucket-only selection helper that the client uses.

Finally, verify the change with `gofmt`, `go test ./...`, and a short normal run. If the bucket-only refactor exposes a new ambiguity about whether the stash should be modeled as a bucket or a special sentinel, record that in the Decision Log rather than guessing silently.

## Concrete Steps

Work from `/mnt/c/Users/gento/Desktop/TERM2026`.

Edit `RE-MVP-ORAM/re_mvp_oram.go` and `RE-MVP-ORAM/experiment3.go`.

Run:

    gofmt -w RE-MVP-ORAM/re_mvp_oram.go RE-MVP-ORAM/experiment3.go
    cd RE-MVP-ORAM
    GOCACHE=/tmp/codex-go-cache go test ./...

Then run a short normal execution:

    cd RE-MVP-ORAM
    RE_MVP_CLIENT_COUNT=1 RE_MVP_ACCESS_LOG=0 timeout 10s GOCACHE=/tmp/codex-go-cache go run . -experiment normal

If that succeeds, run a short multi-client execution:

    cd RE-MVP-ORAM
    RE_MVP_CLIENT_COUNT=50 RE_MVP_ACCESS_LOG=0 timeout 10s GOCACHE=/tmp/codex-go-cache go run . -experiment normal

Expected behavior is that the code compiles, the one-client run does not panic, and the 50-client run no longer depends on slot-level state in `PositionMap`.

## Validation and Acceptance

The change is accepted when the repository still passes `go test ./...`, a one-client normal run reaches steady progress without panic, and search in `RE-MVP-ORAM` no longer shows any live code that expects `PositionMap[addr].Slot` to exist.

## Idempotence and Recovery

The refactor is source-only and can be repeated by rerunning the edits from the current tree. If a compile error appears, fix the first remaining `.Slot` reference before continuing, because it likely indicates a path that still assumes slot-level position tracking.

## Artifacts and Notes

Evidence from the prior concurrency fix:

    fatal error: concurrent map iteration and map write

That bug was caused by shared bucket maps from `GetPS`; the current refactor should not reintroduce shared ownership of tree state.

## Interfaces and Dependencies

At the end of this change, `RE-MVP-ORAM/re_mvp_oram.go` should expose:

    type MvpPositionMapEntry struct {
        Bucket MvpBucketPosition
        Ts Versions
    }

The rest of the code should use that bucket-only entry for logical position tracking, while the path-building code may still use `MvpPosition` internally for tree traversal and bucket placement.

Revision note, 2026-06-10: Updated the plan to record the leftover-priority pattern placement change, the absence of the earlier panic, and the latest measured stash totals under 50 clients.
