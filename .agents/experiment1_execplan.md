# Implement synchronized MVP-ORAM Experiment 1

This ExecPlan is a living document. It follows `.agents/PLANS.md` and must keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date.

## Purpose / Big Picture

The goal is to make `mvp-oram/experiment1.go` run a synchronized Monte Carlo experiment for multiple client counts and Zipf alpha values. Each `(client count, alpha)` experiment starts from the same initial server state before warmup, runs independently in a goroutine, records the number `k` of distinct leaves selected by the clients in each synchronized trial, compares that `k` distribution against a purely random leaf selection baseline, and reports statistical distance.

## Progress

- [x] (2026-06-04T08:18:20Z) Read the current `experiment1.go` and related server/client APIs.
- [x] (2026-06-04T08:24:00Z) Replaced the partial experiment runner with a complete, simple synchronized runner.
- [x] (2026-06-04T08:24:00Z) Added statistical-distance and random-leaf distribution helpers.
- [x] (2026-06-04T08:24:00Z) Ran formatting and `GO111MODULE=off GOCACHE=/tmp/go-build go test ./mvp-oram`.
- [x] (2026-06-04T08:44:04Z) Corrected the measured distribution from per-leaf counts to per-trial distinct-leaf count `k`.
- [x] (2026-06-04T08:44:04Z) Updated the random baseline to sample `clientCount` uniform leaves per trial and count distinct leaves.
- [x] (2026-06-04T14:05:00Z) Aligned `mvp-oram/mvp_oram.go` `populatePath` with the artifact so stash blocks can be substituted into any slot on the accessed path, not only slots that already held a block.
- [x] (2026-06-04T14:22:10Z) Verified with temporary diagnostics that PositionMap no longer collapses to all-stash and `experiment3` reports non-trivial statistical distances.
- [x] (2026-06-04T14:45:00Z) Compared synchronized stash growth against `MVP-ORAM-Artifact` client/server functions and removed temporary diagnostics.

## Surprises & Discoveries

- Observation: The current `client_set` is passed by value in goroutines, so populated path/stash results written inside `GetPmPs` do not reliably update the caller's slice entry.
  Evidence: `GetPmPs(c client_set, ...)` assigns `c.populatePath = populatedPath`; because `c` is a copy, the outer `Clients` entry is not updated.
- Observation: `opgenerater(a, len(positionMap))` can select an address equal to `len(positionMap)` because `rand.NewZipf`'s `imax` is inclusive.
  Evidence: Existing initialization creates addresses `0` through `n-1`, so `addrMax` should be `n-1`.
- Observation: In no-snapshot synchronized mode, multiple clients can access the same address in one synchronized phase. If an older eviction runs after a newer eviction for the same address, deleting by address alone removes the newer live version and causes `Not target block in working set` later.
  Evidence: The user reported `panic: Not target block in working set: client=0 addr=2` during Zipf-heavy experiment execution. The fix preserves live path/stash blocks whose block timestamp is newer than the current eviction's output timestamp for that address.
- Observation: The intended experiment distribution is not a histogram over leaf identities. It is the probability mass function of `X`, where `X` is the number of distinct leaves selected by `c` simultaneous clients.
  Evidence: The user provided the formula `Pr(X=k | C=c) = P(2^L,k) * S(c,k) / 2^(L*c)` and clarified that `k=1` is the worst collision case.
- Observation: The Go `populatePath` can move nearly every block into stash because it selects replacement slots only from `usedSlot`, the slots that already received blocks during initial bucket placement.
  Evidence: A temporary stash-growth test showed PositionMap stash counts reaching `256/256` blocks for alpha `0.1` by step 80 and `254/256` blocks for alpha `1.0` by step 100.
- Observation: The artifact `PositionMap` stores an exact slot location plus block version/access plus a separate `locationUpdateAccess`; it does not store only bucket position.
  Evidence: `PositionMap.java` has `locations`, `versions`, `accesses`, and `locationUpdateAccesses`; `mergePaths` checks exact bucket id and slot index.
