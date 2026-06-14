# Restore RE-MVP-ORAM Slot-Managed Positions

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agents/PLANS.md` in this repository.

## Purpose / Big Picture

The current `RE-MVP-ORAM` working tree stores each position-map entry as a bucket plus a timestamp. The requested behavior is to restore the older slot-managed model, where each position-map entry records the exact bucket and slot containing a block. After this change, a reader can inspect `RE-MVP-ORAM/re_mvp_oram.go` and see `MvpPositionMapEntry.Slot`, can configure the fixed per-address signature slot range `0..N`, and can run the Go package to verify that it still compiles and starts.

## Progress

- [x] (2026-06-14T06:18:24Z) Read `.agents/PLANS.md` and inspected the current and parent slot-managed versions of `RE-MVP-ORAM/re_mvp_oram.go`.
- [x] (2026-06-14T06:25:00Z) Restored `MvpPositionMapEntry` and position-map comparisons to exact `MvpPosition` slot tracking.
- [x] (2026-06-14T06:25:00Z) Made the per-address signature slot maximum variable and configurable via `RE_MVP_MAX_SIGNATURE`.
- [x] (2026-06-14T06:25:00Z) Restored slot-aware populate/shuffle candidate matching and added comments around each major loop explaining what the loop does.
- [x] (2026-06-14T06:30:00Z) Ran `gofmt` and validated `RE-MVP-ORAM` with `GOCACHE=/tmp/re-mvp-go-cache go test ./...`.
- [x] (2026-06-14T21:04:34Z) Update access selection so writes always access signature 0 and reads choose randomly among non-Delete signatures.
- [x] (2026-06-14T21:04:34Z) Update populate splitting so only signature 0 is priority and every nonzero signature goes to drain.
- [x] (2026-06-14T21:10:00Z) Run 50-client stash-size analysis and record observed behavior.

## Surprises & Discoveries

- Observation: The current branch name is `souko1`, but HEAD points to the same commit as `main`, `f3702b2 slot単位で管理するのをやめてみる`.
  Evidence: `git branch --show-current` returned `souko1`, and `git log -1 --oneline --decorate` returned `f3702b2 (HEAD -> souko1, main) slot単位で管理するのをやめてみる`.
- Observation: The bucket physical slot count was already fixed as `0..z-1` in `NewMvpBucket`; the hard-coded non-variable range was the per-address signature slot range.
  Evidence: `NewMvpBucket` loops from `i := 0; i < z; i++`, while `newInitializedPositionMapEntries` looped from `sig := 0; sig <= mvpMaxSignature; sig++`.
- Observation: With 50 clients after the sig0-only priority change, drain blocks were not placed in the sampled run because sig0 priority blocks filled the whole path.
  Evidence: `/tmp/re_mvp_sig0_drain_50.log` showed `priority_placed=52`, `unused_slots=0`, and `drain_placed=0` at sampled sequences 100, 150, 200, 250, 300, 350, 400, and 440.
- Observation: Stash size grows quickly under the 50-client sampled run.
  Evidence: `stash_total` reached 8175 at seq 390, `stash_out` reached 186 at seq 400, and `stash_max_version` reached 192 at seq 400.

## Decision Log

- Decision: Interpret the requested configurable `0..N` fixed slots as the per-address signature/copy slot range currently controlled by `mvpMaxSignature`, while preserving bucket physical slot count as `z`.
  Rationale: The code already has two different slot concepts. Bucket physical slots are already created as fixed `0..z-1` positions in `NewMvpBucket`; the `0..mvpMaxSignature` range is the fixed per-address slot set mentioned by existing comments and is the part that is not currently variable.
  Date/Author: 2026-06-14 / Codex
- Decision: For the new access policy, writes must choose signature 0 even if other signatures exist; reads keep the existing random choice over non-Delete signatures.
  Rationale: This directly matches the requested behavior and makes signature 0 the canonical write target while read accesses can still sample from copies.
  Date/Author: 2026-06-14 / Codex
- Decision: For the new shuffle policy, signature 0 blocks are the only priority blocks and every nonzero signature block is a drain block.
  Rationale: This makes the priority path represent the main block only. Copies are handled only by the drain/copy placement path, which makes stash pressure easier to analyze.
  Date/Author: 2026-06-14 / Codex

## Outcomes & Retrospective

No implementation outcome yet.

Update 2026-06-14T06:25:00Z: The source now uses exact slot positions in the position map again. Validation remains to be run.

Update 2026-06-14T06:30:00Z: The implementation is formatted and `go test ./...` passes when `GOCACHE` points at `/tmp/re-mvp-go-cache`.

Update 2026-06-14T21:04:34Z: A new change is in progress: writes will target sig0, reads will choose a random live signature, and populate will priority-place only sig0 while draining all nonzero signatures.

Update 2026-06-14T21:10:00Z: The new access and populate policies are implemented. `GOCACHE=/tmp/re-mvp-go-cache go test ./...` passes, and a 20-second 50-client run produced stash growth evidence in `/tmp/re_mvp_sig0_drain_50.log`.

## Context and Orientation

The Go implementation lives in `RE-MVP-ORAM/re_mvp_oram.go`. `MvpPosition` is a pair of a bucket and a slot. A bucket is a tree node, and a slot is an integer position inside that bucket. `MvpPositionMapEntry` is the per-address metadata that tells the client where a data block is believed to live. The current working tree stores only `Bucket`; the parent of commit `f3702b2` stored `Slot MvpPosition`.

The code also has signature slots. For each logical address, the position map stores entries keyed by integer signature. These are copy slots for versions of the same logical address. Deleted signature slots point to the special delete position. The current code hard-codes the maximum signature value as `mvpMaxSignature = 8`, creating entries `0..8` for every address.

## Plan of Work

First, change `MvpPositionMapEntry` back to `Slot MvpPosition` and update initialization, consolidation, access, working-set merge, recovery, evaluation, placement, and logging to compare and write exact positions again. The server tree structure will remain `Slots map[MvpSlotPosition]map[Version]MvpSlot`, so the physical storage model does not need replacement.

Second, replace the hard-coded `mvpMaxSignature` constant with a variable initialized from a default. Add a small setter and an environment-variable hook in `Normal` and `Experiment3`, so a run can set the maximum fixed signature slot without editing source. The behavior remains inclusive: setting N creates entries `0, 1, ..., N`.

Third, restore the old slot-aware populate/shuffle behavior: priority blocks first target their exact old slot; selected priority blocks are then sorted by version and reassigned to the root-side slots among the slots selected in that pass. Add comments above each important `for` loop in this shuffle section explaining what that loop does.

## Concrete Steps

Work from `/mnt/c/Users/gento/Desktop/TERM2026`. Edit `RE-MVP-ORAM/re_mvp_oram.go`, `RE-MVP-ORAM/normal.go`, and `RE-MVP-ORAM/experiment3.go`. Run `gofmt` on edited Go files, then run `go test ./...` inside `RE-MVP-ORAM`.

## Validation and Acceptance

Run:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    go test ./...

Acceptance: the package compiles and tests pass, or if there are no test files, Go reports successful package validation. A source inspection should show `MvpPositionMapEntry` has `Slot MvpPosition`, fixed signature slots are created inclusively from `0..N`, and populate/shuffle loops have comments explaining their loop-level behavior.

Actual validation used the writable cache path required in this sandbox:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    GOCACHE=/tmp/re-mvp-go-cache go test ./...
    ?    re-mvp-oram    [no test files]

For the 50-client analysis, run:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    RE_MVP_CLIENT_COUNT=50 RE_MVP_ACCESS_LOG=0 GOCACHE=/tmp/re-mvp-go-cache timeout 20s go run . -experiment normal > /tmp/re_mvp_sig0_drain_50.log 2>&1

The run exits with timeout status 124 because `normal` is intentionally long-running. The sampled result showed `max_stash_total 8175 seq 390`, `max_stash_out 186 seq 400`, and `max_stash_max_version 192 seq 400`.

## Idempotence and Recovery

The changes are source edits only. Re-running `gofmt` and `go test ./...` is safe. If a change fails to compile, inspect compiler errors and update only the directly referenced slot/bucket field usage rather than reverting unrelated working-tree changes.

## Artifacts and Notes

Important reference:

    git show f3702b2^:RE-MVP-ORAM/re_mvp_oram.go

This parent version contains the older `MvpPositionMapEntry.Slot` implementation used as the restoration guide.

## Interfaces and Dependencies

`MvpPositionMapEntry` must expose:

    type MvpPositionMapEntry struct {
        Slot MvpPosition
        Ts   Versions
    }

The configurable maximum signature slot must be accessible through a small function in `RE-MVP-ORAM/re_mvp_oram.go` and used by `newInitializedPositionMapEntries`.
