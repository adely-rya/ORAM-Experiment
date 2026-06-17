package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const statisticalDistanceCSVPath = "CSV/re_mvp_oram_statistical_distance.csv"

type statisticalDistanceConfig struct {
	oramType     string
	accessType   string
	z            int
	l            int
	addrCount    int
	clientCount  int
	warmupRounds int
	trials       int
	seed         int64
	readRatio    float64
	zipfAlpha    float64
	csvPath      string
}

type statisticalDistanceResult struct {
	config        statisticalDistanceConfig
	observed      []int
	ideal         []float64
	distance      float64
	elapsed       time.Duration
	observedTotal int
	accessStats   statisticalDistanceAccessStats
}

type statisticalDistanceLeafInterval struct {
	start int
	end   int
}

type statisticalDistanceAccessStats struct {
	samples           int
	readSamples       int
	writeSamples      int
	activeSigTotal    int
	candidateLeafSum  int
	candidateLeafMin  int
	candidateLeafMax  int
	zeroCandidateOps  int
	fullLeafCandidate int
	candidateByK      map[int]int
	activeSigByCount  map[int]int
}

// RunStatisticalDistanceExperiment は統計的距離実験の設定を読み込み、対象 ORAM の実験を実行して CSV に保存する。
func RunStatisticalDistanceExperiment(oramType string, accessType string) {
	config := readStatisticalDistanceConfig(oramType, accessType)
	log.Printf(
		"statisticaldistance running: oram=%s access=%s z=%d l=%d addr_count=%d clients=%d warmup_rounds=%d trials=%d seed=%d read_ratio=%.6f zipf_alpha=%.6f",
		config.oramType,
		config.accessType,
		config.z,
		config.l,
		config.addrCount,
		config.clientCount,
		config.warmupRounds,
		config.trials,
		config.seed,
		config.readRatio,
		config.zipfAlpha,
	)

	var (
		result statisticalDistanceResult
		err    error
	)
	switch config.oramType {
	case "re-mvp-oram":
		result, err = runReMvpStatisticalDistance(config)
	case "mvp-oram":
		result, err = runBaseMvpStatisticalDistance(config)
	default:
		panic("unknown oram")
	}
	if err != nil {
		log.Printf("statisticaldistance error: %v", err)
		return
	}
	if err := writeStatisticalDistanceCSV(result); err != nil {
		log.Printf("statisticaldistance csv write error: %v", err)
		return
	}

	log.Printf(
		"statisticaldistance result: oram=%s access=%s clients=%d l=%d leaves=%d warmup_rounds=%d trials=%d tvd=%.10f csv=%s elapsed=%s",
		result.config.oramType,
		result.config.accessType,
		result.config.clientCount,
		result.config.l,
		1<<result.config.l,
		result.config.warmupRounds,
		result.config.trials,
		result.distance,
		result.config.csvPath,
		result.elapsed.Round(time.Millisecond),
	)
	logStatisticalDistanceAccessStats(result.config.oramType, result.config.l, result.accessStats)
}

// readStatisticalDistanceConfig は環境変数から統計的距離実験のパラメータを読み込む。
func readStatisticalDistanceConfig(oramType string, accessType string) statisticalDistanceConfig {
	const (
		defaultZ            = 4
		defaultL            = 10
		defaultClientCount  = 40
		defaultWarmupRounds = 300
		defaultTrials       = 200000
		defaultSeed         = 542
		defaultReadRatio    = 0.5
		defaultZipfAlpha    = 1.1
	)

	l := readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_L", defaultL)
	return statisticalDistanceConfig{
		oramType:     normalizeOramType(oramType),
		accessType:   normalizeStatisticalDistanceAccessType(accessType),
		z:            readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_Z", defaultZ),
		l:            l,
		addrCount:    readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_ADDR_COUNT", 1<<(l-1)),
		clientCount:  readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_CLIENT_COUNT", defaultClientCount),
		warmupRounds: readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_WARMUP_ROUNDS", defaultWarmupRounds),
		trials:       readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_TRIALS", defaultTrials),
		seed:         int64(readStatisticalDistancePositiveIntEnv("RE_MVP_STAT_DISTANCE_SEED", defaultSeed)),
		readRatio:    readStatisticalDistanceFloatEnv("RE_MVP_STAT_DISTANCE_READ_RATIO", defaultReadRatio),
		zipfAlpha:    readStatisticalDistanceFloatEnv("RE_MVP_STAT_DISTANCE_ZIPF_ALPHA", defaultZipfAlpha),
		csvPath:      readStatisticalDistanceStringEnv("RE_MVP_STAT_DISTANCE_CSV", statisticalDistanceCSVPath),
	}
}

