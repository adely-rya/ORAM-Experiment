package main

import "testing"

func TestGetPSCountsBucketReads(t *testing.T) {
	const (
		z = 2
		l = 3
	)

	server := NewMvpServer(z, l)
	server.InitializeRandomData(4, 1)

	getPMReply := make(chan GetpmResponse, 1)
	GetpmRequest{ClientID: 0, Reply: getPMReply}.handle(server)
	if res := <-getPMReply; res.Err != nil {
		t.Fatalf("GetPM returned error: %v", res.Err)
	}

	getPSReply := make(chan GetpsResponse, 1)
	GetpsRequest{
		ClientID: 0,
		Leaf:     MvpPosition{bucket: MvpBucketPosition("010")},
		Reply:    getPSReply,
	}.handle(server)
	if res := <-getPSReply; res.Err != nil {
		t.Fatalf("GetPS returned error: %v", res.Err)
	}

	wantTotal := int64(l + 1)
	if server.tree.TotalBucketRead != wantTotal {
		t.Fatalf("TotalBucketRead = %d, want %d", server.tree.TotalBucketRead, wantTotal)
	}

	for _, position := range []MvpBucketPosition{
		mvpRootBucketPosition,
		MvpBucketPosition("010"),
		MvpBucketPosition("01"),
		MvpBucketPosition("0"),
	} {
		if got := server.tree.BucketReadCount[position]; got != 1 {
			t.Fatalf("BucketReadCount[%s] = %d, want 1", position, got)
		}
	}
}

func TestGetPSDoesNotCountMissingSnapshot(t *testing.T) {
	server := NewMvpServer(2, 3)

	reply := make(chan GetpsResponse, 1)
	GetpsRequest{
		ClientID: 99,
		Leaf:     MvpPosition{bucket: MvpBucketPosition("010")},
		Reply:    reply,
	}.handle(server)
	if res := <-reply; res.Err == nil {
		t.Fatal("GetPS returned nil error for missing snapshot")
	}

	if server.tree.TotalBucketRead != 0 {
		t.Fatalf("TotalBucketRead = %d, want 0", server.tree.TotalBucketRead)
	}
}

func TestGetPMReturnsOnlyUnreadPathMapsPerClient(t *testing.T) {
	server := NewMvpServer(2, 3)
	server.InitializeRandomData(4, 1)

	server.PathMaps = append(server.PathMaps,
		newPath(1, mvpStashPosition, Versions{V: 1, A: 1, S: 1}, 1),
		newPath(2, mvpStashPosition, Versions{V: 2, A: 2, S: 2}, 2),
	)

	firstReply := make(chan GetpmResponse, 1)
	GetpmRequest{ClientID: 0, Reply: firstReply}.handle(server)
	first := <-firstReply
	if first.Err != nil {
		t.Fatalf("first GetPM returned error: %v", first.Err)
	}
	if got := len(first.PathMap); got != 2 {
		t.Fatalf("first client 0 PathMap length = %d, want 2", got)
	}

	server.PathMaps = append(server.PathMaps,
		newPath(3, mvpStashPosition, Versions{V: 3, A: 3, S: 3}, 3),
	)

	secondReply := make(chan GetpmResponse, 1)
	GetpmRequest{ClientID: 0, Reply: secondReply}.handle(server)
	second := <-secondReply
	if second.Err != nil {
		t.Fatalf("second GetPM returned error: %v", second.Err)
	}
	if got := len(second.PathMap); got != 1 {
		t.Fatalf("second client 0 PathMap length = %d, want 1", got)
	}
	if second.PathMap[0].addr != 3 {
		t.Fatalf("second client 0 PathMap addr = %d, want 3", second.PathMap[0].addr)
	}

	thirdReply := make(chan GetpmResponse, 1)
	GetpmRequest{ClientID: 0, Reply: thirdReply}.handle(server)
	third := <-thirdReply
	if third.Err != nil {
		t.Fatalf("third GetPM returned error: %v", third.Err)
	}
	if got := len(third.PathMap); got != 0 {
		t.Fatalf("third client 0 PathMap length = %d, want 0", got)
	}

	otherClientReply := make(chan GetpmResponse, 1)
	GetpmRequest{ClientID: 1, Reply: otherClientReply}.handle(server)
	otherClient := <-otherClientReply
	if otherClient.Err != nil {
		t.Fatalf("client 1 GetPM returned error: %v", otherClient.Err)
	}
	if got := len(otherClient.PathMap); got != 3 {
		t.Fatalf("client 1 PathMap length = %d, want 3", got)
	}
}

func TestCompactPathMapsDropsOldPrefixEntriesOnlyWhenAddrAppearsInProtectedTail(t *testing.T) {
	server := NewMvpServer(2, 3)
	server.SetPathMapCompaction(1, 2)
	server.PathMaps = []path{
		newPath(1, mvpStashPosition, Versions{V: 1, A: 1, S: 1}, 1),
		newPath(2, mvpStashPosition, Versions{V: 2, A: 2, S: 2}, 2),
		newPath(1, MvpPosition{bucket: MvpBucketPosition("001"), slot: 0}, Versions{V: 3, A: 3, S: 3}, 3),
		newPath(3, mvpStashPosition, Versions{V: 4, A: 4, S: 4}, 4),
		newPath(2, MvpPosition{bucket: MvpBucketPosition("010"), slot: 1}, Versions{V: 5, A: 5, S: 5}, 5),
	}
	server.PathMapCursor[0] = 0
	server.PathMapCursor[1] = 1
	server.PathMapCursor[2] = len(server.PathMaps)

	server.compactPathMaps()

	wantAddrs := []int{1, 1, 3, 2}
	wantSeqs := []Version{1, 3, 4, 5}
	if got := len(server.PathMaps); got != len(wantAddrs) {
		t.Fatalf("PathMaps length = %d, want %d", got, len(wantAddrs))
	}
	for i := range wantAddrs {
		if server.PathMaps[i].addr != wantAddrs[i] {
			t.Fatalf("PathMaps[%d].addr = %d, want %d", i, server.PathMaps[i].addr, wantAddrs[i])
		}
		if server.PathMaps[i].Seq != wantSeqs[i] {
			t.Fatalf("PathMaps[%d].Seq = %d, want %d", i, server.PathMaps[i].Seq, wantSeqs[i])
		}
	}

	if got := server.PathMapCursor[0]; got != 0 {
		t.Fatalf("client 0 cursor = %d, want 0", got)
	}
	if got := server.PathMapCursor[1]; got != 1 {
		t.Fatalf("client 1 cursor = %d, want 1", got)
	}
	if got := server.PathMapCursor[2]; got != 4 {
		t.Fatalf("client 2 cursor = %d, want 4", got)
	}
}

