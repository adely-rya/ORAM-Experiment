# Add selectable MVP-ORAM backend to RE-MVP-ORAM experiments

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agents/PLANS.md` from the repository root.

## Purpose / Big Picture

The RE-MVP-ORAM command in `RE-MVP-ORAM` currently runs only the Re-MVP-ORAM implementation. After this change, a user can pass `-oram re-mvp-oram` or `-oram mvp-oram` to run the same access generator, run mode, and stash metrics experiment against either implementation. This lets the two algorithms be compared from the same executable while keeping the MVP-ORAM shuffle algorithm, position map management, and path selection behavior copied from `mvp-oram/mvp_oram.go`.

## Progress

- [x] (2026-06-17T04:48:22Z) Read `RE-MVP-ORAM/main.go`, `RE-MVP-ORAM/re_mvp_oram.go`, `mvp-oram/mvp_oram.go`, and `.agents/PLANS.md`.
- [x] (2026-06-17T04:48:22Z) Decided to copy MVP-ORAM into the RE-MVP-ORAM package with prefixed names to avoid duplicate Go identifiers.
- [x] (2026-06-17T04:55:06Z) Added `base_mvp_oram.go` copied from `mvp-oram/mvp_oram.go` with prefixed names and small stash metrics request additions.
- [x] (2026-06-17T04:55:06Z) Added `-oram` CLI branching in `RE-MVP-ORAM/main.go`.
- [x] (2026-06-17T04:55:06Z) Added MVP-ORAM async and sync runners that reuse the current access generator and preserve MVP-ORAM access internals.
- [x] (2026-06-17T04:55:06Z) Validated with `go build`, `go run . -h`, and short MVP-ORAM async/sync startup checks.

## Surprises & Discoveries

- Observation: MVP-ORAM and Re-MVP-ORAM both use identifiers such as `MvpServer`, `MvpClient`, `path`, and `ServerRequest`, but these types have different shapes.
  Evidence: `mvp-oram/mvp_oram.go` has `PositionMap map[int]MvpPositionMapEntry`, while `RE-MVP-ORAM/re_mvp_oram.go` has `PositionMap map[int]map[int]MvpPositionMapEntry` because Re-MVP tracks signatures.

- Observation: The copied MVP-ORAM `Access` method logged every access unconditionally.
  Evidence: `mvp-oram/mvp_oram.go` has direct `log.Printf("access start...")` and `log.Printf("access success...")` calls. In `base_mvp_oram.go`, these were wrapped in the existing `accessLoggingEnabled` guard so the experiment runner does not flood logs.

## Decision Log

- Decision: Keep shared scalar types `Version`, `Versions`, `OramOP`, `Read`, and `Write` from the existing RE-MVP-ORAM package, but prefix copied MVP-ORAM structural types and functions with `BaseMvp` or `base`.
  Rationale: These scalar types are semantically identical and reduce adapter code. The structural types conflict and have different fields, so they must be distinct to compile safely.
  Date/Author: 2026-06-17 / Codex

- Decision: Copy only `mvp-oram/mvp_oram.go` into `RE-MVP-ORAM/base_mvp_oram.go`, not the old MVP experiment files.
  Rationale: The requested behavior is to use the current experiment runner from `RE-MVP-ORAM/main.go`; copying old experiments would add unused entry points and identifier conflicts.
  Date/Author: 2026-06-17 / Codex

- Decision: Implement the MVP sync runner by splitting the copied MVP `Access` sequence into the same phases as the Re-MVP sync runner: `GetPM`, `GetPS`, and `Evict`.
  Rationale: This preserves the MVP position-map consolidation, path selection, working-set merge, target update, and `populatePath` shuffle code while enforcing the requested phase barriers.
  Date/Author: 2026-06-17 / Codex

## Outcomes & Retrospective

The RE-MVP-ORAM command now accepts `-oram re-mvp-oram` and `-oram mvp-oram`. MVP-ORAM is copied into `RE-MVP-ORAM/base_mvp_oram.go` with prefixed names to coexist with Re-MVP. The command can select MVP-ORAM with either async or sync run mode and the existing random or zipf access generator. The MVP shuffle and path selection logic remain in the copied implementation rather than being converted to Re-MVP logic.

## Context and Orientation

The `RE-MVP-ORAM` directory is a Go `package main`. The current command entry point is `RE-MVP-ORAM/main.go`. It provides `-runmode`, `-experimentmode`, and `-accesstype`. The Re-MVP-ORAM algorithm lives in `RE-MVP-ORAM/re_mvp_oram.go`. The MVP-ORAM algorithm to import lives in `mvp-oram/mvp_oram.go`.

In this plan, "backend" means the ORAM implementation selected by `-oram`. "Re-MVP-ORAM" means the existing implementation in `re_mvp_oram.go`. "MVP-ORAM" means the implementation copied from `mvp-oram/mvp_oram.go`. "Sync run" means all clients complete `GetPM` before any client starts `GetPS`, then all clients complete `GetPS` before any client starts `Evict`.

## Plan of Work

First, create `RE-MVP-ORAM/base_mvp_oram.go` by copying `mvp-oram/mvp_oram.go`. Mechanically rename conflicting identifiers so the file can coexist with `re_mvp_oram.go`: `MvpServer` becomes `BaseMvpServer`, `MvpClient` becomes `BaseMvpClient`, `path` becomes `basePath`, and related request, response, tree, bucket, slot, and position types get the same `BaseMvp` prefix. The algorithm bodies for shuffle, position map update, path selection, and working set creation should remain the copied MVP-ORAM logic.

Second, add a small stash metrics request to the copied MVP server so the current `stash-metrics` experiment can sample the maximum stash size among server stash versions. This is an adapter for the experiment runner, not an algorithm change.

Third, update `RE-MVP-ORAM/main.go` so `main` accepts `-oram` with values `re-mvp-oram` and `mvp-oram`. The existing Re-MVP-ORAM path remains the default. The MVP-ORAM path should initialize a `BaseMvpServer`, clone MVP position maps for each `BaseMvpClient`, and run either async or sync mode using the same `AccessOperation` generators.

Fourth, validate compile and CLI behavior. The project is known to have algorithm-level runtime errors under some workloads, so acceptance for this task is compile success and CLI selection wiring. A short startup run can be attempted only if it does not distract from the requested integration.

## Concrete Steps

Run commands from `/mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM`.

Create and transform the copied MVP implementation, then run:

    gofmt -w main.go base_mvp_oram.go
    GOCACHE=/tmp/re-mvp-go-build go build .
    GOCACHE=/tmp/re-mvp-go-build go run . -h
    GOCACHE=/tmp/re-mvp-go-build timeout 3s go run . -oram mvp-oram -runmode sync -experimentmode stash-metrics -accesstype random
    GOCACHE=/tmp/re-mvp-go-build timeout 3s go run . -oram mvp-oram -runmode async -experimentmode stash-metrics -accesstype random

The help output should include:

    -oram string
        oram implementation: re-mvp-oram or mvp-oram

## Validation and Acceptance

Acceptance is met when:

1. `GOCACHE=/tmp/re-mvp-go-build go build .` exits with status 0.
2. `go run . -h` shows `-oram`, `-runmode`, `-experimentmode`, and `-accesstype`.
3. The MVP-ORAM copied implementation is present in `RE-MVP-ORAM/base_mvp_oram.go`.
4. The copied MVP-ORAM path selection and `populatePath` shuffle logic are not rewritten to Re-MVP behavior.

Observed validation on 2026-06-17:

    GOCACHE=/tmp/re-mvp-go-build go build .
    exit status 0

    GOCACHE=/tmp/re-mvp-go-build go run . -h
    Usage includes -oram, -runmode, -experimentmode, and -accesstype.

    GOCACHE=/tmp/re-mvp-go-build timeout 3s go run . -oram mvp-oram -runmode sync -experimentmode stash-metrics -accesstype random
    2026/06/17 13:54:58 running: oram=mvp-oram runmode=sync experimentmode=stash-metrics accesstype=random
    exit status 124 from timeout, with no panic before timeout

    GOCACHE=/tmp/re-mvp-go-build timeout 3s go run . -oram mvp-oram -runmode async -experimentmode stash-metrics -accesstype random
    2026/06/17 13:55:06 running: oram=mvp-oram runmode=async experimentmode=stash-metrics accesstype=random
    exit status 124 from timeout, with no panic before timeout

## Idempotence and Recovery

The changes are additive except for edits to `main.go`. If the generated binary `re-mvp-oram` appears after `go build`, remove only that generated file. Do not revert unrelated dirty files in the repository.

## Artifacts and Notes

No artifacts yet.

## Interfaces and Dependencies

`main.go` should expose:

    RunExperiment(runMode string, experimentMode string, accessType string, oramType string)

The new MVP adapter should provide:

    func NewBaseMvpServer(z int, l int) *BaseMvpServer
    func (s *BaseMvpServer) InitializeRandomData(n int, seed int64) map[int]BaseMvpPositionMapEntry
    func NewBaseMvpClient(l int, z int, clientID int, positionmap map[int]BaseMvpPositionMapEntry, server chan<- BaseMvpServerRequest) *BaseMvpClient
    func (c *BaseMvpClient) Access(op OramOP) error
