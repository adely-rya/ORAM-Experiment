package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"time"
)

type CubeDataBlock struct {
	Addr int
	Data string
}

type CubeBucket struct {
	Z     int
	Value []CubeDataBlock
}

func NewCubeBucket(z int) CubeBucket {
	return CubeBucket{
		Z:     z,
		Value: make([]CubeDataBlock, 0, z),
	}
}

func (b CubeBucket) Clone() CubeBucket {
	return CubeBucket{
		Z:     b.Z,
		Value: append([]CubeDataBlock(nil), b.Value...),
	}
}

func (b *CubeBucket) SetBlock(block CubeDataBlock) bool {
	if len(b.Value) >= b.Z {
		return false
	}

	b.Value = append(b.Value, block)
	return true
}

type ORAMCube struct {
	Bit             int
	Z               int
	PL              int
	Cube            []CubeBucket
	BucketReadCount []int64
	TotalBucketRead int64
}

func NewORAMCube(bit, z, pl int) ORAMCube {
	size := 1 << bit
	cube := make([]CubeBucket, size)
	for i := range cube {
		cube[i] = NewCubeBucket(z)
	}

	return ORAMCube{
		Bit:             bit,
		Z:               z,
		PL:              pl,
		Cube:            cube,
		BucketReadCount: make([]int64, size),
	}
}

func (c ORAMCube) Clone() ORAMCube {
	cube := make([]CubeBucket, len(c.Cube))
	for i, bucket := range c.Cube {
		cube[i] = bucket.Clone()
	}

	return ORAMCube{
		Bit:             c.Bit,
		Z:               c.Z,
		PL:              c.PL,
		Cube:            cube,
		BucketReadCount: append([]int64(nil), c.BucketReadCount...),
		TotalBucketRead: c.TotalBucketRead,
	}
}

func (c *ORAMCube) SetBlock(position int, block CubeDataBlock) bool {
	return c.Cube[position].SetBlock(block)
}

func (c *ORAMCube) SetBucket(position int, bucket CubeBucket) {
	c.Cube[position] = bucket
}

func (c *ORAMCube) GetBucket(position int) *CubeBucket {
	c.BucketReadCount[position]++
	c.TotalBucketRead++
	return &c.Cube[position]
}

type CubeServer struct {
	Cube    ORAMCube
	Counter int64
}

func NewCubeServer(cube ORAMCube) CubeServer {
	return CubeServer{Cube: cube.Clone()}
}

func (s *CubeServer) GiveCounter() int64 {
	counter := s.Counter
	s.Counter++
	return counter
}

func (s *CubeServer) GetPath(path []int) []CubeDataBlock {
	blocks := make([]CubeDataBlock, 0, len(path)*s.Cube.Z)
	for _, position := range path {
		bucket := s.Cube.GetBucket(position)
		blocks = append(blocks, bucket.Value...)
	}
	return blocks
}

func (s *CubeServer) Reallocation(shuffled map[int]CubeBucket) {
	for position, bucket := range shuffled {
		s.Cube.SetBucket(position, bucket)
	}
}

type CubeClient struct {
	PM          []int
	Stash       []CubeDataBlock
	Counter     int64
	Bit         int
	Z           int
	PL          int
	RNG         *rand.Rand
	accessBlock int
	pathList    []int
}

func NewCubeClient(pm []int, stash []CubeDataBlock, bit, z, pl int, rng *rand.Rand) CubeClient {
	return CubeClient{
		PM:          append([]int(nil), pm...),
		Stash:       append([]CubeDataBlock(nil), stash...),
		Bit:         bit,
		Z:           z,
		PL:          pl,
		RNG:         rng,
		accessBlock: -1,
	}
}

