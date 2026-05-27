package main

import (
	"log"
	"math/rand"
	"sort"
	"strconv"
)

type MvpPosition string

const (
	mvpRootPosition   MvpPosition = "root"
	mvpStashPosition  MvpPosition = "-1"
	mvpInitialVersion Version     = 0
)

func (p MvpPosition) String() string {
	return string(p)
}

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
	Addr int
	Data string
}

type MvpBucket struct {
	Z     int
	Value []MvpDataBlock //キューブ
}

func NewMvpBucket(z int) MvpBucket {
	return MvpBucket{
		Z:     z,
		Value: make([]MvpDataBlock, 0, z),
	}
}

func (b *MvpBucket) SetBlock(block MvpDataBlock) bool {
	if len(b.Value) >= b.Z {
		return false
	}

	b.Value = append(b.Value, block)

	return true
}

type MvpBuckets struct {
	Value map[Version]MvpBucket // バージョンを鍵にしたバケットS
}

func NewMvpBuckets(z int) MvpBuckets {
	bucket := NewMvpBucket(z)

	return MvpBuckets{
		Value: map[Version]MvpBucket{
			mvpInitialVersion: bucket,
		},
	}
}

func (b *MvpBuckets) SetBucket(version Version, bucket MvpBucket) {
	b.Value[version] = bucket
}

type MvpTree struct {
	Z               int
	L               int
	Tree            map[MvpPosition]MvpBuckets
	BucketReadCount map[MvpPosition]int64
	TotalBucketRead int64
}

func NewMvpPosition(level, index int) MvpPosition {
	if level == 0 {
		return mvpRootPosition
	}

	key := strconv.FormatInt(int64(index), 2)
	for len(key) < level {
		key = "0" + key
	}
	return MvpPosition(key)
}

func NewMvpTree(z int, l int) MvpTree {
	tree := make(map[MvpPosition]MvpBuckets, 1<<(l+1)-1)
	bucketReadCount := make(map[MvpPosition]int64, 1<<(l+1)-1)

	for level := 0; level <= l; level++ {
		for index := 0; index < 1<<level; index++ {
			position := NewMvpPosition(level, index)

			tree[position] = NewMvpBuckets(z)
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
	Path  map[MvpPosition]MvpBuckets
	Stash []MvpDataBlock
	Err   error
}

type EvictReques struct {
	ClientID int
	PathMap  []path
	Stash    []MvpDataBlock
	Path     map[MvpPosition]MvpBuckets
	Reply    chan EvictResponse
}

type EvictResponse struct {
	Err error
}

type MvpServer struct {
	PositionMaps  []map[int]MvpPosition
	PathMaps      []path
	Stashs        [][]MvpDataBlock
	tree          MvpTree
	counter       Version
	Snapshot      map[Version]MvpTree
	Accesshistory map[int]Version

	Requests chan ServerRequest
}

func NewMvpServer(z int, l int) *MvpServer {
	return &MvpServer{
		PositionMaps:  make([]map[int]MvpPosition, 0),
		PathMaps:      make([]path, 0),
		Stashs:        make([][]MvpDataBlock, 0),
		tree:          NewMvpTree(z, l),
		Requests:      make(chan ServerRequest),
		counter:       mvpInitialVersion,
		Snapshot:      make(map[Version]MvpTree, 50),
		Accesshistory: make(map[int]Version, 50),
	}
}

func (s *MvpServer) InitializeRandomData(n int, seed int64) map[int]MvpPosition {
	rng := rand.New(rand.NewSource(seed))
	positions := make([]MvpPosition, 0, len(s.tree.Tree)) //木に存在するポジション一覧生成
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
			Addr: int(addr),
			Data: strconv.Itoa(addr),
		}
		position := positions[rng.Intn(len(positions))] //ランダムにポジションを選ぶ
		buckets := s.tree.Tree[position]
		bucket := buckets.Value[mvpInitialVersion]

		if bucket.SetBlock(block) {
			buckets.SetBucket(mvpInitialVersion, bucket)
			s.tree.Tree[position] = buckets //バケットに入るか試みる
			positionMap[addr] = position
			continue
		}

		stash = append(stash, block) //溢れたらスタッシュに移動
		positionMap[addr] = mvpStashPosition
	}

	s.PositionMaps = append(s.PositionMaps, positionMap)
	s.Stashs = append(s.Stashs, stash)

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

	difpathMap := make([]path, 100)
	for _, v := range s.PathMaps {
		if v.Ver > lastVersion {
			difpathMap = append(difpathMap, v)
		}
	}

	s.Snapshot[seq] = s.tree

	r.Reply <- GetpmResponse{
		Seq:     seq,
		PathMap: difpathMap,
		Err:     nil,
	}
}

func (r GetpsRequest) handle(s *MvpServer) {
	r.Reply <- GetpsResponse{}
}

func (r EvictReques) handle(s *MvpServer) {
	r.Reply <- EvictResponse{}
}

type MvpClient struct {
	L           int
	PositionMap map[int]MvpPosition
	Stash       []MvpDataBlock
	path        map[MvpPosition]MvpBuckets

	ClientID int
	Server   chan<- ServerRequest
}

func NewMvpClient(l int, clientID int, positionmap map[int]MvpPosition, server chan<- ServerRequest) *MvpClient {
	return &MvpClient{
		L:           l,
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

func (c *MvpClient) GetPS(leaf MvpPosition) (map[MvpPosition]MvpBuckets, []MvpDataBlock, error) {
	reply := make(chan GetpsResponse)

	c.Server <- GetpsRequest{
		ClientID: c.ClientID,
		Leaf:     leaf,
		Reply:    reply,
	}

	res := <-reply
	return res.Path, res.Stash, res.Err
}

func (c *MvpClient) Evict(path map[MvpPosition]MvpBuckets, pathmap []path, stash []MvpDataBlock) error {
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
		err := c.Access(100)
		if err != nil {
			panic(err)
		}

		break
	}

	return nil
}

func (c *MvpClient) Access(addr int) error {
	version, pathMaps, err := c.GetPM()
	if err != nil {
		return err
	}

	c.consolidatePathMaps(pathMaps) //position mapの更新

	log.Println("version is ", version)
	log.Println("Access block's leaf is ", c.PositionMap[addr])

	accessPosition := c.PositionMap[addr]

	log.Println(selectPath(accessPosition, c.L))

	c.path, c.Stash, err = c.GetPS(selectPath(accessPosition, c.L))
	if err != nil {
		return err
	}

	log.Println(c.path)
	log.Println(c.Stash)

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
		accessPosition = ""
	}

	leaf := accessPosition.String()
	for len(leaf) < pathlen {
		if rand.Intn(2) == 0 {
			leaf += "0"
		} else {
			leaf += "1"
		}
	}

	return MvpPosition(leaf)
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
		0,
		positionmap,
		server.Requests,
	)

	if err := client.Run(); err != nil {
		panic(err)
	}

	close(server.Requests)
}
