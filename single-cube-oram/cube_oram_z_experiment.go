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
	zExperimentDefaultRepeats   = 1000
	zExperimentDefaultBlockNum  = 128
	zExperimentDefaultBit       = 7
	zExperimentDefaultMaxZ      = 10
	zExperimentDefaultFixedAddr = 10
	zExperimentDefaultCSVPath   = "cube_oram_z_experiment.csv"
)

func zExperimentParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func zExperimentBuildInitialState(n int, bit int, z int, pl int, seed int64) (ORAMCube, []int, []CubeDataBlock) {
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

func zExperimentRunRandomAccess(client *CubeClient, server *CubeServer) {
	client.Counter = server.GiveCounter()
	path := client.GetRandomData()
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func zExperimentRunFixedAccess(client *CubeClient, server *CubeServer, fixedAddr int) {
	client.Counter = server.GiveCounter()
	path := client.GetData(fixedAddr)
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func zExperimentWriteRow(writer *csv.Writer, values ...string) {
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
	repeats := zExperimentDefaultRepeats
	n := zExperimentDefaultBlockNum
	bit := zExperimentDefaultBit
	maxZ := zExperimentDefaultMaxZ
	fixedAddr := zExperimentDefaultFixedAddr
	seed := time.Now().UnixNano()
	csvPath := zExperimentDefaultCSVPath

	var err error
	if repeats, err = zExperimentParseIntArg(args, 1, repeats); err != nil {
		panic(err)
	}
	if n, err = zExperimentParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if bit, err = zExperimentParseIntArg(args, 3, bit); err != nil {
		panic(err)
	}
	if maxZ, err = zExperimentParseIntArg(args, 4, maxZ); err != nil {
		panic(err)
	}
	if fixedAddr, err = zExperimentParseIntArg(args, 5, fixedAddr); err != nil {
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
		csvPath = args[7]
	}
	if maxZ < 1 {
		panic("maxZ must be at least 1")
	}
	if fixedAddr < 1 || fixedAddr > n {
		panic("fixed address must be within 1..N")
	}
	if n > maxZ*(1<<bit) {
		panic("N is too large for maxZ and Bit: initial placement must have enough total bucket capacity")
	}

	pl := 2 * bit
	if pl >= 1<<bit {
		panic("PL must be smaller than 2^Bit for a simple path")
	}

	file, err := os.Create(csvPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"Z",
		"N",
		"Bit",
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
	zExperimentWriteRow(writer, header...)

	fmt.Printf("repeats: %d\n", repeats)
	fmt.Printf("N: %d\n", n)
	fmt.Printf("Bit: %d\n", bit)
	fmt.Printf("PL: %d\n", pl)
	fmt.Printf("Z range: 1..%d\n", maxZ)
	fmt.Printf("fixed address: %d\n", fixedAddr)
	fmt.Printf("seed: %d\n", seed)
	fmt.Printf("csv: %s\n", csvPath)
	fmt.Println("Z,N,Bit,PL,repeats,fixed_addr,seed,random_vs_random_distance,random_vs_fixed_distance,random1_stash_size,random2_stash_size,fixed_stash_size,elapsed_seconds")

	for z := 1; z <= maxZ; z++ {
		start := time.Now()
		cube, pm, stash := zExperimentBuildInitialState(n, bit, z, pl, seed)

		server1 := NewCubeServer(cube)
		server2 := NewCubeServer(cube)
		server3 := NewCubeServer(cube)
		client1 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(z)*100+1)))
		client2 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(z)*100+2)))
		client3 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(z)*100+3)))

		for i := 0; i < repeats; i++ {
			zExperimentRunRandomAccess(&client1, &server1)
			zExperimentRunRandomAccess(&client2, &server2)
			zExperimentRunFixedAccess(&client3, &server3, fixedAddr)
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
			strconv.Itoa(z),
			strconv.Itoa(n),
			strconv.Itoa(bit),
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
		zExperimentWriteRow(writer, row...)
	}
}
