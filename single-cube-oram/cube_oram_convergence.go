package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	defaultN           = 128
	defaultBit         = 7
	defaultZ           = 4
	defaultPL          = 10
	defaultAccessCount = 100000
	defaultFixedAddr   = 10
)

func parseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func runRandomAccess(client *CubeClient, server *CubeServer) {
	client.Counter = server.GiveCounter()
	path := client.GetRandomData()
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func runFixedAccess(client *CubeClient, server *CubeServer, fixedAddr int) {
	client.Counter = server.GiveCounter()
	path := client.GetData(fixedAddr)
	blocks := server.GetPath(path)
	shuffled := client.Shuffle(blocks)
	server.Reallocation(shuffled)
}

func defaultCheckpoints(maxAccesses int) []int {
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

func buildInitialState(n int, bit int, z int, pl int, seed int64) (ORAMCube, []int, []CubeDataBlock) {
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

func main() {
	args := os.Args
	n := defaultN
	bit := defaultBit
	z := defaultZ
	pl := defaultPL
	maxAccesses := defaultAccessCount
	fixedAddr := defaultFixedAddr
	seed := time.Now().UnixNano()

	var err error
	if maxAccesses, err = parseIntArg(args, 1, maxAccesses); err != nil {
		panic(err)
	}
	if n, err = parseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if bit, err = parseIntArg(args, 3, bit); err != nil {
		panic(err)
	}
	if z, err = parseIntArg(args, 4, z); err != nil {
		panic(err)
	}
	if pl, err = parseIntArg(args, 5, pl); err != nil {
		panic(err)
	}
	if fixedAddr, err = parseIntArg(args, 6, fixedAddr); err != nil {
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

	cube, pm, stash := buildInitialState(n, bit, z, pl, seed)

	server1 := NewCubeServer(cube)
	server2 := NewCubeServer(cube)
	server3 := NewCubeServer(cube)
	client1 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+1)))
	client2 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+2)))
	client3 := NewCubeClient(pm, stash, bit, z, pl, rand.New(rand.NewSource(seed+3)))

	fmt.Printf("N: %d\n", n)
	fmt.Printf("Bit: %d\n", bit)
	fmt.Printf("Z: %d\n", z)
	fmt.Printf("PL: %d\n", pl)
	fmt.Printf("max accesses per workflow: %d\n", maxAccesses)
	fmt.Printf("fixed address: %d\n", fixedAddr)
	fmt.Printf("seed: %d\n", seed)
	fmt.Println("accesses,random_vs_random_distance,random_vs_fixed_distance,client1_stash_size,client2_stash_size,client3_stash_size,elapsed_seconds")

	checkpoints := defaultCheckpoints(maxAccesses)
	start := time.Now()
	completed := 0

	for _, checkpoint := range checkpoints {
		for completed < checkpoint {
			runRandomAccess(&client1, &server1)
			runRandomAccess(&client2, &server2)
			runFixedAccess(&client3, &server3, fixedAddr)
			completed++
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
			checkpoint,
			randomVsRandomDistance,
			randomVsFixedDistance,
			len(client1.Stash),
			len(client2.Stash),
			len(client3.Stash),
			elapsedSeconds,
		)
	}
}