// runReMvpStatisticalDistance は RE-MVP-ORAM でウォームアップ後、ランダムリーフ選択モデルの K 分布を測定する。
func runReMvpStatisticalDistance(config statisticalDistanceConfig) (statisticalDistanceResult, error) {
	startedAt := time.Now()
	server := NewMvpServer(config.z, config.l)
	positionMap := server.InitializeRandomData(config.addrCount, config.seed)
	go server.Run()

	clients := make([]*statisticalDistanceReMvpClient, 0, config.clientCount)
	for clientID := 0; clientID < config.clientCount; clientID++ {
		clients = append(clients, &statisticalDistanceReMvpClient{
			client:    NewMvpClient(config.l, config.z, clientID, clonePositionMap(positionMap), server.Requests),
			operation: newStatisticalDistanceOperation(config, config.seed+int64(clientID)),
		})
	}

	if err := runReMvpStatisticalDistanceWarmup(clients, config.warmupRounds); err != nil {
		return statisticalDistanceResult{}, err
	}
	fmt.Printf("statisticaldistance warmup complete: oram=%s rounds=%d\n", config.oramType, config.warmupRounds)
	if err := syncReMvpStatisticalDistancePositionMaps(clients); err != nil {
		return statisticalDistanceResult{}, err
	}

	leafRNG := rand.New(rand.NewSource(config.seed + 9001))
	observed, accessStats, err := measureReMvpStatisticalDistance(clients, config.trials, leafRNG)
	if err != nil {
		return statisticalDistanceResult{}, err
	}
	ideal := idealStatisticalDistanceRandomKDistribution(config.l, config.clientCount)
	distance := statisticalDistanceObservedToIdeal(observed, ideal)

	return statisticalDistanceResult{
		config:        config,
		observed:      observed,
		ideal:         ideal,
		distance:      distance,
		elapsed:       time.Since(startedAt),
		observedTotal: config.trials,
		accessStats:   accessStats,
	}, nil
}

type statisticalDistanceReMvpClient struct {
	client    *MvpClient
	operation AccessOperation
}

// runReMvpStatisticalDistanceWarmup は RE-MVP-ORAM の状態を事前に動かすため、全クライアントに指定回数アクセスさせる。
func runReMvpStatisticalDistanceWarmup(clients []*statisticalDistanceReMvpClient, rounds int) error {
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousLogOutput)

	for round := 0; round < rounds; round++ {
		errs := make(chan error, len(clients))
		for _, state := range clients {
			op := state.operation.Next()
			go func(client *MvpClient, op OramOP) {
				if err := client.Access(op); err != nil {
					errs <- err
				} else {
					errs <- nil
				}
			}(state.client, op)
		}
		for range clients {
			if err := <-errs; err != nil {
				return fmt.Errorf("warmup round %d: %w", round, err)
			}
		}
	}
	return nil
}

// syncReMvpStatisticalDistancePositionMaps はウォームアップ後の path map を各 RE-MVP-ORAM クライアントに反映する。
func syncReMvpStatisticalDistancePositionMaps(clients []*statisticalDistanceReMvpClient) error {
	errs := make(chan error, len(clients))
	for _, state := range clients {
		go func(client *MvpClient) {
			version, pathMaps, err := client.GetPM()
			if err != nil {
				errs <- err
				return
			}
			client.seq = version
			client.consolidatePathMaps(pathMaps)
			errs <- nil
		}(state.client)
	}
	for range clients {
		if err := <-errs; err != nil {
			return err
		}
	}
	return nil
}

type statisticalDistanceReMvpPreparedAccess struct {
	client    *MvpClient
	operation OramOP
	signature int
	leaf      MvpPosition
}