- Observation: The current Go synchronized no-snapshot server is not equivalent to the artifact's outstanding-tree server for multi-client synchronized phases.
  Evidence: With 10 synchronized clients and 200 warmup rounds, a temporary diagnostic saw roughly 190-220 of 256 addresses in PM stash, while a single normal client stayed around 1-3 PM stash entries. Artifact `ORAMTreeManager.storeBuckets` keeps new evictions as outstanding versions and removes only the versions captured by the client's outstanding tree.
- Observation: Go currently conflates block timestamp and location-update timestamp in `Versions.S`.
  Evidence: Artifact `Block` carries version/access, while path-map update sequence is compared against `positionMap.getLocationUpdateAccess`. Go changes `block.Version.S` during movement and compares full `Versions` during merge/consolidation.

## Decision Log

- Decision: Keep this work confined to `mvp-oram/experiment1.go`.
  Rationale: The user requested experiment code changes and explicitly wants the normal asynchronous implementation preserved.
  Date/Author: 2026-06-04 / Codex
- Decision: Use `NewSynchronizedMvpServer` for each independent experiment and initialize each with the same constants and seed.
  Rationale: This preserves identical pre-warmup state across all experiment combinations while avoiding snapshots during synchronized phases.
  Date/Author: 2026-06-04 / Codex
- Decision: Record the leaf passed to `GetPS`, not the raw position-map slot.
  Rationale: The requested output compares distributions over leaves; when a block is in stash or root, `selectPath` chooses a concrete leaf and that is the accessed leaf.
  Date/Author: 2026-06-04 / Codex
- Decision: Replace the output distribution with counts for `k=1..clientCount`.
  Rationale: The experiment should compare collision patterns among simultaneous clients, not which concrete leaf labels were hit.
  Date/Author: 2026-06-04 / Codex
- Decision: In no-snapshot eviction, delete only old live versions for addresses output by the current eviction, and keep any newer live version of the same address.
  Rationale: Synchronized experiments can generate duplicate Zipf addresses in the same phase. The physical tree/stash must retain the newest committed version because clients consolidate PathMaps by timestamp and will later point to that newest version.
  Date/Author: 2026-06-04 / Codex
- Decision: Match artifact substitution semantics by selecting swap slots from the full accessed path, while still forcing the accessed block's initially placed slot into the substitution set when the target was on-path.
  Rationale: The artifact's `selectRandomSlots` samples from path capacity, allowing stash blocks to return into empty path slots. Limiting substitution to already-used slots prevents recovery once the tree becomes sparse.
  Date/Author: 2026-06-04 / Codex
- Decision: Keep the large location-timestamp refactor as a proposed follow-up, not an immediate patch.
  Rationale: Separating block version/access from location update access touches `Versions`, `path`, `MvpPositionMapEntry`, consolidation, merging, eviction, and tests, so it should be treated as a protocol-level implementation change.
  Date/Author: 2026-06-04 / Codex

## Outcomes & Retrospective

Implemented `Experiment1()` as a parallel set of independent synchronized experiments over client counts and alpha values. Each experiment creates a fresh synchronized server with the same seed, warms up without recording leaves, records the distinct-leaf count `k` for each Monte Carlo trial, compares the resulting `k` distribution against a uniformly random baseline with the same number of clients and trials, and emits CSV rows. The main experiment switch now calls `Experiment1()` for `-experiment experiment1`. A follow-up fix corrected no-snapshot eviction so duplicate same-address accesses in one synchronized phase do not delete newer live versions.

The artifact-aligned `populatePath` change now lets stash blocks return to empty slots on the accessed path. A temporary diagnostic after the change showed `tree_pm` remaining nonzero through 100 synchronized steps instead of collapsing to zero. `experiment3` also moved from near-random distances around `0.009..0.013` to non-trivial values such as `0.207` for `(clients=10, alpha=0.1)` and `0.151` for `(clients=10, alpha=1.0)` under the current parameters.

