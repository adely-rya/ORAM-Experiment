package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
)

type MvpBucketPosition string
type MvpSlotPosition int

const (
	mvpRootBucketPosition  MvpBucketPosition = "root"
	mvpStashBucketPosition MvpBucketPosition = "-1"
	mvpDeleteTag           MvpBucketPosition = "-2"
	mvpStashSlotPosition   MvpSlotPosition   = -1
	mvpInitialVersion      Version           = 0
	// Copy signatures are managed as a fixed addr-sig range; initialization pre-fills 0..mvpMaxSignature.
	mvpMaxSignature                       = 8
	mvpDefaultPathMapCompactInterval      = 10000
	mvpDefaultPathMapCompactProtectedTail = 2000
	mvpStashMetricsInterval               = 10
	mvpDefaultSwapLimit                   = 8
)

var (
	mvpStashPosition  = MvpPosition{bucket: mvpStashBucketPosition, slot: mvpStashSlotPosition}
	mvpDeletePosition = MvpPosition{bucket: mvpDeleteTag}
)

func (p MvpBucketPosition) String() string {
	return string(p)
}

type MvpPosition struct {
	bucket MvpBucketPosition
	slot   MvpSlotPosition
}

type MvpPositionMapEntry struct {
	Slot MvpPosition
	Ts   Versions
}

func (p MvpPosition) String() string {
	return p.bucket.String()
}

type OramOP struct {
	OP     string
	target int
	param  string
}

const (
	Write string = "write"
	Read  string = "read"
)

type Version int

func (v *Version) increment() {
	(*v)++
}

type Versions struct {
	V Version // 最後の書き込みのバージョン
	A Version // 最後の書き込み or 読み取りのバージョン
	S Version // 最後に追加、削除、移動されたバージョン
}

func (ver *Versions) SetV(value Version) {
	ver.V = value
}

func (ver *Versions) SetA(value Version) {
	ver.A = value
}

func (ver *Versions) SetS(value Version) {
	ver.S = value
}

type MvpDataBlock struct {
	Addr      int
	signature int
	Data      string
	Version   Versions
}

func (d *MvpDataBlock) Setsignature(sig int) {
	d.signature = sig
}

type MvpSlot struct {
	Version Version
	Value   MvpDataBlock
	Empty   bool
}

func NewMvpSlot(version Version) MvpSlot {
	return MvpSlot{
		Version: version,
		Empty:   true,
	}
}

func (s MvpSlot) IsEmpty() bool {
	return s.Empty
}

func (s *MvpSlot) SetBlock(block MvpDataBlock) bool {
	if !s.IsEmpty() {
		return false
	}

	s.Value = block
	s.Empty = false
	return true
}

func (s MvpSlot) Clone() MvpSlot {
	return s
}

type MvpBucket struct {
	Z     int
	Slots map[MvpSlotPosition]map[Version]MvpSlot
}

func NewMvpBucket(z int) MvpBucket {
	slots := make(map[MvpSlotPosition]map[Version]MvpSlot, z)

	for i := 0; i < z; i++ {
		slotPosition := MvpSlotPosition(i)
		slots[slotPosition] = map[Version]MvpSlot{
			mvpInitialVersion: NewMvpSlot(mvpInitialVersion),
		}
	}

	return MvpBucket{
		Z:     z,
		Slots: slots,
	}
}

func (b *MvpBucket) SetBlock(slotPosition MvpSlotPosition, version Version, block MvpDataBlock) bool {
	versionedSlots, ok := b.Slots[slotPosition]
	if !ok {
		return false //そのスロットポジションが存在するのか？
	}

	slot, ok := versionedSlots[version]
	if !ok {
		slot = NewMvpSlot(version) //すでにスロットがあるのか？
	}

	if !slot.SetBlock(block) {
		return false //スロットにブロックが入るのか？　すでに入ってしまってないか？
	}

	versionedSlots[version] = slot
	return true
}

func (b MvpBucket) RandomEmptySlot(version Version, rng *rand.Rand) (MvpSlotPosition, bool) {
	slotIndexes := rng.Perm(b.Z)
	for _, slotIndex := range slotIndexes {
		slotPosition := MvpSlotPosition(slotIndex)
		versionedSlots, ok := b.Slots[slotPosition]
		if !ok {
			continue
		}

		slot, ok := versionedSlots[version]
		if !ok || slot.IsEmpty() {
			return slotPosition, true
		}
	}

	return 0, false
}

func (b MvpBucket) Clone() MvpBucket {
	slots := make(map[MvpSlotPosition]map[Version]MvpSlot, len(b.Slots))
	for slotPosition, versionedSlots := range b.Slots {
		slots[slotPosition] = make(map[Version]MvpSlot, len(versionedSlots))
		for version, slot := range versionedSlots {
			slots[slotPosition][version] = slot.Clone()
		}
	}

	return MvpBucket{
		Z:     b.Z,
		Slots: slots,
	}
}

type MvpTree struct {
	Z               int
	L               int
	Tree            map[MvpBucketPosition]MvpBucket
	BucketReadCount map[MvpBucketPosition]int64
	TotalBucketRead int64
}

func NewMvpBucketPosition(level, index int) MvpBucketPosition {
	if level == 0 {
		return mvpRootBucketPosition
	}

	key := strconv.FormatInt(int64(index), 2)
	for len(key) < level {
		key = "0" + key
	}
	return MvpBucketPosition(key)
}

func NewMvpTree(z int, l int) MvpTree {
	tree := make(map[MvpBucketPosition]MvpBucket, 1<<(l+1)-1)
	bucketReadCount := make(map[MvpBucketPosition]int64, 1<<(l+1)-1)

	for level := 0; level <= l; level++ {
		for index := 0; index < 1<<level; index++ {
			position := NewMvpBucketPosition(level, index)

			tree[position] = NewMvpBucket(z)
			bucketReadCount[position] = 0
		}
	}

	return MvpTree{
		Z:               z,
		L:               l,
		Tree:            tree,
		BucketReadCount: bucketReadCount,
	}
}