// measureReMvpStatisticalDistance は設定されたアクセス列で RE-MVP-ORAM を実際に動かし、各試行の異なるリーフ数 K を数える。
func measureReMvpStatisticalDistance(clients []*statisticalDistanceReMvpClient, trials int, leafRNG *rand.Rand) ([]int, statisticalDistanceAccessStats, error) {
	counts := make([]int, len(clients)+1)
	stats := newStatisticalDistanceAccessStats()
	for trial := 0; trial < trials; trial++ {
		seen := make(map[MvpBucketPosition]struct{}, len(clients))
		prepared := make([]statisticalDistanceReMvpPreparedAccess, 0, len(clients))
		for _, state := range clients {
			op := state.operation.Next()
			version, pathMaps, err := state.client.GetPM()
			if err != nil {
				return nil, statisticalDistanceAccessStats{}, fmt.Errorf("trial %d client %d getpm: %w", trial, state.client.ClientID, err)
			}
			state.client.seq = version
			state.client.consolidatePathMaps(pathMaps)

			signature, leaf, ok := chooseReMvpStatisticalDistanceTargetLeaf(state.client, op.target, op.OP, leafRNG)
			recordReMvpStatisticalDistanceAccessStats(&stats, state.client, op)
			if !ok {
				continue
			}
			seen[leaf.bucket] = struct{}{}
			prepared = append(prepared, statisticalDistanceReMvpPreparedAccess{
				client:    state.client,
				operation: op,
				signature: signature,
				leaf:      leaf,
			})
		}
		counts[len(seen)]++
		if err := runPreparedReMvpStatisticalDistanceAccesses(prepared); err != nil {
			return nil, statisticalDistanceAccessStats{}, fmt.Errorf("trial %d: %w", trial, err)
		}
		logStatisticalDistanceTrialProgress(trial+1, trials)
	}
	return counts, stats, nil
}

// runPreparedReMvpStatisticalDistanceAccesses は測定済みのリーフを使って RE-MVP-ORAM のアクセスを実行し、状態を進める。
func runPreparedReMvpStatisticalDistanceAccesses(accesses []statisticalDistanceReMvpPreparedAccess) error {
	errs := make(chan error, len(accesses))
	for _, access := range accesses {
		go func(access statisticalDistanceReMvpPreparedAccess) {
			path, stash, err := access.client.GetPS(access.leaf)
			if err != nil {
				errs <- err
				return
			}
			access.client.path = path
			access.client.Stash = stash

			workingSet := access.client.mergePathStashes()
			populatedPath, populatedStash, populatedPathMap := access.client.populatePath(workingSet, access.operation, access.signature)
			errs <- access.client.Evict(populatedPath, populatedPathMap, populatedStash)
		}(access)
	}
	for range accesses {
		if err := <-errs; err != nil {
			return err
		}
	}
	return nil
}

// chooseReMvpStatisticalDistanceTargetLeaf は RE-MVP-ORAM の position map から指定アドレスが載り得るリーフを選ぶ。
func chooseReMvpStatisticalDistanceTargetLeaf(client *MvpClient, addr int, op string, rng *rand.Rand) (int, MvpPosition, bool) {
	entries, ok := client.PositionMap[addr]
	if !ok || len(entries) == 0 {
		return 0, MvpPosition{}, false
	}

	if op == Write {
		entry, ok := entries[0]
		return 0, chooseReMvpStatisticalDistanceLeafFromPosition(entry.Slot, client.L, rng), ok
	}

	signatures := make([]int, 0, len(entries))
	for sig, position := range entries {
		if position.Slot != mvpDeletePosition {
			signatures = append(signatures, sig)
		}
	}
	if len(signatures) == 0 {
		return 0, MvpPosition{}, false
	}

	intervals := make([]statisticalDistanceLeafInterval, 0, len(signatures))
	for _, sig := range signatures {
		bucketPosition := entries[sig].Slot.bucket
		if entries[sig].Slot == mvpStashPosition || bucketPosition == mvpStashBucketPosition || bucketPosition == mvpRootBucketPosition {
			bucketPosition = ""
		}

		prefix := bucketPosition.String()
		if len(prefix) > client.L {
			prefix = ""
		}
		intervals = append(intervals, statisticalDistancePrefixInterval(prefix, client.L))
	}
	merged := mergeStatisticalDistanceLeafIntervals(intervals)
	if len(merged) == 0 {
		return 0, MvpPosition{}, false
	}

	return 0, statisticalDistanceSampleLeafFromIntervals(merged, client.L, rng), true
}

