# Add RE-MVP-ORAM Concurrent Access Pattern

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agents/PLANS.md` in this repository.

## Purpose / Big Picture

The repository currently has `RE-MVP-ORAM` execution modes for `normal` and `experiment3`. The requested change is to add a separate selectable execution mode that repeatedly runs concurrent client accesses, similar to the warmup access pattern in `mvp-oram/experiment3.go`, without changing `normal`. After this change, a user can run `go run . -experiment concurrent` in `RE-MVP-ORAM` and observe concurrent access rounds plus stash metrics under `z=5`, `l=12`, and `n=1<<(l+1)` by default.

## Progress

- [x] (2026-06-14T21:30:16Z) Read `mvp-oram/experiment3.go`, `RE-MVP-ORAM/main.go`, and the current `RE-MVP-ORAM/experiment3.go`.
- [x] (2026-06-14T21:34:00Z) Add a separate `RE-MVP-ORAM/concurrent.go` file implementing the new concurrent access mode.
- [x] (2026-06-14T21:34:00Z) Add the new mode to `RE-MVP-ORAM/main.go` without changing `normal`.
- [x] (2026-06-14T21:34:00Z) Run `gofmt`, `go test ./...`, and bounded concurrent runs.
- [x] (2026-06-14T21:36:00Z) Analyze stash-size metrics from the bounded concurrent runs.

## Surprises & Discoveries

- Observation: The existing `RE-MVP-ORAM/experiment3.go` already has helper functions named `runConcurrentWarmupAccessTrial`, `opgenerater`, and Zipf sampling helpers.
  Evidence: `rg` found those helpers in `RE-MVP-ORAM/experiment3.go`, so the new file must avoid duplicate function names in the same Go package.
- Observation: The default concurrent run with alpha 1.9 reached round 100 of 200 before a 60-second timeout and showed large stash bursts.
  Evidence: `/tmp/re_mvp_concurrent_z5_l12_n8192_50.log` reported `max_stash_out 1663 seq 5280` and `max_stash_max_version 1663 seq 5280`.
- Observation: A lower-skew alpha 0.1 concurrent run was even larger in the sampled window.
  Evidence: `/tmp/re_mvp_concurrent_z5_l12_n8192_50_alpha01.log` reported `max_stash_out 2495 seq 4430` and `max_stash_max_version 2495 seq 4430`.

## Decision Log

- Decision: Use a new experiment name `concurrent` and implement it in `RE-MVP-ORAM/concurrent.go`.
  Rationale: The user requested a selectable pattern in a separate file while leaving `normal` unchanged.
  Date/Author: 2026-06-14 / Codex
- Decision: Use `NewSynchronizedMvpServer` for this mode.
  Rationale: The user stated snapshots are unnecessary because multiple versions do not run in parallel in this simultaneous-access mode. The synchronized server disables the snapshot path in the server.
  Date/Author: 2026-06-14 / Codex
- Decision: Interpret the requested `l` and `n` relationship as the previous tested relationship `n = 1 << (l + 1)`.
  Rationale: The immediately preceding experiment used `l = x` and `n = 1 << (x+1)`, and the user referred to that relationship.
  Date/Author: 2026-06-14 / Codex

## Outcomes & Retrospective

Implemented `RE-MVP-ORAM/concurrent.go` and added the `concurrent` mode to `RE-MVP-ORAM/main.go`. The mode uses `z=5`, `l=12`, `n=1<<(l+1)`, 50 clients, synchronized server snapshots disabled, and concurrent rounds based on the MVP-ORAM experiment3 warmup pattern.

Validation passed with:

    GOCACHE=/tmp/re-mvp-go-cache go test ./...
    ?    re-mvp-oram    [no test files]

Bounded runs showed that stash size can become large in this simultaneous-access mode. With default alpha 1.9, max `stash_out` was 1663 in the sampled 60-second run. With alpha 0.1, max `stash_out` was 2495.

## Context and Orientation

`RE-MVP-ORAM/main.go` selects an execution mode with the `-experiment` flag. `normal` runs indefinitely with clients continuously choosing random reads and writes. `RE-MVP-ORAM/experiment3.go` runs a Monte Carlo leaf distribution experiment and contains a concurrent access warmup using `sync.WaitGroup`. The new mode should be separate from both files so `normal` remains unchanged and the new behavior is easy to inspect.

The new concurrent mode should create a single server, initialize data, create multiple clients, and then run rounds. In each round, every client gets one generated operation and all operations are issued concurrently with a `sync.WaitGroup`. The server should be `NewSynchronizedMvpServer`, which disables snapshots.

## Plan of Work

Create `RE-MVP-ORAM/concurrent.go`. Define defaults `z=5`, `l=12`, `n=1<<(l+1)`, `clientCount=50`, and a bounded number of rounds for direct runs. Read optional environment variables for client count, rounds, and Zipf alpha, while keeping the defaults aligned with the requested configuration. Reuse existing `OramOP`, `MvpClient`, and server APIs. Avoid helper names that already exist in `experiment3.go`.

Update `RE-MVP-ORAM/main.go` to add `case "concurrent": Concurrent()`.

## Concrete Steps

From `/mnt/c/Users/gento/Desktop/TERM2026`, edit `RE-MVP-ORAM/concurrent.go` and `RE-MVP-ORAM/main.go`. Run:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    GOCACHE=/tmp/re-mvp-go-cache go test ./...
    RE_MVP_ACCESS_LOG=0 GOCACHE=/tmp/re-mvp-go-cache timeout 30s go run . -experiment concurrent

## Validation and Acceptance

Acceptance requires `go test ./...` to pass and a bounded `-experiment concurrent` run to produce progress logs and stash metrics. The stash analysis should report maximum `stash_out`, `stash_max_version`, and whether drain blocks are being placed.

## Idempotence and Recovery

The changes are additive except for the `main.go` switch case. Re-running the concurrent mode is safe. If a run takes too long, use `timeout` or lower `RE_MVP_CONCURRENT_ROUNDS`.

## Artifacts and Notes

The stash analysis will be captured from a log under `/tmp` after implementation.

Captured logs:

    /tmp/re_mvp_concurrent_smoke.log
    /tmp/re_mvp_concurrent_z5_l12_n8192_50.log
    /tmp/re_mvp_concurrent_z5_l12_n8192_50_alpha01.log

## Interfaces and Dependencies

The new public entry point must be:

    func Concurrent()

It must be selectable with:

    go run . -experiment concurrent
