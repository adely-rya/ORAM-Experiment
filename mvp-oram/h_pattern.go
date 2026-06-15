package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const mvpHPatternCSVPath = "CSV/mvp_oram_h_pattern.csv"

func HPatternDistribution() {
	const (
		defaultZ           = 5
		defaultL           = 12
		defaultSeed        = 542
		defaultClientCount = 20
		defaultRounds      = 1000
		defaultAddrAlpha   = 1.0
		defaultOpAlpha     = 1.5
	)

	z := readMvpHPatternPositiveIntEnv("MVP_H_PATTERN_Z", defaultZ)
	l := readMvpHPatternPositiveIntEnv("MVP_H_PATTERN_L", defaultL)
	n := readMvpHPatternPositiveIntEnv("MVP_H_PATTERN_N", 1<<(l+1))
	clientCount := readMvpHPatternPositiveIntEnv("MVP_H_PATTERN_CLIENT_COUNT", defaultClientCount)
	rounds := readMvpHPatternPositiveIntEnv("MVP_H_PATTERN_ROUNDS", defaultRounds)
	addrAlpha := readMvpHPatternFloatEnv("MVP_H_PATTERN_ADDR_ALPHA", defaultAddrAlpha)
	opAlpha := readMvpHPatternFloatEnv("MVP_H_PATTERN_OP_ALPHA", defaultOpAlpha)

	log.Printf(
		"h-pattern running: implementation=mvp-oram z=%d l=%d n=%d clients=%d rounds=%d total_accesses=%d addr_alpha=%.6f op_alpha=%.6f snapshot=true",
		z,
		l,
		n,
		clientCount,
		rounds,
		clientCount*rounds,
		addrAlpha,
		opAlpha,
	)

	startedAt := time.Now()
	server := NewMvpServer(z, l)
	positionMap := server.InitializeRandomData(n, defaultSeed)
	go server.Run()

	clients := make([]*MvpClient, 0, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		clients = append(clients, NewMvpClient(l, z, clientID, clonePositionMap(positionMap), server.Requests))
	}

	if err := runMvpHPatternWarmupRounds(clients, rounds, addrAlpha, opAlpha, n-1); err != nil {
		log.Printf("h-pattern warmup error: err=%v", err)
		return
	}

	if err := syncMvpHPatternPositionMaps(clients); err != nil {
		log.Printf("h-pattern sync error: err=%v", err)
		return
	}

	distribution, summary := mvpHPatternDistribution(clients[0])
	if err := writeMvpHPatternCSV(mvpHPatternCSVPath, "mvp-oram", clientCount, rounds, clientCount*rounds, addrAlpha, opAlpha, distribution, summary); err != nil {
		log.Printf("h-pattern csv write error: err=%v", err)
		return
	}

	log.Printf(
		"h-pattern result: implementation=mvp-oram addresses=%d mean_h=%.6f max_h=%d csv=%s elapsed=%s",
		summary.total,
		summary.mean,
		summary.max,
		mvpHPatternCSVPath,
		time.Since(startedAt).Round(time.Millisecond),
	)
}

func runMvpHPatternWarmupRounds(clients []*MvpClient, rounds int, addrAlpha float64, opAlpha float64, addrMax int) error {
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousLogOutput)

	for round := 0; round < rounds; round++ {
		if err := runMvpHPatternWarmupRound(clients, addrAlpha, opAlpha, addrMax); err != nil {
			return err
		}
	}
	return nil
}

