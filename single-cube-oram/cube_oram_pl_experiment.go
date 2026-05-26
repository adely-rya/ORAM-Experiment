package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	plExperimentDefaultRepeats   = 100000
	plExperimentDefaultBlockNum  = 128
	plExperimentDefaultBit       = 7
	plExperimentDefaultZ         = 4
	plExperimentDefaultMaxPL     = 14
	plExperimentDefaultFixedAddr = 10
)

func plExperimentParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func plExperimentBuildInitialState(n int, bit int, z int, pl int, seed int64) (ORAMCube, []int, []CubeDataBlock) {
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

func plExperimentRunRandomAccess(client *CubeClient, server *CubeServer) {
	client.Counter = server.GiveCounter()
	path := client.GetRandomData()
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func plExperimentRunFixedAccess(client *CubeClient, server *CubeServer, fixedAddr int) {
	client.Counter = server.GiveCounter()
	path := client.GetData(fixedAddr)
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func main() {
	args := os.Args
	repeats := plExperimentDefaultRepeats
	n := plExperimentDefaultBlockNum
	bit := plExperimentDefaultBit
	z := plExperimentDefaultZ
	maxPL := plExperimentDefaultMaxPL
	fixedAddr := plExperimentDefaultFixedAddr
	seed := time.Now().UnixNano()

	var err error
	if repeats, err = plExperimentParseIntArg(args, 1, repeats); err != nil {
		panic(err)
	}
	if n, err = plExperimentParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if bit, err = plExperimentParseIntArg(args, 3, bit); err != nil {
		panic(err)
	}
	if z, err = plExperimentParseIntArg(args, 4, z); err != nil {
		panic(err)
	}
	if maxPL, err = plExperimentParseIntArg(args, 5, maxPL); err != nil {
		panic(err)
	}
	if fixedAddr, err = plExperimentParseIntArg(args, 6, fixedAddr); err != nil {
		panic(err)
	}
	if len(args) >= 8 {
		parsedSeed, parseErr := strconv.ParseInt(args[7], 10, 64)
		if parseErr != nil {
			panic(parseErr)
		}
		seed = parsedSeed
	}
	if fixedAddr < 1 || fixedAddr > n {
		panic("fixed address must be within 1..N")
	}
	if maxPL < bit {
		panic("maxPL must be at least Bit")
	}
	if maxPL >= 1<<bit {
		panic("maxPL must be smaller than 2^Bit for a simple path")
	}

	fmt.Printf("repeats: %d\n", repeats)
	fmt.Printf("N: %d\n", n)
	fmt.Printf("Bit: %d\n", bit)
	fmt.Printf("Z: %d\n", z)
	fmt.Printf("PL range: %d..%d\n", bit, maxPL)
	fmt.Printf("fixed address: %d\n", fixedAddr)
	fmt.Printf("seed: %d\n", seed)
	fmt.Println("PL,random_vs_random_distance,random_vs_fixed_distance,random1_stash_size,random2_stash_size,fixed_stash_size,elapsed_seconds")

	for pl := bit; pl <= maxPL; pl++ {
		start := time.Now()
		cube, pm, stash := plExperimentBuildInitialState(n, bit, z, pl, seed)

		server1 := NewCubeServer(cube)
		server2 := NewCubeServer(cube)
		server3 := NewCubeServer(cube)
		client1 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(pl)*100+1)))
		client2 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(pl)*100+2)))
		client3 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+int64(pl)*100+3)))

		for i := 0; i < repeats; i++ {
			plExperimentRunRandomAccess(&client1, &server1)
			plExperimentRunRandomAccess(&client2, &server2)
			plExperimentRunFixedAccess(&client3, &server3, fixedAddr)
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

		fmt.Printf(
			"%d,%.12g,%.12g,%d,%d,%d,%.6g\n",
			pl,
			randomVsRandomDistance,
			randomVsFixedDistance,
			len(client1.Stash),
			len(client2.Stash),
			len(client3.Stash),
			elapsedSeconds,
		)
	}
}
