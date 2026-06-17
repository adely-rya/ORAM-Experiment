//go:build !random_distance && !latency

package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var accessLoggingEnabled = false //クライアントのアクセスログの表示切替

func main() {
	oramType := flag.String("oram", "re-mvp-oram", "oram implementation: re-mvp-oram or mvp-oram")
	runMode := flag.String("runmode", "async", "run mode: async or sync")
	experimentMode := flag.String("experimentmode", "stash-metrics", "experiment mode: stash-metrics")
	accessType := flag.String("accesstype", "random", "access type: random or zipf")
	flag.Parse()

	log.Printf("running: oram=%s runmode=%s experimentmode=%s accesstype=%s", *oramType, *runMode, *experimentMode, *accessType)
	RunExperiment(*runMode, *experimentMode, *accessType, *oramType)
}

func RunExperiment(runMode string, experimentMode string, accessType string, oramType string) {
	const (
		defaultZ                     = 4
		defaultL                     = 12
		seed                         = 542
		clientCount                  = 50
		readRatio                    = 0.8
		zipfAlpha                    = 1.1
		stashMetricsSequenceInterval = Version(100)
	)

	if normalizeExperimentMode(experimentMode) == "statisticaldistance" {
		RunStatisticalDistanceExperiment(oramType, accessType)
		return
	}

	newOperation := accessOperationFactory(accessType, readRatio, zipfAlpha)

	switch normalizeOramType(oramType) {
	case "re-mvp-oram":
		runReMvpExperiment(defaultZ, defaultL, seed, clientCount, stashMetricsSequenceInterval, runMode, experimentMode, newOperation)
	case "mvp-oram":
		runBaseMvpExperiment(defaultZ, defaultL, seed, clientCount, stashMetricsSequenceInterval, runMode, experimentMode, newOperation)
	default:
		panic("unknown oram")
	}
}

func normalizeExperimentMode(experimentMode string) string {
	switch strings.ToLower(experimentMode) {
	case "statisticaldistance", "statisticdistance", "statistical-distance", "statistic-distance":
		return "statisticaldistance"
	default:
		return strings.ToLower(experimentMode)
	}
}

func normalizeOramType(oramType string) string {
	switch strings.ToLower(oramType) {
	case "re-mvp-oram", "re_mvp_oram", "re":
		return "re-mvp-oram"
	case "mvp-oram", "mvp_oram", "mvp":
		return "mvp-oram"
	default:
		return strings.ToLower(oramType)
	}
}

func runReMvpExperiment(
	z int,
	l int,
	seed int64,
	clientCount int,
	metricsInterval Version,
	runMode string,
	experimentMode string,
	newOperation func(addrCount int, seed int64) AccessOperation,
) {
	server, n, positionmap := initializeRun(z, l, seed)
	startExperimentMetrics(experimentMode, server.Requests, metricsInterval)

	switch runMode {
	case "async":
		AsyncRun(z, l, seed, clientCount, n, positionmap, server.Requests, newOperation)
	case "sync":
		SyncRun(z, l, seed, clientCount, n, positionmap, server.Requests, newOperation)
	default:
		panic("unknown runmode")
	}
}

func startExperimentMetrics(experimentMode string, server chan<- ServerRequest, metricsInterval Version) {
	switch experimentMode {
	case "stash-metrics":
		go logStashMetricsBySequence(server, metricsInterval)
	default:
		panic("unknown experimentmode")
	}
}

func runBaseMvpExperiment(
	z int,
	l int,
	seed int64,
	clientCount int,
	metricsInterval Version,
	runMode string,
	experimentMode string,
	newOperation func(addrCount int, seed int64) AccessOperation,
) {
	server, n, positionmap := initializeBaseMvpRun(z, l, seed)
	startBaseMvpExperimentMetrics(experimentMode, server.Requests, metricsInterval)

	switch runMode {
	case "async":
		BaseMvpAsyncRun(l, z, seed, clientCount, n, positionmap, server.Requests, newOperation)
	case "sync":
		BaseMvpSyncRun(l, z, seed, clientCount, n, positionmap, server.Requests, newOperation)
	default:
		panic("unknown runmode")
	}
}

