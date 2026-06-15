# Add H-Pattern Distribution Experiments

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds. This document follows `.agents/PLANS.md`.

## Purpose / Big Picture

The user wants one more experiment for both `RE-MVP-ORAM` and `mvp-oram`. After running a Zipf-shaped workload for a configurable number of executions, the experiment computes the distribution of `h`, where `h` is the number of distinct leaf paths that one address can take according to the client PositionMap at that moment. The result makes the difference between RE-MVP-ORAM and MVP-ORAM visible: RE-MVP-ORAM can keep multiple signatures/slots for one address, while MVP-ORAM has one PositionMap entry per address.

## Progress

- [x] (2026-06-14T22:47:00Z) Read existing `RE-MVP-ORAM/zip_kdist.go`, `RE-MVP-ORAM/experiment3.go`, `mvp-oram/experiment3.go`, and both ORAM core files.
- [x] (2026-06-14T22:49:24Z) Add an `h-pattern` experiment to `RE-MVP-ORAM`.
- [x] (2026-06-14T22:49:43Z) Add an `h-pattern` experiment to `mvp-oram`.
- [x] (2026-06-14T22:52:49Z) Correct `h` to mean the number of possible leaves from the current bucket position, not one sampled leaf.
- [x] (2026-06-14T22:53:00Z) Run formatting, tests, and short smoke commands.
- [x] (2026-06-14T22:55:19Z) Compare 1000-round outputs for client_count=5 and summarize the observed behavior.

## Surprises & Discoveries

- Observation: `RE-MVP-ORAM` uses `map[int]map[int]MvpPositionMapEntry`, so one address may have multiple live signature positions. `mvp-oram` uses `map[int]MvpPositionMapEntry`, so one address has exactly one client-visible position.
  Evidence: `RE-MVP-ORAM/re_mvp_oram.go` defines `PositionMap map[int]map[int]MvpPositionMapEntry`, while `mvp-oram/mvp_oram.go` defines `PositionMap map[int]MvpPositionMapEntry`.
- Observation: A single MVP-ORAM PositionMap entry can still have h greater than 1 because a block can be stored at an internal bucket such as root. Root can lead to every leaf, so h is `2^L`; a bucket at depth d can lead to `2^(L-d)` leaves; a leaf has h=1.
  Evidence: both ORAM implementations build a random leaf from the current bucket prefix in `selectPath`; when the bucket is root or stash, the prefix is empty.
- Observation: Existing experiment3 measures `GetPS` without `Evict` after a warmup sync. The new h-pattern experiment does not need `GetPS`; it inspects the synced PositionMap directly after warmup.
  Evidence: `runSharedAddressGetPSOnlyTrial` in both experiment3 files chooses leaves from PositionMap after `syncClientPositionMaps`.

## Decision Log

- Decision: Define `h` as the number of possible leaf buckets reachable from the current PositionMap position after workload sync. For a root or stash position h is `2^L`, for an internal bucket at depth d h is `2^(L-d)`, and for a leaf h is 1. In RE-MVP-ORAM, h is the union of these reachable leaf sets over all live signatures for the address.
  Rationale: This matches the user's correction: being stored at root means all leaves are possible paths, while being stored at a leaf means only one path is possible.
  Date/Author: 2026-06-14 / Codex
- Decision: The default workload is 1000 rounds, and each round runs one Zipf-sampled access per configured client, matching the round style used by `zip-kdist`.
  Rationale: The user asked for client count and other parameters to be configurable like `zip-kdist`; this preserves the same parallel access shape while making the 1000 value explicit and overrideable.
  Date/Author: 2026-06-14 / Codex
- Decision: Output CSV rows with `h`, `count`, and probability, plus summary fields repeated on each row.
  Rationale: This makes the distribution easy to plot and still keeps max/mean values visible from the CSV alone.
  Date/Author: 2026-06-14 / Codex

## Outcomes & Retrospective

Implemented `go run . -experiment h-pattern` in both `RE-MVP-ORAM` and `mvp-oram`.

Validation passed:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    GOCACHE=/tmp/re-mvp-go-cache go test ./...

    cd /mnt/c/Users/gento/Desktop/TERM2026/mvp-oram
    GOCACHE=/tmp/mvp-go-cache go test ./...

