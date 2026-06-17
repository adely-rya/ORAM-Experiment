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
}

type statisticalDistanceLeafInterval struct {
	start int
	end   int
}

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
}

func readStatisticalDistanceConfig(oramType string, accessType string) statisticalDistanceConfig {
	const (
		defaultZ            = 4
		defaultL            = 10
		defaultClientCount  = 50
		defaultWarmupRounds = 50
		defaultTrials       = 20000
		defaultSeed         = 542
		defaultReadRatio    = 0.1
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
	observed := measureReMvpStatisticalDistance(clients, config.trials, leafRNG)
	ideal := idealStatisticalDistanceRandomKDistribution(config.l, config.clientCount)
	distance := statisticalDistanceObservedToIdeal(observed, ideal)

	return statisticalDistanceResult{
		config:        config,
		observed:      observed,
		ideal:         ideal,
		distance:      distance,
		elapsed:       time.Since(startedAt),
		observedTotal: config.trials,
	}, nil
}

type statisticalDistanceReMvpClient struct {
	client    *MvpClient
	operation AccessOperation
}

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

func measureReMvpStatisticalDistance(clients []*statisticalDistanceReMvpClient, trials int, leafRNG *rand.Rand) []int {
	counts := make([]int, len(clients)+1)
	for trial := 0; trial < trials; trial++ {
		seen := make(map[MvpBucketPosition]struct{}, len(clients))
		for _, state := range clients {
			op := state.operation.Next()
			_, leaf, ok := chooseReMvpStatisticalDistanceTargetLeaf(state.client, op.target, op.OP, leafRNG)
			if !ok {
				continue
			}
			seen[leaf.bucket] = struct{}{}
		}
		counts[len(seen)]++
		logStatisticalDistanceTrialProgress(trial+1, trials)
	}
	return counts
}

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
	observed := measureBaseMvpStatisticalDistance(clients, config.trials, leafRNG)
	ideal := idealStatisticalDistanceRandomKDistribution(config.l, config.clientCount)
	distance := statisticalDistanceObservedToIdeal(observed, ideal)

	return statisticalDistanceResult{
		config:        config,
		observed:      observed,
		ideal:         ideal,
		distance:      distance,
		elapsed:       time.Since(startedAt),
		observedTotal: config.trials,
	}, nil
}

type statisticalDistanceBaseMvpClient struct {
	client    *BaseMvpClient
	operation AccessOperation
}

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

func measureBaseMvpStatisticalDistance(clients []*statisticalDistanceBaseMvpClient, trials int, leafRNG *rand.Rand) []int {
	counts := make([]int, len(clients)+1)
	for trial := 0; trial < trials; trial++ {
		seen := make(map[BaseMvpBucketPosition]struct{}, len(clients))
		for _, state := range clients {
			op := state.operation.Next()
			entry, ok := state.client.PositionMap[op.target]
			if !ok {
				continue
			}
			leaf := chooseBaseMvpStatisticalDistanceLeafFromPosition(entry.Slot, state.client.L, leafRNG)
			seen[leaf.bucket] = struct{}{}
		}
		counts[len(seen)]++
		logStatisticalDistanceTrialProgress(trial+1, trials)
	}
	return counts
}

func logStatisticalDistanceTrialProgress(done int, total int) {
	if done%100 != 0 && done != total {
		return
	}
	fmt.Printf("statisticaldistance trial progress: %d/%d\n", done, total)
}

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

func normalizeStatisticalDistanceAccessType(accessType string) string {
	switch accessType {
	case "random", "zipf":
		return accessType
	default:
		panic("unknown accesstype")
	}
}

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

func readStatisticalDistanceStringEnv(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