func startBaseMvpExperimentMetrics(experimentMode string, server chan<- BaseMvpServerRequest, metricsInterval Version) {
	switch experimentMode {
	case "stash-metrics":
		go logBaseMvpStashMetricsBySequence(server, metricsInterval)
	default:
		panic("unknown experimentmode")
	}
}

func accessOperationFactory(accessType string, readRatio float64, zipfAlpha float64) func(addrCount int, seed int64) AccessOperation {
	switch accessType {
	case "random":
		return func(addrCount int, seed int64) AccessOperation {
			return NewUniformAccessOperation(addrCount, seed)
		}
	case "zipf":
		return func(addrCount int, seed int64) AccessOperation {
			return NewZipfAccessOperation(addrCount, seed, readRatio, zipfAlpha)
		}
	default:
		panic("unknown accesstype")
	}
}

func AsyncRun(
	z int,
	l int,
	seed int64,
	clientCount int,
	addrCount int,
	positionmap map[int]map[int]MvpPositionMapEntry,
	server chan<- ServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) {
	errs := startAsyncAccessClients(l, z, addrCount, seed, clientCount, positionmap, server, newOperation)

	if err := <-errs; err != nil {
		panic(err)
	}
}

func initializeRun(z int, l int, seed int64) (*MvpServer, int, map[int]map[int]MvpPositionMapEntry) {
	n := 1 << (l - 1)
	server := NewMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)

	go server.Run()
	return server, n, positionmap
}

func startAsyncAccessClients(
	l int,
	z int,
	addrCount int,
	seed int64,
	clientCount int,
	positionmap map[int]map[int]MvpPositionMapEntry,
	server chan<- ServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) chan error {
	errs := make(chan error, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewMvpClient(
			l,
			z,
			clientID,
			clonePositionMap(positionmap),
			server,
		)

		go func(client *MvpClient) {
			operation := newOperation(addrCount, seed+int64(client.ClientID))
			if err := client.NormalRun(operation); err != nil {
				errs <- fmt.Errorf("client %d stopped: %w", client.ClientID, err)
			}
		}(client)
	}

	return errs
}

type syncAccessClient struct {
	client    *MvpClient
	operation AccessOperation

	op               OramOP
	accessSig        int
	accessLeaf       MvpPosition
	populatedPath    map[MvpPosition]MvpSlot
	populatedStash   []MvpDataBlock
	populatedPathMap []path
}

func SyncRun(
	z int,
	l int,
	seed int64,
	clientCount int,
	addrCount int,
	positionmap map[int]map[int]MvpPositionMapEntry,
	server chan<- ServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) {
	clients := makeSyncAccessClients(l, z, seed, clientCount, addrCount, positionmap, server, newOperation)
	for {
		if err := runSyncAccessRound(clients); err != nil {
			panic(err)
		}
	}
}

func makeSyncAccessClients(
	l int,
	z int,
	seed int64,
	clientCount int,
	addrCount int,
	positionmap map[int]map[int]MvpPositionMapEntry,
	server chan<- ServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) []*syncAccessClient {
	clients := make([]*syncAccessClient, 0, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewMvpClient(
			l,
			z,
			clientID,
			clonePositionMap(positionmap),
			server,
		)
		clients = append(clients, &syncAccessClient{
			client:    client,
			operation: newOperation(addrCount, seed+int64(clientID)),
		})
	}
	return clients
}

func runSyncAccessRound(clients []*syncAccessClient) error {
	for _, state := range clients {
		state.op = state.operation.Next()
	}
	if err := runSyncPhase(clients, runSyncGetPMPhase); err != nil {
		return err
	}
	if err := runSyncPhase(clients, runSyncGetPSPhase); err != nil {
		return err
	}
	if err := runSyncPhase(clients, runSyncEvictPhase); err != nil {
		return err
	}
	return nil
}

