package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"
)

const (
	algTimingDefaultAccessCount = 1000000
	algTimingDefaultN           = 128
	algTimingDefaultBit         = 7
	algTimingDefaultZ           = 4
	algTimingDefaultPL          = 10
	algTimingDefaultFixedAddr   = 10
	algTimingDefaultCSVPath     = "cube_oram_alg_access_timing.csv"
)

type algTimingRunner func(*CubeClient, int) []int

type algTimingStats struct {
	values []int64
	min    int64
	max    int64
	sum    int64
}

type algTimingSnapshot struct {
	min  int64
	max  int64
	mean float64
	p50  int64
	p90  int64
	p95  int64
	p99  int64
}

func algTimingParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func algTimingBuildInitialState(n int, bit int, z int, pl int, seed int64) (ORAMCube, []int, []CubeDataBlock) {
	initRNG := rand.New(rand.NewSource(seed))
	pm := make([]int, n+1)
	for i := range pm {
		pm[i] = stashPosition
	}
	stash := make([]CubeDataBlock, 0)
	cube := NewORAMCube(bit, z, pl)

	for addr := 1; addr <= n; addr++ {
		position := initRNG.Intn(1 << bit)
		block := CubeDataBlock{Addr: addr, Data: strconv.Itoa(addr)}

		if cube.SetBlock(position, block) {
			pm[addr] = position
		} else {
			stash = append(stash, block)
			pm[addr] = stashPosition
		}
	}

	return cube, pm, stash
}

func algTimingCheckpoints(maxAccesses int) []int {
	checkpoints := make([]int, 0)
	for value := 10; value <= maxAccesses; value *= 10 {
		checkpoints = append(checkpoints, value)
		if value > math.MaxInt/10 {
			break
		}
	}

	if len(checkpoints) == 0 || checkpoints[len(checkpoints)-1] != maxAccesses {
		checkpoints = append(checkpoints, maxAccesses)
	}

	return checkpoints
}

func algTimingWriteRow(writer *csv.Writer, values ...string) {
	if err := writer.Write(values); err != nil {
		panic(err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		panic(err)
	}
}

func (s *algTimingStats) add(duration time.Duration) {
	value := duration.Nanoseconds()
	if len(s.values) == 0 || value < s.min {
		s.min = value
	}
	if len(s.values) == 0 || value > s.max {
		s.max = value
	}
	s.sum += value
	s.values = append(s.values, value)
}

func (s *algTimingStats) snapshot() algTimingSnapshot {
	if len(s.values) == 0 {
		return algTimingSnapshot{}
	}

	sorted := append([]int64(nil), s.values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	percentile := func(value float64) int64 {
		index := int(math.Ceil(value/100*float64(len(sorted)))) - 1
		if index < 0 {
			index = 0
		}
		if index >= len(sorted) {
			index = len(sorted) - 1
		}
		return sorted[index]
	}

	return algTimingSnapshot{
		min:  s.min,
		max:  s.max,
		mean: float64(s.sum) / float64(len(s.values)),
		p50:  percentile(50),
		p90:  percentile(90),
		p95:  percentile(95),
		p99:  percentile(99),
	}
}

func algTimingRunAccess(client *CubeClient, server *CubeServer, addr int, getData algTimingRunner) time.Duration {
	start := time.Now()
	client.Counter = server.GiveCounter()
	path := getData(client, addr)
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
	return time.Since(start)
}

func algTimingRunAlgorithm(
	writer *csv.Writer,
	algName string,
	getData algTimingRunner,
	maxAccesses int,
	n int,
	bit int,
	z int,
	pl int,
	fixedAddr int,
	seed int64,
) {
	cube, pm, stash := algTimingBuildInitialState(n, bit, z, pl, seed)

	randomServer1 := NewCubeServer(cube)
	randomServer2 := NewCubeServer(cube)
	fixedServer := NewCubeServer(cube)
	randomClient1 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+1)))
	randomClient2 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+2)))
	fixedClient := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+3)))

	randomStats := algTimingStats{values: make([]int64, 0, maxAccesses*2)}
	fixedStats := algTimingStats{values: make([]int64, 0, maxAccesses)}
	checkpoints := algTimingCheckpoints(maxAccesses)
	start := time.Now()
	completed := 0

	for _, checkpoint := range checkpoints {
		for completed < checkpoint {
			randomAddr1 := 1 + randomClient1.RNG.Intn(len(randomClient1.PM)-1)
			randomAddr2 := 1 + randomClient2.RNG.Intn(len(randomClient2.PM)-1)

			randomStats.add(algTimingRunAccess(&randomClient1, &randomServer1, randomAddr1, getData))
			randomStats.add(algTimingRunAccess(&randomClient2, &randomServer2, randomAddr2, getData))
			fixedStats.add(algTimingRunAccess(&fixedClient, &fixedServer, fixedAddr, getData))
			completed++
		}

		randomVsRandomDistance := statisticalDistance(
			randomServer1.Cube.BucketReadCount,
			randomServer1.Cube.TotalBucketRead,
			randomServer2.Cube.BucketReadCount,
			randomServer2.Cube.TotalBucketRead,
		)
		randomVsFixedDistance := statisticalDistance(
			randomServer1.Cube.BucketReadCount,
			randomServer1.Cube.TotalBucketRead,
			fixedServer.Cube.BucketReadCount,
			fixedServer.Cube.TotalBucketRead,
		)
		randomTiming := randomStats.snapshot()
		fixedTiming := fixedStats.snapshot()

		row := []string{
			algName,
			strconv.Itoa(checkpoint),
			strconv.Itoa(n),
			strconv.Itoa(bit),
			strconv.Itoa(z),
			strconv.Itoa(pl),
			strconv.Itoa(fixedAddr),
			strconv.FormatInt(seed, 10),
			strconv.FormatFloat(randomVsRandomDistance, 'g', 12, 64),
			strconv.FormatFloat(randomVsFixedDistance, 'g', 12, 64),
			strconv.Itoa(len(randomClient1.Stash)),
			strconv.Itoa(len(randomClient2.Stash)),
			strconv.Itoa(len(fixedClient.Stash)),
			strconv.FormatFloat(time.Since(start).Seconds(), 'g', 6, 64),
			strconv.FormatInt(randomTiming.min, 10),
			strconv.FormatInt(randomTiming.max, 10),
			strconv.FormatFloat(randomTiming.mean, 'g', 12, 64),
			strconv.FormatInt(randomTiming.p50, 10),
			strconv.FormatInt(randomTiming.p90, 10),
			strconv.FormatInt(randomTiming.p95, 10),
			strconv.FormatInt(randomTiming.p99, 10),
			strconv.FormatInt(fixedTiming.min, 10),
			strconv.FormatInt(fixedTiming.max, 10),
			strconv.FormatFloat(fixedTiming.mean, 'g', 12, 64),
			strconv.FormatInt(fixedTiming.p50, 10),
			strconv.FormatInt(fixedTiming.p90, 10),
			strconv.FormatInt(fixedTiming.p95, 10),
			strconv.FormatInt(fixedTiming.p99, 10),
		}

		fmt.Println(row)
		algTimingWriteRow(writer, row...)
	}
}

