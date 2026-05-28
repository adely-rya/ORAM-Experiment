package main

import (
	"math"
	"math/rand"
	"sort"
)

const (
	stashPosition = -1
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

func (c *CubeClient) GetData_alg1(addr int) []int { //adelyが考えたアルゴリズム
	c.accessBlock = addr

	root := 0
	target := c.PM[addr]
	if target == stashPosition || target == root {
		target = c.RNG.Intn(1 << c.Bit)
	}

	distance := bitsCount(root ^ target)
	possibleTargetSteps := make([]int, 0, c.PL-distance+1)
	if distance == 0 {
		possibleTargetSteps = append(possibleTargetSteps, 0)
	} else {
		for step := distance; step <= c.PL; step++ {
			if step%2 == distance%2 {
				possibleTargetSteps = append(possibleTargetSteps, step)
			}
		}
	}
	if len(possibleTargetSteps) == 0 {
		panic("cannot build a path through target with this PL")
	}

	const maxRetry = 1000
	for retry := 0; retry < maxRetry; retry++ {
		selectedTargetStep := possibleTargetSteps[c.RNG.Intn(len(possibleTargetSteps))]
		path := make([]int, 0, c.PL+1)
		visited := make(map[int]bool, c.PL+1)
		current := root
		path = append(path, current)
		visited[current] = true
		success := true

		for len(path)-1 < selectedTargetStep {
			currentStep := len(path) - 1
			remainingToTarget := selectedTargetStep - currentStep
			candidates := make([]int, 0, c.Bit)

			for bit := 0; bit < c.Bit; bit++ {
				next := flipHypercubeBit(current, bit, c.Bit)
				if visited[next] {
					continue
				}

				nextRemaining := remainingToTarget - 1
				distToTarget := bitsCount(next ^ target)
				if distToTarget > nextRemaining {
					continue
				}
				if distToTarget%2 != nextRemaining%2 {
					continue
				}
				if next == target && nextRemaining != 0 {
					continue
				}

				candidates = append(candidates, next)
			}

			if len(candidates) == 0 {
				success = false
				break
			}

			current = candidates[c.RNG.Intn(len(candidates))]
			path = append(path, current)
			visited[current] = true
		}

		if !success || current != target {
			continue
		}

		for len(path)-1 < c.PL {
			candidates := unvisitedNeighbors(current, c.Bit, visited)
			if len(candidates) == 0 {
				success = false
				break
			}

			current = candidates[c.RNG.Intn(len(candidates))]
			path = append(path, current)
			visited[current] = true
		}

		if success {
			c.pathList = path
			return path
		}
	}

	panic("failed to build a simple path")
}

func (c *CubeClient) GetData(addr int) []int {
	return c.GetData_alg1(addr)
}

func (c *CubeClient) GetData_alg2(addr int) []int { //adelyが考えたアルゴリズム
	c.accessBlock = addr

	root := 0
	target := c.PM[addr]
	if target == stashPosition || target == root {
		target = c.RNG.Intn(1 << c.Bit)
	}

	for {
		path := make([]int, 0, c.PL+1)
		visited := make(map[int]bool, 0)
		current := 0
		success := true

		path = append(path, current)
		visited[current] = true

		for len(path) < c.PL+1 {
			candidates := unvisitedNeighbors(current, c.Bit, visited)
			if len(candidates) == 0 {
				success = false
				break
			}

			current = candidates[c.RNG.Intn(len(candidates))]
			path = append(path, current)
			visited[current] = true
		}

		if !success {
			continue
		}

		for _, n := range path {
			if n == target {
				c.pathList = path
				return path
			}
		}
	}

}

func (c *CubeClient) GetRandomData() []int {
	return c.GetData_alg1(1 + c.RNG.Intn(len(c.PM)-1))
}

func (c *CubeClient) Shuffle(blocks []CubeDataBlock) map[int]CubeBucket {
	if len(c.pathList) == 0 {
		panic("GetData or GetRandomData must be called before Shuffle")
	}

	shuffled := make(map[int]CubeBucket, len(c.pathList))
	allBlocks := make([]CubeDataBlock, 0, len(blocks)+len(c.Stash))

	for _, position := range c.pathList {
		shuffled[position] = NewCubeBucket(c.Z)
	}

	allBlocks = append(allBlocks, blocks...)
	allBlocks = append(allBlocks, c.Stash...)

	keyList := append([]int(nil), c.pathList...)
	newStash := make([]CubeDataBlock, 0)

	for _, block := range allBlocks {
		availableKeys := make([]int, 0, len(keyList))
		for _, key := range keyList {
			if len(shuffled[key].Value) < c.Z {
				availableKeys = append(availableKeys, key)
			}
		}

		if len(availableKeys) == 0 {
			newStash = append(newStash, block)
			c.PM[block.Addr] = stashPosition
			continue
		}

		key := availableKeys[c.RNG.Intn(len(availableKeys))]
		if c.accessBlock == block.Addr {
			sort.Slice(availableKeys, func(i, j int) bool {
				return bitsCount(availableKeys[i]) < bitsCount(availableKeys[j])
			})
			key = availableKeys[0]
		}

		bucket := shuffled[key]
		bucket.SetBlock(block)
		shuffled[key] = bucket
		c.PM[block.Addr] = key
	}

	c.Stash = newStash
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

func statisticalDistance(count1 []int64, total1 int64, count2 []int64, total2 int64) float64 {
	if total1 == 0 || total2 == 0 {
		return 0
	}

	sum := 0.0
	maxLen := len(count1)
	if len(count2) > maxLen {
		maxLen = len(count2)
	}
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