func (c *CubeClient) GetData(addr int) []int {
	c.accessBlock = addr

	pmPosition := c.PM[addr]
	blockPosition := 0
	if pmPosition == 0 {
		blockPosition = c.RNG.Intn(1 << c.Bit)
	} else {
		blockPosition = pmPosition - 1
	}

	distance := 0
	flipList := make([]int, 0, c.Bit)
	for bit := 0; bit < c.Bit; bit++ {
		mask := 1 << (c.Bit - bit - 1)
		if blockPosition&mask != 0 {
			distance++
			flipList = append(flipList, bit)
		}
	}
	c.RNG.Shuffle(len(flipList), func(i, j int) {
		flipList[i], flipList[j] = flipList[j], flipList[i]
	})

	dif := c.PL - distance
	path := make([]int, 0, c.PL+1)
	visited := make(map[int]bool, c.PL+1)
	last := 0

	for _, bit := range flipList {
		path = append(path, last)
		visited[last] = true
		last = flipHypercubeBit(last, bit, c.Bit)
	}

	path = append(path, last)
	visited[last] = true

	for i := 0; i < dif; i++ {
		candidates := unvisitedNeighbors(last, c.Bit, visited)
		if len(candidates) == 0 {
			panic("next point is missing")
		}

		last = candidates[c.RNG.Intn(len(candidates))]
		visited[last] = true
		path = append(path, last)
	}

	c.pathList = path
	return path
}

func (c *CubeClient) GetRandomData() []int {
	return c.GetData(1 + c.RNG.Intn(len(c.PM)-1))
}

func (c *CubeClient) Shuffle(blocks []CubeDataBlock) map[int]CubeBucket {
	shuffled := make(map[int]CubeBucket, len(c.pathList))
	allBlocks := make([]CubeDataBlock, 0, len(blocks)+len(c.Stash))
	targetBlock := CubeDataBlock{}
	hasTargetBlock := false

	for _, block := range blocks {
		if block.Addr == c.accessBlock {
			targetBlock = block
			hasTargetBlock = true
			continue
		}
		allBlocks = append(allBlocks, block)
	}

	for _, block := range c.Stash {
		if block.Addr == c.accessBlock {
			targetBlock = block
			hasTargetBlock = true
			continue
		}
		allBlocks = append(allBlocks, block)
	}
	c.Stash = nil

	for _, position := range c.pathList {
		shuffled[position] = NewCubeBucket(c.Z)
	}

	nextStash := make([]CubeDataBlock, 0)

	if hasTargetBlock {
		rootPosition := 0
		bucket := shuffled[rootPosition]
		if bucket.SetBlock(targetBlock) {
			shuffled[rootPosition] = bucket
			c.PM[c.accessBlock] = rootPosition + 1
		} else {
			nextStash = append(nextStash, targetBlock)
			c.PM[c.accessBlock] = 0
		}
	}

	for _, block := range allBlocks {
		pmPosition := c.PM[block.Addr]
		if pmPosition == 0 {
			nextStash = append(nextStash, block)
			c.PM[block.Addr] = 0
			continue
		}

		position := pmPosition - 1
		bucket, ok := shuffled[position]
		if !ok {
			nextStash = append(nextStash, block)
			c.PM[block.Addr] = 0
			continue
		}

		if bucket.SetBlock(block) {
			shuffled[position] = bucket
			c.PM[block.Addr] = position + 1
		} else {
			nextStash = append(nextStash, block)
			c.PM[block.Addr] = 0
		}
	}

	c.Stash = nil
	rootFirstPositions := append([]int(nil), c.pathList...)
	slices.SortFunc(rootFirstPositions, func(a, b int) int {
		return bitsCount(a) - bitsCount(b)
	})

	for _, block := range nextStash {
		placed := false

		for _, position := range rootFirstPositions {
			bucket := shuffled[position]
			if bucket.SetBlock(block) {
				shuffled[position] = bucket
				c.PM[block.Addr] = position + 1
				placed = true
				break
			}
		}

		if !placed {
			c.Stash = append(c.Stash, block)
			c.PM[block.Addr] = 0
		}
	}

	return shuffled
}

func bitsCount(value int) int {
	count := 0
	for value > 0 {
		count += value & 1
		value >>= 1
	}
	return count
}

func flipHypercubeBit(position, bit, bitCount int) int {
	return position ^ (1 << (bitCount - bit - 1))
}