// recordReMvpStatisticalDistanceAccessStats は RE-MVP-ORAM の現在の position map から active sig 数と候補 leaf 数を集計する。
func recordReMvpStatisticalDistanceAccessStats(stats *statisticalDistanceAccessStats, client *MvpClient, op OramOP) {
	entries, ok := client.PositionMap[op.target]
	if !ok || len(entries) == 0 {
		stats.record(op.OP, 0, 0, client.L)
		return
	}

	activeSigCount := 0
	intervals := make([]statisticalDistanceLeafInterval, 0, len(entries))
	if op.OP == Write {
		entry, ok := entries[0]
		if !ok || entry.Slot == mvpDeletePosition {
			stats.record(op.OP, 0, 0, client.L)
			return
		}
		activeSigCount = 1
		intervals = append(intervals, statisticalDistancePositionInterval(entry.Slot, client.L))
	} else {
		for _, entry := range entries {
			if entry.Slot == mvpDeletePosition {
				continue
			}
			activeSigCount++
			intervals = append(intervals, statisticalDistancePositionInterval(entry.Slot, client.L))
		}
	}

	stats.record(op.OP, activeSigCount, statisticalDistanceIntervalLeafCount(mergeStatisticalDistanceLeafIntervals(intervals)), client.L)
}

// chooseReMvpStatisticalDistanceLeafFromPosition は RE-MVP-ORAM の位置をリーフまでランダムに補完する。
func chooseReMvpStatisticalDistanceLeafFromPosition(position MvpPosition, pathLen int, rng *rand.Rand) MvpPosition {
	bucketPosition := position.bucket
	if position == mvpStashPosition || bucketPosition == mvpStashBucketPosition || bucketPosition == mvpRootBucketPosition {
		bucketPosition = ""
	}
	leaf := bucketPosition.String()
	if len(leaf) > pathLen {
		leaf = ""
	}
	for len(leaf) < pathLen {
		if rng.Intn(2) == 0 {
			leaf += "0"
		} else {
			leaf += "1"
		}
	}
	return MvpPosition{bucket: MvpBucketPosition(leaf)}
}

// runBaseMvpStatisticalDistance は通常の MVP-ORAM でウォームアップ後、ランダムリーフ選択モデルの K 分布を測定する。
func runBaseMvpStatisticalDistance(config statisticalDistanceConfig) (statisticalDistanceResult, error) {
	startedAt := time.Now()
	server := NewBaseMvpServer(config.z, config.l)
	positionMap := server.InitializeRandomData(config.addrCount, config.seed)
	go server.Run()

	clients := make([]*statisticalDistanceBaseMvpClient, 0, config.clientCount)
	for clientID := 0; clientID < config.clientCount; clientID++ {
		clients = append(clients, &statisticalDistanceBaseMvpClient{
			client:    NewBaseMvpClient(config.l, config.z, clientID, cloneBaseMvpPositionMap(positionMap), server.Requests),
			operation: newStatisticalDistanceOperation(config, config.seed+int64(clientID)),
		})
	}

	if err := runBaseMvpStatisticalDistanceWarmup(clients, config.warmupRounds); err != nil {
		return statisticalDistanceResult{}, err
	}
	fmt.Printf("statisticaldistance warmup complete: oram=%s rounds=%d\n", config.oramType, config.warmupRounds)
	if err := syncBaseMvpStatisticalDistancePositionMaps(clients); err != nil {
		return statisticalDistanceResult{}, err
	}

	leafRNG := rand.New(rand.NewSource(config.seed + 9001))
	observed, accessStats, err := measureBaseMvpStatisticalDistance(clients, config.trials, leafRNG)
	if err != nil {
		return statisticalDistanceResult{}, err
	}
	ideal := idealStatisticalDistanceRandomKDistribution(config.l, config.clientCount)
	distance := statisticalDistanceObservedToIdeal(observed, ideal)

	return statisticalDistanceResult{
		config:        config,
		observed:      observed,
		ideal:         ideal,
		distance:      distance,
		elapsed:       time.Since(startedAt),
		observedTotal: config.trials,
		accessStats:   accessStats,
	}, nil
}

type statisticalDistanceBaseMvpClient struct {
	client    *BaseMvpClient
	operation AccessOperation
}

// runBaseMvpStatisticalDistanceWarmup は通常の MVP-ORAM の状態を事前に動かすため、全クライアントに指定回数アクセスさせる。
func runBaseMvpStatisticalDistanceWarmup(clients []*statisticalDistanceBaseMvpClient, rounds int) error {
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousLogOutput)

	for round := 0; round < rounds; round++ {
		errs := make(chan error, len(clients))
		for _, state := range clients {
			op := state.operation.Next()
			go func(client *BaseMvpClient, op OramOP) {
				if err := client.Access(op); err != nil {
					errs <- err
				} else {
					errs <- nil
				}
			}(state.client, op)
		}
		for range clients {
			if err := <-errs; err != nil {
				return fmt.Errorf("warmup round %d: %w", round, err)
			}
		}
	}
	return nil
}