func (t MvpTree) Clone() MvpTree {
	tree := make(map[MvpBucketPosition]MvpBucket, len(t.Tree))
	for position, bucket := range t.Tree {
		tree[position] = bucket.Clone()
	}

	bucketReadCount := make(map[MvpBucketPosition]int64, len(t.BucketReadCount))
	for position, count := range t.BucketReadCount {
		bucketReadCount[position] = count
	}

	return MvpTree{
		Z:               t.Z,
		L:               t.L,
		Tree:            tree,
		BucketReadCount: bucketReadCount,
		TotalBucketRead: t.TotalBucketRead,
	}
}

type path struct {
	addr int
	sig  int
	to   MvpPosition
	Ver  Versions
	Seq  Version
}

type pathKey struct {
	addr int
	sig  int
}

func newPath(addr int, sig int, to any, ver Versions, seq Version) path {
	return path{
		addr: addr,
		sig:  sig,
		to:   pathPosition(to),
		Ver:  ver,
		Seq:  seq,
	}
}

func appendPositionMapUpdate(pathMaps *[]path, update path) {
	compacted := (*pathMaps)[:0]
	for _, existing := range *pathMaps {
		if existing.addr == update.addr && existing.sig == update.sig {
			continue
		}
		compacted = append(compacted, existing)
	}
	// Keep only the latest update for each addr-sig; this removes stale Delete updates before a new placement.
	*pathMaps = append(compacted, update)
}

func pathPosition(to any) MvpPosition {
	switch position := to.(type) {
	case MvpPosition:
		return position
	case MvpBucketPosition:
		return MvpPosition{bucket: position}
	default:
		panic("unsupported path destination type")
	}
}

func pathBucketPosition(to any) MvpBucketPosition {
	switch position := to.(type) {
	case MvpBucketPosition:
		return position
	case MvpPosition:
		return position.bucket
	default:
		panic("unsupported path destination type")
	}
}

type ServerRequest interface {
	handle(s *MvpServer)
}

type GetpmRequest struct {
	ClientID int
	Reply    chan GetpmResponse
}

type GetpmResponse struct {
	PathMap []path
	Seq     Version
	Err     error
}

type GetpsRequest struct {
	ClientID int
	Leaf     MvpPosition
	Reply    chan GetpsResponse
}

type GetpsResponse struct {
	Path  map[MvpBucketPosition]MvpBucket
	Stash map[int][]MvpDataBlock
	Err   error
}

type EvictReques struct {
	ClientID int
	Seq      Version
	PathMap  []path
	Stash    []MvpDataBlock
	Path     map[MvpPosition]MvpSlot
	Reply    chan EvictResponse
}

type EvictResponse struct {
	Err error
}

type OramState struct {
	TreeState  MvpTree
	StashState map[int][]MvpDataBlock
}

func cloneStashs(stashs map[int][]MvpDataBlock) map[int][]MvpDataBlock {
	cloned := make(map[int][]MvpDataBlock, len(stashs))
	for clientID, stash := range stashs {
		cloned[clientID] = append([]MvpDataBlock(nil), stash...)
	}

	return cloned
}

func clonePositionMap(positionMap map[int]map[int]MvpPositionMapEntry) map[int]map[int]MvpPositionMapEntry {
	cloned := make(map[int]map[int]MvpPositionMapEntry, len(positionMap))
	for addr, entries := range positionMap {
		cloned[addr] = make(map[int]MvpPositionMapEntry, len(entries))
		for sig, entry := range entries {
			cloned[addr][sig] = entry
		}
	}

	return cloned
}

func newInitializedPositionMapEntries() map[int]MvpPositionMapEntry {
	entries := make(map[int]MvpPositionMapEntry, mvpMaxSignature+1)
	deleteEntry := MvpPositionMapEntry{
		Slot: mvpDeletePosition,
		Ts: Versions{
			V: mvpInitialVersion,
			A: mvpInitialVersion,
			S: mvpInitialVersion,
		},
	}
	// Every addr-sig slot starts as Delete at timestamp 000 so copy slots are managed explicitly.
	for sig := 0; sig <= mvpMaxSignature; sig++ {
		entries[sig] = deleteEntry
	}
	return entries
}

type MvpServer struct {
	PositionMaps                 []map[int]map[int]MvpPositionMapEntry
	PathMaps                     []path
	Stashs                       map[int][]MvpDataBlock
	tree                         MvpTree
	counter                      Version
	useSnapshot                  bool
	Snapshot                     map[int]OramState
	PathMapCursor                map[int]int
	PathMapCompactInterval       int
	PathMapCompactProtectedTail  int
	pathMapEvictsSinceCompaction int

	Requests chan ServerRequest
}

func NewMvpServer(z int, l int) *MvpServer {
	return &MvpServer{
		PositionMaps:  make([]map[int]map[int]MvpPositionMapEntry, 0),
		PathMaps:      make([]path, 0),
		Stashs:        make(map[int][]MvpDataBlock, 0),
		tree:          NewMvpTree(z, l),
		Requests:      make(chan ServerRequest),
		counter:       mvpInitialVersion,
		useSnapshot:   true,
		Snapshot:      make(map[int]OramState, 50),
		PathMapCursor: make(map[int]int, 50),

		PathMapCompactInterval:      mvpDefaultPathMapCompactInterval,
		PathMapCompactProtectedTail: mvpDefaultPathMapCompactProtectedTail,
	}
}

func NewSynchronizedMvpServer(z int, l int) *MvpServer {
	server := NewMvpServer(z, l)
	server.useSnapshot = false
	server.Snapshot = nil
	return server
}

func (s *MvpServer) SetPathMapCompaction(interval int, protectedTail int) {
	s.PathMapCompactInterval = interval
	s.PathMapCompactProtectedTail = protectedTail
	s.pathMapEvictsSinceCompaction = 0
}

