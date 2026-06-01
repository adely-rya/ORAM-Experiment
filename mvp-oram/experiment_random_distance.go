//go:build random_distance

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
	"strconv"
	"time"
)

const (
	randomDistanceDefaultMaxAccesses  = 1000000
	randomDistanceDefaultN            = 256
	randomDistanceDefaultL            = 8
	randomDistanceDefaultZ            = 4
	randomDistanceDefaultFixedAddr    = 10
	randomDistanceDefaultInitialSeed  = 542
	randomDistanceDefaultCSVPath      = "CSV/mvp_oram_random_distance3.csv"
	randomDistanceDefaultCompactEvery = mvpDefaultPathMapCompactInterval
	randomDistanceDefaultCompactTail  = mvpDefaultPathMapCompactProtectedTail
)

type randomDistanceSnapshot struct {
	counts map[MvpBucketPosition]int64
	total  int64
}

func randomDistanceParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func randomDistanceParseInt64Arg(args []string, index int, fallback int64) (int64, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.ParseInt(args[index], 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func randomDistanceCheckpoints(maxAccesses int) []int {
	checkpoints := make([]int, 0)
	for value := 10; value <= maxAccesses; value *= 10 {
		checkpoints = append(checkpoints, value)
		if value > math.MaxInt/10 {
			break
		}
	}
	return checkpoints
}

func randomDistanceRunAccesses(accesses int, n int, l int, z int, initialSeed int64, runSeed int64, fixedAddr int, compactEvery int, compactTail int) (randomDistanceSnapshot, error) {
	server := NewMvpServer(z, l)
	server.SetPathMapCompaction(compactEvery, compactTail)
	positionMap := server.InitializeRandomData(n, initialSeed)
	go server.Run()
	defer close(server.Requests)

	client := NewMvpClient(
		l,
		z,
		0,
		clonePositionMap(positionMap),
		server.Requests,
	)

	rng := rand.New(rand.NewSource(runSeed))
	rand.Seed(runSeed)
	for i := 0; i < accesses; i++ {
		target := rng.Intn(n)
		if fixedAddr >= 0 {
			target = fixedAddr
		}
		op := OramOP{
			OP:     Write,
			target: target,
			param:  fmt.Sprintf("run-%d-access-%d-addr-%d", runSeed, i, target),
		}
		if err := client.Access(op); err != nil {
			return randomDistanceSnapshot{}, err
		}
	}

	counts := make(map[MvpBucketPosition]int64, len(server.tree.BucketReadCount))
	for position, count := range server.tree.BucketReadCount {
		counts[position] = count
	}

	return randomDistanceSnapshot{
		counts: counts,
		total:  server.tree.TotalBucketRead,
	}, nil
}

func randomDistanceStatisticalDistance(left randomDistanceSnapshot, right randomDistanceSnapshot) float64 {
	if left.total == 0 || right.total == 0 {
		return 0
	}

	sum := 0.0
	for position, leftCount := range left.counts {
		leftProbability := float64(leftCount) / float64(left.total)
		rightProbability := float64(right.counts[position]) / float64(right.total)
		sum += math.Abs(leftProbability - rightProbability)
	}

	for position, rightCount := range right.counts {
		if _, ok := left.counts[position]; ok {
			continue
		}
		rightProbability := float64(rightCount) / float64(right.total)
		sum += math.Abs(rightProbability)
	}

	return 0.5 * sum
}

func randomDistanceWriteRow(writer *csv.Writer, values ...string) error {
	if err := writer.Write(values); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func main() {
	log.SetOutput(io.Discard)

	args := os.Args
	maxAccesses := randomDistanceDefaultMaxAccesses
	n := randomDistanceDefaultN
	l := randomDistanceDefaultL
	z := randomDistanceDefaultZ
	fixedAddr := randomDistanceDefaultFixedAddr
	initialSeed := int64(randomDistanceDefaultInitialSeed)
	outputPath := randomDistanceDefaultCSVPath
	compactEvery := randomDistanceDefaultCompactEvery
	compactTail := randomDistanceDefaultCompactTail

	var err error
	if maxAccesses, err = randomDistanceParseIntArg(args, 1, maxAccesses); err != nil {
		panic(err)
	}
	if n, err = randomDistanceParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if l, err = randomDistanceParseIntArg(args, 3, l); err != nil {
		panic(err)
	}
	if z, err = randomDistanceParseIntArg(args, 4, z); err != nil {
		panic(err)
	}
	if fixedAddr, err = randomDistanceParseIntArg(args, 5, fixedAddr); err != nil {
		panic(err)
	}
	if initialSeed, err = randomDistanceParseInt64Arg(args, 6, initialSeed); err != nil {
		panic(err)
	}
	if len(args) >= 8 {
		outputPath = args[7]
	}
	if compactEvery, err = randomDistanceParseIntArg(args, 8, compactEvery); err != nil {
		panic(err)
	}
	if compactTail, err = randomDistanceParseIntArg(args, 9, compactTail); err != nil {
		panic(err)
	}
	if fixedAddr < 0 || fixedAddr >= n {
		panic("fixed address must be within 0..N-1")
	}

	checkpoints := randomDistanceCheckpoints(maxAccesses)
	if len(checkpoints) == 0 {
		panic("max accesses must be at least 10")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		panic(err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := randomDistanceWriteRow(
		writer,
		"accesses",
		"n",
		"l",
		"z",
		"fixed_addr",
		"initial_seed",
		"pathmap_compact_every",
		"pathmap_compact_protected_tail",
		"run1_seed",
		"run2_seed",
		"run3_seed",
		"run1_total_bucket_reads",
		"run2_total_bucket_reads",
		"run3_total_bucket_reads",
		"random_vs_random_distance",
		"random_vs_fixed_distance",
		"elapsed_ms",
	); err != nil {
		panic(err)
	}

	started := time.Now()
	for _, accesses := range checkpoints {
		run1Seed := initialSeed + int64(accesses)*2 + 1
		run2Seed := initialSeed + int64(accesses)*2 + 2
		run3Seed := initialSeed + int64(accesses)*2 + 3

		run1, err := randomDistanceRunAccesses(accesses, n, l, z, initialSeed, run1Seed, -1, compactEvery, compactTail)
		if err != nil {
			panic(err)
		}
		run2, err := randomDistanceRunAccesses(accesses, n, l, z, initialSeed, run2Seed, -1, compactEvery, compactTail)
		if err != nil {
			panic(err)
		}
		run3, err := randomDistanceRunAccesses(accesses, n, l, z, initialSeed, run3Seed, fixedAddr, compactEvery, compactTail)
		if err != nil {
			panic(err)
		}

		randomVsRandomDistance := randomDistanceStatisticalDistance(run1, run2)
		randomVsFixedDistance := randomDistanceStatisticalDistance(run1, run3)
		if err := randomDistanceWriteRow(
			writer,
			strconv.Itoa(accesses),
			strconv.Itoa(n),
			strconv.Itoa(l),
			strconv.Itoa(z),
			strconv.Itoa(fixedAddr),
			strconv.FormatInt(initialSeed, 10),
			strconv.Itoa(compactEvery),
			strconv.Itoa(compactTail),
			strconv.FormatInt(run1Seed, 10),
			strconv.FormatInt(run2Seed, 10),
			strconv.FormatInt(run3Seed, 10),
			strconv.FormatInt(run1.total, 10),
			strconv.FormatInt(run2.total, 10),
			strconv.FormatInt(run3.total, 10),
			strconv.FormatFloat(randomVsRandomDistance, 'f', 10, 64),
			strconv.FormatFloat(randomVsFixedDistance, 'f', 10, 64),
			strconv.FormatInt(time.Since(started).Milliseconds(), 10),
		); err != nil {
			panic(err)
		}

		fmt.Printf(
			"accesses=%d random_vs_random=%.10f random_vs_fixed=%.10f output=%s\n",
			accesses,
			randomVsRandomDistance,
			randomVsFixedDistance,
			outputPath,
		)
	}
}
