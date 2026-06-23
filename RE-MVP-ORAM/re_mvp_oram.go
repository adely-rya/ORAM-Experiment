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

	mvpDefaultMaxSignature                = 10
	mvpDefaultPathMapCompactInterval      = 10000
	mvpDefaultPathMapCompactProtectedTail = 2000
	mvpDefaultSwapLimit                   = 8
)

var (
	mvpStashPosition  = MvpPosition{bucket: mvpStashBucketPosition, slot: mvpStashSlotPosition} //スタッシュポジション
	mvpDeletePosition = MvpPosition{bucket: mvpDeleteTag}                                       //墓石
	mvpMaxSignature   = mvpDefaultMaxSignature
)

func SetMvpMaxSignature(maxSignature int) {
	if maxSignature < 0 {
		maxSignature = 0
	}
	mvpMaxSignature = maxSignature
}

func ConfigureMvpMaxSignatureFromEnv() {
	value := os.Getenv("RE_MVP_MAX_SIGNATURE")
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("invalid RE_MVP_MAX_SIGNATURE=%q; using %d", value, mvpMaxSignature)
		return
	}
	SetMvpMaxSignature(parsed)
}

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

func NewMvpBucket(z int) MvpBucket { //指定された個数分スロットを作る
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
	Z    int
	L    int
	Tree map[MvpBucketPosition]MvpBucket
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

func NewMvpTree(z int, l int) MvpTree { //すべてが空のスロットのツリーを制作
	tree := make(map[MvpBucketPosition]MvpBucket, 1<<(l+1)-1)

	for level := 0; level <= l; level++ {
		for index := 0; index < 1<<level; index++ {
			position := NewMvpBucketPosition(level, index)

			tree[position] = NewMvpBucket(z)
		}
	}

	return MvpTree{
		Z:    z,
		L:    l,
		Tree: tree,
	}
}

func (t MvpTree) Clone() MvpTree {
	tree := make(map[MvpBucketPosition]MvpBucket, len(t.Tree))
	for position, bucket := range t.Tree {
		tree[position] = bucket.Clone()
	}

	return MvpTree{
		Z:    t.Z,
		L:    t.L,
		Tree: tree,
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

func newPath(addr int, sig int, to MvpPosition, ver Versions, seq Version) path {
	return path{
		addr: addr,
		sig:  sig,
		to:   to,
		Ver:  ver,
		Seq:  seq,
	}
}

func appendPositionMapUpdate(pathMaps *[]path, update path) { //同じスロットに対する古いappendは消す
	compacted := (*pathMaps)[:0]
	for _, existing := range *pathMaps {
		if existing.addr == update.addr && existing.sig == update.sig {
			continue
		}
		compacted = append(compacted, existing)
	}
	*pathMaps = append(compacted, update)
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

type GetStashMetricsRequest struct {
	Reply chan GetStashMetricsResponse
}

type GetStashMetricsResponse struct {
	Seq                 Version
	StashVersions       int
	StashTotal          int
	StashMaxVersionSize int
	Err                 error
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

// 全てのスロットがdelete状態になっているPSのエントリーを生成
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
	PathMapCursor                map[int]int //ドのクライアントがどのpathまで読み込んだのかを記録
	PathMapCompactInterval       int
	PathMapCompactProtectedTail  int
	pathMapEvictsSinceCompaction int

	Requests chan ServerRequest
}

// Server生成
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

// スナップショットオフ版
func NewNoSnapshotMvpServer(z int, l int) *MvpServer {
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

	for addr := 0; addr < n; addr++ { //sig0にブロックを置く
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
	s.counter.increment() //サーバーが発行するシーケンシャル番号をインクリメント
	seq := s.counter

	if s.useSnapshot { //スナップショット保存
		s.Snapshot[r.ClientID] = OramState{
			TreeState:  s.tree.Clone(),
			StashState: cloneStashs(s.Stashs),
		}
	}

	cursor := s.PathMapCursor[r.ClientID]

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
				delete(slots, version) //読み取り時点で存在していたスロットは消す
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

func (r GetStashMetricsRequest) handle(s *MvpServer) {
	totalStashBlocks := 0
	maxStashVersionBlocks := 0
	for _, stash := range s.Stashs {
		totalStashBlocks += len(stash)
		if len(stash) > maxStashVersionBlocks {
			maxStashVersionBlocks = len(stash)
		}
	}

	r.Reply <- GetStashMetricsResponse{
		Seq:                 s.counter,
		StashVersions:       len(s.Stashs),
		StashTotal:          totalStashBlocks,
		StashMaxVersionSize: maxStashVersionBlocks,
		Err:                 nil,
	}
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

func (c *MvpClient) Access(op OramOP) error {
	version, pathMaps, err := c.GetPM() //Getpm操作
	c.seq = version
	if err != nil {
		return err
	}

	c.consolidatePathMaps(pathMaps) //position mapの更新

	accessSig, accessLeaf, ok := c.choosetargetLeaf(op.target, op.OP)

	if !ok {
		return fmt.Errorf("no position map entry for addr %d", op.target)
	}
	if accessLoggingEnabled {
		log.Printf("access start: client=%d seq=%d op=%s addr=%d sig=%d position=%v", c.ClientID, version, op.OP, op.target, accessSig, accessLeaf)
	}

	c.path, c.Stash, err = c.GetPS(accessLeaf) //Getps操作
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

	// 受信したPathMapをアドレス単位で記録しつつ、(addr, sig)ごとの最新更新だけを残す。
	for _, v := range pathMaps {
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
		if int(path.Ver.V) < maxversion[key.addr] {
			delete(latestPathMap, key)
			continue
		}
		if currentMax, ok := currentMaxVersion[key.addr]; ok && path.Ver.V < currentMax {
			delete(latestPathMap, key)
		}
	}

	// 最後に残った最新PathMap更新をPositionMapへ反映する。Deleteも固定sigの位置として上書きする。
	for _, v := range latestPathMap {
		current, _ := c.PositionMap[v.addr][v.sig]
		if newerPositionUpdate(v, current) {
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

func (c *MvpClient) choosetargetLeaf(addr int, op string) (int, MvpPosition, bool) {
	entries, ok := c.PositionMap[addr]
	if !ok || len(entries) == 0 {
		return 0, MvpPosition{}, false
	}

	if op == Write { //writeならsig0を選ぶ
		entry, ok := entries[0]
		return 0, selectPath(entry.Slot, c.L), ok
	}

	signatures := make([]int, 0, len(entries))
	for sig, position := range entries { //アクセス可能なsigを集める
		if position.Slot != mvpDeletePosition {
			signatures = append(signatures, sig) //deleteタグのブロックは無視
		}
	}
	if len(signatures) == 0 {
		return 0, MvpPosition{}, false
	}

	cadinates := make(map[MvpPosition]int, 1<<c.L)
	positions := make([]MvpPosition, 0, 1<<c.L)
	for _, sig := range signatures {
		bucketPosition := entries[sig].Slot.bucket
		if entries[sig].Slot == mvpStashPosition || bucketPosition == mvpStashBucketPosition || bucketPosition == mvpRootBucketPosition {
			bucketPosition = ""
		}

		prefix := bucketPosition.String()
		if len(prefix) > c.L {
			prefix = ""
		}

		remainingDepth := c.L - len(prefix)
		for leafIndex := 0; leafIndex < 1<<remainingDepth; leafIndex++ {
			leaf := ""
			if remainingDepth > 0 {
				leaf = strconv.FormatInt(int64(leafIndex), 2)
				for len(leaf) < remainingDepth {
					leaf = "0" + leaf
				}
			}
			position := MvpPosition{bucket: MvpBucketPosition(prefix + leaf)}
			if _, ok := cadinates[position]; ok {
				continue
			}
			cadinates[position] = sig
			positions = append(positions, position)
		}

	}
	if len(positions) == 0 {
		return 0, MvpPosition{}, false
	}

	position := positions[rand.Intn(len(positions))] //ランダムに選ぶ
	return cadinates[position], position, true
}

func randomLeafPosition(pathlen int) MvpPosition { //ランダムなパス選定
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

func selectPath(accessPosition MvpPosition, pathlen int) MvpPosition {
	if accessPosition == mvpStashPosition {
		return randomLeafPosition(pathlen) //スタッシュにあるなら完全にランダム
	}

	bucketPosition := accessPosition.bucket
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

	for _, v := range c.Stash { //スタッシュのブロックをワーキングセットに移動
		for _, block := range v {
			pm, _ := c.PositionMap[block.Addr][block.signature]
			if pm.Slot == mvpStashPosition && sameDataVersion(pm.Ts, block.Version) {
				appendWorkingBlock(W, block)
			}
		}
	}

	for bucketPosition, bucket := range c.path { //パスのブロックをワーキングセットに移動
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

func workingSetSignatures(blocks []MvpDataBlock) []int {
	signatures := make([]int, 0, len(blocks))
	for _, block := range blocks {
		signatures = append(signatures, block.signature)
	}
	sort.Ints(signatures)
	return signatures
}

func (c *MvpClient) debugRawTargetBlocks(addr int) []string {
	found := make([]string, 0)
	for stashVersion, stash := range c.Stash {
		for _, block := range stash {
			if block.Addr == addr {
				found = append(found, fmt.Sprintf("stash[%d]:sig=%d ver=%+v", stashVersion, block.signature, block.Version))
			}
		}
	}
	for bucketPosition, bucket := range c.path {
		for slotPosition, versionedSlots := range bucket.Slots {
			for version, slot := range versionedSlots {
				if slot.IsEmpty() || slot.Value.Addr != addr {
					continue
				}
				found = append(found, fmt.Sprintf("path[%s,%d,%d]:sig=%d ver=%+v", bucketPosition, slotPosition, version, slot.Value.signature, slot.Value.Version))
			}
		}
	}
	sort.Strings(found)
	return found
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
	if update.to == mvpDeletePosition {
		return update.Seq > current.Ts.S
	}
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

// buildPatternPlacementState prepares score and reusable-signature state for placing existing priority blocks.
func (c *MvpClient) buildPatternPlacementState(blocksByAddr map[int][]MvpDataBlock, pathList []path) ([]int, map[int]int, map[int][]int) {
	addrkey := make([]int, 0, len(blocksByAddr))
	// 配置候補を持つアドレスだけを評価対象として集める。
	for addr := range blocksByAddr {
		addrkey = append(addrkey, addr)
	}

	evaluationResult := c.evaluationPathpattern(addrkey, pathList)
	virtualPositionMap := clonePositionMap(c.PositionMap)
	applyPathMapsToPositionMap(virtualPositionMap, pathList)

	addrVSemptysig := make(map[int][]int, len(blocksByAddr))
	// 各アドレスの position map を見て、Delete 済みで再利用できる signature slot を集める。
	for _, addr := range addrkey {
		// そのアドレスに固定生成されている signature slot 0..N のうち、空いているものだけを残す。
		for sig, position := range virtualPositionMap[addr] {
			if position.Slot == mvpDeletePosition {
				addrVSemptysig[addr] = append(addrVSemptysig[addr], sig)
			}
		}
	}

	return addrkey, evaluationResult, addrVSemptysig
}

// buildCopySourceByAddr picks one newest visible block per address as the source for future copy blocks.
func buildCopySourceByAddr(W map[int][]MvpDataBlock) map[int]MvpDataBlock {
	copySourceByAddr := make(map[int]MvpDataBlock, len(W))
	for addr, blocks := range W {
		for _, block := range blocks {
			current, ok := copySourceByAddr[addr]
			if !ok || newerVersions(block.Version, current.Version) {
				copySourceByAddr[addr] = block
			}
		}
	}
	return copySourceByAddr
}

// buildCopyPlacementState computes address scores and reusable signatures for generated copy placement.
func (c *MvpClient) buildCopyPlacementState(copySourceByAddr map[int]MvpDataBlock, pathList []path) ([]int, map[int]int, map[int][]int) {
	addrkey := make([]int, 0, len(copySourceByAddr))
	for addr := range copySourceByAddr {
		addrkey = append(addrkey, addr)
	}

	evaluationResult := c.evaluationPathpattern(addrkey, pathList)
	virtualPositionMap := clonePositionMap(c.PositionMap)
	applyPathMapsToPositionMap(virtualPositionMap, pathList)

	addrVSemptysig := make(map[int][]int, len(copySourceByAddr))
	for _, addr := range addrkey {
		for sig, position := range virtualPositionMap[addr] {
			if position.Slot == mvpDeletePosition {
				addrVSemptysig[addr] = append(addrVSemptysig[addr], sig)
			}
		}
		sort.Ints(addrVSemptysig[addr])
	}

	return addrkey, evaluationResult, addrVSemptysig
}

// placePriorityBlocks places leftover priority blocks into empty path slots without changing signatures.
func (c *MvpClient) placePriorityBlocks(
	blocksByAddr map[int][]MvpDataBlock,
	unusedSlot []MvpPosition,
	populatedPath map[MvpPosition]MvpSlot,
	populatedPathMap *[]path,
) ([]MvpPosition, int) {
	if len(blocksByAddr) == 0 || len(unusedSlot) == 0 {
		return unusedSlot, 0
	}

	addrkey, evaluationResult, _ := c.buildPatternPlacementState(blocksByAddr, *populatedPathMap)
	if len(addrkey) == 0 {
		return unusedSlot, 0
	}

	index := len(addrkey)
	placed := 0
	remainingUnused := make([]MvpPosition, 0, len(unusedSlot))

	// unusedSlot を前から埋め、置けなかった残り slot は remainingUnused として次段に返す。
	for unusedIndex, position := range unusedSlot {
		if len(blocksByAddr) == 0 {
			remainingUnused = append(remainingUnused, unusedSlot[unusedIndex:]...)
			break
		}
		if index == len(addrkey) { //必ず一周回ったらソートする。
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

		var (
			addr       int
			foundBlock bool
		)

		// 評価値の小さいアドレスから順に、残った priority block を空き slot へ置く。
		for attempts := 0; attempts < len(addrkey); attempts++ {
			addr = addrkey[index]

			if len(blocksByAddr[addr]) == 0 {
				index++
				if index == len(addrkey) {
					index = 0
				}
				continue
			}

			block := blocksByAddr[addr][0]
			blocksByAddr[addr] = blocksByAddr[addr][1:]
			if len(blocksByAddr[addr]) == 0 {
				delete(blocksByAddr, addr)
			}

			block.Version.SetS(c.seq)
			slot := NewMvpSlot(c.seq)
			slot.SetBlock(block)
			populatedPath[position] = slot

			update := newPath(block.Addr, block.signature, position, positionMapVersion(block.Version, c.seq), c.seq)
			appendPositionMapUpdate(populatedPathMap, update)

			evaluationResult[addr] += pathPatternCount(position, c.L)
			placed++
			index++
			foundBlock = true
			break
		}

		if !foundBlock {
			remainingUnused = append(remainingUnused, unusedSlot[unusedIndex:]...)
			break
		}
	}

	return remainingUnused, placed
}

// placeCopyBlocks fills remaining empty path slots with newly generated copies using deleted signatures.
func (c *MvpClient) placeCopyBlocks(
	copySourceByAddr map[int]MvpDataBlock,
	unusedSlot []MvpPosition,
	populatedPath map[MvpPosition]MvpSlot,
	populatedPathMap *[]path,
) []MvpPosition {
	if len(copySourceByAddr) == 0 || len(unusedSlot) == 0 {
		return unusedSlot
	}

	addrkey, evaluationResult, addrVSemptysig := c.buildCopyPlacementState(copySourceByAddr, *populatedPathMap)
	if len(addrkey) == 0 {
		return unusedSlot
	}

	index := len(addrkey)
	remainingUnused := make([]MvpPosition, 0, len(unusedSlot))

	// unusedSlot を前から埋め、addrkey を一周するたびに評価値の小さい順へ並び替える。
	for unusedIndex, position := range unusedSlot {
		if index == len(addrkey) {
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

		placed := false
		for attempts := 0; attempts < len(addrkey); attempts++ {
			addr := addrkey[index]
			if len(addrVSemptysig[addr]) == 0 {
				index++
				if index == len(addrkey) {
					index = 0
				}
				continue
			}

			signature := addrVSemptysig[addr][0]
			addrVSemptysig[addr] = addrVSemptysig[addr][1:]

			block := copySourceByAddr[addr]
			block.signature = signature
			block.Version.SetS(c.seq)

			slot := NewMvpSlot(c.seq)
			slot.SetBlock(block)
			populatedPath[position] = slot

			update := newPath(block.Addr, block.signature, position, positionMapVersion(block.Version, c.seq), c.seq)
			appendPositionMapUpdate(populatedPathMap, update)

			evaluationResult[addr] += pathPatternCount(position, c.L)
			index++
			placed = true
			break
		}

		if !placed {
			remainingUnused = append(remainingUnused, unusedSlot[unusedIndex:]...)
			break
		}
	}

	return remainingUnused
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

// populatePath writes priority blocks first, then creates fresh copy blocks from one source block per address.
func (c *MvpClient) populatePath(W map[int][]MvpDataBlock, op OramOP, targetSig int) (map[MvpPosition]MvpSlot, []MvpDataBlock, []path) {

	// Phase 1: prepare the path, stash, and position-map update outputs for this eviction.
	populatedPath := make(map[MvpPosition]MvpSlot, c.L+1)
	populatedStash := make([]MvpDataBlock, 0, 40)
	populatedPathMap := make([]path, 0, len(W)*2)

	// Phase 2: remember empty path slots that can later receive drained/copy blocks.
	unusedSlot := make([]MvpPosition, 0, c.L*c.Z)
	usedSlot := make([]MvpPosition, 0, c.L*c.Z)

	prioritylist := make([]MvpDataBlock, 0)

	// Phase 3: locate and update the accessed target block before splitting W.

	targetBlock := MvpDataBlock{}
	blockshere := false
	for _, block := range W[op.target] {
		if block.signature == targetSig {
			targetBlock = block
			blockshere = true
			break
		}
	}

	if blockshere {
		targetBlock.Version.SetA(c.seq) //バージョンAを更新
		setWorkingBlock(W, targetBlock)
	} else {
		entry, ok := c.PositionMap[op.target][targetSig]
		if !ok {
			log.Panicf("No target block in Workingset: client=%d seq=%d op=%s addr=%d sig=%d pm=missing w_sigs=%v", c.ClientID, c.seq, op.OP, op.target, targetSig, workingSetSignatures(W[op.target]))
		}
		log.Panicf("No target block in Workingset: client=%d seq=%d op=%s addr=%d sig=%d pm_slot=%v pm_ts=%+v w_sigs=%v raw=%v", c.ClientID, c.seq, op.OP, op.target, targetSig, entry.Slot, entry.Ts, workingSetSignatures(W[op.target]), c.debugRawTargetBlocks(op.target))
	}

	if op.OP == Write {
		targetBlock.Data = op.param
		targetBlock.Version.SetV(c.seq)
		for sig, entry := range c.PositionMap[op.target] {
			if sig == targetSig {
				continue
			}

			if entry.Slot == mvpDeletePosition {
				//continue
			}
			path := newPath(op.target, sig, mvpDeletePosition, Versions{V: c.seq, A: c.seq, S: c.seq}, c.seq)
			appendPositionMapUpdate(&populatedPathMap, path)
		}
		delete(W, op.target)
		targetBlock.Version.SetS(c.seq)
		prioritylist = append(prioritylist, targetBlock)
	}

	// Phase 4: keep one copy source per address and delete all old non-priority copies immediately.
	copySourceByAddr := buildCopySourceByAddr(W)

	// W の各アドレスを走査し、sig0 とアクセス対象 copy だけを prioritylist に残す。
	for _, blocks := range W {
		// 同一アドレスの全ブロックを確認し、signature が 0 なら本体、非 0 ならコピーとして扱う。
		for _, block := range blocks {
			if block.signature == 0 || block == targetBlock {

				prioritylist = append(prioritylist, block)
				continue
			}

			// 既存 copy は移動せず削除し、後段で copySourceByAddr から新しい copy として作り直す。
			deletePath := newPath(block.Addr, block.signature, mvpDeletePosition, Versions{V: block.Version.V, A: block.Version.A, S: c.seq}, c.seq)
			appendPositionMapUpdate(&populatedPathMap, deletePath)
		}
	}

	sort.Slice(prioritylist, func(i, j int) bool {
		return prioritylist[i].Version.A > prioritylist[j].Version.A
	})

	// Phase 5: place priority blocks
	// path 上の bucket を走査し、各 bucket 内の固定 slot 0..Z-1 を順に出力対象として初期化する。
	for bucketPosition := range c.path {
		// この bucket の全物理 slot を確認し、priority block の旧 position と一致する slot だけを一時配置に使う。
		for i := 0; i < c.Z; i++ {
			position := MvpPosition{bucket: bucketPosition, slot: MvpSlotPosition(i)}

			if len(prioritylist) == 0 {
				populatedPath[position] = NewMvpSlot(c.seq)
				unusedSlot = append(unusedSlot, position)
				continue
			}

			candidates := make(map[int]MvpDataBlock, 0)
			// prioritylist から、この exact slot に元々いたブロックだけを候補にする。
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
			// 同じ exact slot に複数候補がある場合は、バージョンが最も新しいものを残す。
			for priorityIndex, candidate := range candidates {
				if selectedIndex == -1 || newerVersions(candidate.Version, block.Version) {
					selectedIndex = priorityIndex
					block = candidate
				}
			}
			prioritylist = append(prioritylist[:selectedIndex], prioritylist[selectedIndex+1:]...)
			block.Version.SetS(c.seq)

			slot := NewMvpSlot(c.seq)
			slot.SetBlock(block)
			populatedPath[position] = slot
			usedSlot = append(usedSlot, position)
		}
	}

	// Phase 5.5: re-allocate
	setBlock := make([]MvpDataBlock, 0, len(usedSlot))

	// 一時配置済み slot から実ブロックだけを取り出し、シャッフル対象の配列に移す。
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

	// 使用済み slot を root に近い順へ並べ、次の loop で新しいブロックを root 側に寄せられるようにする。
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

	// 並べ替えた block と slot を同じ index で対応させ、priority block を root 側へ詰め直す。
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
	}

	if len(prioritylist) > 0 && len(unusedSlot) > 0 {
		priorityByAddr := make(map[int][]MvpDataBlock, len(prioritylist))
		// exact slot に戻せなかった priority block をアドレス別にまとめ、空き slot への copy 配置に回す。
		for _, block := range prioritylist {
			priorityByAddr[block.Addr] = append(priorityByAddr[block.Addr], block)
		}
		unusedSlot, _ = c.placePriorityBlocks(priorityByAddr, unusedSlot, populatedPath, &populatedPathMap)

		// 空き slot に置けなかった priority block をすべて新しい stash 出力へ移す。
		for _, blocks := range priorityByAddr {
			// 同じアドレスに残った block を順に stash へ落とし、position map も stash 位置へ更新する。
			for blockIndex, block := range blocks {
				if blockIndex > 0 {
					path := newPath(block.Addr, block.signature, mvpDeletePosition, Versions{V: block.Version.V, A: block.Version.A, S: c.seq}, c.seq)
					appendPositionMapUpdate(&populatedPathMap, path)
					continue
				}
				block.Version.SetS(c.seq)
				populatedStash = append(populatedStash, block)
				appendPositionMapUpdate(&populatedPathMap, newPath(block.Addr, block.signature, mvpStashPosition, block.Version, c.seq))
			}
		}
	} else if len(prioritylist) > 0 {
		// 空き slot がない場合は、残った priority block をそのまま stash 出力へ移す。
		stashedAddr := make(map[int]bool, len(prioritylist))
		for _, block := range prioritylist {
			if stashedAddr[block.Addr] {
				path := newPath(block.Addr, block.signature, mvpDeletePosition, Versions{V: block.Version.V, A: block.Version.A, S: c.seq}, c.seq)
				appendPositionMapUpdate(&populatedPathMap, path)
				continue
			}
			block.Version.SetS(c.seq)
			populatedStash = append(populatedStash, block)
			appendPositionMapUpdate(&populatedPathMap, newPath(block.Addr, block.signature, mvpStashPosition, block.Version, c.seq))
			stashedAddr[block.Addr] = true
		}
	}

	// Phase 6: fill any remaining empty slots with newly generated copies using unused signatures.
	if len(unusedSlot) > 0 {
		unusedSlot = c.placeCopyBlocks(copySourceByAddr, unusedSlot, populatedPath, &populatedPathMap)
	}

	// 新しいpath、新しいstash、新しいPathMapを返す。
	return populatedPath, populatedStash, populatedPathMap
}