func (s *MvpServer) compactPathMaps() {
	if len(s.PathMaps) == 0 || s.PathMapCompactProtectedTail <= 0 {
		return
	}

	compactEnd := len(s.PathMaps) - s.PathMapCompactProtectedTail
	if compactEnd <= 0 {
		return
	}

	keyInProtectedTail := make(map[pathKey]struct{}, s.PathMapCompactProtectedTail)
	for _, entry := range s.PathMaps[compactEnd:] {
		keyInProtectedTail[pathKey{addr: entry.addr, sig: entry.sig}] = struct{}{}
	}
	if len(keyInProtectedTail) == 0 {
		return
	}

	removedPrefix := make([]int, len(s.PathMaps)+1)
	compacted := make([]path, 0, len(s.PathMaps))
	for index, entry := range s.PathMaps {
		removedPrefix[index+1] = removedPrefix[index]
		if index < compactEnd {
			if _, ok := keyInProtectedTail[pathKey{addr: entry.addr, sig: entry.sig}]; ok {
				removedPrefix[index+1]++
				continue
			}
		}
		compacted = append(compacted, entry)
	}

	if len(compacted) == len(s.PathMaps) {
		return
	}

	for clientID, cursor := range s.PathMapCursor {
		if cursor < 0 {
			cursor = 0
		}
		if cursor > len(s.PathMaps) {
			cursor = len(s.PathMaps)
		}
		s.PathMapCursor[clientID] = cursor - removedPrefix[cursor]
	}

	s.PathMaps = compacted
}

func (s *MvpServer) InitializeRandomData(n int, seed int64) map[int]map[int]MvpPositionMapEntry {
	rng := rand.New(rand.NewSource(seed))
	positions := make([]MvpBucketPosition, 0, len(s.tree.Tree)) //木に存在するポジション一覧生成
	for position := range s.tree.Tree {
		positions = append(positions, position)
	}

	sort.Slice(positions, func(i, j int) bool {
		return positions[i] < positions[j]
	}) //小さい順にソート

	positionMap := make(map[int]map[int]MvpPositionMapEntry, n)
	stash := make([]MvpDataBlock, 0)

	for addr := 0; addr < n; addr++ {
		block := MvpDataBlock{
			Addr:      addr,
			signature: 0,
			Data:      strconv.Itoa(addr),
			Version: Versions{
				V: mvpInitialVersion,
				A: mvpInitialVersion,
				S: mvpInitialVersion,
			},
		}
		// Initialize all signatures as Delete first; the real initial block always occupies sig=0.
		positionMap[addr] = newInitializedPositionMapEntries()
		position := positions[rng.Intn(len(positions))] //ランダムにポジションを選ぶ
		bucket := s.tree.Tree[position]

		if slotPosition, ok := bucket.RandomEmptySlot(mvpInitialVersion, rng); ok && bucket.SetBlock(slotPosition, mvpInitialVersion, block) {
			s.tree.Tree[position] = bucket
			positionMap[addr][block.signature] = MvpPositionMapEntry{
				Slot: MvpPosition{
					bucket: position,
					slot:   slotPosition,
				},
				Ts: block.Version,
			}
			continue
		}

		stash = append(stash, block) //溢れたらスタッシュに移動
		positionMap[addr][block.signature] = MvpPositionMapEntry{
			Slot: mvpStashPosition,
			Ts:   block.Version,
		}
	}

	s.PositionMaps = append(s.PositionMaps, positionMap)
	s.Stashs[0] = stash
	log.Printf("init metrics: n=%d initial_stash=%d", n, len(stash))

	return positionMap

}

func (s *MvpServer) Run() {
	for req := range s.Requests {
		req.handle(s)
	}
}

func (r GetpmRequest) handle(s *MvpServer) {
	s.counter.increment()
	seq := s.counter

	if s.useSnapshot {
		s.Snapshot[r.ClientID] = OramState{
			TreeState:  s.tree.Clone(),
			StashState: cloneStashs(s.Stashs),
		}
	}

	cursor := s.PathMapCursor[r.ClientID]
	if cursor < 0 || cursor > len(s.PathMaps) {
		cursor = len(s.PathMaps)
	}

	difpathMap := append([]path(nil), s.PathMaps[cursor:]...)
	s.PathMapCursor[r.ClientID] = len(s.PathMaps)

	r.Reply <- GetpmResponse{
		Seq:     seq,
		PathMap: difpathMap,
		Err:     nil,
	}
}

func (r GetpsRequest) handle(s *MvpServer) {
	var tree MvpTree
	var stash map[int][]MvpDataBlock
	if s.useSnapshot {
		oramstate, ok := s.Snapshot[r.ClientID]
		if !ok {
			r.Reply <- GetpsResponse{
				Err: fmt.Errorf("snapshot for client %d not found", r.ClientID),
			}
			return
		}
		tree = oramstate.TreeState
		stash = oramstate.StashState
	} else {
		tree = s.tree
		stash = cloneStashs(s.Stashs)
	}

	path := make(map[MvpBucketPosition]MvpBucket, s.tree.L+1)
	root, ok := tree.Tree[mvpRootBucketPosition]
	if !ok {
		r.Reply <- GetpsResponse{
			Err: fmt.Errorf("root bucket not found"),
		}
		return
	}

	path[mvpRootBucketPosition] = root.Clone()

	leaf := r.Leaf.bucket.String()
	for len(leaf) != 0 {
		bucketPosition := MvpBucketPosition(leaf)
		path[bucketPosition] = tree.Tree[bucketPosition].Clone()
		leaf = leaf[:len(leaf)-1]
	}

	r.Reply <- GetpsResponse{
		Path:  path,
		Stash: stash,
		Err:   nil,
	}
}

