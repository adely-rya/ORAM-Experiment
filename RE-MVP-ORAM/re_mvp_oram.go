package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
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
	V Version // 最後の書き込み or 削除バージョン
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
	Addr      int
	signature int
	Data      string
	Version   Versions
}

func (d MvpDataBlock) generateSig() {
	d.signature = rand.Int()
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
	addr   int
	sig    int
	to     MvpPosition
	delete bool
	Ver    Versions
	Seq    Version
}

func newPath(addr int, sig int, to any, ver Versions, seq Version) path {
	return path{
		addr:   addr,
		sig:    sig,
		to:     pathPosition(to),
		delete: false,
		Ver:    ver,
		Seq:    seq,
	}
}

func newDeletePath(addr int, sig int, ver Versions, seq Version) path {
	return path{
		addr:   addr,
		sig:    sig,
		delete: true,
		Ver:    ver,
		Seq:    seq,
	}
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

func clonePositionMap(positionMap map[int]MvpPositionMapEntry) map[int]MvpPositionMapEntry {
	cloned := make(map[int]MvpPositionMapEntry, len(positionMap))
	for addr, entry := range positionMap {
		cloned[addr] = entry
	}

	return cloned
}

type MvpServer struct {
	PositionMap                  map[int]map[int]MvpPositionMapEntry
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
		PositionMap:   make(map[int]map[int]MvpPositionMapEntry, 0),
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
			Addr: addr,
			Data: strconv.Itoa(addr),
			Version: Versions{
				V: mvpInitialVersion,
				A: mvpInitialVersion,
				S: mvpInitialVersion,
			},
		}
		block.generateSig()
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

	s.PositionMap = positionMap
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
	if !s.useSnapshot {
		panic("Evict requires snapshots; snapshot-off experiments must stop after GetPM/GetPS")
	}

	nowtree := s.tree.Tree
	var oldtree map[MvpBucketPosition]MvpBucket
	var oldstash map[int][]MvpDataBlock

	oldstate := s.Snapshot[r.ClientID]
	oldtree = oldstate.TreeState.Tree
	oldstash = oldstate.StashState
	delete(s.Snapshot, r.ClientID)

	for position, newslot := range r.Path {
		slots := nowtree[position.bucket].Slots[position.slot]
		oldslots := oldtree[position.bucket].Slots[position.slot]
		for version := range oldslots {
			_, ok := slots[version]
			if ok {
				delete(slots, version)
			}

		}
		slots[newslot.Version] = newslot
	}

	for version := range oldstash {
		_, ok := s.Stashs[version]
		if ok {
			delete(s.Stashs, version)
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
	PositionMap map[int]map[int]MvpPositionMapEntry
	Stash       map[int][]MvpDataBlock
	path        map[MvpBucketPosition]MvpBucket

	ClientID int
	Server   chan<- ServerRequest

	seq Version
}

func NewMvpClient(l int, z int, clientID int, positionmap map[int]map[int]MvpPositionMapEntry, server chan<- ServerRequest) *MvpClient {
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
	for {
		target := rand.Intn(addrCount)
		param := fmt.Sprintf("client-%d-%d", c.ClientID, target)

		err := c.Access(OramOP{Write, target, param})
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

	accesscadinate := make([]MvpPositionMapEntry, 0, 10)
	for _, position := range c.PositionMap[op.target] {
		accesscadinate = append(accesscadinate, position)
	}
	accessPosition := accesscadinate[rand.Intn(len(accesscadinate)-1)].Slot

	log.Printf("access start: client=%d seq=%d op=%s addr=%d position=%v", c.ClientID, version, op.OP, op.target, accessPosition)

	c.path, c.Stash, err = c.GetPS(c.selectPath(accessPosition, c.L)) //Getps操作
	if err != nil {
		return err
	}

	W := c.mergePathStashes() //ワーキングセット制作

	if len(W[op.target]) == 0 {
		return fmt.Errorf("Not target block in working set")
	}
	targetBlock := W[op.target][0]

	if op.OP == Write {
		targetBlock.Data = op.param

		targetBlock.Version = Versions{version, version, version}
	} else {
		targetBlock.Version.SetA(version)
	}
	W[op.target][0] = targetBlock

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
		latest, ok := latestPathMap[v.sig]
		if !ok || newerPathUpdate(v, latest) {
			latestPathMap[v.sig] = v
		}
	}

	for _, v := range latestPathMap {
		current, ok := c.PositionMap[v.addr][v.sig]
		if v.delete {
			delete(c.PositionMap[v.addr], v.sig)
			continue
		}
		if !ok || newerPositionUpdate(v, current) {
			c.PositionMap[v.addr][v.sig] = MvpPositionMapEntry{Slot: v.to, Ts: v.Ver}
		}
	}
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
				W[block.Addr] = append(W[block.Addr], block)
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
					W[block.Addr] = append(W[block.Addr], block)
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

func sameDataVersion(left Versions, right Versions) bool {
	return left.V == right.V && left.A == right.A
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
			return left.V < right.V
		}
		if left.A != right.A {
			return left.A < right.A
		}
		return false
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

func (c *MvpClient) populatePath(W map[int][]MvpDataBlock, op OramOP) (map[MvpPosition]MvpSlot, []MvpDataBlock, []path) {
	populatedPath := make(map[MvpPosition]MvpSlot, c.L+1)
	populatedStash := make([]MvpDataBlock, 0, 40)
	populatedPathMap := make([]path, 0, len(W)*2)

	// アクセス対象の現在位置はslot単位で読む。
	targetPosition := c.PositionMap[op.target][W[op.target][0].signature].Slot
	// 読み込んだpathに含まれる全slotを、stashからpathへ戻す置換候補として記録する。
	allSlot := make([]MvpPosition, 0, (c.L+1)*c.Z)
	// 初回配置で実際にブロックを置いたslotだけを、後段のshuffle対象として記録する。
	usedSlot := make([]MvpPosition, 0, c.L*c.Z)
	// アクセス対象ブロックが初回配置されたslotを、後で必ずstashへ退避するために覚える。
	var targetPlacedPosition MvpPosition
	// アクセス対象ブロックが初回配置されたかどうかを記録する。
	targetPlaced := false

	//writeの時はmwからターゲットアドレスのリストにおいてインデックス０（新しく読み書きしたもの）以外を消す

	if op.OP == Write {
		W[op.target] = W[op.target][:1]

		for sig, _ := range c.PositionMap[op.target] {
			if sig == W[op.target][0].signature {
				continue
			}
			path := newDeletePath(op.target, sig, Versions{V: c.seq, A: c.seq, S: c.seq}, c.seq)
			delete(c.PositionMap[op.target], sig)
			populatedPathMap = append(populatedPathMap, path)
		}
	}

	sinmpleW := make(map[int]MvpDataBlock, 0)

	// 読み込んだpath上の各bucketを走査する。
	for bucketPosition := range c.path {
		// bucket内の各slotを順番に埋める。
		for i := 0; i < c.Z; i++ {
			// このslotに書くための実位置を作る。
			position := MvpPosition{bucket: bucketPosition, slot: MvpSlotPosition(i)}
			// このslotはpath上に存在するので、後段の置換候補へ追加する。
			allSlot = append(allSlot, position)
			// このbucketに所属するブロック候補をWから集める。
			candidates := make([]MvpDataBlock, 0, len(W))
			// W内の各ブロックを確認する。
			for _, block := range W {
				// PositionMapがこのexact slotを指すブロックだけを候補にする。
				if c.PositionMap[block.Addr].Slot == position {
					candidates = append(candidates, block)
				}
			}
			// 候補がなければ、このslotは空slotとして出力する。
			if len(candidates) == 0 {
				populatedPath[position] = NewMvpSlot(c.seq)
				continue
			}
			// 同じbucketに複数候補がある場合はversion順に並べる。
			candidates = sort_block(candidates)
			// 一番新しいversionのブロックをこのslotに置く。
			selected := candidates[len(candidates)-1]
			// 書き込み用の新しいslotを作る。
			slot := NewMvpSlot(c.seq)
			// 選んだブロックをslotへ入れる。
			slot.SetBlock(selected)
			// 実際にこのslotをpath出力へ登録する。
			populatedPath[position] = slot
			// このslotは後段のshuffleで使えるslotとして覚える。
			usedSlot = append(usedSlot, position)
			// アクセス対象ブロックを置いたslotなら、その実slotを覚える。
			if selected.Addr == op.target {
				targetPlacedPosition = position
				targetPlaced = true
			}
			// 選んだブロックはWから取り除き、同じブロックを複数slotへ置かないようにする。
			delete(W, selected.Addr)
		}
	}

	// stashからpathへ戻す候補を作る。
	blockList := make([]MvpDataBlock, 0, len(W))
	// Wに残ったブロックを走査する。
	for _, block := range W {
		// アクセス対象ブロックはstashへ退避するので、pathへ戻す候補から外す。
		if block.Addr == op.target {
			continue
		}
		// アクセス対象以外の残りブロックをswap候補へ入れる。
		blockList = append(blockList, block)
	}

	// 実際にpathへ戻すブロックを最大Z個まで選ぶ。
	swapCandidate := make([]MvpDataBlock, 0, c.Z)
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
	swapSlot := make([]MvpPosition, 0, c.Z)
	// 同じslotを二重に選ばないための集合を作る。
	swapSlotSet := make(map[MvpPosition]bool, c.Z)
	// アクセス対象が初回配置された場合は、そのslotを必ずswap対象にする。
	if targetPlaced && targetPosition != mvpStashPosition {
		swapSlot = append(swapSlot, targetPlacedPosition)
		swapSlotSet[targetPlacedPosition] = true
	}
	// path全体のslotの中から、残りのswap slotをランダムに選ぶ。
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
		newslot := NewMvpSlot(c.seq)
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
		// swap後のslotをpath出力へ反映する。
		populatedPath[slotPosition] = newslot
	}

	// path上に残ったブロックを集め直す。
	onPathBlock := make([]MvpDataBlock, 0)
	// path上に残ったブロックの実slotを集め直す。
	onPathPosition := make([]MvpPosition, 0, len(allSlot))
	// path全体を再配置対象として見る。
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
	onPathPosition = sort_position(onPathPosition)

	// 並べ替え後のブロックをslotへ書き戻す。
	for index, position := range onPathPosition {
		// 新しいslot versionでslotを作る。
		slot := NewMvpSlot(c.seq)
		// root側から順に置くブロックを取り出す。
		block := onPathBlock[index]
		// ブロックをslotへ入れる。
		slot.SetBlock(block)
		// path出力へslotを書き戻す。
		populatedPath[position] = slot
		// PositionMap更新はslot単位で記録する。
		path := newPath(block.Addr, position, positionMapVersion(block.Version, c.seq), c.seq)
		// PathMapへslot位置の更新を追加する。
		populatedPathMap = append(populatedPathMap, path)
	}

	// pathへ残らなかったブロックをstashへ入れる。
	for _, block := range W {
		// 新しいstash出力へブロックを追加する。
		populatedStash = append(populatedStash, block)
		if c.PositionMap[block.Addr].Slot == mvpStashPosition && sameDataVersion(c.PositionMap[block.Addr].Ts, block.Version) {
			continue
		}
		// PositionMap更新はstash位置として記録する。
		path := newPath(block.Addr, mvpStashPosition, positionMapVersion(block.Version, c.seq), c.seq)
		// PathMapへstash位置の更新を追加する。
		populatedPathMap = append(populatedPathMap, path)
	}

	// 新しいpath、新しいstash、新しいPathMapを返す。
	return populatedPath, populatedStash, populatedPathMap
}