1000-round comparison with `client_count=5`, `L=12`, `N=8192`, `addr_alpha=1.0`, and `op_alpha=1.5`:

    RE-MVP-ORAM: mean_h=234.0549316406, max_h=4096
    MVP-ORAM:    mean_h=8.1065673828,   max_h=4096

RE-MVP-ORAM has a much larger mean because one address can have multiple live signatures and the h value is the union of their reachable leaves. MVP-ORAM can still have h greater than 1 when the one live position is an internal bucket or root, but it has no extra signature slots to union together.

## Context and Orientation

`RE-MVP-ORAM/main.go` and `mvp-oram/main.go` dispatch command-line experiments through `-experiment`. Existing experiments create a server, initialize random data, create clients, warm up with concurrent accesses, synchronize client PositionMaps using `GetPM`, then measure without changing ORAM state. `h` in this plan means the number of distinct selected leaves available for one address after synchronization.

For RE-MVP-ORAM, a client PositionMap entry is nested by address and signature. Live signatures are entries whose slot is not the delete marker. The experiment counts the union of reachable leaves across these live signatures. For MVP-ORAM, a client PositionMap entry is one position per address, so the equivalent count is the number of reachable leaves from that one position.

## Plan of Work

Create `RE-MVP-ORAM/h_pattern.go`. It will use `NewSynchronizedMvpServer`, initialize data, create clients, run Zipf access rounds using the existing `zipAccessOperation`, synchronize client PositionMaps, compute an h distribution over all addresses in one reference client, and write `CSV/re_mvp_oram_h_pattern.csv`.

Create `mvp-oram/h_pattern.go`. It will perform the same workload shape, with local environment readers and a local Zipf operation sampler if needed, then compute the h distribution for MVP-ORAM and write `CSV/mvp_oram_h_pattern.csv`.

Update both `main.go` files to add `case "h-pattern"`.

## Concrete Steps

Run validation from each package:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    GOCACHE=/tmp/re-mvp-go-cache go test ./...
    RE_MVP_H_PATTERN_CLIENT_COUNT=5 RE_MVP_H_PATTERN_ROUNDS=5 GOCACHE=/tmp/re-mvp-go-cache go run . -experiment h-pattern

    cd /mnt/c/Users/gento/Desktop/TERM2026/mvp-oram
    GOCACHE=/tmp/mvp-go-cache go test ./...
    MVP_H_PATTERN_CLIENT_COUNT=5 MVP_H_PATTERN_ROUNDS=5 GOCACHE=/tmp/mvp-go-cache go run . -experiment h-pattern

## Validation and Acceptance

Acceptance requires both Go packages to compile, both test commands to pass, and both smoke commands to produce CSV files with h distribution rows. The RE-MVP-ORAM CSV should be able to show h values greater than 1 when multiple live signatures or internal bucket positions exist for an address. The MVP-ORAM CSV can also show h greater than 1 when its single PositionMap entry is root or an internal bucket; however, it should not have additional growth from unioning multiple signatures.

## Idempotence and Recovery

The experiments are additive. They overwrite only their own CSV output files and can be rerun safely. If a smoke run is too slow, reduce the round count and client count with the documented environment variables.

## Artifacts and Notes

No artifacts yet.

## Interfaces and Dependencies

The new command in both packages must be:

    go run . -experiment h-pattern

The RE-MVP-ORAM environment variables must include `RE_MVP_H_PATTERN_Z`, `RE_MVP_H_PATTERN_L`, `RE_MVP_H_PATTERN_N`, `RE_MVP_H_PATTERN_CLIENT_COUNT`, `RE_MVP_H_PATTERN_ROUNDS`, `RE_MVP_H_PATTERN_ADDR_ALPHA`, and `RE_MVP_H_PATTERN_OP_ALPHA`.

The MVP-ORAM environment variables must include `MVP_H_PATTERN_Z`, `MVP_H_PATTERN_L`, `MVP_H_PATTERN_N`, `MVP_H_PATTERN_CLIENT_COUNT`, `MVP_H_PATTERN_ROUNDS`, `MVP_H_PATTERN_ADDR_ALPHA`, and `MVP_H_PATTERN_OP_ALPHA`.
