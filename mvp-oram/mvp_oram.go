package main

import "strconv"

const stashPosition = -1

type MvpPosition string

const mvpRootPosition MvpPosition = "root"

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
	Addr uint
	Data string
}

type MvpBucket struct {
	Z     int            //バケットに入る最大サイズ
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
	Value []MvpBucket //複数バージョン入れるためのリスト
}

func NewMvpBuckets() MvpBuckets {
	const MAX_CAP = 100

	return MvpBuckets{
		Value: make([]MvpBucket, 0, MAX_CAP),
	}
}

func (b *MvpBuckets) SetBucket(bucket MvpBucket) {
	b.Value = append(b.Value, bucket)
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

			tree[position] = NewMvpBuckets()
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
	addr uint
	from MvpPosition
	to   MvpPosition
}

type ServerRequest interface {
	handle(s *MvpSurver)
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
	Path  map[MvpPosition][]MvpBuckets
	Stash []MvpDataBlock
	Err   error
}

type EvictReques struct {
	ClientID int
	PathMap  []path
	Stash    []MvpDataBlock
	Path     map[MvpPosition][]MvpBucket
	Reply    chan EvictResponse
}

type EvictResponse struct {
	Err error
}

type MvpSurver struct {
	PositionMaps []map[int][]MvpPosition
	PathMaps     []path
	Stashs       [][]MvpDataBlock
	tree         MvpTree
	counter      Version

	Requests chan ServerRequest
}

func (s *MvpSurver) Run() {
	for req := range s.Requests {
		req.handle(s)
	}
}

func (r GetpmRequest) handle(s *MvpSurver) {
	seq := s.counter
	s.counter.increment()

	r.Reply <- GetpmResponse{
		Seq: seq,
	}
}

func (r GetpsRequest) handle(s *MvpSurver) {
	r.Reply <- GetpsResponse{}
}

func (r EvictReques) handle(s *MvpSurver) {
	r.Reply <- EvictResponse{}
}

type MvpClient struct {
	PositionMap map[int][]MvpPosition
	PathMaps    []path
	Stash       []MvpDataBlock
	Access_addr uint
	path        map[MvpPosition][]MvpBucket

	ClientID int
	Server   chan<- ServerRequest
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

func (c *MvpClient) GetPS(leaf MvpPosition) (map[MvpPosition][]MvpBuckets, []MvpDataBlock, error) {
	reply := make(chan GetpsResponse)

	c.Server <- GetpsRequest{
		ClientID: c.ClientID,
		Leaf:     leaf,
		Reply:    reply,
	}

	res := <-reply
	return res.Path, res.Stash, res.Err
}

func (c *MvpClient) Evict(path map[MvpPosition][]MvpBucket, pathmap []path, stash []MvpDataBlock) error {
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

func (c *MvpClient) Access(addr uint) error {
	return nil
}