func (r EvictReques) handle(s *MvpServer) {
	nowtree := s.tree.Tree
	var oldtree map[MvpBucketPosition]MvpBucket
	var oldstash map[int][]MvpDataBlock
	outputVersions := make(map[pathKey]Versions, len(r.PathMap))
	for _, path := range r.PathMap {
		key := pathKey{addr: path.addr, sig: path.sig}
		if current, ok := outputVersions[key]; !ok || newerVersions(path.Ver, current) {
			outputVersions[key] = path.Ver
		}
	}

	if s.useSnapshot {
		oldstate := s.Snapshot[r.ClientID]
		oldtree = oldstate.TreeState.Tree
		oldstash = oldstate.StashState
		delete(s.Snapshot, r.ClientID)
	} else {
		oldtree = nowtree
		oldstash = s.Stashs
	}

	for position, newslot := range r.Path {
		slots := nowtree[position.bucket].Slots[position.slot]
		oldslots := oldtree[position.bucket].Slots[position.slot]
		for version, oldslot := range oldslots {
			if !s.useSnapshot && !oldslot.IsEmpty() {
				key := pathKey{addr: oldslot.Value.Addr, sig: oldslot.Value.signature}
				outputVersion, ok := outputVersions[key]
				if !ok || newerVersions(oldslot.Value.Version, outputVersion) {
					continue
				}
			}

			_, ok := slots[version]
			if ok {
				delete(slots, version)
			}

		}
		slots[newslot.Version] = newslot
	}

	if s.useSnapshot {
		for version := range oldstash {
			_, ok := s.Stashs[version]
			if ok {
				delete(s.Stashs, version)
			}
		}
	} else {
		for version, stash := range oldstash {
			remaining := make([]MvpDataBlock, 0, len(stash))
			for _, block := range stash {
				key := pathKey{addr: block.Addr, sig: block.signature}
				outputVersion, ok := outputVersions[key]
				if ok && !newerVersions(block.Version, outputVersion) {
					continue
				}
				remaining = append(remaining, block)
			}
			if len(remaining) == 0 {
				delete(s.Stashs, version)
			} else {
				s.Stashs[version] = remaining
			}
		}
	}
	s.Stashs[int(r.Seq)] = r.Stash
	s.logStashMetrics(r)

	for _, path := range r.PathMap {
		s.PathMaps = append(s.PathMaps, path)
	}
	s.pathMapEvictsSinceCompaction++
	if s.PathMapCompactInterval > 0 && s.pathMapEvictsSinceCompaction >= s.PathMapCompactInterval {
		s.compactPathMaps()
		s.pathMapEvictsSinceCompaction = 0
	}

	r.Reply <- EvictResponse{
		Err: nil,
	}
}

func (s *MvpServer) logStashMetrics(r EvictReques) {
	if !shouldLogMetrics(r.Seq) {
		return
	}

	pathBlocks := 0
	for _, slot := range r.Path {
		if !slot.IsEmpty() {
			pathBlocks++
		}
	}

	totalStashBlocks := 0
	maxStashVersionBlocks := 0
	for _, stash := range s.Stashs {
		totalStashBlocks += len(stash)
		if len(stash) > maxStashVersionBlocks {
			maxStashVersionBlocks = len(stash)
		}
	}

	log.Printf(
		"stash metrics: seq=%d client=%d path_out=%d stash_out=%d stash_versions=%d stash_total=%d stash_max_version=%d pathmap_updates=%d",
		r.Seq,
		r.ClientID,
		pathBlocks,
		len(r.Stash),
		len(s.Stashs),
		totalStashBlocks,
		maxStashVersionBlocks,
		len(r.PathMap),
	)
}

type MvpClient struct {
	L           int
	Z           int
	SwapLimit   int
	PositionMap map[int]map[int]MvpPositionMapEntry
	Stash       map[int][]MvpDataBlock
	path        map[MvpBucketPosition]MvpBucket

	ClientID int
	Server   chan<- ServerRequest

	seq Version
}

var accessLoggingEnabled = true

func NewMvpClient(l int, z int, clientID int, positionmap map[int]map[int]MvpPositionMapEntry, server chan<- ServerRequest) *MvpClient {
	return &MvpClient{
		L: l,
		Z: z,
		// SwapLimit is independent from bucket slot count Z so experiments can tune swap volume only.
		SwapLimit:   mvpDefaultSwapLimit,
		ClientID:    clientID,
		Server:      server,
		PositionMap: positionmap,
	}
}

func (c *MvpClient) GetPM() (Version, []path, error) {
	reply := make(chan GetpmResponse)

	c.Server <- GetpmRequest{
		ClientID: c.ClientID,
		Reply:    reply,
	}

	res := <-reply
	return res.Seq, res.PathMap, res.Err
}

func (c *MvpClient) GetPS(leaf MvpPosition) (map[MvpBucketPosition]MvpBucket, map[int][]MvpDataBlock, error) {
	reply := make(chan GetpsResponse)

	c.Server <- GetpsRequest{
		ClientID: c.ClientID,
		Leaf:     leaf,
		Reply:    reply,
	}

	res := <-reply
	return res.Path, res.Stash, res.Err
}

func (c *MvpClient) Evict(path map[MvpPosition]MvpSlot, pathmap []path, stash []MvpDataBlock) error {
	reply := make(chan EvictResponse)

	c.Server <- EvictReques{
		ClientID: c.ClientID,
		Seq:      c.seq,
		Path:     path,
		PathMap:  pathmap,
		Stash:    stash,
		Reply:    reply,
	}

	res := <-reply
	return res.Err

}

func (c *MvpClient) Run(addrCount int) error {

	for {
		target := rand.Intn(addrCount)
		param := fmt.Sprintf("client-%d-%d", c.ClientID, target)
		operation := Read
		if rand.Intn(2) == 0 {
			operation = Write
		}

		err := c.Access(OramOP{operation, target, param})
		if err != nil {
			return err
		}
	}
}

func (c *MvpClient) Access(op OramOP) error {
	version, pathMaps, err := c.GetPM() //Getpm操作
	c.seq = version
	if err != nil {
		return err
	}

	c.consolidatePathMaps(pathMaps) //position mapの更新
	accessSig, accessEntry, ok := c.choosePositionMapEntry(op.target)
	if !ok {
		return fmt.Errorf("no position map entry for addr %d", op.target)
	}
	if accessLoggingEnabled {
		log.Printf("access start: client=%d seq=%d op=%s addr=%d sig=%d position=%v", c.ClientID, version, op.OP, op.target, accessSig, accessEntry.Slot)
	}

	accessPosition := accessEntry.Slot

	c.path, c.Stash, err = c.GetPS(c.selectPath(accessPosition, c.L)) //Getps操作
	if err != nil {
		return err
	}

	W := c.mergePathStashes() //ワーキングセット制作

	populatedPath, populatedStash, populatedPathMap := c.populatePath(W, op, accessSig)

	err = c.Evict(populatedPath, populatedPathMap, populatedStash)
	if err != nil {
		return err
	}
	if accessLoggingEnabled {
		log.Printf("access success: client=%d seq=%d op=%s addr=%d", c.ClientID, version, op.OP, op.target)
	}

	return nil
}