func runSyncPhase(clients []*syncAccessClient, phase func(*syncAccessClient) error) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(clients))
	for _, state := range clients {
		wg.Add(1)
		go func(state *syncAccessClient) {
			defer wg.Done()
			if err := phase(state); err != nil {
				errs <- err
			}
		}(state)
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

func runSyncGetPMPhase(state *syncAccessClient) error {
	client := state.client
	version, pathMaps, err := client.GetPM()
	client.seq = version
	if err != nil {
		return fmt.Errorf("client %d getpm stopped: %w", client.ClientID, err)
	}

	client.consolidatePathMaps(pathMaps)
	accessSig, accessLeaf, ok := client.choosetargetLeaf(state.op.target, state.op.OP)
	if !ok {
		return fmt.Errorf("client %d no position map entry for addr %d", client.ClientID, state.op.target)
	}
	state.accessSig = accessSig
	state.accessLeaf = accessLeaf
	if accessLoggingEnabled {
		log.Printf("access start: client=%d seq=%d op=%s addr=%d sig=%d position=%v", client.ClientID, version, state.op.OP, state.op.target, accessSig, accessLeaf)
	}
	return nil
}

func runSyncGetPSPhase(state *syncAccessClient) error {
	client := state.client
	path, stash, err := client.GetPS(state.accessLeaf)
	if err != nil {
		return fmt.Errorf("client %d getps stopped: %w", client.ClientID, err)
	}
	client.path = path
	client.Stash = stash

	W := client.mergePathStashes()
	state.populatedPath, state.populatedStash, state.populatedPathMap = client.populatePath(W, state.op, state.accessSig)
	return nil
}

func runSyncEvictPhase(state *syncAccessClient) error {
	client := state.client
	if err := client.Evict(state.populatedPath, state.populatedPathMap, state.populatedStash); err != nil {
		return fmt.Errorf("client %d evict stopped: %w", client.ClientID, err)
	}
	if accessLoggingEnabled {
		log.Printf("access success: client=%d seq=%d op=%s addr=%d", client.ClientID, client.seq, state.op.OP, state.op.target)
	}
	return nil
}

func initializeBaseMvpRun(z int, l int, seed int64) (*BaseMvpServer, int, map[int]BaseMvpPositionMapEntry) {
	n := 1 << (l - 1)
	server := NewBaseMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)

	go server.Run()
	return server, n, positionmap
}

func BaseMvpAsyncRun(
	l int,
	z int,
	seed int64,
	clientCount int,
	addrCount int,
	positionmap map[int]BaseMvpPositionMapEntry,
	server chan<- BaseMvpServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) {
	errs := startBaseMvpAsyncAccessClients(l, z, addrCount, seed, clientCount, positionmap, server, newOperation)

	if err := <-errs; err != nil {
		panic(err)
	}
}

func startBaseMvpAsyncAccessClients(
	l int,
	z int,
	addrCount int,
	seed int64,
	clientCount int,
	positionmap map[int]BaseMvpPositionMapEntry,
	server chan<- BaseMvpServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) chan error {
	errs := make(chan error, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewBaseMvpClient(
			l,
			z,
			clientID,
			cloneBaseMvpPositionMap(positionmap),
			server,
		)

		go func(client *BaseMvpClient) {
			operation := newOperation(addrCount, seed+int64(client.ClientID))
			if err := client.NormalRun(operation); err != nil {
				errs <- fmt.Errorf("mvp client %d stopped: %w", client.ClientID, err)
			}
		}(client)
	}

	return errs
}

type baseMvpSyncAccessClient struct {
	client    *BaseMvpClient
	operation AccessOperation

	op               OramOP
	accessPosition   BaseMvpPosition
	populatedPath    map[BaseMvpPosition]BaseMvpSlot
	populatedStash   []BaseMvpDataBlock
	populatedPathMap []basePath
}

func BaseMvpSyncRun(
	l int,
	z int,
	seed int64,
	clientCount int,
	addrCount int,
	positionmap map[int]BaseMvpPositionMapEntry,
	server chan<- BaseMvpServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) {
	clients := makeBaseMvpSyncAccessClients(l, z, seed, clientCount, addrCount, positionmap, server, newOperation)
	for {
		if err := runBaseMvpSyncAccessRound(clients); err != nil {
			panic(err)
		}
	}
}

