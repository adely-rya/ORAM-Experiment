package main

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	bitExperimentDefaultRepeats   = 100000
	bitExperimentDefaultBlockNum  = 128
	bitExperimentDefaultMinBit    = 5
	bitExperimentDefaultMaxBit    = 10
	bitExperimentDefaultZ         = 4
	bitExperimentDefaultFixedAddr = 10
	bitExperimentDefaultCSVPath   = "cube_oram_bit_experiment.csv"
)

func bitExperimentParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func bitExperimentBuildInitialState(n int, bit int, z int, pl int, seed int64) (ORAMCube, []int, []CubeDataBlock) {
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

func bitExperimentRunRandomAccess(client *CubeClient, server *CubeServer) {
	client.Counter = server.GiveCounter()
	path := client.GetRandomData()
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func bitExperimentRunFixedAccess(client *CubeClient, server *CubeServer, fixedAddr int) {
	client.Counter = server.GiveCounter()
	path := client.GetData(fixedAddr)
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func bitExperimentWriteRow(writer *csv.Writer, values ...string) {
	if err := writer.Write(values); err != nil {
		panic(err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		panic(err)
	}
}

func main() {
	args := os.Args
	repeats := bitExperimentDefaultRepeats
	n := bitExperimentDefaultBlockNum
	minBit := bitExperimentDefaultMinBit
	maxBit := bitExperimentDefaultMaxBit
	z := bitExperimentDefaultZ
	fixedAddr := bitExperimentDefaultFixedAddr
	seed := time.Now().UnixNano()
	csvPath := bitExperimentDefaultCSVPath

	var err error
	if repeats, err = bitExperimentParseIntArg(args, 1, repeats); err != nil {
		panic(err)
	}
	if n, err = bitExperimentParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if minBit, err = bitExperimentParseIntArg(args, 3, minBit); err != nil {
		panic(err)
	}
	if maxBit, err = bitExperimentParseIntArg(args, 4, maxBit); err != nil {
		panic(err)
	}
	if z, err = bitExperimentParseIntArg(args, 5, z); err != nil {
		panic(err)
	}
	if fixedAddr, err = bitExperimentParseIntArg(args, 6, fixedAddr); err != nil {
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
	if minBit < 1 {
		panic("minBit must be positive")
	}
	if maxBit < minBit {
		panic("maxBit must be at least minBit")
	}
	if fixedAddr < 1 || fixedAddr > n {
		panic("fixed address must be within 1..N")
	}
	if n > z*(1<<minBit) {
		panic("N is too large for minBit and Z: initial placement must have enough total bucket capacity")
	}

	file, err := os.Create(csvPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"bit",
		"N",
		"Z",
		"PL",
		"repeats",
		"fixed_addr",
		"seed",
		"random_vs_random_distance",
		"random_vs_fixed_distance",
		"random1_stash_size",
		"random2_stash_size",
		"fixed_stash_size",
		"elapsed_seconds",
	}
	bitExperimentWriteRow(writer, header...)

	fmt.Printf("repeats: %d\n", repeats)
	fmt.Printf("N: %d\n", n)
	fmt.Printf("Bit range: %d..%d\n", minBit, maxBit)
	fmt.Printf("Z: %d\n", z)
	fmt.Println("PL: 2 * Bit")
	fmt.Printf("fixed address: %d\n", fixedAddr)
	fmt.Printf("seed: %d\n", seed)
	fmt.Printf("csv: %s\n", csvPath)
	fmt.Println("bit,N,Z,PL,repeats,fixed_addr,seed,random_vs_random_distance,random_vs_fixed_distance,random1_stash_size,random2_stash_size,fixed_stash_size,elapsed_seconds")

	for bit := minBit; bit <= maxBit; bit++ {
		pl := 2 * bit
		if pl >= 1<<bit {
			panic("PL must be smaller than 2^Bit for a simple path")
		}

		start := time.Now()
		cube, pm, stash := bitExperimentBuildInitialState(n, bit, z, pl, seed)

		server1 := NewCubeServer(cube)
		server2 := NewCubeServer(cube)
		server3 := NewCubeServer(cube)
		client1 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(bit)*100+1)))
		client2 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(bit)*100+2)))
		client3 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(bit)*100+3)))

		for i := 0; i < repeats; i++ {
			bitExperimentRunRandomAccess(&client1, &server1)
			bitExperimentRunRandomAccess(&client2, &server2)
			bitExperimentRunFixedAccess(&client3, &server3, fixedAddr)
		}

		randomVsRandomDistance := statisticalDistance(
			server1.Cube.BucketReadCount,
			server1.Cube.TotalBucketRead,
			server2.Cube.BucketReadCount,
			server2.Cube.TotalBucketRead,
		)
		randomVsFixedDistance := statisticalDistance(
			server1.Cube.BucketReadCount,
			server1.Cube.TotalBucketRead,
			server3.Cube.BucketReadCount,
			server3.Cube.TotalBucketRead,
		)
		elapsedSeconds := time.Since(start).Seconds()

		row := []string{
			strconv.Itoa(bit),
			strconv.Itoa(n),
			strconv.Itoa(z),
			strconv.Itoa(pl),
			strconv.Itoa(repeats),
			strconv.Itoa(fixedAddr),
			strconv.FormatInt(seed, 10),
			strconv.FormatFloat(randomVsRandomDistance, 'g', 12, 64),
			strconv.FormatFloat(randomVsFixedDistance, 'g', 12, 64),
			strconv.Itoa(len(client1.Stash)),
			strconv.Itoa(len(client2.Stash)),
			strconv.Itoa(len(client3.Stash)),
			strconv.FormatFloat(elapsedSeconds, 'g', 6, 64),
		}

		fmt.Println(
			row[0] + "," + row[1] + "," + row[2] + "," + row[3] + "," +
				row[4] + "," + row[5] + "," + row[6] + "," + row[7] + "," +
				row[8] + "," + row[9] + "," + row[10] + "," + row[11] + "," + row[12],
		)
		bitExperimentWriteRow(writer, row...)
	}
}
