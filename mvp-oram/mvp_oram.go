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
	mvpRootBucketPosition  MvpBucketPosition = "root"
	mvpStashBucketPosition MvpBucketPosition = "-1"
	mvpStashSlotPosition   MvpSlotPosition   = -1
	mvpInitialVersion      Version           = 0
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

type path struct {
	addr int
	from MvpPosition
	to   MvpPosition
	Ver  Version
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
	PathMap  []path
	Stash    map[int][]MvpDataBlock
	Path     map[MvpBucketPosition]MvpBucket
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

type MvpServer struct {
	PositionMaps  []map[int]MvpPosition
	PathMaps      []path
	Stashs        map[int][]MvpDataBlock
	tree          MvpTree
	counter       Version
	Snapshot      map[int]OramState
	Accesshistory map[int]Version

	Requests chan ServerRequest
}

func NewMvpServer(z int, l int) *MvpServer {
	return &MvpServer{
		PositionMaps:  make([]map[int]MvpPosition, 0),
		PathMaps:      make([]path, 0),
		Stashs:        make(map[int][]MvpDataBlock, 0),
		tree:          NewMvpTree(z, l),
		Requests:      make(chan ServerRequest),
		counter:       mvpInitialVersion,
		Snapshot:      make(map[int]OramState, 50),
		Accesshistory: make(map[int]Version, 50),
	}
}

func (s *MvpServer) InitializeRandomData(n int, seed int64) map[int]MvpPosition {
	rng := rand.New(rand.NewSource(seed))
	positions := make([]MvpBucketPosition, 0, len(s.tree.Tree)) //木に存在するポジション一覧生成
	for position := range s.tree.Tree {
		positions = append(positions, position)
	}

	sort.Slice(positions, func(i, j int) bool {
		return positions[i] < positions[j]
	}) //小さい順にソート

	positionMap := make(map[int]MvpPosition, n)
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
			positionMap[addr] = MvpPosition{
				bucket: position,
				slot:   slotPosition,
			}
			continue
		}

		stash = append(stash, block) //溢れたらスタッシュに移動
		positionMap[addr] = mvpStashPosition
	}

	s.PositionMaps = append(s.PositionMaps, positionMap)
	s.Stashs[0] = stash

	return positionMap

}

func (s *MvpServer) Run() {
	log.Println("Serve is running")

	//log.Println(s.PositionMaps)
	//log.Println(s.Stashs)

	for req := range s.Requests {
		req.handle(s)
	}
}

func (r GetpmRequest) handle(s *MvpServer) {
	s.counter.increment()
	seq := s.counter

	v, ok := s.Accesshistory[r.ClientID]
	lastVersion := Version(0)
	if ok {
		lastVersion = v
	}

	difpathMap := make([]path, 0, len(s.PathMaps))
	for _, v := range s.PathMaps {
		if v.Ver > lastVersion {
			difpathMap = append(difpathMap, v)
		}
	}

	s.Snapshot[r.ClientID] = OramState{
		TreeState:  s.tree.Clone(),
		StashState: cloneStashs(s.Stashs),
	}
	s.Accesshistory[r.ClientID] = seq

	r.Reply <- GetpmResponse{
		Seq:     seq,
		PathMap: difpathMap,
		Err:     nil,
	}
}

func (r GetpsRequest) handle(s *MvpServer) {
	oramstate, ok := s.Snapshot[r.ClientID]
	if !ok {
		r.Reply <- GetpsResponse{
			Err: fmt.Errorf("snapshot for client %d not found", r.ClientID),
		}
		return
	}

	path := make(map[MvpBucketPosition]MvpBucket, s.tree.L+1)
	path[mvpRootBucketPosition] = oramstate.TreeState.Tree[mvpRootBucketPosition]

	leaf := r.Leaf.bucket.String()
	for len(leaf) != 0 {
		bucketPosition := MvpBucketPosition(leaf)
		path[bucketPosition] = oramstate.TreeState.Tree[bucketPosition]
		leaf = leaf[:len(leaf)-1]
	}

	r.Reply <- GetpsResponse{
		Path:  path,
		Stash: oramstate.StashState,
		Err:   nil,
	}
}