func makeBaseMvpSyncAccessClients(
	l int,
	z int,
	seed int64,
	clientCount int,
	addrCount int,
	positionmap map[int]BaseMvpPositionMapEntry,
	server chan<- BaseMvpServerRequest,
	newOperation func(addrCount int, seed int64) AccessOperation,
) []*baseMvpSyncAccessClient {
	clients := make([]*baseMvpSyncAccessClient, 0, clientCount)
	for clientID := 0; clientID < clientCount; clientID++ {
		client := NewBaseMvpClient(
			l,
			z,
			clientID,
			cloneBaseMvpPositionMap(positionmap),
			server,
		)
		clients = append(clients, &baseMvpSyncAccessClient{
			client:    client,
			operation: newOperation(addrCount, seed+int64(clientID)),
		})
	}
	return clients
}

func runBaseMvpSyncAccessRound(clients []*baseMvpSyncAccessClient) error {
	for _, state := range clients {
		state.op = state.operation.Next()
	}
	if err := runBaseMvpSyncPhase(clients, runBaseMvpSyncGetPMPhase); err != nil {
		return err
	}
	if err := runBaseMvpSyncPhase(clients, runBaseMvpSyncGetPSPhase); err != nil {
		return err
	}
	if err := runBaseMvpSyncPhase(clients, runBaseMvpSyncEvictPhase); err != nil {
		return err
	}
	return nil
}

