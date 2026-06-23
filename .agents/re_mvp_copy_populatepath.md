# Re-MVP populatePath copy generation

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agents/PLANS.md` in this repository.

## Purpose / Big Picture

The Re-MVP ORAM implementation should actively create replacement copy blocks after priority blocks have been placed. A copy block is a duplicate of one address's visible data block stored under an unused signature, and more copies increase the set of leaf paths that a future read can choose. After this change, `populatePath` will delete old copy signatures early, keep one source block per address from the working set, place priority blocks first, and then fill remaining path slots with newly generated copy blocks selected by the smallest path-pattern evaluation.

## Progress

- [x] (2026-06-18 00:00Z) Read `.agents/PLANS.md` and inspected `RE-MVP-ORAM/re_mvp_oram.go`.
- [x] (2026-06-18 06:36Z) Implemented helper functions that build one copy source per address and place new copy blocks with unused signatures.
- [x] (2026-06-18 06:37Z) Rewrote `MvpClient.populatePath` so existing non-priority copies are immediately deleted and the old drain phase is removed.
- [x] (2026-06-18 06:38Z) Fixed statistical-distance Read selection so it returns the signature that matches the selected leaf.
- [x] (2026-06-18 06:39Z) Ran `gofmt`, `GOCACHE=/tmp/go-build-cache go test ./...`, and a lightweight statistical-distance run in `RE-MVP-ORAM`.
- [x] (2026-06-18 10:27Z) Inspected the base MVP-ORAM stash replacement logic.
- [x] (2026-06-18 10:31Z) Removed the attempted stash swap addition after the user decided swap should be omitted.
- [x] (2026-06-18 10:32Z) Ran `gofmt` and `GOCACHE=/tmp/go-build-cache go test ./...` after removing the attempted swap addition.

## Surprises & Discoveries

- Observation: `RE-MVP-ORAM/re_mvp_oram.go` is already modified in the working tree, and `placePatternBlocks` has intermediate logic that mixes priority placement and drain placement.
  Evidence: `git diff -- RE-MVP-ORAM/re_mvp_oram.go` shows a large existing diff, and lines around `placePatternBlocks` contain both a direct placement branch and the older signature-reassignment branch.
- Observation: The statistical-distance measurement path chose a Read leaf from the union of all active signatures but returned signature `0` unconditionally. After measurement began executing real accesses, this caused `populatePath` to look for sig0 on a path that only contained a copy signature.
  Evidence: Lightweight run failed with `panic: No target block in Workingset` at `re_mvp_oram.go:1346`; `statistical_distance.go` returned `0, statisticalDistanceSampleLeafFromIntervals(...)` for all reads.

## Decision Log

- Decision: Split copy generation into a new helper instead of continuing to use `placePatternBlocks` with a `move` string.
  Rationale: Priority placement moves an existing block without changing its signature, while copy generation duplicates a source block and assigns a deleted signature. Separate helpers make the behavior explicit and make function-level comments accurate.
  Date/Author: 2026-06-18 / Codex
- Decision: Update `chooseReMvpStatisticalDistanceTargetLeaf` to mirror `MvpClient.choosetargetLeaf` by building leaf-to-signature candidates and returning the signature for the selected leaf.
  Rationale: Statistical-distance measurement now executes real accesses after counting selected leaves, so the chosen signature must be physically reachable on the chosen path.
  Date/Author: 2026-06-18 / Codex
- Decision: Do not add stash swap behavior to Re-MVP-ORAM.
  Rationale: The user explicitly decided the swap behavior should be removed. The implementation should keep priority placement and copy generation without the base MVP-ORAM stash replacement phase.
  Date/Author: 2026-06-18 / Codex

## Outcomes & Retrospective

Implemented the requested populatePath behavior and fixed a measurement-path bug that was exposed by executing real accesses. The lightweight zipf statistical-distance run completed without panic and reported TVD `0.0411169405` for `L=6`, `clients=20`, `warmup=10`, and `trials=1000`.

## Context and Orientation

The target file is `RE-MVP-ORAM/re_mvp_oram.go`. `MvpClient.populatePath` receives a working set `W`, which maps each logical address to one or more physical `MvpDataBlock` values visible in the path and stash read by the client. A block with `signature == 0` is the main block for an address. A block with `signature != 0` is a copy. `path` entries update the position map; `mvpDeletePosition` marks a signature slot as reusable.

The helper `evaluationPathpattern` estimates how many leaf paths are already covered by each address's live signatures. Smaller values mean the address has less path coverage and should be preferred when creating new copies. The helper `buildPatternPlacementState` computes evaluation values and deleted signatures available for reuse.

## Plan of Work

First, replace the mixed `placePatternBlocks` helper with a priority-only helper that places leftover priority blocks into unused slots without changing signatures. Then add a copy helper that receives `map[int]MvpDataBlock`, chooses addresses in ascending evaluation order, assigns one unused signature per placed copy, updates only the source block's `S` timestamp, writes the copy into a path slot, and emits a new path-map update.

Next, rewrite `populatePath` classification. It should build `copySourceByAddr map[int]MvpDataBlock` for every address in `W`, keeping the newest block by `newerVersions`. It should add `signature == 0` blocks and the selected target block to `prioritylist`. Every other block should immediately emit a delete path. After priority placement and priority stashing, it should call the copy helper for remaining unused slots. The old `drainlist` phase should be removed.

Finally, run formatting and tests from `RE-MVP-ORAM`.

## Concrete Steps

Work from `/mnt/c/Users/gento/Desktop/TERM2026`. Edit `RE-MVP-ORAM/re_mvp_oram.go`, `RE-MVP-ORAM/statistical_distance.go`, and this plan file. Run:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    gofmt -w re_mvp_oram.go statistical_distance.go
    GOCACHE=/tmp/go-build-cache go test ./...

Expected test result:

    ok  	re-mvp-oram	...

The lightweight behavior check used:

    env GOCACHE=/tmp/go-build-cache RE_MVP_STAT_DISTANCE_L=6 RE_MVP_STAT_DISTANCE_CLIENT_COUNT=20 RE_MVP_STAT_DISTANCE_WARMUP_ROUNDS=10 RE_MVP_STAT_DISTANCE_TRIALS=1000 RE_MVP_STAT_DISTANCE_READ_RATIO=0.9 RE_MVP_STAT_DISTANCE_CSV=/tmp/re_stat_after.csv go run . -oram re-mvp-oram -experimentmode statisticaldistance -accesstype zipf

## Validation and Acceptance

Acceptance is that `go test ./...` passes with `GOCACHE=/tmp/go-build-cache`, and `populatePath` no longer has a drain placement phase. The code should show function-level comments for the new priority and copy helper functions, and comments inside `populatePath` should describe the early deletion of old copies and the generation of new copy blocks. The lightweight statistical-distance command should complete without `No target block in Workingset`.

## Idempotence and Recovery

The edit is safe to repeat because it is limited to pure Go source and a plan document. If tests fail, inspect the failing line and rerun `gofmt` and `GOCACHE=/tmp/go-build-cache go test ./...` after the fix. Do not revert unrelated working-tree changes.

## Artifacts and Notes

Validation output:

    ok  	re-mvp-oram	0.003s

    statisticaldistance result: oram=re-mvp-oram access=zipf clients=20 l=6 leaves=64 warmup_rounds=10 trials=1000 tvd=0.0411169405 csv=/tmp/re_stat_after.csv elapsed=11.987s

## Interfaces and Dependencies

At completion, `RE-MVP-ORAM/re_mvp_oram.go` should contain these helpers:

    func buildCopySourceByAddr(W map[int][]MvpDataBlock) map[int]MvpDataBlock
    func (c *MvpClient) buildCopyPlacementState(copySourceByAddr map[int]MvpDataBlock, pathList []path) ([]int, map[int]int, map[int][]int)
    func (c *MvpClient) placePriorityBlocks(blocksByAddr map[int][]MvpDataBlock, unusedSlot []MvpPosition, populatedPath map[MvpPosition]MvpSlot, populatedPathMap *[]path) ([]MvpPosition, int)
    func (c *MvpClient) placeCopyBlocks(copySourceByAddr map[int]MvpDataBlock, unusedSlot []MvpPosition, populatedPath map[MvpPosition]MvpSlot, populatedPathMap *[]path) []MvpPosition