func (r EvictReques) handle(s *MvpServer) {
	for position, buckets := range r.Path {
		s.tree.Tree[position] = buckets
	}

	s.Stashs = cloneStashs(r.Stash)
	s.PathMaps = append(s.PathMaps, r.PathMap...)

	r.Reply <- EvictResponse{}
}

type MvpClient struct {
	L           int
	Z           int
	PositionMap map[int]MvpPosition
	Stash       map[int][]MvpDataBlock
	path        map[MvpBucketPosition]MvpBucket

	ClientID int
	Server   chan<- ServerRequest
}

func NewMvpClient(l int, z int, clientID int, positionmap map[int]MvpPosition, server chan<- ServerRequest) *MvpClient {
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

func (c *MvpClient) Evict(path map[MvpBucketPosition]MvpBucket, pathmap []path, stash map[int][]MvpDataBlock) error {
	reply := make(chan EvictResponse)

	c.Server <- EvictReques{
		ClientID: c.ClientID,
		Path:     path,
		PathMap:  pathmap,
		Stash:    stash,
		Reply:    reply,
	}

	res := <-reply
	return res.Err

}

func (c *MvpClient) Run() error {
	log.Println("Client is running")

	for {

		err := c.Access(OramOP{Write, 100, "hoge"})
		if err != nil {
			panic(err)
		}

		break
	}

	return nil
}

func (c *MvpClient) Access(op OramOP) error {
	version, pathMaps, err := c.GetPM() //Getpm操作
	if err != nil {
		return err
	}

	c.consolidatePathMaps(pathMaps) //position mapの更新

	log.Println("version is ", version)
	log.Println("Access block's leaf is ", c.PositionMap[op.target])

	accessPosition := c.PositionMap[op.target]

	log.Println(selectPath(accessPosition, c.L))

	c.path, c.Stash, err = c.GetPS(selectPath(accessPosition, c.L)) //Getps操作
	if err != nil {
		return err
	}

	log.Println(c.path)
	log.Println(c.Stash)

	W := c.mergePathStashes() //ワーキングセット制作

	log.Println(W)

	targetBlock, ok := W[op.target]
	if !ok {
		return fmt.Errorf("Not target block in working set")
	}

	log.Println(targetBlock)
	if op.OP == Write {
		targetBlock.Data = op.param

		targetBlock.Version = Versions{version, version, version}
	} else {
		targetBlock.Version.SetA(version)
		targetBlock.Version.SetS(version)
	}
	W[op.target] = targetBlock
	log.Println(targetBlock)

	return nil
}

func (c *MvpClient) consolidatePathMaps(pathMaps []path) {
	for _, v := range pathMaps {
		if c.PositionMap[v.addr] == v.from {
			c.PositionMap[v.addr] = v.to
		}
	}
}

func selectPath(accessPosition MvpPosition, pathlen int) MvpPosition {
	if accessPosition == mvpStashPosition || accessPosition == mvpRootPosition {
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
			if mvpStashPosition == c.PositionMap[block.Addr] {
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
				if c.PositionMap[block.Addr] == (MvpPosition{bucket: bucketPosition, slot: slotPosition}) {
					W[block.Addr] = block
				}
			}
		}
	}

	return W
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

func (c *MvpClient) populatePath(W map[int]MvpDataBlock, version Version) (map[MvpPosition]MvpBucket, map[int][]MvpDataBlock, []path) {
	populatedPath := make(map[MvpPosition]MvpBucket, c.L+1)
	populatedStash := make(map[int][]MvpDataBlock, 20)
	populatedPathMap := make([]path, 0, len(W))

	return populatedPath, populatedStash, populatedPathMap
}

func main() {
	const (
		z    = 4
		l    = 8
		n    = 256
		seed = 542
	)

	server := NewMvpServer(z, l)
	positionmap := server.InitializeRandomData(n, seed)

	go server.Run()

	client := NewMvpClient(
		l,
		z,
		0,
		positionmap,
		server.Requests,
	)

	if err := client.Run(); err != nil {
		panic(err)
	}

	close(server.Requests)
}
