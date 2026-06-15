# Add Zip K-Distance Workload

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agents/PLANS.md` in this repository.

## Purpose / Big Picture

The user wants a new `RE-MVP-ORAM` workload like MVP-ORAM experiment3: warm up the ORAM, then measure the distribution of `k`, the number of distinct leaves chosen by a group of clients, and compare it with an independent random-leaf distribution using statistical distance. The new workload must live in a separate file, use Zipf sampling for both addresses and operation types, run with snapshots disabled, and avoid Evict during the measurement phase.

## Progress

- [x] (2026-06-14T22:23:01Z) Read existing `RE-MVP-ORAM/experiment3.go`, `mvp-oram/experiment3.go`, `RE-MVP-ORAM/main.go`, and `RE-MVP-ORAM/zip.go`.
- [x] (2026-06-14T22:28:04Z) Add a separate `RE-MVP-ORAM/zip_kdist.go` workload.
- [x] (2026-06-14T22:28:04Z) Add a selectable `-experiment zip-kdist` mode.
- [x] (2026-06-14T22:28:04Z) Run formatting and Go validation.
- [x] (2026-06-14T22:28:05Z) Run a small workload and record output.

## Surprises & Discoveries

- Observation: `experiment3.go` already provides reusable package-level helpers for finite Zipf sampling, k-distribution construction, distinct leaf counting, random baseline generation, and statistical distance.
  Evidence: `finiteZipfAddr`, `makeKDistribution`, `countDistinctLeaves`, `makeRandomKDistribution`, and `statisticalDistance` are defined in `RE-MVP-ORAM/experiment3.go`.

## Decision Log

- Decision: Name the mode `zip-kdist` and the file `RE-MVP-ORAM/zip_kdist.go`.
  Rationale: The user wants names that match behavior. This mode is Zipf-driven and measures k-distribution distance.
  Date/Author: 2026-06-14 / Codex
- Decision: Use `NewSynchronizedMvpServer` for this workload.
  Rationale: The user explicitly requested no snapshots for this experiment.
  Date/Author: 2026-06-14 / Codex
- Decision: Warmup uses full `Access`, while measurement uses `GetPM`/position-map consolidation and local leaf selection only, without `GetPS` or `Evict`.
  Rationale: Warmup must mutate ORAM state to create a realistic pre-measurement position map. Measurement should not perturb state, and the user requested Evict is unnecessary.
  Date/Author: 2026-06-14 / Codex
- Decision: In each measurement trial, every client receives its own independently sampled Zipf operation.
  Rationale: The user asked for the distribution of independently selected leaves. This differs from the same-address measurement in the older experiment3.
  Date/Author: 2026-06-14 / Codex

## Outcomes & Retrospective

Implemented `go run . -experiment zip-kdist`. The workload warms up a snapshot-disabled RE-MVP-ORAM server with full Zipf-sampled accesses, then measures independent per-client leaf choices without `GetPS` or `Evict`. It writes `CSV/re_mvp_oram_zip_kdist.csv` with one row per `k`, including observed and random-baseline counts/probabilities, and logs the total variation distance.

Validation passed:

    GOCACHE=/tmp/re-mvp-go-cache go test ./...

Small smoke run passed:

    RE_MVP_ZIP_KDIST_CLIENT_COUNT=5 RE_MVP_ZIP_KDIST_WARMUP=5 RE_MVP_ZIP_KDIST_MONTE_CARLO=20 GOCACHE=/tmp/re-mvp-go-cache go run . -experiment zip-kdist

Smoke result:

    distance=0.0000000000

## Context and Orientation

`RE-MVP-ORAM/main.go` selects execution modes. Existing `experiment3.go` measures distinct leaves for a same-address setup. The new workload should not replace that file. `zip.go` defines Zipf operation sampling that chooses both address and operation type. The new workload can reuse its package-level helper `zipAccessOperation`.

The measured value `k` is the number of distinct leaf buckets among all clients in one trial. The random baseline samples the same number of leaves uniformly and independently from `2^L` leaves. Statistical distance is total variation distance between the observed k-count distribution and the random baseline.

## Plan of Work

Create `RE-MVP-ORAM/zip_kdist.go`. It will create a synchronized server, initialize data, create clients, run warmup rounds with Zipf operations, synchronize client position maps, then run Monte Carlo trials. In each trial, each client independently samples a Zipf operation, consolidates pending path-map updates, chooses the accessed position, computes the leaf via `selectPath`, and records it. No `Evict` is performed during measurement.

Update `RE-MVP-ORAM/main.go` to add `case "zip-kdist": ZipKDistance()`.

## Concrete Steps

Run:

    cd /mnt/c/Users/gento/Desktop/TERM2026/RE-MVP-ORAM
    GOCACHE=/tmp/re-mvp-go-cache go test ./...
    RE_MVP_ZIP_KDIST_CLIENT_COUNT=5 RE_MVP_ZIP_KDIST_WARMUP=5 RE_MVP_ZIP_KDIST_MONTE_CARLO=20 GOCACHE=/tmp/re-mvp-go-cache go run . -experiment zip-kdist

## Validation and Acceptance

The code is accepted when `go test ./...` passes and `go run . -experiment zip-kdist` prints a CSV-like result line including client count, address alpha, operation alpha, Monte Carlo count, and distance. Logs should also explain the selected z, l, n, warmup, and snapshot=false settings.

## Idempotence and Recovery

The workload is additive. It writes no repository files. Runs can be repeated safely.

## Artifacts and Notes

No artifacts yet.

## Interfaces and Dependencies

The new entry point must be:

    func ZipKDistance()

The selectable command must be:

    go run . -experiment zip-kdist