func (c *MvpClient) consolidatePathMaps(pathMaps []path) {
	latestPathMap := make(map[pathKey]path, len(pathMaps))
	maxversion := make(map[int]int)
	currentMaxVersion := c.currentMaxWriteVersions()
	updatesByAddr := make(map[int][]path)

	// 受信したPathMapをアドレス単位で記録しつつ、(addr, sig)ごとの最新更新だけを残す。
	for _, v := range pathMaps {
		updatesByAddr[v.addr] = append(updatesByAddr[v.addr], v)
		key := pathKey{addr: v.addr, sig: v.sig}
		latest, ok := latestPathMap[key]
		if !ok || newerPathUpdate(v, latest) {
			latestPathMap[key] = v
		}

		if max, ok := maxversion[v.addr]; !ok || max < int(v.Ver.V) {
			maxversion[v.addr] = int(v.Ver.V) //そのアドレスでwriteされた最大のバージョンを保持
		}
	}

	// 古いwriteバージョンの追加・移動更新を捨て、現在のPositionMapより古い更新も除外する。
	for key, path := range latestPathMap { //そのアドレスでwriteされた最大バージョンの追加パス、移動パスを排除
		if _, ok := c.PositionMap[path.addr][path.sig]; int(path.Ver.V) < maxversion[key.addr] && !ok {
			delete(latestPathMap, key)
			continue
		}
		if currentMax, ok := currentMaxVersion[key.addr]; ok && path.Ver.V < currentMax {
			delete(latestPathMap, key)
		}
	}

	// 最後に残った最新PathMap更新をPositionMapへ反映する。Deleteも固定sigの位置として上書きする。
	for _, v := range latestPathMap {
		current, ok := c.PositionMap[v.addr][v.sig]
		if !ok || newerPositionUpdate(v, current) {
			c.PositionMap[v.addr][v.sig] = MvpPositionMapEntry{Slot: v.to, Ts: v.Ver}
		}
	}
}

func (c *MvpClient) currentMaxWriteVersions() map[int]Version {
	maxVersions := make(map[int]Version, len(c.PositionMap))
	for addr, entries := range c.PositionMap {
		for _, entry := range entries {
			if current, ok := maxVersions[addr]; !ok || entry.Ts.V > current {
				maxVersions[addr] = entry.Ts.V
			}
		}
	}
	return maxVersions
}

func (c *MvpClient) choosePositionMapEntry(addr int) (int, MvpPositionMapEntry, bool) {
	entries, ok := c.PositionMap[addr]
	if !ok || len(entries) == 0 {
		return 0, MvpPositionMapEntry{}, false
	}

	signatures := make([]int, 0, len(entries))
	for sig, position := range entries {
		if position.Slot != mvpDeletePosition {
			signatures = append(signatures, sig) //deleteタグのブロックは無視
		}
	}
	if len(signatures) == 0 {
		return 0, MvpPositionMapEntry{}, false
	}
	sig := signatures[rand.Intn(len(signatures))]
	return sig, entries[sig], true
}

func (c *MvpClient) selectPath(accessPosition MvpPosition, pathlen int) MvpPosition {
	if accessPosition == mvpStashPosition {
		return randomLeafPosition(pathlen)
	}

	return selectPath(accessPosition, pathlen)
}

func randomLeafPosition(pathlen int) MvpPosition {
	leaf := ""
	for len(leaf) < pathlen {
		if rand.Intn(2) == 0 {
			leaf += "0"
		} else {
			leaf += "1"
		}
	}

	return MvpPosition{
		bucket: MvpBucketPosition(leaf),
	}
}

func selectPath(accessPosition any, pathlen int) MvpPosition {
	bucketPosition := pathBucketPosition(accessPosition)
	if bucketPosition == mvpStashBucketPosition || bucketPosition == mvpRootBucketPosition {
		bucketPosition = ""
	}

	leaf := bucketPosition.String()
	for len(leaf) < pathlen {
		if rand.Intn(2) == 0 {
			leaf += "0"
		} else {
			leaf += "1"
		}
	}

	return MvpPosition{
		bucket: MvpBucketPosition(leaf),
	}
}

func (c *MvpClient) mergePathStashes() map[int][]MvpDataBlock {
	W := make(map[int][]MvpDataBlock, 0)

	for _, v := range c.Stash {
		for _, block := range v {
			pm, ok := c.PositionMap[block.Addr][block.signature]
			if ok && pm.Slot == mvpStashPosition && sameDataVersion(pm.Ts, block.Version) {
				appendWorkingBlock(W, block)
			}
		}
	}

	for bucketPosition, bucket := range c.path {
		for slotPosition, versionedSlots := range bucket.Slots {
			for _, slot := range versionedSlots {
				if slot.IsEmpty() {
					continue
				}

				block := slot.Value
				pm, ok := c.PositionMap[block.Addr][block.signature]
				blockPosition := MvpPosition{bucket: bucketPosition, slot: slotPosition}
				if ok && pm.Slot == blockPosition && sameDataVersion(pm.Ts, block.Version) {
					appendWorkingBlock(W, block)
				}
			}
		}
	}

	return W
}

func appendWorkingBlock(W map[int][]MvpDataBlock, block MvpDataBlock) {
	W[block.Addr] = append(W[block.Addr], block)
}

func getWorkingBlock(W map[int][]MvpDataBlock, addr int, sig int) (MvpDataBlock, bool) {
	for _, block := range W[addr] {
		if block.signature == sig {
			return block, true
		}
	}
	return MvpDataBlock{}, false
}