// syncBaseMvpStatisticalDistancePositionMaps はウォームアップ後の path map を各 MVP-ORAM クライアントに反映する。
func syncBaseMvpStatisticalDistancePositionMaps(clients []*statisticalDistanceBaseMvpClient) error {
	errs := make(chan error, len(clients))
	for _, state := range clients {
		go func(client *BaseMvpClient) {
			version, pathMaps, err := client.GetPM()
			if err != nil {
				errs <- err
				return
			}
			client.seq = version
			client.consolidatePathMaps(pathMaps)
			errs <- nil
		}(state.client)
	}
	for range clients {
		if err := <-errs; err != nil {
			return err
		}
	}
	return nil
}

type statisticalDistanceBaseMvpPreparedAccess struct {
	client    *BaseMvpClient
	operation OramOP
	leaf      BaseMvpPosition
}

// measureBaseMvpStatisticalDistance は設定されたアクセス列で通常の MVP-ORAM を実際に動かし、各試行の異なるリーフ数 K を数える。
func measureBaseMvpStatisticalDistance(clients []*statisticalDistanceBaseMvpClient, trials int, leafRNG *rand.Rand) ([]int, statisticalDistanceAccessStats, error) {
	counts := make([]int, len(clients)+1)
	stats := newStatisticalDistanceAccessStats()
	for trial := 0; trial < trials; trial++ {
		seen := make(map[BaseMvpBucketPosition]struct{}, len(clients))
		prepared := make([]statisticalDistanceBaseMvpPreparedAccess, 0, len(clients))
		for _, state := range clients {
			op := state.operation.Next()
			version, pathMaps, err := state.client.GetPM()
			if err != nil {
				return nil, statisticalDistanceAccessStats{}, fmt.Errorf("trial %d client %d getpm: %w", trial, state.client.ClientID, err)
			}
			state.client.seq = version
			state.client.consolidatePathMaps(pathMaps)

			entry, ok := state.client.PositionMap[op.target]
			if !ok {
				continue
			}
			recordBaseMvpStatisticalDistanceAccessStats(&stats, state.client, op, entry)
			leaf := chooseBaseMvpStatisticalDistanceLeafFromPosition(entry.Slot, state.client.L, leafRNG)
			seen[leaf.bucket] = struct{}{}
			prepared = append(prepared, statisticalDistanceBaseMvpPreparedAccess{
				client:    state.client,
				operation: op,
				leaf:      leaf,
			})
		}
		counts[len(seen)]++
		if err := runPreparedBaseMvpStatisticalDistanceAccesses(prepared); err != nil {
			return nil, statisticalDistanceAccessStats{}, fmt.Errorf("trial %d: %w", trial, err)
		}
		logStatisticalDistanceTrialProgress(trial+1, trials)
	}
	return counts, stats, nil
}

// runPreparedBaseMvpStatisticalDistanceAccesses は測定済みのリーフを使って通常の MVP-ORAM のアクセスを実行し、状態を進める。
func runPreparedBaseMvpStatisticalDistanceAccesses(accesses []statisticalDistanceBaseMvpPreparedAccess) error {
	errs := make(chan error, len(accesses))
	for _, access := range accesses {
		go func(access statisticalDistanceBaseMvpPreparedAccess) {
			basePath, stash, err := access.client.GetPS(access.leaf)
			if err != nil {
				errs <- err
				return
			}
			access.client.basePath = basePath
			access.client.Stash = stash

			workingSet := access.client.mergePathStashes()
			targetBlock, ok := workingSet[access.operation.target]
			if !ok {
				errs <- fmt.Errorf("not target block in working set")
				return
			}
			if access.operation.OP == Write {
				targetBlock.Data = access.operation.param
				targetBlock.Version = Versions{access.client.seq, access.client.seq, access.client.seq}
			} else {
				targetBlock.Version.SetA(access.client.seq)
			}
			workingSet[access.operation.target] = targetBlock

			populatedPath, populatedStash, populatedPathMap := access.client.populatePath(workingSet, access.operation)
			errs <- access.client.Evict(populatedPath, populatedPathMap, populatedStash)
		}(access)
	}
	for range accesses {
		if err := <-errs; err != nil {
			return err
		}
	}
	return nil
}