func runMvpHPatternWarmupRound(clients []*MvpClient, addrAlpha float64, opAlpha float64, addrMax int) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(clients))
	for _, client := range clients {
		op := mvpHPatternZipAccessOperation(addrAlpha, opAlpha, addrMax)
		wg.Add(1)
		go func(client *MvpClient, op OramOP) {
			defer wg.Done()
			if err := client.Access(op); err != nil {
				errs <- err
			}
		}(client, op)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func mvpHPatternZipAccessOperation(addrAlpha float64, opAlpha float64, addrMax int) OramOP {
	if addrAlpha < 0 {
		panic("zipf address alpha must be greater than or equal to 0")
	}
	if opAlpha < 0 {
		panic("zipf operation alpha must be greater than or equal to 0")
	}
	if addrMax < 0 {
		panic("addrMax must be greater than or equal to 0")
	}

	zipfGeneratorMu.Lock()
	addr := finiteZipfAddr(zipfGeneratorRand, addrAlpha, addrMax)
	opRank := finiteZipfAddr(zipfGeneratorRand, opAlpha, 1)
	zipfGeneratorMu.Unlock()

	operation := Read
	if opRank == 1 {
		operation = Write
	}

	return OramOP{
		OP:     operation,
		target: addr,
		param:  strconv.Itoa(addr),
	}
}

func syncMvpHPatternPositionMaps(clients []*MvpClient) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(clients))
	for _, client := range clients {
		wg.Add(1)
		go func(client *MvpClient) {
			defer wg.Done()
			version, pathMaps, err := client.GetPM()
			if err != nil {
				errs <- err
				return
			}
			client.seq = version
			client.consolidatePathMaps(pathMaps)
		}(client)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type mvpHPatternSummary struct {
	total int
	mean  float64
	max   int
}

func mvpHPatternDistribution(client *MvpClient) (map[int]int, mvpHPatternSummary) {
	distribution := make(map[int]int)
	totalH := 0
	maxH := 0

	for _, entry := range client.PositionMap {
		h := countMvpAddressPathPatterns(client, entry)
		distribution[h]++
		totalH += h
		if h > maxH {
			maxH = h
		}
	}

	mean := 0.0
	if len(client.PositionMap) > 0 {
		mean = float64(totalH) / float64(len(client.PositionMap))
	}
	return distribution, mvpHPatternSummary{total: len(client.PositionMap), mean: mean, max: maxH}
}

func countMvpAddressPathPatterns(client *MvpClient, entry MvpPositionMapEntry) int {
	prefix := mvpHPatternLeafPrefix(entry.Slot, client.L)
	return 1 << (client.L - len(prefix))
}

func mvpHPatternLeafPrefix(position MvpPosition, l int) string {
	bucket := position.bucket
	if bucket == mvpStashBucketPosition || bucket == mvpRootBucketPosition {
		return ""
	}
	prefix := bucket.String()
	if len(prefix) > l {
		prefix = prefix[:l]
	}
	return prefix
}

func writeMvpHPatternCSV(path string, implementation string, clientCount int, rounds int, totalAccesses int, addrAlpha float64, opAlpha float64, distribution map[int]int, summary mvpHPatternSummary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"implementation", "client_count", "rounds", "total_accesses", "addr_alpha", "op_alpha", "total_addresses", "mean_h", "max_h", "h", "count", "probability"}); err != nil {
		return err
	}

	keys := make([]int, 0, len(distribution))
	for h := range distribution {
		keys = append(keys, h)
	}
	sort.Ints(keys)

	for _, h := range keys {
		probability := 0.0
		if summary.total > 0 {
			probability = float64(distribution[h]) / float64(summary.total)
		}
		if err := writer.Write([]string{
			implementation,
			strconv.Itoa(clientCount),
			strconv.Itoa(rounds),
			strconv.Itoa(totalAccesses),
			strconv.FormatFloat(addrAlpha, 'f', 6, 64),
			strconv.FormatFloat(opAlpha, 'f', 6, 64),
			strconv.Itoa(summary.total),
			strconv.FormatFloat(summary.mean, 'f', 10, 64),
			strconv.Itoa(summary.max),
			strconv.Itoa(h),
			strconv.Itoa(distribution[h]),
			strconv.FormatFloat(probability, 'f', 10, 64),
		}); err != nil {
			return err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return file.Sync()
}

func readMvpHPatternPositiveIntEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		log.Printf("invalid %s=%q; using %d", name, value, fallback)
		return fallback
	}
	return parsed
}

func readMvpHPatternFloatEnv(name string, fallback float64) float64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("invalid %s=%q; using %.6f", name, value, fallback)
		return fallback
	}
	return parsed
}