func newestWorkingBlock(blocks []MvpDataBlock) (MvpDataBlock, bool) {
	if len(blocks) == 0 {
		return MvpDataBlock{}, false
	}
	newest := blocks[0]
	for _, candidate := range blocks[1:] {
		if newerVersions(candidate.Version, newest.Version) {
			newest = candidate
		}
	}
	return newest, true
}

func (c *MvpClient) recoverWorkingBlock(W map[int][]MvpDataBlock, addr int, sig int) (MvpDataBlock, bool) {
	if block, ok := getWorkingBlock(W, addr, sig); ok {
		return block, true
	}
	if block, ok := newestWorkingBlock(W[addr]); ok {
		return block, true
	}

	for _, stashBlocks := range c.Stash {
		for _, block := range stashBlocks {
			if block.Addr == addr {
				if sig == block.signature {
					return block, true
				}
			}
		}
	}

	for _, bucket := range c.path {
		for _, versionedSlots := range bucket.Slots {
			for _, slot := range versionedSlots {
				if slot.IsEmpty() {
					continue
				}
				if slot.Value.Addr == addr && slot.Value.signature == sig {
					return slot.Value, true
				}
			}
		}
	}

	if entries, ok := c.PositionMap[addr]; ok {
		var candidate MvpDataBlock
		found := false
		for candidateSig, entry := range entries {
			if entry.Slot == mvpDeletePosition {
				continue
			}
			if !found || newerVersions(Versions{V: entry.Ts.V, A: entry.Ts.A, S: entry.Ts.S}, candidate.Version) {
				candidate = MvpDataBlock{
					Addr:      addr,
					signature: candidateSig,
					Version:   entry.Ts,
				}
				found = true
			}
		}
		if found {
			return candidate, true
		}
	}

	return MvpDataBlock{}, false
}

func setWorkingBlock(W map[int][]MvpDataBlock, block MvpDataBlock) {
	blocks := W[block.Addr]
	for i := range blocks {
		if blocks[i].signature == block.signature {
			blocks[i] = block
			W[block.Addr] = blocks
			return
		}
	}
	appendWorkingBlock(W, block)
}

func newerVersions(left Versions, right Versions) bool {
	if left.V != right.V {
		return left.V > right.V
	}
	if left.A != right.A {
		return left.A > right.A
	}
	return left.S > right.S
}

func sameDataVersion(left Versions, right Versions) bool {
	return left.V == right.V && left.A == right.A && left.S == right.S
}

func newerPathUpdate(left path, right path) bool {
	if left.Ver.V != right.Ver.V {
		return left.Ver.V > right.Ver.V
	}
	if left.Ver.A != right.Ver.A {
		return left.Ver.A > right.Ver.A
	}
	return left.Seq > right.Seq
}

func newerPositionUpdate(update path, current MvpPositionMapEntry) bool {
	if update.Ver.V != current.Ts.V {
		return update.Ver.V > current.Ts.V
	}
	if update.Ver.A != current.Ts.A {
		return update.Ver.A > current.Ts.A
	}
	return update.Seq > current.Ts.S
}

func positionMapVersion(blockVersion Versions, locationUpdate Version) Versions {
	return Versions{
		V: blockVersion.V,
		A: blockVersion.A,
		S: locationUpdate,
	}
}

func sort_block(blockList []MvpDataBlock) []MvpDataBlock {
	sort.Slice(blockList, func(i, j int) bool {
		left := blockList[i].Version
		right := blockList[j].Version

		if left.V != right.V {
			return left.V > right.V
		}
		if left.A != right.A {
			return left.A > right.A
		}
		return false
	})

	return blockList
}

func (c *MvpClient) evaluationPathpattern(addrlist []int, path_list []path) map[int]int {
	positionMap := clonePositionMap(c.PositionMap)
	applyPathMapsToPositionMap(positionMap, path_list)

	result := make(map[int]int, len(addrlist))
	for _, addr := range addrlist {
		for _, entry := range positionMap[addr] {
			if entry.Slot.bucket == mvpRootBucketPosition || entry.Slot.bucket == mvpStashBucketPosition {
				continue
			}
			result[addr] += pathPatternCount(entry.Slot, c.L)
		}
	}
	return result
}

func applyPathMapsToPositionMap(positionMap map[int]map[int]MvpPositionMapEntry, pathMaps []path) {
	for _, v := range pathMaps {
		if positionMap[v.addr] == nil {
			positionMap[v.addr] = newInitializedPositionMapEntries()
		}
		// Delete is represented as the fixed Delete position so sig slots stay available for reuse.
		positionMap[v.addr][v.sig] = MvpPositionMapEntry{Slot: v.to, Ts: v.Ver}
	}
}

