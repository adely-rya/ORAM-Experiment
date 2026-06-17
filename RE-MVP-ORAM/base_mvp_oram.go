package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
)

type BaseMvpBucketPosition string
type BaseMvpSlotPosition int

const (
	baseMvpRootBucketPosition                 BaseMvpBucketPosition = "root"
	baseMvpStashBucketPosition                BaseMvpBucketPosition = "-1"
	baseMvpStashSlotPosition                  BaseMvpSlotPosition   = -1
	baseMvpInitialVersion                     Version               = 0
	baseMvpDefaultPathMapCompactInterval                            = 10000
	baseMvpDefaultPathMapCompactProtectedTail                       = 2000
	baseMvpStashMetricsInterval                                     = 100
)

var (
	baseMvpStashPosition = BaseMvpPosition{bucket: baseMvpStashBucketPosition, slot: baseMvpStashSlotPosition}
)

func (p BaseMvpBucketPosition) String() string {
	return string(p)
}

type BaseMvpPosition struct {
	bucket BaseMvpBucketPosition
	slot   BaseMvpSlotPosition
}

type BaseMvpPositionMapEntry struct {
	Slot BaseMvpPosition
	Ts   Versions
}

func (p BaseMvpPosition) String() string {
	return p.bucket.String()
}

type BaseMvpDataBlock struct {
	Addr    int
	Data    string
	Version Versions
}

type BaseMvpSlot struct {
	Version Version
	Value   BaseMvpDataBlock
	Empty   bool
}

func NewBaseMvpSlot(version Version) BaseMvpSlot {
	return BaseMvpSlot{
		Version: version,
		Empty:   true,
	}
}

func (s BaseMvpSlot) IsEmpty() bool {
	return s.Empty
}

func (s *BaseMvpSlot) SetBlock(block BaseMvpDataBlock) bool {
	if !s.IsEmpty() {
		return false
	}

	s.Value = block
	s.Empty = false
	return true
}

func (s BaseMvpSlot) Clone() BaseMvpSlot {
	return s
}

type BaseMvpBucket struct {
	Z     int
	Slots map[BaseMvpSlotPosition]map[Version]BaseMvpSlot
}

func NewBaseMvpBucket(z int) BaseMvpBucket {
	slots := make(map[BaseMvpSlotPosition]map[Version]BaseMvpSlot, z)

	for i := 0; i < z; i++ {
		slotPosition := BaseMvpSlotPosition(i)
		slots[slotPosition] = map[Version]BaseMvpSlot{
			baseMvpInitialVersion: NewBaseMvpSlot(baseMvpInitialVersion),
		}
	}

	return BaseMvpBucket{
		Z:     z,
		Slots: slots,
	}
}

func (b *BaseMvpBucket) SetBlock(slotPosition BaseMvpSlotPosition, version Version, block BaseMvpDataBlock) bool {
	versionedSlots, ok := b.Slots[slotPosition]
	if !ok {
		return false //そのスロットポジションが存在するのか？
	}

	slot, ok := versionedSlots[version]
	if !ok {
		slot = NewBaseMvpSlot(version) //すでにスロットがあるのか？
	}

	if !slot.SetBlock(block) {
		return false //スロットにブロックが入るのか？　すでに入ってしまってないか？
	}

	versionedSlots[version] = slot
	return true
}

