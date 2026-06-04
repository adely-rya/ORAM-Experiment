// Synchronized Monte Carlo experiment for leaf access distributions.
package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	z          = 4
	l          = 8
	n          = 256
	seed       = 542
	WarmUp     = 10
	MonteCarlo = 10000
	minClient  = 1
	maxClient  = 50
)

// var experiment1Alphas = []float32{0.90436, 1.0945, 1.2353, 1.537}
var experiment1Alphas = []float32{0.1}
var zipfGeneratorRand = rand.New(rand.NewSource(time.Now().UnixNano()))
var zipfGeneratorMu sync.Mutex

type client_set struct {
	client           *MvpClient
	selectedPosition MvpPosition
	populatePath     map[MvpPosition]MvpSlot
	populateStash    []MvpDataBlock
	populatedPathMap []path
	ready            bool
}

type experiment1Result struct {
	clientCount int
	alpha       float32
	distance    float64
}

func NewCientSet(client *MvpClient) *client_set {
	return &client_set{client: client}
}

func Experiment1() {
	clientCounts := experiment1ClientCounts(minClient, maxClient)
	results := make(chan experiment1Result, len(clientCounts)*len(experiment1Alphas))

	var experimentwg sync.WaitGroup
	for _, clientCount := range clientCounts {
		for _, alpha := range experiment1Alphas {
			experimentwg.Add(1)
			go leafDistribution(alpha, clientCount, results, &experimentwg)
		}
	}

	experimentwg.Wait()
	close(results)

	ordered := make([]experiment1Result, 0, len(clientCounts)*len(experiment1Alphas))
	for result := range results {
		ordered = append(ordered, result)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].clientCount != ordered[j].clientCount {
			return ordered[i].clientCount < ordered[j].clientCount
		}
		return ordered[i].alpha < ordered[j].alpha
	})

	fmt.Println("client_count,alpha,k_distribution_statistical_distance")
	for _, result := range ordered {
		fmt.Printf("%d,%.6f,%.10f\n", result.clientCount, result.alpha, result.distance)
	}
}

// Build the requested client-count series: 1, then 5, 10, ... up to maxClient.
func experiment1ClientCounts(minClient int, maxClient int) []int {
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

// Run one independent alpha/client-count experiment from the same initial server state.
func leafDistribution(alpha float32, clientCount int, results chan<- experiment1Result, experimentwg *sync.WaitGroup) {
	defer experimentwg.Done()

	server := NewSynchronizedMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)
	go server.Run()

	clients := make([]*client_set, 0, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewMvpClient(
			l,
			z,
			clientID,
			clonePositionMap(positionmap),
			server.Requests,
		)
		clients = append(clients, NewCientSet(client))
	}

	// Warm up the synchronized ORAM state without recording selected leaves.
	for i := 0; i < WarmUp; i++ {
		runSynchronizedTrial(clients, alpha)
	}

	// Count the number of distinct leaves selected in each Monte Carlo trial.
	observedDistribution := makeKDistribution(clientCount)
	for i := 0; i < MonteCarlo; i++ {
		leaves := runSynchronizedTrial(clients, alpha)
		observedDistribution[countDistinctLeaves(leaves)]++
	}

	randomDistribution := makeRandomKDistribution(l, clientCount, MonteCarlo, seed+int64(clientCount)*1000003+int64(alpha*1000))

	results <- experiment1Result{
		clientCount: clientCount,
		alpha:       alpha,
		distance:    statisticalDistance(observedDistribution, randomDistribution),
	}
}

// Run the three synchronized phases: all GetPM/GetPS, then all Evict.
func runSynchronizedTrial(clients []*client_set, alpha float32) []MvpPosition {
	var pspmwg sync.WaitGroup
	for _, clientSet := range clients {
		op := opgenerater(alpha, len(clientSet.client.PositionMap)-1)
		pspmwg.Add(1)
		go GetPmPs(clientSet, op, &pspmwg)
	}
	pspmwg.Wait()

	var evictwg sync.WaitGroup
	for _, clientSet := range clients {
		evictwg.Add(1)
		go syncevict(clientSet, &evictwg)
	}
	evictwg.Wait()

	leaves := make([]MvpPosition, 0, len(clients))
	for _, clientSet := range clients {
		leaves = append(leaves, clientSet.selectedPosition)
	}
	return leaves
}

// Execute GetPM and GetPS, then prepare the eviction data for one client.
func GetPmPs(c *client_set, op OramOP, pspmwg *sync.WaitGroup) {
	defer pspmwg.Done()

	client := c.client
	c.ready = false
	version, pathMaps, err := client.GetPM()
	if err != nil {
		log.Printf("GetPM error: %v", err)
		return
	}
	client.seq = version

	client.consolidatePathMaps(pathMaps)

	accessPosition := client.PositionMap[op.target].Slot
	leaf := client.selectPath(accessPosition, client.L)
	c.selectedPosition = leaf

	client.path, client.Stash, err = client.GetPS(leaf)
	if err != nil {
		log.Printf("GetPS error: %v", err)
		return
	}

	W := client.mergePathStashes()
	targetBlock, ok := W[op.target]
	if !ok {
		log.Panicf("Not target block in working set: client=%d addr=%d", client.ClientID, op.target)
	}

	if op.OP == Write {
		targetBlock.Data = op.param
		targetBlock.Version = Versions{version, version, version}
	} else {
		targetBlock.Version.SetA(version)
		targetBlock.Version.SetS(version)
	}
	W[op.target] = targetBlock

	c.populatePath, c.populateStash, c.populatedPathMap = client.populatePath(W, op)
	c.ready = true
}

// Commit the eviction data prepared by GetPmPs.
func syncevict(c *client_set, evictwg *sync.WaitGroup) {
	defer evictwg.Done()
	if !c.ready {
		return
	}
	if err := c.client.Evict(c.populatePath, c.populatedPathMap, c.populateStash); err != nil {
		log.Printf("Evict error: %v", err)
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

// Count how many distinct leaves appear in one synchronized trial.
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