func main() {
	args := os.Args
	maxAccesses := algTimingDefaultAccessCount
	n := algTimingDefaultN
	bit := algTimingDefaultBit
	z := algTimingDefaultZ
	pl := algTimingDefaultPL
	fixedAddr := algTimingDefaultFixedAddr
	seed := time.Now().UnixNano()
	csvPath := algTimingDefaultCSVPath

	var err error
	if maxAccesses, err = algTimingParseIntArg(args, 1, maxAccesses); err != nil {
		panic(err)
	}
	if n, err = algTimingParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if bit, err = algTimingParseIntArg(args, 3, bit); err != nil {
		panic(err)
	}
	if z, err = algTimingParseIntArg(args, 4, z); err != nil {
		panic(err)
	}
	if pl, err = algTimingParseIntArg(args, 5, pl); err != nil {
		panic(err)
	}
	if fixedAddr, err = algTimingParseIntArg(args, 6, fixedAddr); err != nil {
		panic(err)
	}
	if len(args) >= 8 {
		parsedSeed, parseErr := strconv.ParseInt(args[7], 10, 64)
		if parseErr != nil {
			panic(parseErr)
		}
		seed = parsedSeed
	}
	if len(args) >= 9 {
		csvPath = args[8]
	}
	if fixedAddr < 1 || fixedAddr > n {
		panic("fixed address must be within 1..N")
	}
	if pl >= 1<<bit {
		panic("PL must be smaller than 2^Bit for a simple path")
	}
	if n > z*(1<<bit) {
		panic("N is too large for Bit and Z: initial placement must have enough total bucket capacity")
	}

	file, err := os.Create(csvPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"algorithm",
		"accesses",
		"N",
		"Bit",
		"Z",
		"PL",
		"fixed_addr",
		"seed",
		"random_vs_random_distance",
		"random_vs_fixed_distance",
		"random1_stash_size",
		"random2_stash_size",
		"fixed_stash_size",
		"elapsed_seconds",
		"random_time_min_ns",
		"random_time_max_ns",
		"random_time_mean_ns",
		"random_time_p50_ns",
		"random_time_p90_ns",
		"random_time_p95_ns",
		"random_time_p99_ns",
		"fixed_time_min_ns",
		"fixed_time_max_ns",
		"fixed_time_mean_ns",
		"fixed_time_p50_ns",
		"fixed_time_p90_ns",
		"fixed_time_p95_ns",
		"fixed_time_p99_ns",
	}
	algTimingWriteRow(writer, header...)

	fmt.Printf("max accesses per workflow: %d\n", maxAccesses)
	fmt.Printf("N: %d\n", n)
	fmt.Printf("Bit: %d\n", bit)
	fmt.Printf("Z: %d\n", z)
	fmt.Printf("PL: %d\n", pl)
	fmt.Printf("fixed address: %d\n", fixedAddr)
	fmt.Printf("seed: %d\n", seed)
	fmt.Printf("csv: %s\n", csvPath)

	algTimingRunAlgorithm(writer, "alg1", func(client *CubeClient, addr int) []int {
		return client.GetData_alg1(addr)
	}, maxAccesses, n, bit, z, pl, fixedAddr, seed)

	algTimingRunAlgorithm(writer, "alg2", func(client *CubeClient, addr int) []int {
		return client.GetData_alg2(addr)
	}, maxAccesses, n, bit, z, pl, fixedAddr, seed)
}