func (b BaseMvpBucket) RandomEmptySlot(version Version, rng *rand.Rand) (BaseMvpSlotPosition, bool) {
	slotIndexes := rng.Perm(b.Z)
	for _, slotIndex := range slotIndexes {
		slotPosition := BaseMvpSlotPosition(slotIndex)
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

func (b BaseMvpBucket) Clone() BaseMvpBucket {
	slots := make(map[BaseMvpSlotPosition]map[Version]BaseMvpSlot, len(b.Slots))
	for slotPosition, versionedSlots := range b.Slots {
		slots[slotPosition] = make(map[Version]BaseMvpSlot, len(versionedSlots))
		for version, slot := range versionedSlots {
			slots[slotPosition][version] = slot.Clone()
		}
	}

	return BaseMvpBucket{
		Z:     b.Z,
		Slots: slots,
	}
}

type BaseMvpTree struct {
	Z               int
	L               int
	Tree            map[BaseMvpBucketPosition]BaseMvpBucket
	BucketReadCount map[BaseMvpBucketPosition]int64
	TotalBucketRead int64
}

func NewBaseMvpBucketPosition(level, index int) BaseMvpBucketPosition {
	if level == 0 {
		return baseMvpRootBucketPosition
	}

	key := strconv.FormatInt(int64(index), 2)
	for len(key) < level {
		key = "0" + key
	}
	return BaseMvpBucketPosition(key)
}

func NewBaseMvpTree(z int, l int) BaseMvpTree {
	tree := make(map[BaseMvpBucketPosition]BaseMvpBucket, 1<<(l+1)-1)
	bucketReadCount := make(map[BaseMvpBucketPosition]int64, 1<<(l+1)-1)

	for level := 0; level <= l; level++ {
		for index := 0; index < 1<<level; index++ {
			position := NewBaseMvpBucketPosition(level, index)

			tree[position] = NewBaseMvpBucket(z)
			bucketReadCount[position] = 0
		}
	}

	return BaseMvpTree{
		Z:               z,
		L:               l,
		Tree:            tree,
		BucketReadCount: bucketReadCount,
	}
}

func (t BaseMvpTree) Clone() BaseMvpTree {
	tree := make(map[BaseMvpBucketPosition]BaseMvpBucket, len(t.Tree))
	for position, bucket := range t.Tree {
		tree[position] = bucket.Clone()
	}

	bucketReadCount := make(map[BaseMvpBucketPosition]int64, len(t.BucketReadCount))
	for position, count := range t.BucketReadCount {
		bucketReadCount[position] = count
	}

	return BaseMvpTree{
		Z:               t.Z,
		L:               t.L,
		Tree:            tree,
		BucketReadCount: bucketReadCount,
		TotalBucketRead: t.TotalBucketRead,
	}
}

func (t *BaseMvpTree) CountBucketRead(position BaseMvpBucketPosition) {
	t.BucketReadCount[position]++
	t.TotalBucketRead++
}

type basePath struct {
	addr int
	to   BaseMvpPosition
	Ver  Versions
	Seq  Version
}

func newBaseMvpPath(addr int, to any, ver Versions, seq Version) basePath {
	return basePath{
		addr: addr,
		to:   baseMvpPathPosition(to),
		Ver:  ver,
		Seq:  seq,
	}
}

func baseMvpPathPosition(to any) BaseMvpPosition {
	switch position := to.(type) {
	case BaseMvpPosition:
		return position
	case BaseMvpBucketPosition:
		return BaseMvpPosition{bucket: position}
	default:
		panic("unsupported basePath destination type")
	}
}

func baseMvpPathBucketPosition(to any) BaseMvpBucketPosition {
	switch position := to.(type) {
	case BaseMvpBucketPosition:
		return position
	case BaseMvpPosition:
		return position.bucket
	default:
		panic("unsupported basePath destination type")
	}
}

type BaseMvpServerRequest interface {
	handle(s *BaseMvpServer)
}

type BaseMvpGetpmRequest struct {
	ClientID int
	Reply    chan BaseMvpGetpmResponse
}

type BaseMvpGetpmResponse struct {
	PathMap []basePath
	Seq     Version
	Err     error
}

type BaseMvpGetpsRequest struct {
	ClientID int
	Leaf     BaseMvpPosition
	Reply    chan BaseMvpGetpsResponse
}

type BaseMvpGetpsResponse struct {
	Path  map[BaseMvpBucketPosition]BaseMvpBucket
	Stash map[int][]BaseMvpDataBlock
	Err   error
}

type BaseMvpEvictRequest struct {
	ClientID int
	Seq      Version
	PathMap  []basePath
	Stash    []BaseMvpDataBlock
	Path     map[BaseMvpPosition]BaseMvpSlot
	Reply    chan BaseMvpEvictResponse
}

type BaseMvpEvictResponse struct {
	Err error
}

type BaseMvpGetStashMetricsRequest struct {
	Reply chan BaseMvpGetStashMetricsResponse
}

type BaseMvpGetStashMetricsResponse struct {
	Seq                 Version
	StashVersions       int
	StashTotal          int
	StashMaxVersionSize int
	Err                 error
}

type BaseMvpOramState struct {
	TreeState  BaseMvpTree
	StashState map[int][]BaseMvpDataBlock
}

func cloneBaseMvpStashs(stashs map[int][]BaseMvpDataBlock) map[int][]BaseMvpDataBlock {
	cloned := make(map[int][]BaseMvpDataBlock, len(stashs))
	for clientID, stash := range stashs {
		cloned[clientID] = append([]BaseMvpDataBlock(nil), stash...)
	}

	return cloned
}

func cloneBaseMvpPositionMap(positionMap map[int]BaseMvpPositionMapEntry) map[int]BaseMvpPositionMapEntry {
	cloned := make(map[int]BaseMvpPositionMapEntry, len(positionMap))
	for addr, entry := range positionMap {
		cloned[addr] = entry
	}

	return cloned
}

type BaseMvpServer struct {
	PositionMaps                 []map[int]BaseMvpPositionMapEntry
	PathMaps                     []basePath
	Stashs                       map[int][]BaseMvpDataBlock
	tree                         BaseMvpTree
	counter                      Version
	useSnapshot                  bool
	Snapshot                     map[int]BaseMvpOramState
	PathMapCursor                map[int]int
	PathMapCompactInterval       int
	PathMapCompactProtectedTail  int
	pathMapEvictsSinceCompaction int

	Requests chan BaseMvpServerRequest
}

func NewBaseMvpServer(z int, l int) *BaseMvpServer {
	return &BaseMvpServer{
		PositionMaps:  make([]map[int]BaseMvpPositionMapEntry, 0),
		PathMaps:      make([]basePath, 0),
		Stashs:        make(map[int][]BaseMvpDataBlock, 0),
		tree:          NewBaseMvpTree(z, l),
		Requests:      make(chan BaseMvpServerRequest),
		counter:       baseMvpInitialVersion,
		useSnapshot:   true,
		Snapshot:      make(map[int]BaseMvpOramState, 50),
		PathMapCursor: make(map[int]int, 50),

		PathMapCompactInterval:      baseMvpDefaultPathMapCompactInterval,
		PathMapCompactProtectedTail: baseMvpDefaultPathMapCompactProtectedTail,
	}
}

func NewSynchronizedBaseMvpServer(z int, l int) *BaseMvpServer {
	server := NewBaseMvpServer(z, l)
	server.useSnapshot = false
	server.Snapshot = nil
	return server
}

func (s *BaseMvpServer) SetPathMapCompaction(interval int, protectedTail int) {
	s.PathMapCompactInterval = interval
	s.PathMapCompactProtectedTail = protectedTail
	s.pathMapEvictsSinceCompaction = 0
}

func (s *BaseMvpServer) compactPathMaps() {
	if len(s.PathMaps) == 0 || s.PathMapCompactProtectedTail <= 0 {
		return
	}

	compactEnd := len(s.PathMaps) - s.PathMapCompactProtectedTail
	if compactEnd <= 0 {
		return
	}

	addrInProtectedTail := make(map[int]struct{}, s.PathMapCompactProtectedTail)
	for _, entry := range s.PathMaps[compactEnd:] {
		addrInProtectedTail[entry.addr] = struct{}{}
	}
	if len(addrInProtectedTail) == 0 {
		return
	}

	removedPrefix := make([]int, len(s.PathMaps)+1)
	compacted := make([]basePath, 0, len(s.PathMaps))
	for index, entry := range s.PathMaps {
		removedPrefix[index+1] = removedPrefix[index]
		if index < compactEnd {
			if _, ok := addrInProtectedTail[entry.addr]; ok {
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

func (s *BaseMvpServer) InitializeRandomData(n int, seed int64) map[int]BaseMvpPositionMapEntry {
	rng := rand.New(rand.NewSource(seed))
	positions := make([]BaseMvpBucketPosition, 0, len(s.tree.Tree)) //木に存在するポジション一覧生成
	for position := range s.tree.Tree {
		positions = append(positions, position)
	}

	sort.Slice(positions, func(i, j int) bool {
		return positions[i] < positions[j]
	}) //小さい順にソート

	positionMap := make(map[int]BaseMvpPositionMapEntry, n)
	stash := make([]BaseMvpDataBlock, 0)

	for addr := 0; addr < n; addr++ {
		block := BaseMvpDataBlock{
			Addr: addr,
			Data: strconv.Itoa(addr),
			Version: Versions{
				V: baseMvpInitialVersion,
				A: baseMvpInitialVersion,
				S: baseMvpInitialVersion,
			},
		}
		position := positions[rng.Intn(len(positions))] //ランダムにポジションを選ぶ
		bucket := s.tree.Tree[position]

		if slotPosition, ok := bucket.RandomEmptySlot(baseMvpInitialVersion, rng); ok && bucket.SetBlock(slotPosition, baseMvpInitialVersion, block) {
			s.tree.Tree[position] = bucket
			positionMap[addr] = BaseMvpPositionMapEntry{
				Slot: BaseMvpPosition{
					bucket: position,
					slot:   slotPosition,
				},
				Ts: block.Version,
			}
			continue
		}

		stash = append(stash, block) //溢れたらスタッシュに移動
		positionMap[addr] = BaseMvpPositionMapEntry{
			Slot: baseMvpStashPosition,
			Ts:   block.Version,
		}
	}

	s.PositionMaps = append(s.PositionMaps, positionMap)
	s.Stashs[0] = stash

	return positionMap

}

func (s *BaseMvpServer) Run() {
	for req := range s.Requests {
		req.handle(s)
	}
}

func (r BaseMvpGetpmRequest) handle(s *BaseMvpServer) {
	s.counter.increment()
	seq := s.counter

	if s.useSnapshot {
		s.Snapshot[r.ClientID] = BaseMvpOramState{
			TreeState:  s.tree.Clone(),
			StashState: cloneBaseMvpStashs(s.Stashs),
		}
	}

	cursor := s.PathMapCursor[r.ClientID]
	if cursor < 0 || cursor > len(s.PathMaps) {
		cursor = len(s.PathMaps)
	}

	difpathMap := append([]basePath(nil), s.PathMaps[cursor:]...)
	s.PathMapCursor[r.ClientID] = len(s.PathMaps)

	r.Reply <- BaseMvpGetpmResponse{
		Seq:     seq,
		PathMap: difpathMap,
		Err:     nil,
	}
}

func (r BaseMvpGetpsRequest) handle(s *BaseMvpServer) {
	var tree BaseMvpTree
	var stash map[int][]BaseMvpDataBlock
	if s.useSnapshot {
		oramstate, ok := s.Snapshot[r.ClientID]
		if !ok {
			r.Reply <- BaseMvpGetpsResponse{
				Err: fmt.Errorf("snapshot for client %d not found", r.ClientID),
			}
			return
		}
		tree = oramstate.TreeState
		stash = oramstate.StashState
	} else {
		tree = s.tree
		stash = cloneBaseMvpStashs(s.Stashs)
	}

	basePath := make(map[BaseMvpBucketPosition]BaseMvpBucket, s.tree.L+1)
	root, ok := tree.Tree[baseMvpRootBucketPosition]
	if !ok {
		r.Reply <- BaseMvpGetpsResponse{
			Err: fmt.Errorf("root bucket not found"),
		}
		return
	}

	basePath[baseMvpRootBucketPosition] = root
	s.tree.CountBucketRead(baseMvpRootBucketPosition)

	leaf := r.Leaf.bucket.String()
	for len(leaf) != 0 {
		bucketPosition := BaseMvpBucketPosition(leaf)
		basePath[bucketPosition] = tree.Tree[bucketPosition]
		s.tree.CountBucketRead(bucketPosition)
		leaf = leaf[:len(leaf)-1]
	}

	r.Reply <- BaseMvpGetpsResponse{
		Path:  basePath,
		Stash: stash,
		Err:   nil,
	}
}

func (r BaseMvpEvictRequest) handle(s *BaseMvpServer) {
	nowtree := s.tree.Tree
	var oldtree map[BaseMvpBucketPosition]BaseMvpBucket
	var oldstash map[int][]BaseMvpDataBlock
	outputVersions := make(map[int]Versions, len(r.PathMap))
	for _, basePath := range r.PathMap {
		if current, ok := outputVersions[basePath.addr]; !ok || baseMvpNewerVersions(basePath.Ver, current) {
			outputVersions[basePath.addr] = basePath.Ver
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
				outputVersion, ok := outputVersions[oldslot.Value.Addr]
				if !ok || baseMvpNewerVersions(oldslot.Value.Version, outputVersion) {
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
			remaining := make([]BaseMvpDataBlock, 0, len(stash))
			for _, block := range stash {
				outputVersion, ok := outputVersions[block.Addr]
				if ok && !baseMvpNewerVersions(block.Version, outputVersion) {
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

	for _, basePath := range r.PathMap {
		s.PathMaps = append(s.PathMaps, basePath)
	}
	s.pathMapEvictsSinceCompaction++
	if s.PathMapCompactInterval > 0 && s.pathMapEvictsSinceCompaction >= s.PathMapCompactInterval {
		s.compactPathMaps()
		s.pathMapEvictsSinceCompaction = 0
	}

	r.Reply <- BaseMvpEvictResponse{
		Err: nil,
	}
}

func (r BaseMvpGetStashMetricsRequest) handle(s *BaseMvpServer) {
	totalStashBlocks := 0
	maxStashVersionBlocks := 0
	for _, stash := range s.Stashs {
		totalStashBlocks += len(stash)
		if len(stash) > maxStashVersionBlocks {
			maxStashVersionBlocks = len(stash)
		}
	}

	r.Reply <- BaseMvpGetStashMetricsResponse{
		Seq:                 s.counter,
		StashVersions:       len(s.Stashs),
		StashTotal:          totalStashBlocks,
		StashMaxVersionSize: maxStashVersionBlocks,
		Err:                 nil,
	}
}

type BaseMvpClient struct {
	L           int
	Z           int
	PositionMap map[int]BaseMvpPositionMapEntry
	Stash       map[int][]BaseMvpDataBlock
	basePath    map[BaseMvpBucketPosition]BaseMvpBucket

	ClientID int
	Server   chan<- BaseMvpServerRequest

	seq Version
}

func NewBaseMvpClient(l int, z int, clientID int, positionmap map[int]BaseMvpPositionMapEntry, server chan<- BaseMvpServerRequest) *BaseMvpClient {
	return &BaseMvpClient{
		L:           l,
		Z:           z,
		ClientID:    clientID,
		Server:      server,
		PositionMap: positionmap,
	}
}

func (c *BaseMvpClient) GetPM() (Version, []basePath, error) {
	reply := make(chan BaseMvpGetpmResponse)

	c.Server <- BaseMvpGetpmRequest{
		ClientID: c.ClientID,
		Reply:    reply,
	}

	res := <-reply
	return res.Seq, res.PathMap, res.Err
}

func (c *BaseMvpClient) GetPS(leaf BaseMvpPosition) (map[BaseMvpBucketPosition]BaseMvpBucket, map[int][]BaseMvpDataBlock, error) {
	reply := make(chan BaseMvpGetpsResponse)

	c.Server <- BaseMvpGetpsRequest{
		ClientID: c.ClientID,
		Leaf:     leaf,
		Reply:    reply,
	}

	res := <-reply
	return res.Path, res.Stash, res.Err
}

func (c *BaseMvpClient) Evict(basePath map[BaseMvpPosition]BaseMvpSlot, pathmap []basePath, stash []BaseMvpDataBlock) error {
	reply := make(chan BaseMvpEvictResponse)

	c.Server <- BaseMvpEvictRequest{
		ClientID: c.ClientID,
		Seq:      c.seq,
		Path:     basePath,
		PathMap:  pathmap,
		Stash:    stash,
		Reply:    reply,
	}

	res := <-reply
	return res.Err

}

func (c *BaseMvpClient) Run(addrCount int) error {

	for {
		target := rand.Intn(addrCount)
		param := fmt.Sprintf("client-%d-%d", c.ClientID, target)

		err := c.Access(OramOP{Write, target, param})
		if err != nil {
			return err
		}
	}
}

func (c *BaseMvpClient) Access(op OramOP) error {
	version, pathMaps, err := c.GetPM() //Getpm操作
	c.seq = version
	if err != nil {
		return err
	}

	c.consolidatePathMaps(pathMaps) //position mapの更新
	if accessLoggingEnabled {
		log.Printf("access start: client=%d seq=%d op=%s addr=%d position=%v", c.ClientID, version, op.OP, op.target, c.PositionMap[op.target].Slot)
	}

	accessPosition := c.PositionMap[op.target].Slot

	c.basePath, c.Stash, err = c.GetPS(c.baseMvpSelectPath(accessPosition, c.L)) //Getps操作
	if err != nil {
		return err
	}

	W := c.mergePathStashes() //ワーキングセット制作

	targetBlock, ok := W[op.target]
	if !ok {
		return fmt.Errorf("Not target block in working set")
	}

	if op.OP == Write {
		targetBlock.Data = op.param

		targetBlock.Version = Versions{version, version, version}
	} else {
		targetBlock.Version.SetA(version)
	}
	W[op.target] = targetBlock

	populatedPath, populatedStash, populatedPathMap := c.populatePath(W, op)

	err = c.Evict(populatedPath, populatedPathMap, populatedStash)
	if err != nil {
		return err
	}
	if accessLoggingEnabled {
		log.Printf("access success: client=%d seq=%d op=%s addr=%d", c.ClientID, version, op.OP, op.target)
	}

	return nil
}

func (c *BaseMvpClient) consolidatePathMaps(pathMaps []basePath) {
	latestPathMap := make(map[int]basePath, len(pathMaps))
	// 受信したPathMapから、アドレスごとの最新更新だけを残す。
	for _, v := range pathMaps {
		latest, ok := latestPathMap[v.addr]
		if !ok || baseMvpNewerPathUpdate(v, latest) {
			latestPathMap[v.addr] = v
		}
	}

	// 最新更新をPositionMapへ反映する。現在のPositionMapより新しい更新だけを上書きする。
	for _, v := range latestPathMap {
		current, ok := c.PositionMap[v.addr]
		if !ok || baseMvpNewerPositionUpdate(v, current) {
			c.PositionMap[v.addr] = BaseMvpPositionMapEntry{Slot: v.to, Ts: v.Ver}
		}
	}
}

func (c *BaseMvpClient) baseMvpSelectPath(accessPosition BaseMvpPosition, pathlen int) BaseMvpPosition {
	if accessPosition == baseMvpStashPosition {
		return baseMvpRandomLeafPosition(pathlen)
	}

	return baseMvpSelectPath(accessPosition, pathlen)
}

func baseMvpRandomLeafPosition(pathlen int) BaseMvpPosition {
	leaf := ""
	for len(leaf) < pathlen {
		if rand.Intn(2) == 0 {
			leaf += "0"
		} else {
			leaf += "1"
		}
	}

	return BaseMvpPosition{
		bucket: BaseMvpBucketPosition(leaf),
	}
}

func baseMvpSelectPath(accessPosition any, pathlen int) BaseMvpPosition {
	bucketPosition := baseMvpPathBucketPosition(accessPosition)
	if bucketPosition == baseMvpStashBucketPosition || bucketPosition == baseMvpRootBucketPosition {
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

	return BaseMvpPosition{
		bucket: BaseMvpBucketPosition(leaf),
	}
}

func (c *BaseMvpClient) mergePathStashes() map[int]BaseMvpDataBlock {
	W := make(map[int]BaseMvpDataBlock, 0)

	for _, v := range c.Stash {
		for _, block := range v {
			pm, ok := c.PositionMap[block.Addr]
			if ok && pm.Slot == baseMvpStashPosition && baseMvpSameDataVersion(pm.Ts, block.Version) {
				W[block.Addr] = block
			}
		}
	}

	for bucketPosition, bucket := range c.basePath {
		for slotPosition, versionedSlots := range bucket.Slots {
			for _, slot := range versionedSlots {
				if slot.IsEmpty() {
					continue
				}

				block := slot.Value
				pm, ok := c.PositionMap[block.Addr]
				blockPosition := BaseMvpPosition{bucket: bucketPosition, slot: slotPosition}
				if ok && pm.Slot == blockPosition && baseMvpSameDataVersion(pm.Ts, block.Version) {
					W[block.Addr] = block
				}
			}
		}
	}

	return W
}

func baseMvpNewerVersions(left Versions, right Versions) bool {
	if left.V != right.V {
		return left.V > right.V
	}
	if left.A != right.A {
		return left.A > right.A
	}
	return left.S > right.S
}

func baseMvpSameDataVersion(left Versions, right Versions) bool {
	return left.V == right.V && left.A == right.A
}

func baseMvpNewerPathUpdate(left basePath, right basePath) bool {
	if left.Ver.V != right.Ver.V {
		return left.Ver.V > right.Ver.V
	}
	if left.Ver.A != right.Ver.A {
		return left.Ver.A > right.Ver.A
	}
	return left.Seq > right.Seq
}

func baseMvpNewerPositionUpdate(update basePath, current BaseMvpPositionMapEntry) bool {
	if update.Ver.V != current.Ts.V {
		return update.Ver.V > current.Ts.V
	}
	if update.Ver.A != current.Ts.A {
		return update.Ver.A > current.Ts.A
	}
	return update.Seq > current.Ts.S
}

func baseMvpPositionMapVersion(blockVersion Versions, locationUpdate Version) Versions {
	return Versions{
		V: blockVersion.V,
		A: blockVersion.A,
		S: locationUpdate,
	}
}

func baseMvpSortBlock(blockList []BaseMvpDataBlock) []BaseMvpDataBlock {
	sort.Slice(blockList, func(i, j int) bool {
		left := blockList[i].Version
		right := blockList[j].Version

		if left.V != right.V {
			return left.V < right.V
		}
		if left.A != right.A {
			return left.A < right.A
		}
		return false
	})

	return blockList
}

func baseMvpSortPosition(positionList []BaseMvpPosition) []BaseMvpPosition {
	bucketDepth := func(position BaseMvpPosition) int {
		switch position.bucket {
		case baseMvpRootBucketPosition:
			return 0
		case baseMvpStashBucketPosition:
			return 1 << 30
		default:
			return len(position.bucket.String())
		}
	}

	sort.Slice(positionList, func(i, j int) bool {
		leftDepth := bucketDepth(positionList[i])
		rightDepth := bucketDepth(positionList[j])
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return positionList[i].bucket < positionList[j].bucket
	})

	return positionList
}

func (c *BaseMvpClient) populatePath(W map[int]BaseMvpDataBlock, op OramOP) (map[BaseMvpPosition]BaseMvpSlot, []BaseMvpDataBlock, []basePath) {
	// 返却するbasePathはslot単位でサーバーへ渡すので、実際の書き込み位置はBaseMvpPositionで持つ。
	populatedPath := make(map[BaseMvpPosition]BaseMvpSlot, c.L+1)
	// 返却するstashは、basePathへ残らなかったブロックだけを入れる。
	populatedStash := make([]BaseMvpDataBlock, 0, 20)
	// PositionMap更新はslot単位で、artifactのlocationと同じ意味のBaseMvpPositionを記録する。
	populatedPathMap := make([]basePath, 0, len(W))
	// アクセス対象の現在位置はslot単位で読む。
	targetPosition := c.PositionMap[op.target].Slot
	// 読み込んだbasePathに含まれる全slotを、stashからbasePathへ戻す置換候補として記録する。
	allSlot := make([]BaseMvpPosition, 0, (c.L+1)*c.Z)
	// 初回配置で実際にブロックを置いたslotだけを、後段のshuffle対象として記録する。
	usedSlot := make([]BaseMvpPosition, 0, c.L*c.Z)
	// アクセス対象ブロックが初回配置されたslotを、後で必ずstashへ退避するために覚える。
	var targetPlacedPosition BaseMvpPosition
	// アクセス対象ブロックが初回配置されたかどうかを記録する。
	targetPlaced := false

	// 読み込んだbasePath上の各bucketを走査する。
	initialPlaced := 0
	initialCandidateSlots := 0
	for bucketPosition := range c.basePath {
		// bucket内の各slotを順番に埋める。
		for i := 0; i < c.Z; i++ {
			// このslotに書くための実位置を作る。
			position := BaseMvpPosition{bucket: bucketPosition, slot: BaseMvpSlotPosition(i)}
			// このslotはbasePath上に存在するので、後段の置換候補へ追加する。
			allSlot = append(allSlot, position)
			// このbucketに所属するブロック候補をWから集める。
			candidates := make([]BaseMvpDataBlock, 0, len(W))
			// W内の各ブロックを確認する。
			for _, block := range W {
				// PositionMapがこのexact slotを指すブロックだけを候補にする。
				if c.PositionMap[block.Addr].Slot == position {
					candidates = append(candidates, block)
				}
			}
			// 候補がなければ、このslotは空slotとして出力する。
			if len(candidates) == 0 {
				populatedPath[position] = NewBaseMvpSlot(c.seq)
				continue
			}
			initialCandidateSlots++
			// 同じbucketに複数候補がある場合はversion順に並べる。
			candidates = baseMvpSortBlock(candidates)
			// 一番新しいversionのブロックをこのslotに置く。
			selected := candidates[len(candidates)-1]
			// 書き込み用の新しいslotを作る。
			slot := NewBaseMvpSlot(c.seq)
			// 選んだブロックをslotへ入れる。
			slot.SetBlock(selected)
			// 実際にこのslotをbasePath出力へ登録する。
			populatedPath[position] = slot
			// このslotは後段のshuffleで使えるslotとして覚える。
			usedSlot = append(usedSlot, position)
			initialPlaced++
			// アクセス対象ブロックを置いたslotなら、その実slotを覚える。
			if selected.Addr == op.target {
				targetPlacedPosition = position
				targetPlaced = true
			}
			// 選んだブロックはWから取り除き、同じブロックを複数slotへ置かないようにする。
			delete(W, selected.Addr)
		}
	}

	// stashからbasePathへ戻す候補を作る。
	blockList := make([]BaseMvpDataBlock, 0, len(W))
	// Wに残ったブロックを走査する。
	for _, block := range W {
		// アクセス対象ブロックはstashへ退避するので、basePathへ戻す候補から外す。
		if block.Addr == op.target {
			continue
		}
		// アクセス対象以外の残りブロックをswap候補へ入れる。
		blockList = append(blockList, block)
	}

	// 実際にbasePathへ戻すブロックを最大Z個まで選ぶ。
	swapCandidate := make([]BaseMvpDataBlock, 0, c.Z)
	// 候補がZ個以下なら全て選ぶ。
	if len(blockList) <= c.Z {
		swapCandidate = append(swapCandidate, blockList...)
	} else {
		// 候補がZ個より多ければランダムにZ個だけ選ぶ。
		for _, index := range rand.Perm(len(blockList))[:c.Z] {
			swapCandidate = append(swapCandidate, blockList[index])
		}
	}

	// swapするslotを最大Z個まで選ぶ。
	swapSlot := make([]BaseMvpPosition, 0, c.Z)
	// 同じslotを二重に選ばないための集合を作る。
	swapSlotSet := make(map[BaseMvpPosition]bool, c.Z)
	// アクセス対象が初回配置された場合は、そのslotを必ずswap対象にする。
	if targetPlaced && targetPosition != baseMvpStashPosition {
		swapSlot = append(swapSlot, targetPlacedPosition)
		swapSlotSet[targetPlacedPosition] = true
	}
	// basePath全体のslotの中から、残りのswap slotをランダムに選ぶ。
	for _, index := range rand.Perm(len(allSlot)) {
		// Z個選べたら終了する。
		if len(swapSlot) >= c.Z {
			break
		}
		// ランダム順で次のslot候補を取り出す。
		position := allSlot[index]
		// 既に選んだslotなら飛ばす。
		if swapSlotSet[position] {
			continue
		}
		// このslotをswap対象へ追加する。
		swapSlot = append(swapSlot, position)
		// このslotを選択済みにする。
		swapSlotSet[position] = true
	}

	// 選んだslotをstash候補と入れ替える。
	for index, slotPosition := range swapSlot {
		// swap前にそのslotへ置かれていたブロックを読む。
		slot := populatedPath[slotPosition]
		// swap後に置く新しいslotを作る。
		newslot := NewBaseMvpSlot(c.seq)
		// swap候補ブロックが残っていれば、このslotへ入れる。
		if len(swapCandidate) > index {
			newslot.SetBlock(swapCandidate[index])
			delete(W, swapCandidate[index].Addr)
		}
		// swap前のslotにブロックがあれば、stashへ戻すためにWへ戻す。
		if !slot.IsEmpty() {
			swaped := slot.Value
			W[swaped.Addr] = swaped
		}
		// swap後のslotをbasePath出力へ反映する。
		populatedPath[slotPosition] = newslot
	}

	// basePath上に残ったブロックを集め直す。
	onPathBlock := make([]BaseMvpDataBlock, 0)
	// basePath上に残ったブロックの実slotを集め直す。
	onPathPosition := make([]BaseMvpPosition, 0, len(allSlot))
	// basePath全体を再配置対象として見る。
	for _, position := range allSlot {
		// 現在そのslotにあるブロックを読む。
		slot := populatedPath[position]
		// 空slotは再配置対象にしない。
		if slot.IsEmpty() {
			continue
		}
		// slot内のブロックを再配置用リストへ入れる。
		onPathBlock = append(onPathBlock, slot.Value)
		// 実際に使われているslotを再配置用リストへ入れる。
		onPathPosition = append(onPathPosition, position)
	}

	// アクセス時刻Aが大きいブロックから順に並べ、よくアクセスされるブロックをroot側へ置く。
	sort.Slice(onPathBlock, func(i, j int) bool {
		return onPathBlock[i].Version.A > onPathBlock[j].Version.A
	})
	// slotはbucket深さでroot側からleaf側へ並べる。
	onPathPosition = baseMvpSortPosition(onPathPosition)

	// 並べ替え後のブロックをslotへ書き戻す。
	for index, position := range onPathPosition {
		// 新しいslot versionでslotを作る。
		slot := NewBaseMvpSlot(c.seq)
		// root側から順に置くブロックを取り出す。
		block := onPathBlock[index]
		// ブロックをslotへ入れる。
		slot.SetBlock(block)
		// basePath出力へslotを書き戻す。
		populatedPath[position] = slot
		// PositionMap更新はslot単位で記録する。
		basePath := newBaseMvpPath(block.Addr, position, baseMvpPositionMapVersion(block.Version, c.seq), c.seq)
		// PathMapへslot位置の更新を追加する。
		populatedPathMap = append(populatedPathMap, basePath)
	}

	// basePathへ残らなかったブロックをstashへ入れる。
	for _, block := range W {
		// 新しいstash出力へブロックを追加する。
		populatedStash = append(populatedStash, block)
		if c.PositionMap[block.Addr].Slot == baseMvpStashPosition && baseMvpSameDataVersion(c.PositionMap[block.Addr].Ts, block.Version) {
			continue
		}
		// PositionMap更新はstash位置として記録する。
		basePath := newBaseMvpPath(block.Addr, baseMvpStashPosition, baseMvpPositionMapVersion(block.Version, c.seq), c.seq)
		// PathMapへstash位置の更新を追加する。
		populatedPathMap = append(populatedPathMap, basePath)
	}

	// 新しいbasePath、新しいstash、新しいPathMapを返す。
	return populatedPath, populatedStash, populatedPathMap
}