func runBaseMvpSyncPhase(clients []*baseMvpSyncAccessClient, phase func(*baseMvpSyncAccessClient) error) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(clients))
	for _, state := range clients {
		wg.Add(1)
		go func(state *baseMvpSyncAccessClient) {
			defer wg.Done()
			if err := phase(state); err != nil {
				errs <- err
			}
		}(state)
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

func runBaseMvpSyncGetPMPhase(state *baseMvpSyncAccessClient) error {
	client := state.client
	version, pathMaps, err := client.GetPM()
	client.seq = version
	if err != nil {
		return fmt.Errorf("mvp client %d getpm stopped: %w", client.ClientID, err)
	}

	client.consolidatePathMaps(pathMaps)
	entry, ok := client.PositionMap[state.op.target]
	if !ok {
		return fmt.Errorf("mvp client %d no position map entry for addr %d", client.ClientID, state.op.target)
	}
	state.accessPosition = entry.Slot
	if accessLoggingEnabled {
		log.Printf("mvp access start: client=%d seq=%d op=%s addr=%d position=%v", client.ClientID, version, state.op.OP, state.op.target, state.accessPosition)
	}
	return nil
}

func runBaseMvpSyncGetPSPhase(state *baseMvpSyncAccessClient) error {
	client := state.client
	path, stash, err := client.GetPS(client.baseMvpSelectPath(state.accessPosition, client.L))
	if err != nil {
		return fmt.Errorf("mvp client %d getps stopped: %w", client.ClientID, err)
	}
	client.basePath = path
	client.Stash = stash

	W := client.mergePathStashes()
	targetBlock, ok := W[state.op.target]
	if !ok {
		return fmt.Errorf("mvp client %d no target block in working set", client.ClientID)
	}
	if state.op.OP == Write {
		targetBlock.Data = state.op.param
		targetBlock.Version = Versions{client.seq, client.seq, client.seq}
	} else {
		targetBlock.Version.SetA(client.seq)
	}
	W[state.op.target] = targetBlock

	state.populatedPath, state.populatedStash, state.populatedPathMap = client.populatePath(W, state.op)
	return nil
}

func runBaseMvpSyncEvictPhase(state *baseMvpSyncAccessClient) error {
	client := state.client
	if err := client.Evict(state.populatedPath, state.populatedPathMap, state.populatedStash); err != nil {
		return fmt.Errorf("mvp client %d evict stopped: %w", client.ClientID, err)
	}
	if accessLoggingEnabled {
		log.Printf("mvp access success: client=%d seq=%d op=%s addr=%d", client.ClientID, client.seq, state.op.OP, state.op.target)
	}
	return nil
}

func (c *BaseMvpClient) NormalRun(operation AccessOperation) error {
	for {
		if err := c.Access(operation.Next()); err != nil {
			return err
		}
	}
}

func logStashMetricsBySequence(server chan<- ServerRequest, interval Version) {
	if interval <= 0 {
		panic("stash metrics interval must be greater than 0")
	}

	nextSeq := interval
	for {
		metrics := getStashMetrics(server)
		if metrics.Seq >= nextSeq {
			log.Printf(
				"stash metrics sample: seq=%d stash_versions=%d stash_total=%d stash_max_version_size=%d",
				metrics.Seq,
				metrics.StashVersions,
				metrics.StashTotal,
				metrics.StashMaxVersionSize,
			)
			for nextSeq <= metrics.Seq {
				nextSeq += interval
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func getStashMetrics(server chan<- ServerRequest) GetStashMetricsResponse {
	reply := make(chan GetStashMetricsResponse)
	server <- GetStashMetricsRequest{Reply: reply}
	res := <-reply
	if res.Err != nil {
		panic(res.Err)
	}
	return res
}

func logBaseMvpStashMetricsBySequence(server chan<- BaseMvpServerRequest, interval Version) {
	if interval <= 0 {
		panic("stash metrics interval must be greater than 0")
	}

	nextSeq := interval
	for {
		metrics := getBaseMvpStashMetrics(server)
		if metrics.Seq >= nextSeq {
			log.Printf(
				"mvp stash metrics sample: seq=%d stash_versions=%d stash_total=%d stash_max_version_size=%d",
				metrics.Seq,
				metrics.StashVersions,
				metrics.StashTotal,
				metrics.StashMaxVersionSize,
			)
			for nextSeq <= metrics.Seq {
				nextSeq += interval
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func getBaseMvpStashMetrics(server chan<- BaseMvpServerRequest) BaseMvpGetStashMetricsResponse {
	reply := make(chan BaseMvpGetStashMetricsResponse)
	server <- BaseMvpGetStashMetricsRequest{Reply: reply}
	res := <-reply
	if res.Err != nil {
		panic(res.Err)
	}
	return res
}

type AccessOperation interface {
	Next() OramOP
}

type UniformAccessOperation struct {
	addrCount int
	rng       *rand.Rand
}

func NewUniformAccessOperation(addrCount int, seed int64) *UniformAccessOperation {
	if addrCount <= 0 {
		panic("addrCount must be greater than 0")
	}

	return &UniformAccessOperation{
		addrCount: addrCount,
		rng:       rand.New(rand.NewSource(seed)),
	}
}

func (o *UniformAccessOperation) Next() OramOP {
	target := o.rng.Intn(o.addrCount)
	operation := Read
	if o.rng.Intn(2) == 0 {
		operation = Write
	}

	return OramOP{
		OP:     operation,
		target: target,
		param:  fmt.Sprintf("addr-%d", target),
	}
}

type ZipfAccessOperation struct {
	rng       *rand.Rand
	zipf      *rand.Zipf
	readRatio float64
}

func NewZipfAccessOperation(addrCount int, seed int64, readRatio float64, zipfAlpha float64) *ZipfAccessOperation {
	if addrCount <= 0 {
		panic("addrCount must be greater than 0")
	}

	rng := rand.New(rand.NewSource(seed))
	return &ZipfAccessOperation{
		rng:       rng,
		zipf:      rand.NewZipf(rng, zipfAlpha, 1, uint64(addrCount-1)),
		readRatio: readRatio,
	}
}

func (o *ZipfAccessOperation) Next() OramOP {
	target := int(o.zipf.Uint64())
	operation := Read
	if o.rng.Float64() >= o.readRatio {
		operation = Write
	}

	return OramOP{
		OP:     operation,
		target: target,
		param:  fmt.Sprintf("addr-%d", target),
	}
}

func (c *MvpClient) NormalRun(operation AccessOperation) error {
	for {
		if err := c.Access(operation.Next()); err != nil {
			return err
		}
	}
}
