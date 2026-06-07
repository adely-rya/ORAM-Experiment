// PositionMap path-count distribution experiment.
package main

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

type experiment4Result struct {
	clientCount  int
	alpha        float32
	distribution map[int]int
	total        int
}

func Experiment4() {
	clientCounts := experimentClientCounts(minClient, maxClient)
	results := make(chan experiment4Result, len(clientCounts)*len(experimentAlphas))

	for _, clientCount := range clientCounts {
		var experimentwg sync.WaitGroup
		for _, alpha := range experimentAlphas {
			experimentwg.Add(1)
			go positionMapPathCountDistribution(alpha, clientCount, results, &experimentwg)
		}
		experimentwg.Wait()
	}
	close(results)

	printExperiment4Results(results, len(clientCounts)*len(experimentAlphas))
}

func positionMapPathCountDistribution(alpha float32, clientCount int, results chan<- experiment4Result, experimentwg *sync.WaitGroup) {
	defer experimentwg.Done()

	server := NewMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)
	go server.Run()

	clients := make([]*client_set, 0, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewMvpClient(l, z, clientID, clonePositionMap(positionmap), server.Requests)
		clients = append(clients, NewCientSet(client))
	}

	for i := 0; i < WarmUp; i++ {
		if err := runConcurrentWarmupAccessTrial(clients, alpha); err != nil {
			log.Printf("experiment4 warmup access error: client_count=%d alpha=%.6f err=%v", clientCount, alpha, err)
			return
		}
	}
	syncClientPositionMaps(clients)

	distribution := makePathCountDistribution(l)
	total := 0
	for i := 0; i < MonteCarlo; i++ {
		if err := runConcurrentWarmupAccessTrial(clients, alpha); err != nil {
			log.Printf("experiment4 measurement access error: client_count=%d alpha=%.6f trial=%d err=%v", clientCount, alpha, i, err)
			return
		}
		syncClientPositionMaps(clients)
		total += observePositionMapPathCounts(clients, distribution)
	}

	results <- experiment4Result{
		clientCount:  clientCount,
		alpha:        alpha,
		distribution: distribution,
		total:        total,
	}
}

func makePathCountDistribution(pathLen int) map[int]int {
	distribution := make(map[int]int, pathLen+1)
	for k := 1; k <= 1<<pathLen; k *= 2 {
		distribution[k] = 0
	}
	return distribution
}

func observePositionMapPathCounts(clients []*client_set, distribution map[int]int) int {
	total := 0
	for _, clientSet := range clients {
		for _, entry := range clientSet.client.PositionMap {
			distribution[positionPathCount(entry.Slot, clientSet.client.L)]++
			total++
		}
	}
	return total
}

func positionPathCount(position MvpPosition, pathLen int) int {
	switch position.bucket {
	case mvpRootBucketPosition, mvpStashBucketPosition:
		return 1 << pathLen
	default:
		depth := len(position.bucket.String())
		if depth <= 0 || depth > pathLen {
			return 1 << pathLen
		}
		return 1 << (pathLen - depth)
	}
}

func printExperiment4Results(results <-chan experiment4Result, resultCount int) {
	ordered := make([]experiment4Result, 0, resultCount)
	for result := range results {
		ordered = append(ordered, result)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].clientCount != ordered[j].clientCount {
			return ordered[i].clientCount < ordered[j].clientCount
		}
		return ordered[i].alpha < ordered[j].alpha
	})

	fmt.Println("client_count,alpha,k,count,total,percentage")
	for _, result := range ordered {
		for _, k := range experiment4KValues(l) {
			count := result.distribution[k]
			percentage := 0.0
			if result.total > 0 {
				percentage = float64(count) / float64(result.total) * 100
			}
			fmt.Printf("%d,%.6f,%d,%d,%d,%.10f\n", result.clientCount, result.alpha, k, count, result.total, percentage)
		}
	}
}

func experiment4KValues(pathLen int) []int {
	values := make([]int, 0, pathLen+1)
	for k := 1; k <= 1<<pathLen; k *= 2 {
		values = append(values, k)
	}
	return values
}
