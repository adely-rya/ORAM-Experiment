//go:build latency

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
	"sort"
	"strconv"
	"time"
)

const (
	latencyDefaultMaxAccesses  = 10000
	latencyDefaultN            = 256
	latencyDefaultL            = 8
	latencyDefaultZ            = 4
	latencyDefaultSeed         = 542
	latencyDefaultCSVPath      = "CSV/mvp_oram_latency.csv"
	latencyDefaultCompactEvery = mvpDefaultPathMapCompactInterval
	latencyDefaultCompactTail  = mvpDefaultPathMapCompactProtectedTail
)

type latencyStats struct {
	values []int64
	min    int64
	max    int64
	sum    int64
}

func latencyParseIntArg(args []string, index int, fallback int) (int, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.Atoi(args[index])
	if err != nil {
		return 0, err
	}
	return value, nil
}

func latencyParseInt64Arg(args []string, index int, fallback int64) (int64, error) {
	if len(args) <= index {
		return fallback, nil
	}

	value, err := strconv.ParseInt(args[index], 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func latencyCheckpoints(maxAccesses int) []int {
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

func (s *latencyStats) add(duration time.Duration) {
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

func (s latencyStats) mean() float64 {
	if len(s.values) == 0 {
		return 0
	}
	return float64(s.sum) / float64(len(s.values))
}

func (s latencyStats) percentile(value float64) int64 {
	if len(s.values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), s.values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	index := int(math.Ceil(value/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func latencyWriteRow(writer *csv.Writer, values ...string) error {
	if err := writer.Write(values); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func latencyRunAccess(client *MvpClient, rng *rand.Rand, accessIndex int, n int) (time.Duration, error) {
	target := rng.Intn(n)
	op := OramOP{
		OP:     Write,
		target: target,
		param:  fmt.Sprintf("access-%d-addr-%d", accessIndex, target),
	}

	start := time.Now()
	err := client.Access(op)
	return time.Since(start), err
}

func main() {
	log.SetOutput(io.Discard)

	args := os.Args
	maxAccesses := latencyDefaultMaxAccesses
	n := latencyDefaultN
	l := latencyDefaultL
	z := latencyDefaultZ
	seed := int64(latencyDefaultSeed)
	outputPath := latencyDefaultCSVPath
	compactEvery := latencyDefaultCompactEvery
	compactTail := latencyDefaultCompactTail

	var err error
	if maxAccesses, err = latencyParseIntArg(args, 1, maxAccesses); err != nil {
		panic(err)
	}
	if n, err = latencyParseIntArg(args, 2, n); err != nil {
		panic(err)
	}
	if l, err = latencyParseIntArg(args, 3, l); err != nil {
		panic(err)
	}
	if z, err = latencyParseIntArg(args, 4, z); err != nil {
		panic(err)
	}
	if seed, err = latencyParseInt64Arg(args, 5, seed); err != nil {
		panic(err)
	}
	if len(args) >= 7 {
		outputPath = args[6]
	}
	if compactEvery, err = latencyParseIntArg(args, 7, compactEvery); err != nil {
		panic(err)
	}
	if compactTail, err = latencyParseIntArg(args, 8, compactTail); err != nil {
		panic(err)
	}

	if maxAccesses <= 0 {
		panic("max accesses must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		panic(err)
	}

	server := NewMvpServer(z, l)
	server.SetPathMapCompaction(compactEvery, compactTail)
	positionMap := server.InitializeRandomData(n, seed)
	go server.Run()
	defer close(server.Requests)

	client := NewMvpClient(l, z, 0, clonePositionMap(positionMap), server.Requests)
	rng := rand.New(rand.NewSource(seed + 1))
	rand.Seed(seed + 2)

	file, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := latencyWriteRow(
		writer,
		"accesses",
		"window_start",
		"window_end",
		"n",
		"l",
		"z",
		"seed",
		"pathmap_compact_every",
		"pathmap_compact_protected_tail",
		"window_min_ns",
		"window_mean_ns",
		"window_p50_ns",
		"window_p90_ns",
		"window_p95_ns",
		"window_p99_ns",
		"window_max_ns",
		"cumulative_mean_ns",
		"cumulative_p95_ns",
		"total_bucket_reads",
		"elapsed_ms",
	); err != nil {
		panic(err)
	}

	checkpoints := latencyCheckpoints(maxAccesses)
	nextCheckpointIndex := 0
	nextCheckpoint := checkpoints[nextCheckpointIndex]
	cumulative := latencyStats{values: make([]int64, 0, maxAccesses)}
	window := latencyStats{values: make([]int64, 0, nextCheckpoint)}
	windowStart := 1
	started := time.Now()

	for access := 1; access <= maxAccesses; access++ {
		duration, err := latencyRunAccess(client, rng, access, n)
		if err != nil {
			panic(err)
		}
		cumulative.add(duration)
		window.add(duration)

		if access != nextCheckpoint {
			continue
		}

		if err := latencyWriteRow(
			writer,
			strconv.Itoa(access),
			strconv.Itoa(windowStart),
			strconv.Itoa(access),
			strconv.Itoa(n),
			strconv.Itoa(l),
			strconv.Itoa(z),
			strconv.FormatInt(seed, 10),
			strconv.Itoa(compactEvery),
			strconv.Itoa(compactTail),
			strconv.FormatInt(window.min, 10),
			strconv.FormatFloat(window.mean(), 'f', 2, 64),
			strconv.FormatInt(window.percentile(50), 10),
			strconv.FormatInt(window.percentile(90), 10),
			strconv.FormatInt(window.percentile(95), 10),
			strconv.FormatInt(window.percentile(99), 10),
			strconv.FormatInt(window.max, 10),
			strconv.FormatFloat(cumulative.mean(), 'f', 2, 64),
			strconv.FormatInt(cumulative.percentile(95), 10),
			strconv.FormatInt(server.tree.TotalBucketRead, 10),
			strconv.FormatInt(time.Since(started).Milliseconds(), 10),
		); err != nil {
			panic(err)
		}

		fmt.Printf(
			"accesses=%d window_mean=%.2fms cumulative_mean=%.2fms output=%s\n",
			access,
			window.mean()/float64(time.Millisecond),
			cumulative.mean()/float64(time.Millisecond),
			outputPath,
		)

		nextCheckpointIndex++
		if nextCheckpointIndex >= len(checkpoints) {
			continue
		}
		windowStart = access + 1
		nextCheckpoint = checkpoints[nextCheckpointIndex]
		window = latencyStats{values: make([]int64, 0, nextCheckpoint-windowStart+1)}
	}
}