// logStatisticalDistanceTrialProgress は統計的距離実験の試行進捗を一定間隔で表示する。
func logStatisticalDistanceTrialProgress(done int, total int) {
	if done%100 != 0 && done != total {
		return
	}
	fmt.Printf("statisticaldistance trial progress: %d/%d\n", done, total)
}

func newStatisticalDistanceAccessStats() statisticalDistanceAccessStats {
	return statisticalDistanceAccessStats{
		candidateLeafMin: 1 << 30,
		candidateByK:     make(map[int]int),
		activeSigByCount: make(map[int]int),
	}
}

func (s *statisticalDistanceAccessStats) record(op string, activeSigCount int, candidateLeafCount int, pathLen int) {
	s.samples++
	if op == Read {
		s.readSamples++
	} else if op == Write {
		s.writeSamples++
	}
	s.activeSigTotal += activeSigCount
	s.candidateLeafSum += candidateLeafCount
	if candidateLeafCount < s.candidateLeafMin {
		s.candidateLeafMin = candidateLeafCount
	}
	if candidateLeafCount > s.candidateLeafMax {
		s.candidateLeafMax = candidateLeafCount
	}
	if candidateLeafCount == 0 {
		s.zeroCandidateOps++
	}
	if candidateLeafCount == 1<<pathLen {
		s.fullLeafCandidate++
	}
	s.candidateByK[candidateLeafCount]++
	s.activeSigByCount[activeSigCount]++
}

// recordBaseMvpStatisticalDistanceAccessStats は通常の MVP-ORAM の現在位置から候補 leaf 数を集計する。
func recordBaseMvpStatisticalDistanceAccessStats(stats *statisticalDistanceAccessStats, client *BaseMvpClient, op OramOP, entry BaseMvpPositionMapEntry) {
	stats.record(op.OP, 1, statisticalDistanceBasePositionLeafCount(entry.Slot, client.L), client.L)
}

// logStatisticalDistanceAccessStats は実測アクセスでクライアントから見えた候補数の要約を出力する。
func logStatisticalDistanceAccessStats(oramType string, pathLen int, stats statisticalDistanceAccessStats) {
	if stats.samples == 0 {
		log.Printf("statisticaldistance access stats: oram=%s samples=0", oramType)
		return
	}

	log.Printf(
		"statisticaldistance access stats: oram=%s samples=%d reads=%d writes=%d avg_active_sig=%.4f avg_candidate_leaves=%.4f min_candidate_leaves=%d max_candidate_leaves=%d full_leaf_candidates=%d zero_candidates=%d leaf_count=%d",
		oramType,
		stats.samples,
		stats.readSamples,
		stats.writeSamples,
		float64(stats.activeSigTotal)/float64(stats.samples),
		float64(stats.candidateLeafSum)/float64(stats.samples),
		stats.candidateLeafMin,
		stats.candidateLeafMax,
		stats.fullLeafCandidate,
		stats.zeroCandidateOps,
		1<<pathLen,
	)
	log.Printf("statisticaldistance access stats buckets: oram=%s candidate_leaf_counts=%s active_sig_counts=%s", oramType, formatStatisticalDistanceIntHistogram(stats.candidateByK), formatStatisticalDistanceIntHistogram(stats.activeSigByCount))
}

