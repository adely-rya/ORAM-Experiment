# Build Re-MVP-ORAM Double Position Map

This ExecPlan is a living document. It follows `.agents/PLANS.md` and must keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` updated as work proceeds.

## Purpose / Big Picture

Re-MVP-ORAM needs to track multiple physical copies of the same logical address. The observable outcome is that `RE-MVP-ORAM/re_mvp_oram.go` compiles with a position map shaped as `addr -> signature -> position entry`, where `addr` is the logical address and `signature` identifies one concrete copy of that address. The change does not attempt to prove the algorithm is experimentally correct; it makes the data model and map handling internally consistent so future algorithm work can build on it.

## Progress

- [x] (2026-06-07T03:27:26Z) Read `.agents/PLANS.md` and inspected the current one-map Re-MVP implementation.
- [x] (2026-06-07T03:27:26Z) Convert data structures in `RE-MVP-ORAM/re_mvp_oram.go` to carry `signature` and nested position maps.
- [x] (2026-06-07T03:27:26Z) Update client access, merge, and populate logic to use `map[int][]MvpDataBlock` without treating slices as single blocks.
- [x] (2026-06-07T03:27:26Z) Update `RE-MVP-ORAM/experiment3.go` references that assume `PositionMap[addr].Slot`.
- [x] (2026-06-07T03:27:26Z) Run `gofmt` and `env GOCACHE=/tmp/go-build GO111MODULE=off go test` in `RE-MVP-ORAM`.

## Surprises & Discoveries

- Observation: The previous broken implementation had the right high-level shape but mixed `map[int][]MvpDataBlock` with single-block code in `populatePath`.
  Evidence: Earlier build errors included `block.Addr undefined (type []MvpDataBlock has no field or method Addr)`.
- Observation: After the nested map conversion, the only remaining compile error was in `RE-MVP-ORAM/experiment3.go`.
  Evidence: `client.PositionMap[op.target].Slot undefined (type map[int]MvpPositionMapEntry has no field or method Slot)`.

## Decision Log

- Decision: Keep one `signature` per `MvpDataBlock` and use `PositionMap map[int]map[int]MvpPositionMapEntry`.
  Rationale: The user explicitly wants copies of the same address to be separated by signature while retaining address as the outer key.
  Date/Author: 2026-06-07 / Codex.
- Decision: On write, keep the accessed block signature and emit delete PathMap entries for other signatures of the same address.
  Rationale: This matches the earlier partial implementation and prevents stale copies from remaining live in the nested position map after a write.
  Date/Author: 2026-06-07 / Codex.
- Decision: Make working-set deletion signature-aware instead of deleting by address.
  Rationale: Deleting by address is incorrect when several blocks share the same address but have different signatures.
  Date/Author: 2026-06-07 / Codex.

## Outcomes & Retrospective

The nested map implementation is now in place. `RE-MVP-ORAM/re_mvp_oram.go` carries signatures on data blocks and PathMap entries, initializes inner maps before assigning to them, clones nested maps correctly, and keeps working-set operations signature-aware. `RE-MVP-ORAM/experiment3.go` no longer assumes `PositionMap[addr].Slot`; it uses the same candidate-selection helper as normal access. The algorithm may still need research-level validation, but the implementation now compiles and no longer has the obvious map/slice type errors from the previous partial version.

## Context and Orientation

The relevant implementation is `RE-MVP-ORAM/re_mvp_oram.go`. A `MvpDataBlock` represents one stored data block. A `MvpPositionMapEntry` records where a block currently lives: either a tree slot or the stash. A `PathMap` entry is represented by the local `path` struct and is sent from one client eviction to later clients so they can update their position maps.

Before this work, the current file had been reverted to the simpler one-map shape `map[int]MvpPositionMapEntry`. The target shape is `map[int]map[int]MvpPositionMapEntry`: the outer key is the logical address, and the inner key is a signature identifying a concrete copy. The working set `W` must therefore be `map[int][]MvpDataBlock` because one address can have multiple live block copies.

## Plan of Work

In `RE-MVP-ORAM/re_mvp_oram.go`, add `signature int` to `MvpDataBlock` and use a pointer receiver method to assign it so the mutation is retained. Extend `path` with `sig int` and `delete bool`. Update constructors so normal movement updates include both address and signature, and delete updates identify a signature to remove.

Change `clonePositionMap`, `MvpServer.PositionMaps`, `InitializeRandomData`, `MvpClient.PositionMap`, and `NewMvpClient` to use nested maps. Ensure `InitializeRandomData` initializes `positionMap[addr]` before assigning `positionMap[addr][sig]`.

Update `Access` so it chooses an access candidate from the inner map without using `rand.Intn(len-1)`, fetches the path, then finds the matching target block by signature in `W`. Update `consolidatePathMaps` to group updates by `(addr, sig)`, create missing inner maps, and handle delete entries.

Update `mergePathStashes` and `populatePath` so every block lookup uses `PositionMap[addr][signature]`. In `populatePath`, flatten the working set where needed for candidate lists, remove blocks by `(addr, signature)`, and append swapped blocks back into the appropriate address slice.

In `RE-MVP-ORAM/experiment3.go`, replace the old `PositionMap[addr].Slot` lookup with the same helper used by access selection, while accepting that the experiment may still be algorithmically questionable.

## Concrete Steps

Edit only `RE-MVP-ORAM/re_mvp_oram.go`, `RE-MVP-ORAM/experiment3.go`, and this plan. After edits, run from `/mnt/c/Users/gento/Desktop/TERM2026`:

    gofmt -w RE-MVP-ORAM/re_mvp_oram.go RE-MVP-ORAM/experiment3.go
    cd RE-MVP-ORAM
    env GOCACHE=/tmp/go-build GO111MODULE=off go test

Expected test output:

    ?   	_/mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM	[no test files]

Actual validation output observed:

    ?   	_/mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM	[no test files]

## Validation and Acceptance

The code is accepted when `env GOCACHE=/tmp/go-build GO111MODULE=off go test` run inside `RE-MVP-ORAM` exits successfully. Search should also show no old single-map `PositionMap[addr].Slot` access remains in `RE-MVP-ORAM`.

## Idempotence and Recovery

The edits are ordinary source changes and can be repeated by reapplying the plan from the current working tree. If a compile error appears, use the error location to find a remaining single-map assumption or a `[]MvpDataBlock` value being treated as a single `MvpDataBlock`.

## Artifacts and Notes

Validation output and any final caveats will be recorded here after the implementation is complete.

## Interfaces and Dependencies

At completion, these signatures should exist in `RE-MVP-ORAM/re_mvp_oram.go`:

    type MvpDataBlock struct {
        Addr int
        signature int
        Data string
        Version Versions
    }

    type path struct {
        addr int
        sig int
        to MvpPosition
        delete bool
        Ver Versions
        Seq Version
    }

    func clonePositionMap(positionMap map[int]map[int]MvpPositionMapEntry) map[int]map[int]MvpPositionMapEntry
    func (s *MvpServer) InitializeRandomData(n int, seed int64) map[int]map[int]MvpPositionMapEntry
    func (c *MvpClient) mergePathStashes() map[int][]MvpDataBlock
    func (c *MvpClient) populatePath(W map[int][]MvpDataBlock, op OramOP, targetSig int) (map[MvpPosition]MvpSlot, []MvpDataBlock, []path)

Revision note, 2026-06-07: Updated the plan after implementation to record the completed nested-map conversion, the experiment3 compile fix, and the successful validation command.