func TestCompactPathMapsKeepsPrefixEntriesWithoutAddrInProtectedTail(t *testing.T) {
	server := NewMvpServer(2, 3)
	server.SetPathMapCompaction(1, 1)
	server.PathMaps = []path{
		newPath(1, mvpStashPosition, Versions{V: 1, A: 1, S: 1}, 1),
		newPath(1, MvpPosition{bucket: MvpBucketPosition("001"), slot: 0}, Versions{V: 2, A: 2, S: 2}, 2),
		newPath(2, mvpStashPosition, Versions{V: 3, A: 3, S: 3}, 3),
	}

	server.compactPathMaps()

	if got := len(server.PathMaps); got != 3 {
		t.Fatalf("PathMaps length = %d, want 3", got)
	}
}

func TestEvictTriggersWindowedPathMapCompactionAtConfiguredInterval(t *testing.T) {
	server := NewMvpServer(2, 3)
	server.SetPathMapCompaction(2, 1)

	EvictReques{
		ClientID: 0,
		Seq:      1,
		PathMap: []path{
			newPath(1, mvpStashPosition, Versions{V: 1, A: 1, S: 1}, 1),
		},
		Reply: make(chan EvictResponse, 1),
	}.handle(server)
	if got := len(server.PathMaps); got != 1 {
		t.Fatalf("PathMaps length after first evict = %d, want 1", got)
	}

	reply := make(chan EvictResponse, 1)
	EvictReques{
		ClientID: 0,
		Seq:      2,
		PathMap: []path{
			newPath(1, MvpPosition{bucket: MvpBucketPosition("001"), slot: 0}, Versions{V: 2, A: 2, S: 2}, 2),
		},
		Reply: reply,
	}.handle(server)
	if res := <-reply; res.Err != nil {
		t.Fatalf("Evict returned error: %v", res.Err)
	}
	if got := len(server.PathMaps); got != 1 {
		t.Fatalf("PathMaps length after triggered compaction = %d, want 1", got)
	}
	if got := server.PathMaps[0].Seq; got != 2 {
		t.Fatalf("remaining PathMap Seq = %d, want 2", got)
	}
}

func TestSelectPathTreatsAnyRootSlotAsRoot(t *testing.T) {
	leaf := selectPath(MvpPosition{bucket: mvpRootBucketPosition, slot: 1}, 4)
	if len(leaf.bucket.String()) != 4 {
		t.Fatalf("leaf length = %d, want 4", len(leaf.bucket.String()))
	}
	for _, char := range leaf.bucket.String() {
		if char != '0' && char != '1' {
			t.Fatalf("leaf = %q, want only binary digits", leaf.bucket.String())
		}
	}
}

func TestPopulatePathAlwaysSwapsAccessedPathSlot(t *testing.T) {
	const targetAddr = 7

	targetPosition := MvpPosition{bucket: MvpBucketPosition("0"), slot: 0}
	targetBlock := MvpDataBlock{
		Addr: targetAddr,
		Data: "target",
		Version: Versions{
			V: 1,
			A: 1,
			S: 1,
		},
	}

	slot := NewMvpSlot(1)
	if ok := slot.SetBlock(targetBlock); !ok {
		t.Fatal("failed to set target block")
	}

	client := &MvpClient{
		L:   1,
		Z:   1,
		seq: 2,
		PositionMap: map[int]MvpPositionMapEntry{
			targetAddr: {
				Slot: targetPosition,
				Ts:   targetBlock.Version,
			},
		},
		path: map[MvpBucketPosition]MvpBucket{
			MvpBucketPosition("0"): {
				Z: 1,
				Slots: map[MvpSlotPosition]map[Version]MvpSlot{
					0: {
						1: slot,
					},
				},
			},
		},
	}

	populatedPath, populatedStash, populatedPathMap := client.populatePath(
		map[int]MvpDataBlock{targetAddr: targetBlock},
		OramOP{OP: Write, target: targetAddr, param: "updated"},
	)

	if slot := populatedPath[targetPosition]; !slot.IsEmpty() {
		t.Fatalf("target slot remains occupied by addr %d, want it swapped out", slot.Value.Addr)
	}

	if got := len(populatedStash); got != 1 {
		t.Fatalf("populated stash length = %d, want 1", got)
	}
	if populatedStash[0].Addr != targetAddr {
		t.Fatalf("populated stash addr = %d, want %d", populatedStash[0].Addr, targetAddr)
	}

	foundStashPathMap := false
	for _, entry := range populatedPathMap {
		if entry.addr == targetAddr && entry.to == mvpStashPosition {
			foundStashPathMap = true
			break
		}
	}
	if !foundStashPathMap {
		t.Fatal("missing path map entry that moves target addr to stash")
	}
}