The later artifact comparison showed that the synchronized no-snapshot server still produces excessive stash in multi-client warmup, while single-client normal access remains low. The likely correction is not another small `populatePath` tweak: the implementation should separate block version/access from location update sequence and either keep artifact-style outstanding tree semantics for concurrent evictions or introduce a phase-level merge that is explicitly equivalent to those semantics.

## Context and Orientation

`mvp-oram/experiment1.go` is a package-main experiment file selected by running `go run . -experiment experiment1` from `mvp-oram`. `MvpClient.GetPM`, `MvpClient.GetPS`, and `MvpClient.Evict` communicate with `MvpServer` through request channels. `NewSynchronizedMvpServer` is the constructor for the no-snapshot server intended for synchronized experiments. An ORAM access has three phases in this experiment: all clients run GetPM/GetPS and prepare eviction data, then all clients evict, and then the next trial begins.

## Plan of Work

Edit `mvp-oram/experiment1.go` only. Define a small result type for `(clientCount, alpha, distance)`. Implement `Experiment1` so it creates one goroutine per client-count and alpha combination, each goroutine calling a `leafDistribution` helper. Make `leafDistribution` create its own synchronized server with the shared seed, warm up without recording results, run Monte Carlo trials while recording the number of distinct selected leaves `k`, evict after each synchronized access phase, then compute statistical distance against a random `k` distribution with the same number of clients and trials. Keep the helper comments short and focused.

## Concrete Steps

From repository root `/mnt/c/Users/gento/Desktop/TERM2026`, edit `mvp-oram/experiment1.go`, run `gofmt -w mvp-oram/experiment1.go`, then run `GO111MODULE=off GOCACHE=/tmp/go-build go test ./mvp-oram`.

## Validation and Acceptance

The change is accepted when `go test ./mvp-oram` passes and `experiment1.go` compiles. The experiment code should expose readable functions for generating Zipf addresses, running synchronized phases, generating a random `k` distribution, and computing statistical distance.

## Idempotence and Recovery

The edits are source-only. Re-running formatting and tests is safe. If a test fails, inspect only `mvp-oram/experiment1.go` first because the intended change scope is limited to that file.

## Artifacts and Notes

Validation output:

    ok  	_/mnt/c/Users/gento/Desktop/TERM2026/mvp-oram	0.006s
    ok  	_/mnt/c/Users/gento/Desktop/TERM2026/mvp-oram	0.003s
    ok  	_/mnt/c/Users/gento/Desktop/TERM2026/mvp-oram	0.003s
    ok  	mvp-oram	0.002s

Experiment3 after artifact-aligned populatePath:

    client_count,alpha,k_distribution_statistical_distance
    10,0.100000,0.2070000000
    10,1.000000,0.1510000000
    15,0.100000,0.1440000000
    15,1.000000,0.0910000000

Revision note: Updated after implementation to record completed work, validation output, and the fact that `main.go` now invokes `Experiment1()` in the `experiment1` case. Updated again after the no-snapshot duplicate-address eviction bug was fixed and validated. Updated again after correcting the metric from concrete leaf distribution to distinct-leaf-count distribution. Updated again after aligning `populatePath` with the artifact's full-path slot substitution and validating Experiment3.

## Interfaces and Dependencies

`Experiment1()` remains the entry point called from `mvp-oram/main.go`. `opgenerater(a float32, addrMax int) OramOP` remains available and selects an address in `0..addrMax`. `makeKDistribution(clientCount int)` creates the `k=1..clientCount` histogram, `countDistinctLeaves(leaves []MvpPosition)` computes one trial's `k`, and `makeRandomKDistribution(l, clientCount, monteCarlo, seed)` creates the random baseline.