func formatStatisticalDistanceIntHistogram(histogram map[int]int) string {
	if len(histogram) == 0 {
		return "{}"
	}
	keys := make([]int, 0, len(histogram))
	for key := range histogram {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%d:%d", key, histogram[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// chooseBaseMvpStatisticalDistanceLeafFromPosition は通常の MVP-ORAM の位置をリーフまでランダムに補完する。
func chooseBaseMvpStatisticalDistanceLeafFromPosition(position BaseMvpPosition, pathLen int, rng *rand.Rand) BaseMvpPosition {
	bucketPosition := position.bucket
	if position == baseMvpStashPosition || bucketPosition == baseMvpStashBucketPosition || bucketPosition == baseMvpRootBucketPosition {
		bucketPosition = ""
	}
	leaf := bucketPosition.String()
	if len(leaf) > pathLen {
		leaf = ""
	}
	for len(leaf) < pathLen {
		if rng.Intn(2) == 0 {
			leaf += "0"
		} else {
			leaf += "1"
		}
	}
	return BaseMvpPosition{bucket: BaseMvpBucketPosition(leaf)}
}

func statisticalDistancePositionInterval(position MvpPosition, pathLen int) statisticalDistanceLeafInterval {
	bucketPosition := position.bucket
	if position == mvpStashPosition || bucketPosition == mvpStashBucketPosition || bucketPosition == mvpRootBucketPosition {
		bucketPosition = ""
	}

	prefix := bucketPosition.String()
	if len(prefix) > pathLen {
		prefix = ""
	}
	return statisticalDistancePrefixInterval(prefix, pathLen)
}

func statisticalDistanceBasePositionLeafCount(position BaseMvpPosition, pathLen int) int {
	bucketPosition := position.bucket
	if position == baseMvpStashPosition || bucketPosition == baseMvpStashBucketPosition || bucketPosition == baseMvpRootBucketPosition {
		bucketPosition = ""
	}

	prefix := bucketPosition.String()
	if len(prefix) > pathLen {
		prefix = ""
	}
	return 1 << (pathLen - len(prefix))
}

// statisticalDistancePrefixInterval はリーフ接頭辞が表す整数区間 [start, end) を返す。
func statisticalDistancePrefixInterval(prefix string, pathLen int) statisticalDistanceLeafInterval {
	start := 0
	if prefix != "" {
		parsed, err := strconv.ParseInt(prefix, 2, 64)
		if err != nil {
			return statisticalDistanceLeafInterval{start: 0, end: 1 << pathLen}
		}
		start = int(parsed) << (pathLen - len(prefix))
	}
	width := 1 << (pathLen - len(prefix))
	return statisticalDistanceLeafInterval{start: start, end: start + width}
}

// mergeStatisticalDistanceLeafIntervals は重なったリーフ区間を併合して重複のない区間列にする。
func mergeStatisticalDistanceLeafIntervals(intervals []statisticalDistanceLeafInterval) []statisticalDistanceLeafInterval {
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool {
		if intervals[i].start != intervals[j].start {
			return intervals[i].start < intervals[j].start
		}
		return intervals[i].end < intervals[j].end
	})

	merged := make([]statisticalDistanceLeafInterval, 0, len(intervals))
	for _, interval := range intervals {
		if interval.end <= interval.start {
			continue
		}
		if len(merged) == 0 || interval.start > merged[len(merged)-1].end {
			merged = append(merged, interval)
			continue
		}
		if interval.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = interval.end
		}
	}
	return merged
}

func statisticalDistanceIntervalLeafCount(intervals []statisticalDistanceLeafInterval) int {
	total := 0
	for _, interval := range intervals {
		total += interval.end - interval.start
	}
	return total
}

// statisticalDistanceSampleLeafFromIntervals は複数のリーフ区間の和集合から一様ランダムに 1 つのリーフを選ぶ。
func statisticalDistanceSampleLeafFromIntervals(intervals []statisticalDistanceLeafInterval, pathLen int, rng *rand.Rand) MvpPosition {
	total := 0
	for _, interval := range intervals {
		total += interval.end - interval.start
	}
	selected := rng.Intn(total)
	leafIndex := 0
	for _, interval := range intervals {
		width := interval.end - interval.start
		if selected < width {
			leafIndex = interval.start + selected
			break
		}
		selected -= width
	}

	leaf := strconv.FormatInt(int64(leafIndex), 2)
	for len(leaf) < pathLen {
		leaf = "0" + leaf
	}
	return MvpPosition{bucket: MvpBucketPosition(leaf)}
}

// newStatisticalDistanceOperation は実測値を取るため、設定された random または Zipf のアクセス生成器を作る。
func newStatisticalDistanceOperation(config statisticalDistanceConfig, seed int64) AccessOperation {
	switch config.accessType {
	case "random":
		return NewUniformAccessOperation(config.addrCount, seed)
	case "zipf":
		return NewZipfAccessOperation(config.addrCount, seed, config.readRatio, config.zipfAlpha)
	default:
		panic("unknown accesstype")
	}
}

