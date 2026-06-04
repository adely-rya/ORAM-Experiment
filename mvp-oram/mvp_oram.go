package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"time"
)

type MvpBucketPosition string
type MvpSlotPosition int

const (
	mvpRootBucketPosition                 MvpBucketPosition = "root"
	mvpStashBucketPosition                MvpBucketPosition = "-1"
	mvpStashSlotPosition                  MvpSlotPosition   = -1
	mvpInitialVersion                     Version           = 0
	mvpDefaultPathMapCompactInterval                        = 10000
	mvpDefaultPathMapCompactProtectedTail                   = 2000
)

var (
	mvpRootPosition  = MvpPosition{bucket: mvpRootBucketPosition}
	mvpStashPosition = MvpPosition{bucket: mvpStashBucketPosition, slot: mvpStashSlotPosition}
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
	V Version // 最後の書き込み/読み込みバージョン
	A Version // 最後の書き込み or 読み取り
	S Version // 最後に移動されたバージョン
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
	Addr    int
	Data    string
	Version Versions
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

func (t *MvpTree) CountBucketRead(position MvpBucketPosition) {
	t.BucketReadCount[position]++
	t.TotalBucketRead++
}

type path struct {
	addr int
	to   MvpPosition
	Ver  Versions
	Seq  Version
}

func newPath(addr int, to MvpPosition, ver Versions, seq Version) path {
	return path{
		addr: addr,
		to:   to,
		Ver:  ver,
		Seq:  seq,
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

func clonePositionMap(positionMap map[int]MvpPositionMapEntry) map[int]MvpPositionMapEntry {
	cloned := make(map[int]MvpPositionMapEntry, len(positionMap))
	for addr, entry := range positionMap {
		cloned[addr] = entry
	}

	return cloned
}

type MvpServer struct {
	PositionMaps                 []map[int]MvpPositionMapEntry
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
		PositionMaps:  make([]map[int]MvpPositionMapEntry, 0),
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

	addrInProtectedTail := make(map[int]struct{}, s.PathMapCompactProtectedTail)
	for _, entry := range s.PathMaps[compactEnd:] {
		addrInProtectedTail[entry.addr] = struct{}{}
	}
	if len(addrInProtectedTail) == 0 {
		return
	}

	removedPrefix := make([]int, len(s.PathMaps)+1)
	compacted := make([]path, 0, len(s.PathMaps))
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

func (s *MvpServer) InitializeRandomData(n int, seed int64) map[int]MvpPositionMapEntry {
	rng := rand.New(rand.NewSource(seed))
	positions := make([]MvpBucketPosition, 0, len(s.tree.Tree)) //木に存在するポジション一覧生成
	for position := range s.tree.Tree {
		positions = append(positions, position)
	}

	sort.Slice(positions, func(i, j int) bool {
		return positions[i] < positions[j]
	}) //小さい順にソート

	positionMap := make(map[int]MvpPositionMapEntry, n)
	stash := make([]MvpDataBlock, 0)

	for addr := 0; addr < n; addr++ {
		block := MvpDataBlock{
			Addr: addr,
			Data: strconv.Itoa(addr),
			Version: Versions{
				V: mvpInitialVersion,
				A: mvpInitialVersion,
				S: mvpInitialVersion,
			},
		}
		position := positions[rng.Intn(len(positions))] //ランダムにポジションを選ぶ
		bucket := s.tree.Tree[position]

		if slotPosition, ok := bucket.RandomEmptySlot(mvpInitialVersion, rng); ok && bucket.SetBlock(slotPosition, mvpInitialVersion, block) {
			s.tree.Tree[position] = bucket
			positionMap[addr] = MvpPositionMapEntry{
				Slot: MvpPosition{
					bucket: position,
					slot:   slotPosition,
				},
				Ts: block.Version,
			}
			continue
		}

		stash = append(stash, block) //溢れたらスタッシュに移動
		positionMap[addr] = MvpPositionMapEntry{
			Slot: mvpStashPosition,
			Ts:   block.Version,
		}
	}

	s.PositionMaps = append(s.PositionMaps, positionMap)
	s.Stashs[0] = stash

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

	path[mvpRootBucketPosition] = root
	s.tree.CountBucketRead(mvpRootBucketPosition)

	leaf := r.Leaf.bucket.String()
	for len(leaf) != 0 {
		bucketPosition := MvpBucketPosition(leaf)
		path[bucketPosition] = tree.Tree[bucketPosition]
		s.tree.CountBucketRead(bucketPosition)
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
	outputVersions := make(map[int]Versions, len(r.PathMap))
	for _, path := range r.PathMap {
		if current, ok := outputVersions[path.addr]; !ok || newerVersions(path.Ver, current) {
			outputVersions[path.addr] = path.Ver
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
				outputVersion, ok := outputVersions[block.Addr]
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

type MvpClient struct {
	L           int
	Z           int
	PositionMap map[int]MvpPositionMapEntry
	Stash       map[int][]MvpDataBlock
	path        map[MvpBucketPosition]MvpBucket

	ClientID int
	Server   chan<- ServerRequest

	seq Version
}

func NewMvpClient(l int, z int, clientID int, positionmap map[int]MvpPositionMapEntry, server chan<- ServerRequest) *MvpClient {
	return &MvpClient{
		L:           l,
		Z:           z,
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
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		target := rand.Intn(addrCount)
		param := fmt.Sprintf("client-%d-%d", c.ClientID, target)

		err := c.Access(OramOP{Write, target, param})
		if err != nil {
			return err
		}

		<-ticker.C
	}
}

func (c *MvpClient) Access(op OramOP) error {
	version, pathMaps, err := c.GetPM() //Getpm操作
	c.seq = version
	if err != nil {
		return err
	}

	c.consolidatePathMaps(pathMaps) //position mapの更新
	log.Printf("access start: client=%d seq=%d op=%s addr=%d position=%v", c.ClientID, version, op.OP, op.target, c.PositionMap[op.target].Slot)

	accessPosition := c.PositionMap[op.target].Slot

	c.path, c.Stash, err = c.GetPS(c.selectPath(accessPosition, c.L)) //Getps操作
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
		targetBlock.Version.SetS(version)
	}
	W[op.target] = targetBlock

	populatedPath, populatedStash, populatedPathMap := c.populatePath(W, op)

	err = c.Evict(populatedPath, populatedPathMap, populatedStash)
	if err != nil {
		return err
	}
	log.Printf("access success: client=%d seq=%d op=%s addr=%d", c.ClientID, version, op.OP, op.target)

	return nil
}

func (c *MvpClient) consolidatePathMaps(pathMaps []path) {
	latestPathMap := make(map[int]path, len(pathMaps))
	for _, v := range pathMaps {
		latest, ok := latestPathMap[v.addr]
		if !ok || newerVersions(v.Ver, latest.Ver) {
			latestPathMap[v.addr] = v
		}
	}

	for _, v := range latestPathMap {
		current, ok := c.PositionMap[v.addr]
		if !ok || newerVersions(v.Ver, current.Ts) {
			c.PositionMap[v.addr] = MvpPositionMapEntry{Slot: v.to, Ts: v.Ver}
		}
	}
}

func (c *MvpClient) selectPath(accessPosition MvpPosition, pathlen int) MvpPosition {
	if accessPosition == mvpStashPosition || accessPosition.bucket == mvpRootBucketPosition {
		accessPosition = c.randomPositionMapSlot()
	}

	return selectPath(accessPosition, pathlen)
}

func (c *MvpClient) randomPositionMapSlot() MvpPosition {
	if len(c.PositionMap) == 0 {
		return MvpPosition{}
	}

	fakeTargetAddr := rand.Intn(len(c.PositionMap))
	if entry, ok := c.PositionMap[fakeTargetAddr]; ok {
		return entry.Slot
	}

	index := rand.Intn(len(c.PositionMap))
	for _, entry := range c.PositionMap {
		if index == 0 {
			return entry.Slot
		}
		index--
	}

	return MvpPosition{}
}

func selectPath(accessPosition MvpPosition, pathlen int) MvpPosition {
	if accessPosition == mvpStashPosition || accessPosition.bucket == mvpRootBucketPosition {
		accessPosition = MvpPosition{}
	}

	leaf := accessPosition.String()
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

func (c *MvpClient) mergePathStashes() map[int]MvpDataBlock {
	W := make(map[int]MvpDataBlock, 0)

	for _, v := range c.Stash {
		for _, block := range v {
			pm, ok := c.PositionMap[block.Addr]
			if ok && pm.Slot == mvpStashPosition && pm.Ts == block.Version {
				W[block.Addr] = block
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
				pm, ok := c.PositionMap[block.Addr]
				blockPosition := MvpPosition{bucket: bucketPosition, slot: slotPosition}
				if ok && pm.Slot == blockPosition && pm.Ts == block.Version {
					W[block.Addr] = block
				}
			}
		}
	}

	return W
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

func sort_block(blockList []MvpDataBlock) []MvpDataBlock {
	sort.Slice(blockList, func(i, j int) bool {
		left := blockList[i].Version
		right := blockList[j].Version

		if left.V != right.V {
			return left.V < right.V
		}
		if left.A != right.A {
			return left.A < right.A
		}
		return left.S < right.S
	})

	return blockList
}

func sort_position(positionList []MvpPosition) []MvpPosition {
	bucketDepth := func(position MvpPosition) int {
		switch position.bucket {
		case mvpRootBucketPosition:
			return 0
		case mvpStashBucketPosition:
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

func (c *MvpClient) populatePath(W map[int]MvpDataBlock, op OramOP) (map[MvpPosition]MvpSlot, []MvpDataBlock, []path) {
	populatedPath := make(map[MvpPosition]MvpSlot, c.L+1)
	populatedStash := make([]MvpDataBlock, 0, 20)
	populatedPathMap := make([]path, 0, len(W))
	targetPosition := c.PositionMap[op.target].Slot

	used_slot := make([]MvpPosition, 0, c.L*4)
	for position := range c.path {
		for i := 0; i < c.Z; i++ {
			mp := MvpPosition{position, MvpSlotPosition(i)}

			candinates := make([]MvpDataBlock, 0, len(W))
			for _, block := range W {
				if c.PositionMap[block.Addr].Slot == mp {
					candinates = append(candinates, block)
				}
			}

			if len(candinates) == 0 {
				populatedPath[mp] = NewMvpSlot(c.seq)
				continue
			}

			candinates = sort_block(candinates)

			selected := candinates[len(candinates)-1]

			slot := NewMvpSlot(c.seq)
			slot.SetBlock(selected)

			populatedPath[mp] = slot
			used_slot = append(used_slot, mp)

			delete(W, selected.Addr)
		}
	}

	blockList := make([]MvpDataBlock, 0, len(W))
	for _, block := range W {
		if block.Addr == op.target {
			continue
		}
		blockList = append(blockList, block)
	}

	swap_cadinate := make([]MvpDataBlock, 0, c.Z)
	if len(blockList) <= c.Z {
		swap_cadinate = append(swap_cadinate, blockList...)
	} else {
		for _, index := range rand.Perm(len(blockList))[:c.Z] {
			swap_cadinate = append(swap_cadinate, blockList[index])
		}
	}

	swap_slot := make([]MvpPosition, 0, c.Z)
	swapSlotSet := make(map[MvpPosition]bool, c.Z)
	if targetPosition != mvpStashPosition {
		if _, ok := populatedPath[targetPosition]; ok {
			swap_slot = append(swap_slot, targetPosition)
			swapSlotSet[targetPosition] = true
		}
	}
	for _, index := range rand.Perm(len(used_slot)) {
		if len(swap_slot) >= c.Z {
			break
		}

		position := used_slot[index]
		if swapSlotSet[position] {
			continue
		}
		swap_slot = append(swap_slot, position)
		swapSlotSet[position] = true
	}

	for index, slot_position := range swap_slot {
		slot := populatedPath[slot_position]

		newslot := NewMvpSlot(c.seq)
		if len(swap_cadinate) > index {
			newslot.SetBlock(swap_cadinate[index])
			delete(W, swap_cadinate[index].Addr)
		}

		if !slot.IsEmpty() {
			swaped := slot.Value
			W[swaped.Addr] = swaped
		}

		populatedPath[slot_position] = newslot
	}

	onPathBlock := make([]MvpDataBlock, 0)
	onPathPosition := make([]MvpPosition, 0, len(used_slot))

	for _, position := range used_slot {
		slot := populatedPath[position]
		if slot.IsEmpty() {
			continue
		}
		onPathBlock = append(onPathBlock, slot.Value)
		onPathPosition = append(onPathPosition, position)
	}

	onPathBlock = sort_block(onPathBlock)
	onPathPosition = sort_position(onPathPosition)

	for index, position := range onPathPosition {
		slot := NewMvpSlot(c.seq)
		block := onPathBlock[index]
		block.Version.SetS(c.seq)
		slot.SetBlock(block)
		populatedPath[position] = slot

		path := newPath(block.Addr, position, block.Version, c.seq)
		populatedPathMap = append(populatedPathMap, path)
	}

	for _, block := range W {
		block.Version.SetS(c.seq)
		populatedStash = append(populatedStash, block)
		path := newPath(block.Addr, mvpStashPosition, block.Version, c.seq)
		populatedPathMap = append(populatedPathMap, path)
	}

	return populatedPath, populatedStash, populatedPathMap
}
