// Monte Carlo experiment for same-address concurrent leaf selection.
package main

import (
	"encoding/csv"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	z                  = 4
	l                  = 12
	n                  = 1 << 12
	seed               = 542
	WarmUp             = 200
	MonteCarlo         = 20000
	minClient          = 1
	maxClient          = 20
	experiment3CSVPath = "CSV/re_mvp_oram_experiment3.csv"
)

var experimentAlphas = []float32{0.1, 1.9}
var zipfGeneratorRand = rand.New(rand.NewSource(time.Now().UnixNano()))
var zipfGeneratorMu sync.Mutex

type client_set struct {
	client           *MvpClient
	selectedPosition MvpPosition
}

type experimentResult struct {
	clientCount int
	alpha       float32
	distance    float64
	elapsed     time.Duration
	err         error
}

func NewCientSet(client *MvpClient) *client_set {
	return &client_set{client: client}
}

func Experiment3() {
	clientCounts := experimentClientCounts(minClient, maxClient)
	accessLoggingEnabled = false
	ConfigureMvpMaxSignatureFromEnv()
	defer func() {
		accessLoggingEnabled = true
	}()

	if err := os.MkdirAll(filepath.Dir(experiment3CSVPath), 0755); err != nil {
		log.Printf("experiment3 csv directory create error: %v", err)
		return
	}

	file, err := os.Create(experiment3CSVPath)
	if err != nil {
		log.Printf("experiment3 csv create error: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"client_count", "alpha", "k_distribution_statistical_distance"}); err != nil {
		log.Printf("experiment3 csv header write error: %v", err)
		return
	}
	if err := flushExperiment3CSV(writer, file); err != nil {
		log.Printf("experiment3 csv header flush error: %v", err)
		return
	}

	totalExperiments := len(clientCounts) * len(experimentAlphas)
	completedExperiments := 0
	log.Printf("experiment3 progress: start total=%d client_counts=%v alphas=%v csv=%s", totalExperiments, clientCounts, experimentAlphas, experiment3CSVPath)

	for _, clientCount := range clientCounts {
		results := make(chan experimentResult, len(experimentAlphas))
		var experimentwg sync.WaitGroup
		log.Printf("experiment3 progress: client_count=%d start alpha_experiments=%d", clientCount, len(experimentAlphas))

		for _, alpha := range experimentAlphas {
			experimentwg.Add(1)
			go sharedAddressLeafDistribution(alpha, clientCount, results, &experimentwg)
		}

		go func() {
			experimentwg.Wait()
			close(results)
		}()

		for result := range results {
			completedExperiments++
			if result.err != nil {
				log.Printf("experiment3 progress: failed client_count=%d alpha=%.6f completed=%d/%d elapsed=%s err=%v", result.clientCount, result.alpha, completedExperiments, totalExperiments, result.elapsed.Round(time.Millisecond), result.err)
				continue
			}

			if err := writeExperiment3Result(writer, file, result); err != nil {
				log.Printf("experiment3 csv write error: client_count=%d alpha=%.6f err=%v", result.clientCount, result.alpha, err)
				return
			}
			log.Printf("experiment3 progress: csv flushed client_count=%d alpha=%.6f distance=%.10f completed=%d/%d elapsed=%s", result.clientCount, result.alpha, result.distance, completedExperiments, totalExperiments, result.elapsed.Round(time.Millisecond))
		}

		log.Printf("experiment3 progress: client_count=%d done completed=%d/%d", clientCount, completedExperiments, totalExperiments)
	}

	log.Printf("experiment3 progress: done csv=%s completed=%d/%d", experiment3CSVPath, completedExperiments, totalExperiments)
}

func writeExperiment3Result(writer *csv.Writer, file *os.File, result experimentResult) error {
	if err := writer.Write([]string{
		strconv.Itoa(result.clientCount),
		strconv.FormatFloat(float64(result.alpha), 'f', 6, 32),
		strconv.FormatFloat(result.distance, 'f', 10, 64),
	}); err != nil {
		return err
	}
	return flushExperiment3CSV(writer, file)
}

func flushExperiment3CSV(writer *csv.Writer, file *os.File) error {
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return file.Sync()
}

// Build the client-count series: 1 if requested, then 5, 10, ... up to maxClient.
func experimentClientCounts(minClient int, maxClient int) []int {
	clientCounts := make([]int, 0)
	if minClient <= 1 && maxClient >= 1 {
		clientCounts = append(clientCounts, 1)
	}
	for clientCount := 5; clientCount <= maxClient; clientCount += 5 {
		if clientCount >= minClient {
			clientCounts = append(clientCounts, clientCount)
		}
	}
	return clientCounts
}

// Warm up with the configured clients, then measure leaves when all clients access one shared Zipf address per trial.
func sharedAddressLeafDistribution(alpha float32, clientCount int, results chan<- experimentResult, experimentwg *sync.WaitGroup) {
	defer experimentwg.Done()
	startedAt := time.Now()
	log.Printf("experiment3 progress: start client_count=%d alpha=%.6f warmup=%d monte_carlo=%d", clientCount, alpha, WarmUp, MonteCarlo)

	server := NewMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)
	go server.Run()

	clients := make([]*client_set, 0, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewMvpClient(l, z, clientID, clonePositionMap(positionmap), server.Requests)
		clients = append(clients, NewCientSet(client))
	}

	// Warm up the ORAM state with all configured clients issuing concurrent Zipf-selected accesses.
	for i := 0; i < WarmUp; i++ {
		if err := runConcurrentWarmupAccessTrial(clients, alpha); err != nil {
			results <- experimentResult{clientCount: clientCount, alpha: alpha, elapsed: time.Since(startedAt), err: err}
			return
		}
	}
	log.Printf("experiment3 progress: warmup done client_count=%d alpha=%.6f elapsed=%s", clientCount, alpha, time.Since(startedAt).Round(time.Millisecond))

	// Pull warmup PathMaps so every measuring client starts from the same PositionMap.
	syncClientPositionMaps(clients)

	// Count distinct leaves when all clients access the same Zipf-selected address.
	observedDistribution := makeKDistribution(clientCount)
	for i := 0; i < MonteCarlo; i++ {
		op := opgenerater(alpha, len(clients[0].client.PositionMap)-1)
		leaves := runSharedAddressGetPSOnlyTrial(clients, op)
		observedDistribution[countDistinctLeaves(leaves)]++
	}

	randomDistribution := makeRandomKDistribution(l, clientCount, MonteCarlo, seed+int64(clientCount)*1000003+int64(alpha*1000))

	results <- experimentResult{
		clientCount: clientCount,
		alpha:       alpha,
		distance:    statisticalDistance(observedDistribution, randomDistribution),
		elapsed:     time.Since(startedAt),
	}
	log.Printf("experiment3 progress: measurement done client_count=%d alpha=%.6f elapsed=%s", clientCount, alpha, time.Since(startedAt).Round(time.Millisecond))
}