func pathPatternCount(position MvpPosition, pathLen int) int {
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

func shouldLogMetrics(seq Version) bool {
	if seq <= 100 {
		return true
	}
	return mvpStashMetricsInterval > 0 && int(seq)%mvpStashMetricsInterval == 0
}

func shouldLogPopulateDetail(seq Version) bool {
	return seq <= 10 && os.Getenv("RE_MVP_POPULATE_DETAIL") == "1"
}

func logPopulateMove(seq Version, clientID int, move string, block MvpDataBlock, from MvpPosition, to MvpPosition) {
	if !shouldLogPopulateDetail(seq) {
		return
	}
	log.Printf(
		"populate detail: seq=%d client=%d move=%s addr=%d sig=%d from=%v to=%v ver=(V:%d A:%d S:%d)",
		seq,
		clientID,
		move,
		block.Addr,
		block.signature,
		from,
		to,
		block.Version.V,
		block.Version.A,
		block.Version.S,
	)
}

func blockMapCount(blocksByAddr map[int][]MvpDataBlock) int {
	count := 0
	for _, blocks := range blocksByAddr {
		count += len(blocks)
	}
	return count
}

func pathBlockCount(path map[MvpPosition]MvpSlot) int {
	count := 0
	for _, slot := range path {
		if !slot.IsEmpty() {
			count++
		}
	}
	return count
}

func (c *MvpClient) logPopulateBreakdown(op OramOP, wAddr int, wBlocks int, priorityTotal int, priorityPlaced int, priorityStashed int, drainAddr int, drainBlocks int, drainPlaced int, unusedSlots int, populatedPath map[MvpPosition]MvpSlot, populatedStash []MvpDataBlock, populatedPathMap []path) {
	if !shouldLogMetrics(c.seq) {
		return
	}
	log.Printf(
		"populate breakdown: seq=%d client=%d op=%s addr=%d W_addr=%d W_blocks=%d priority_total=%d priority_placed=%d priority_stashed=%d drain_addr=%d drain_blocks=%d drain_placed=%d unused_slots=%d final_path=%d final_stash=%d pathmap_updates=%d",
		c.seq,
		c.ClientID,
		op.OP,
		op.target,
		wAddr,
		wBlocks,
		priorityTotal,
		priorityPlaced,
		priorityStashed,
		drainAddr,
		drainBlocks,
		drainPlaced,
		unusedSlots,
		pathBlockCount(populatedPath),
		len(populatedStash),
		len(populatedPathMap),
	)
}

func (c *MvpClient) populatePath(W map[int][]MvpDataBlock, op OramOP, targetSig int) (map[MvpPosition]MvpSlot, []MvpDataBlock, []path) {

	// Phase 1: prepare the path, stash, and position-map update outputs for this eviction.
	populatedPath := make(map[MvpPosition]MvpSlot, c.L+1)
	populatedStash := make([]MvpDataBlock, 0, 40)
	populatedPathMap := make([]path, 0, len(W)*2)

	// Phase 2: remember empty path slots that can later receive drained/copy blocks.
	unusedSlot := make([]MvpPosition, 0, c.L*c.Z)
	usedSlot := make([]MvpPosition, 0, c.L*c.Z)

	wAddrCount := len(W)
	wBlockCount := blockMapCount(W)
	prioritylist := make([]MvpDataBlock, 0)
	drainlist := make(map[int][]MvpDataBlock, 0)
	addrlist := make([]int, 0)

	// Phase 3: locate and update the accessed target block before splitting W.
	targetBlock, ok := c.recoverWorkingBlock(W, op.target, targetSig)
	if !ok {
		log.Printf("target fallback failed: seq=%d client=%d addr=%d sig=%d", c.seq, c.ClientID, op.target, targetSig)
	}
	if ok {
		targetBlock.Version.SetA(c.seq)
		setWorkingBlock(W, targetBlock)
	} else {
		targetBlock = MvpDataBlock{
			Addr:      op.target,
			signature: targetSig,
			Version: Versions{
				V: c.seq,
				A: c.seq,
				S: c.seq,
			},
		}
		log.Printf("target synthesized: seq=%d client=%d addr=%d sig=%d", c.seq, c.ClientID, op.target, targetSig)
	}

	if op.OP == Write {
		targetBlock.Data = op.param
		targetBlock.Version.SetV(c.seq)
		for sig := range c.PositionMap[op.target] {
			if sig == targetSig {
				continue
			}
			path := newPath(op.target, sig, mvpDeletePosition, Versions{V: c.seq, A: c.seq, S: c.seq}, c.seq)
			appendPositionMapUpdate(&populatedPathMap, path)
		}
		delete(W, op.target)
		targetBlock.Version.SetS(c.seq)
		prioritylist = append(prioritylist, targetBlock)
		//populatedStash = append(populatedStash, targetBlock)
		//appendPositionMapUpdate(&populatedPathMap, newPath(targetBlock.Addr, targetBlock.signature, mvpStashPosition, targetBlock.Version, c.seq))
	}

	// Phase 4: split W into high-priority blocks placed first and drain blocks used to fill remaining slots.

	for addr, blocks := range W {
		addrlist = append(addrlist, addr)
		blocks = sort_block(blocks)
		for index, block := range blocks {

			if index == 0 { //コピーがないならindex0のブロックを優先リストに置く
				needSet := true
				for sig, entry := range c.PositionMap[addr] {

					if sig == block.signature {
						continue
					}

					if entry.Slot != mvpDeletePosition {
						needSet = false //mvpDelete以外が存在、つまりどこかにデリートがあったらブロックを消す
						break
					}
				}

				if needSet {
					prioritylist = append(prioritylist, block)
					continue
				}
			}

			drainlist[addr] = append(drainlist[addr], block)
		}
	}

	sort.Slice(prioritylist, func(i, j int) bool {
		return prioritylist[i].Version.A > prioritylist[j].Version.A
	})

	priorityTotal := len(prioritylist)
	// Phase 5: place priority blocks
	priorityPlaced := 0
	for bucketPosition := range c.path {
		for i := 0; i < c.Z; i++ {
			position := MvpPosition{bucket: bucketPosition, slot: MvpSlotPosition(i)}

			if len(prioritylist) == 0 {
				populatedPath[position] = NewMvpSlot(c.seq)
				unusedSlot = append(unusedSlot, position)
				continue
			}

			candidates := make(map[int]MvpDataBlock, 0)
			for priorityIndex, block := range prioritylist {
				if entry, ok := c.PositionMap[block.Addr][block.signature]; ok && entry.Slot == position {
					candidates[priorityIndex] = block
				}
			}

			if len(candidates) == 0 {
				populatedPath[position] = NewMvpSlot(c.seq)
				unusedSlot = append(unusedSlot, position)
				continue
			}

			selectedIndex := -1
			var block MvpDataBlock
			for priorityIndex, candidate := range candidates {
				if selectedIndex == -1 || newerVersions(candidate.Version, block.Version) {
					selectedIndex = priorityIndex
					block = candidate
				}
			}
			prioritylist = append(prioritylist[:selectedIndex], prioritylist[selectedIndex+1:]...)
			priorityPlaced++
			block.Version.SetS(c.seq)

			slot := NewMvpSlot(c.seq)
			slot.SetBlock(block)
			populatedPath[position] = slot
			usedSlot = append(usedSlot, position)
		}
	}

	// Phase 5.5: re-allocate
	setBlock := make([]MvpDataBlock, 0, len(usedSlot))

	for _, position := range usedSlot {
		slot := populatedPath[position]
		if slot.IsEmpty() {
			continue
		}
		setBlock = append(setBlock, slot.Value)
	}

	// ブロックを V -> A -> S の順で降順ソートし、root側に新しいものを寄せる。
	sort.Slice(setBlock, func(i, j int) bool {
		left := setBlock[i].Version
		right := setBlock[j].Version
		if left.V != right.V {
			return left.V > right.V
		}
		if left.A != right.A {
			return left.A > right.A
		}
		return left.S > right.S
	})

	sort.Slice(usedSlot, func(i, j int) bool {
		leftDepth := len(usedSlot[i].bucket.String())
		rightDepth := len(usedSlot[j].bucket.String())
		if usedSlot[i].bucket == mvpRootBucketPosition {
			leftDepth = 0
		}
		if usedSlot[j].bucket == mvpRootBucketPosition {
			rightDepth = 0
		}
		if usedSlot[i].bucket == mvpStashBucketPosition {
			leftDepth = 1 << 30
		}
		if usedSlot[j].bucket == mvpStashBucketPosition {
			rightDepth = 1 << 30
		}
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return usedSlot[i].bucket < usedSlot[j].bucket
	})

	for index, position := range usedSlot { //再配置
		if index >= len(setBlock) {
			populatedPath[position] = NewMvpSlot(c.seq)
			continue
		}
		slot := NewMvpSlot(c.seq)
		block := setBlock[index]
		slot.SetBlock(block)
		populatedPath[position] = slot
		path := newPath(block.Addr, block.signature, position, positionMapVersion(block.Version, c.seq), c.seq)
		appendPositionMapUpdate(&populatedPathMap, path)
		logPopulateMove(c.seq, c.ClientID, "priority_place", block, c.PositionMap[block.Addr][block.signature].Slot, position)
	}

	priorityStashed := 0
	for _, block := range prioritylist {
		block.Version.SetS(c.seq)
		populatedStash = append(populatedStash, block)
		appendPositionMapUpdate(&populatedPathMap, newPath(block.Addr, block.signature, mvpStashPosition, block.Version, c.seq))
		logPopulateMove(c.seq, c.ClientID, "priority_stash", block, c.PositionMap[block.Addr][block.signature].Slot, mvpStashPosition)
		priorityStashed++
	}

	// Phase 6: if no drain blocks exist, return the priority-only path and stash outputs.
	if len(drainlist) == 0 {
		c.logPopulateBreakdown(op, wAddrCount, wBlockCount, priorityTotal, priorityPlaced, priorityStashed, len(drainlist), blockMapCount(drainlist), 0, len(unusedSlot), populatedPath, populatedStash, populatedPathMap)
		return populatedPath, populatedStash, populatedPathMap
	}

	// Phase 7: compute tentative PositionMap state and available signatures for drain placement.
	evaluationResult := c.evaluationPathpattern(addrlist, populatedPathMap)
	virtualPositionMap := clonePositionMap(c.PositionMap)
	applyPathMapsToPositionMap(virtualPositionMap, populatedPathMap)

	addrkey := make([]int, 0, len(evaluationResult))
	addrVSemptysig := make(map[int][]int, 0)

	for addr := range evaluationResult {
		addrkey = append(addrkey, addr)

		for slots, position := range virtualPositionMap[addr] {
			if position.Slot == mvpDeletePosition {
				addrVSemptysig[addr] = append(addrVSemptysig[addr], slots)
			}
		}
	}

	// Phase 8: fill unused slots by re-signing drain blocks into currently deleted signature slots.
	index := len(addrkey)
	drainPlaced := 0
	for _, position := range unusedSlot {
		if len(addrkey) == 0 {
			break
		}
		if index == len(addrkey) { //更新
			sort.Slice(addrkey, func(i, j int) bool {
				left := evaluationResult[addrkey[i]]
				right := evaluationResult[addrkey[j]]
				if left != right {
					return left < right
				}
				return addrkey[i] < addrkey[j]
			})
			index = 0
		}

		var signature int
		var addr int
		foundSignature := false
		for attempts := 0; attempts < len(addrkey); attempts++ {
			addr = addrkey[index]

			if len(addrVSemptysig[addr]) > 0 {
				signature = addrVSemptysig[addr][0]
				addrVSemptysig[addr] = addrVSemptysig[addr][1:]
				index++
				foundSignature = true
				break
			}
			index++
			if index == len(addrkey) {
				index = 0
			}
		}
		if !foundSignature {
			break
		}

		var block MvpDataBlock
		if len(drainlist[addr]) > 0 {
			block = drainlist[addr][0]
			block.Version.SetS(c.seq)
		} else {
			block = W[addr][0]
			block.Version.SetS(c.seq)
		}

		block.signature = signature

		slot := NewMvpSlot(c.seq)
		slot.SetBlock(block)

		populatedPath[position] = slot

		update := newPath(block.Addr, block.signature, position, positionMapVersion(block.Version, c.seq), c.seq)
		appendPositionMapUpdate(&populatedPathMap, update)
		logPopulateMove(c.seq, c.ClientID, "drain_place", block, c.PositionMap[block.Addr][block.signature].Slot, position)
		drainPlaced++

		evaluationResult[addr] += pathPatternCount(position, c.L)

	}

	// Phase 9: delete the original drain signatures after their replacement/copy updates have been emitted.
	if len(drainlist) > 0 {
		for addr, blocks := range drainlist {
			for _, block := range blocks {
				path := newPath(addr, block.signature, mvpDeletePosition, Versions{V: block.Version.V, A: block.Version.A, S: c.seq}, c.seq)
				appendPositionMapUpdate(&populatedPathMap, path)
			}
		}
	}

	c.logPopulateBreakdown(op, wAddrCount, wBlockCount, priorityTotal, priorityPlaced, priorityStashed, len(drainlist), blockMapCount(drainlist), drainPlaced, len(unusedSlot), populatedPath, populatedStash, populatedPathMap)

	// 新しいpath、新しいstash、新しいPathMapを返す。
	return populatedPath, populatedStash, populatedPathMap
}
