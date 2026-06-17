package main

import (
	"math"
	"testing"
)

func TestIdealStatisticalDistanceRandomKDistributionSingleClient(t *testing.T) {
	distribution := idealStatisticalDistanceRandomKDistribution(6, 1)
	if distribution[0] != 0 {
		t.Fatalf("distribution[0] = %f, want 0", distribution[0])
	}
	if distribution[1] != 1 {
		t.Fatalf("distribution[1] = %f, want 1", distribution[1])
	}
}

func TestIdealStatisticalDistanceRandomKDistributionTwoClients(t *testing.T) {
	const pathLen = 4
	leafCount := float64(1 << pathLen)
	distribution := idealStatisticalDistanceRandomKDistribution(pathLen, 2)

	if math.Abs(distribution[1]-1/leafCount) > 1e-12 {
		t.Fatalf("distribution[1] = %.15f, want %.15f", distribution[1], 1/leafCount)
	}
	if math.Abs(distribution[2]-(leafCount-1)/leafCount) > 1e-12 {
		t.Fatalf("distribution[2] = %.15f, want %.15f", distribution[2], (leafCount-1)/leafCount)
	}
}

func TestIdealStatisticalDistanceRandomKDistributionSumsToOne(t *testing.T) {
	distribution := idealStatisticalDistanceRandomKDistribution(8, 20)
	sum := 0.0
	for _, probability := range distribution {
		sum += probability
	}
	if math.Abs(sum-1) > 1e-12 {
		t.Fatalf("sum = %.15f, want 1", sum)
	}
}

func TestMergeStatisticalDistanceLeafIntervals(t *testing.T) {
	merged := mergeStatisticalDistanceLeafIntervals([]statisticalDistanceLeafInterval{
		{start: 4, end: 8},
		{start: 0, end: 2},
		{start: 2, end: 5},
		{start: 12, end: 16},
	})

	if len(merged) != 2 {
		t.Fatalf("len(merged) = %d, want 2", len(merged))
	}
	if merged[0] != (statisticalDistanceLeafInterval{start: 0, end: 8}) {
		t.Fatalf("merged[0] = %+v, want {start:0 end:8}", merged[0])
	}
	if merged[1] != (statisticalDistanceLeafInterval{start: 12, end: 16}) {
		t.Fatalf("merged[1] = %+v, want {start:12 end:16}", merged[1])
	}
}

func TestStatisticalDistancePrefixInterval(t *testing.T) {
	interval := statisticalDistancePrefixInterval("10", 4)
	if interval != (statisticalDistanceLeafInterval{start: 8, end: 12}) {
		t.Fatalf("interval = %+v, want {start:8 end:12}", interval)
	}
}