func unvisitedNeighbors(position, bitCount int, visited map[int]bool) []int {
	candidates := make([]int, 0, bitCount)
	for bit := 0; bit < bitCount; bit++ {
		next := flipHypercubeBit(position, bit, bitCount)
		if !visited[next] {
			candidates = append(candidates, next)
		}
	}
	return candidates
}

func cubeStatisticalDistanceFromCounts(count1 []int64, total1 int64, count2 []int64, total2 int64) float64 {
	if total1 == 0 || total2 == 0 {
		return 0
	}

	sum := 0.0
	maxLen := max(len(count1), len(count2))
	for i := 0; i < maxLen; i++ {
		p1 := 0.0
		if i < len(count1) {
			p1 = float64(count1[i]) / float64(total1)
		}

		p2 := 0.0
		if i < len(count2) {
			p2 = float64(count2[i]) / float64(total2)
		}

		sum += math.Abs(p1 - p2)
	}

	return 0.5 * sum
}

func cubeParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func cubeParseInt64Arg(args []string, index int, fallback int64) (int64, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.ParseInt(args[index], 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func cubeDefaultCheckpoints(maxAccesses int64) []int64 {
	checkpoints := make([]int64, 0)
	for value := int64(10); value <= maxAccesses; value *= 10 {
		checkpoints = append(checkpoints, value)
		if value > math.MaxInt64/10 {
			break
		}
	}

	if len(checkpoints) == 0 || checkpoints[len(checkpoints)-1] != maxAccesses {
		checkpoints = append(checkpoints, maxAccesses)
	}

	return checkpoints
}

func runCubeRandomAccess(client *CubeClient, server *CubeServer) {
	client.Counter = server.GiveCounter()
	path := client.GetRandomData()
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func runCubeModeAccess(client *CubeClient, server *CubeServer, mode string) {
	client.Counter = server.GiveCounter()

	var path []int
	if mode == "fixed" {
		path = client.GetData(10)
	} else {
		path = client.GetRandomData()
	}

	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func main() {
	args := os.Args
	n := 128
	bit := 8
	z := 4
	pl := 10
	maxAccesses := int64(100000000)
	seed := int64(10)
	workflowMode := "random"
	csvPath := "cube_oram_convergence.csv"

	var err error
	if maxAccesses, err = cubeParseInt64Arg(args, 1, maxAccesses); err != nil {
		panic(err)
	}
	if n, err = cubeParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if bit, err = cubeParseIntArg(args, 3, bit); err != nil {
		panic(err)
	}
	if z, err = cubeParseIntArg(args, 4, z); err != nil {
		panic(err)
	}
	if pl, err = cubeParseIntArg(args, 5, pl); err != nil {
		panic(err)
	}
	if len(args) >= 7 {
		parsedSeed, parseErr := strconv.ParseInt(args[6], 10, 64)
		if parseErr != nil {
			panic(parseErr)
		}
		seed = parsedSeed
	}
	if len(args) >= 8 {
		workflowMode = args[7]
		if workflowMode != "random" && workflowMode != "fixed" && workflowMode != "both" {
			panic("workflow_mode must be random, fixed, or both")
		}
	}
	if len(args) >= 9 {
		csvPath = args[8]
	}

	initRNG := rand.New(rand.NewSource(seed))
	pm := make([]int, n+1)
	stash := make([]CubeDataBlock, 0)
	cube := NewORAMCube(bit, z, pl)

	for addr := 1; addr <= n; addr++ {
		position := initRNG.Intn(1 << bit)
		block := CubeDataBlock{Addr: addr, Data: strconv.Itoa(addr)}

		if cube.SetBlock(position, block) {
			pm[addr] = position + 1
		} else {
			stash = append(stash, block)
			pm[addr] = 0
		}
	}

	server1 := NewCubeServer(cube)
	server2 := NewCubeServer(cube)
	server3 := NewCubeServer(cube)

	rng1 := rand.New(rand.NewSource(seed + 1))
	rng2 := rand.New(rand.NewSource(seed + 2))
	rng3 := rand.New(rand.NewSource(seed + 3))
	client1 := NewCubeClient(pm, stash, bit, z, pl, rng1)
	client2 := NewCubeClient(pm, stash, bit, z, pl, rng2)
	client3 := NewCubeClient(pm, stash, bit, z, pl, rng3)

	csvFile, err := os.Create(csvPath)
	if err != nil {
		panic(err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	err = writer.Write([]string{
		"accesses",
		"N",
		"Bit",
		"Z",
		"PL",
		"seed",
		"workflow_mode",
		"bucket_statistical_distance",
		"random_vs_random_distance",
		"random_vs_fixed_distance",
		"client1_stash_size",
		"client2_stash_size",
		"client3_stash_size",
		"elapsed_seconds",
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("N: %d\n", n)
	fmt.Printf("Bit: %d\n", bit)
	fmt.Printf("Z: %d\n", z)
	fmt.Printf("PL: %d\n", pl)
	fmt.Printf("max accesses per workflow: %d\n", maxAccesses)
	fmt.Printf("workflow mode: %s\n", workflowMode)
	fmt.Printf("csv: %s\n", csvPath)
	if workflowMode == "both" {
		fmt.Println("accesses,random_vs_random_distance,random_vs_fixed_distance,client1_stash_size,client2_stash_size,client3_stash_size,elapsed_seconds")
	} else {
		fmt.Println("accesses,bucket_statistical_distance,client1_stash_size,client2_stash_size,elapsed_seconds")
	}

	checkpoints := cubeDefaultCheckpoints(maxAccesses)
	start := time.Now()
	completed := int64(0)

	for _, checkpoint := range checkpoints {
		for completed < checkpoint {
			runCubeRandomAccess(&client1, &server1)
			if workflowMode == "both" {
				runCubeModeAccess(&client2, &server2, "random")
				runCubeModeAccess(&client3, &server3, "fixed")
			} else {
				runCubeModeAccess(&client2, &server2, workflowMode)
			}
			completed++
		}

		distance := cubeStatisticalDistanceFromCounts(
			server1.Cube.BucketReadCount,
			server1.Cube.TotalBucketRead,
			server2.Cube.BucketReadCount,
			server2.Cube.TotalBucketRead,
		)
		randomVsRandomDistance := distance
		randomVsFixedDistance := 0.0
		if workflowMode == "both" {
			randomVsFixedDistance = cubeStatisticalDistanceFromCounts(
				server1.Cube.BucketReadCount,
				server1.Cube.TotalBucketRead,
				server3.Cube.BucketReadCount,
				server3.Cube.TotalBucketRead,
			)
		} else if workflowMode == "fixed" {
			randomVsRandomDistance = 0
			randomVsFixedDistance = distance
		}
		elapsedSeconds := time.Since(start).Seconds()

		if workflowMode == "both" {
			fmt.Printf(
				"%d,%.12g,%.12g,%d,%d,%d,%.6g\n",
				checkpoint,
				randomVsRandomDistance,
				randomVsFixedDistance,
				len(client1.Stash),
				len(client2.Stash),
				len(client3.Stash),
				elapsedSeconds,
			)
		} else {
			fmt.Printf(
				"%d,%.12g,%d,%d,%.6g\n",
				checkpoint,
				distance,
				len(client1.Stash),
				len(client2.Stash),
				elapsedSeconds,
			)
		}

		err = writer.Write([]string{
			strconv.FormatInt(checkpoint, 10),
			strconv.Itoa(n),
			strconv.Itoa(bit),
			strconv.Itoa(z),
			strconv.Itoa(pl),
			strconv.FormatInt(seed, 10),
			workflowMode,
			strconv.FormatFloat(distance, 'g', 12, 64),
			strconv.FormatFloat(randomVsRandomDistance, 'g', 12, 64),
			strconv.FormatFloat(randomVsFixedDistance, 'g', 12, 64),
			strconv.Itoa(len(client1.Stash)),
			strconv.Itoa(len(client2.Stash)),
			strconv.Itoa(len(client3.Stash)),
			strconv.FormatFloat(elapsedSeconds, 'g', 6, 64),
		})
		if err != nil {
			panic(err)
		}
		writer.Flush()
	}
}