func runConcurrentWarmupAccessTrial(clients []*client_set, alpha float32) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(clients))
	for _, clientSet := range clients {
		op := opgenerater(alpha, len(clientSet.client.PositionMap)-1)
		wg.Add(1)
		go func(client *MvpClient, op OramOP) {
			defer wg.Done()
			if err := client.Access(op); err != nil {
				errs <- err
			}
		}(clientSet.client, op)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func syncClientPositionMaps(clients []*client_set) {
	var wg sync.WaitGroup
	for _, clientSet := range clients {
		wg.Add(1)
		go func(client *MvpClient) {
			defer wg.Done()
			version, pathMaps, err := client.GetPM()
			if err != nil {
				log.Printf("GetPM error: %v", err)
				return
			}
			client.seq = version
			client.consolidatePathMaps(pathMaps)
		}(clientSet.client)
	}
	wg.Wait()
}

func runSharedAddressGetPSOnlyTrial(clients []*client_set, op OramOP) []MvpPosition {
	var wg sync.WaitGroup
	for _, clientSet := range clients {
		wg.Add(1)
		go getPSOnly(clientSet, op, &wg)
	}
	wg.Wait()

	leaves := make([]MvpPosition, 0, len(clients))
	for _, clientSet := range clients {
		leaves = append(leaves, clientSet.selectedPosition)
	}
	return leaves
}

func getPSOnly(c *client_set, op OramOP, wg *sync.WaitGroup) {
	defer wg.Done()

	client := c.client
	_, accessEntry, ok := client.choosePositionMapEntry(op.target, op.OP)
	if !ok {
		log.Printf("missing position map entry for addr %d", op.target)
		return
	}
	accessPosition := accessEntry.Slot
	leaf := client.selectPath(accessPosition, client.L)
	c.selectedPosition = leaf

	if _, _, err := client.GetPS(leaf); err != nil {
		log.Printf("GetPS error: %v", err)
	}
}

func opgenerater(a float32, addrMax int) OramOP {
	if a < 0 {
		panic("zipf alpha must be greater than or equal to 0")
	}
	if addrMax < 0 {
		panic("addrMax must be greater than or equal to 0")
	}

	zipfGeneratorMu.Lock()
	addr := finiteZipfAddr(zipfGeneratorRand, float64(a), addrMax)
	zipfGeneratorMu.Unlock()

	return OramOP{
		OP:     Write,
		target: addr,
		param:  strconv.Itoa(addr),
	}
}

// Draw one address from a finite Zipf-like distribution over 0..addrMax.
func finiteZipfAddr(rng *rand.Rand, alpha float64, addrMax int) int {
	if alpha == 0 {
		return rng.Intn(addrMax + 1)
	}

	total := 0.0
	for addr := 0; addr <= addrMax; addr++ {
		total += 1 / math.Pow(float64(addr+1), alpha)
	}

	target := rng.Float64() * total
	cumulative := 0.0
	for addr := 0; addr <= addrMax; addr++ {
		cumulative += 1 / math.Pow(float64(addr+1), alpha)
		if target < cumulative {
			return addr
		}
	}

	return addrMax
}

// Initialize k=1..clientCount counts to zero.
func makeKDistribution(clientCount int) map[int]int {
	distribution := make(map[int]int, clientCount)
	for k := 1; k <= clientCount; k++ {
		distribution[k] = 0
	}
	return distribution
}

// Count how many distinct leaves appear in one trial.
func countDistinctLeaves(leaves []MvpPosition) int {
	seen := make(map[MvpBucketPosition]struct{}, len(leaves))
	for _, leaf := range leaves {
		seen[leaf.bucket] = struct{}{}
	}
	return len(seen)
}

// Simulate Monte Carlo trials where clients choose leaves uniformly at random.
func makeRandomKDistribution(l int, clientCount int, monteCarlo int, seed int64) map[int]int {
	leafCount := 1 << l
	distribution := makeKDistribution(clientCount)
	rng := rand.New(rand.NewSource(seed))

	for trial := 0; trial < monteCarlo; trial++ {
		seen := make(map[int]struct{}, clientCount)
		for client := 0; client < clientCount; client++ {
			seen[rng.Intn(leafCount)] = struct{}{}
		}
		distribution[len(seen)]++
	}

	return distribution
}

// Compute total variation distance between two k-count distributions.
func statisticalDistance(left map[int]int, right map[int]int) float64 {
	leftTotal := distributionTotal(left)
	rightTotal := distributionTotal(right)
	if leftTotal == 0 && rightTotal == 0 {
		return 0
	}

	keys := make(map[int]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}

	sum := 0.0
	for key := range keys {
		leftProbability := 0.0
		if leftTotal > 0 {
			leftProbability = float64(left[key]) / float64(leftTotal)
		}

		rightProbability := 0.0
		if rightTotal > 0 {
			rightProbability = float64(right[key]) / float64(rightTotal)
		}

		sum += math.Abs(leftProbability - rightProbability)
	}

	return sum / 2
}

func distributionTotal(distribution map[int]int) int {
	total := 0
	for _, count := range distribution {
		total += count
	}
	return total
}