// idealStatisticalDistanceRandomKDistribution は clientCount 回だけ独立に一様ランダムなリーフを選んだときの K 分布を計算する。
func idealStatisticalDistanceRandomKDistribution(pathLen int, clientCount int) []float64 {
	leafCountInt := 1 << pathLen
	leafCount := float64(leafCountInt)
	distribution := make([]float64, clientCount+1)
	distribution[0] = 1

	for draw := 0; draw < clientCount; draw++ {
		next := make([]float64, clientCount+1)
		for k := 0; k <= draw; k++ {
			probability := distribution[k]
			if probability == 0 {
				continue
			}
			if k > 0 {
				next[k] += probability * float64(k) / leafCount
			}
			next[k+1] += probability * (leafCount - float64(k)) / leafCount
		}
		distribution = next
	}
	return distribution
}

// statisticalDistanceObservedToIdeal は観測分布と理論分布の全変動距離を計算する。
func statisticalDistanceObservedToIdeal(observed []int, ideal []float64) float64 {
	total := 0
	for _, count := range observed {
		total += count
	}
	if total == 0 {
		return 0
	}

	sum := 0.0
	for k := 0; k < len(observed) || k < len(ideal); k++ {
		observedProbability := 0.0
		if k < len(observed) {
			observedProbability = float64(observed[k]) / float64(total)
		}
		idealProbability := 0.0
		if k < len(ideal) {
			idealProbability = ideal[k]
		}
		sum += math.Abs(observedProbability - idealProbability)
	}
	return sum / 2
}

// writeStatisticalDistanceCSV は統計的距離実験の観測値、理論値、差分を CSV に出力する。
func writeStatisticalDistanceCSV(result statisticalDistanceResult) error {
	if err := os.MkdirAll(filepath.Dir(result.config.csvPath), 0755); err != nil {
		return err
	}

	file, err := os.Create(result.config.csvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		"experiment",
		"oram",
		"access",
		"l",
		"leaf_count",
		"addr_count",
		"client_count",
		"warmup_rounds",
		"trials",
		"read_ratio",
		"zipf_alpha",
		"seed",
		"tvd",
		"k",
		"observed_count",
		"observed_probability",
		"ideal_probability",
		"abs_diff",
	}); err != nil {
		return err
	}

	for k := 1; k <= result.config.clientCount; k++ {
		observedProbability := 0.0
		if result.observedTotal > 0 {
			observedProbability = float64(result.observed[k]) / float64(result.observedTotal)
		}
		idealProbability := result.ideal[k]
		if err := writer.Write([]string{
			"statisticaldistance",
			result.config.oramType,
			result.config.accessType,
			strconv.Itoa(result.config.l),
			strconv.Itoa(1 << result.config.l),
			strconv.Itoa(result.config.addrCount),
			strconv.Itoa(result.config.clientCount),
			strconv.Itoa(result.config.warmupRounds),
			strconv.Itoa(result.config.trials),
			strconv.FormatFloat(result.config.readRatio, 'f', 6, 64),
			strconv.FormatFloat(result.config.zipfAlpha, 'f', 6, 64),
			strconv.FormatInt(result.config.seed, 10),
			strconv.FormatFloat(result.distance, 'f', 10, 64),
			strconv.Itoa(k),
			strconv.Itoa(result.observed[k]),
			strconv.FormatFloat(observedProbability, 'f', 10, 64),
			strconv.FormatFloat(idealProbability, 'f', 10, 64),
			strconv.FormatFloat(math.Abs(observedProbability-idealProbability), 'f', 10, 64),
		}); err != nil {
			return err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return file.Sync()
}

// normalizeStatisticalDistanceAccessType は統計的距離実験で使う実測側アクセス種別を正規化する。
func normalizeStatisticalDistanceAccessType(accessType string) string {
	switch accessType {
	case "random", "zipf":
		return accessType
	default:
		panic("unknown accesstype")
	}
}

// readStatisticalDistancePositiveIntEnv は正の整数の環境変数を読み、無効なら fallback を返す。
func readStatisticalDistancePositiveIntEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		log.Printf("invalid %s=%q; using %d", name, value, fallback)
		return fallback
	}
	return parsed
}

// readStatisticalDistanceFloatEnv は浮動小数点数の環境変数を読み、無効なら fallback を返す。
func readStatisticalDistanceFloatEnv(name string, fallback float64) float64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("invalid %s=%q; using %.6f", name, value, fallback)
		return fallback
	}
	return parsed
}

// readStatisticalDistanceStringEnv は文字列の環境変数を読み、未設定なら fallback を返す。
func readStatisticalDistanceStringEnv(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
