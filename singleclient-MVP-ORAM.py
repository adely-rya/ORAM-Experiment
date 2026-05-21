from __future__ import annotations
from collections import Counter
import random
from typing import Optional
import copy


random.seed(542)


ROOT_KEY: str = "root"


class ORAMtree:
    def __init__(self, L: int, Z: int) -> None:
        self.L: int = L
        self.Z: int = Z
        self.tree: dict[str, bucket] = {}
        self.leaflog: list[str] = list()

        for h in range(self.L + 1):
            for i in range(2**h):
                if h == 0:
                    key: str = ROOT_KEY
                else:
                    key = format(i, f"0{h}b")

                b: bucket = bucket(Z)
                self.tree[key] = b

    def __repr__(self) -> str:
        out: list[str] = []

        for h in range(self.L + 1):
            out.append(f"Level {h}")

            for i in range(2**h):
                if h == 0:
                    key: str = ROOT_KEY
                else:
                    key = format(i, f"0{h}b")

                out.append(f"  [{key}] {self.tree[key]}")

        return "\n".join(out)

    def read_bucket(self, leaf: str, h: int) -> bucket:
        if h == 0:
            key: str = ROOT_KEY
        else:
            key = leaf[:h]
        
        self.bucketlog.append(key)

        return self.tree[key]

    def read_path(self, leaf: str) -> list[datablock]:
        self.leaflog.append(leaf)
        datablocks: list[datablock] = []

        for i in range(self.L + 1):
            bucket_obj: bucket = self.read_bucket(leaf, i)

            for block in bucket_obj.value:
                datablocks.append(block)

        return datablocks

    def set_block(self, key: str, block: datablock) -> bool:
        if self.tree[key].setblock(block):
            return True
        else:
            return False
    
    def set_bucket(self,key: str, value: bucket) -> None:
        self.tree[key] = value
        


class bucket:
    def __init__(self, Z: int) -> None:
        self.Z: int = Z
        self.value: list[datablock] = []

    def generate_dummy(self) -> None:
        for _ in range(self.Z):
            self.value.append(datablock(None, ""))

    def __repr__(self) -> str:
        return "[" + ",".join(repr(i) for i in self.value) + "]"

    def setblock(self, block: datablock) -> bool:
        if len(self.value) < self.Z:
            self.value.append(block)
            return True
        else:
            return False


class datablock:
    def __init__(self, addr: Optional[int], data: str) -> None:
        self.addr: Optional[int] = addr
        self.data: str = data

    def isdummy(self) -> bool:
        return self.addr is None

    def __repr__(self) -> str:
        if self.isdummy():
            return "dummy block"

        return f"addr:{self.addr} data:{self.data}"


class server:
    def __init__(self, tree: ORAMtree) -> None:
        self.tree: ORAMtree = tree

    def getpath(self, path: str) -> list[datablock]:
        return self.tree.read_path(path)

    def reallocation(self,shuffle_dict: dict[str, bucket]) -> None:
        for k,v in shuffle_dict.items():
            self.tree.set_bucket(k,v)


class client:
    def __init__(self, pm: dict[int, str], stash: list[datablock], L: int, Z: int) -> None:
        self.pm: dict[int, str] = pm
        self.stash: list[datablock] = stash

        self.L: int = L
        self.Z: int = Z

        self.accessblock: Optional[int] = None
        self.leaf: str = ""

    def get_data(self, addr: int) -> str:

        instash: bool = False
        for i in self.stash:
            if addr == i.addr:
                instash = True
                break
        
        self.accessblock = addr
        

        leaf_list:list[str] = []
        for i in range(2**self.L):
            leaf_list.append(format(i, f"0{self.L}b"))
        #リーフのリストを生成


        #スタッシュにあった場合はランダムなパス
        if instash:
            self.leaf = random.choice(leaf_list)
        
            #リーフリストかランダムにリーフを選択してreturn

            return self.leaf

        #スタッシュなかった場合はアクセスブロックを通るリーフを選定

        candinate_leaf:list[str] = []
        block_position: str = self.pm[addr]

        for i in leaf_list:
            if i.startswith(block_position):
                candinate_leaf.append(i)
        
        #候補からランダムにリーフを選んでリターン
        if not candinate_leaf:
            raise ValueError("access block position does not match any leaf")

        self.leaf = random.choice(candinate_leaf)
        return self.leaf

    def get_random_data(self) -> str:
        keys_list: list[int] = list(self.pm)
        random_key: int = random.choice(keys_list)

        return self.get_data(random_key)

    def can_place_block(self, block_path: str, bucket_position: str) -> bool:
        if bucket_position == ROOT_KEY:
            return True

        return block_path[:len(bucket_position)] == bucket_position

    def shuffle(self, blocks: list[datablock]) -> dict[str, bucket]:
        if self.accessblock is None:
            raise ValueError("get_data or get_random_data must be called before shuffle")

        W: list[datablock] = []

        #Wにブロックを全部入れる
        for block in blocks:
            if not block.isdummy():
                W.append(block)
        
        for block in self.stash:
            W.append(block)


        shuffled: dict[str, bucket] = {}

        for level in range(self.L, -1, -1):
            if level == 0:
                key = ROOT_KEY
            else:
                key = self.leaf[:level]
            
            new_bucket = bucket(self.Z)

            shuffled[key] = new_bucket
        
        



        


        return shuffled


def random_leaf(L: int) -> str:
    return format(random.randrange(2**L), f"0{L}b")


def insert_block_to_path_random(tree: ORAMtree, block: datablock, path: str) -> bool:
    leaf: str = path

    levels: list[int] = list(range(tree.L + 1))
    random.shuffle(levels)

    for h in levels:
        if h == 0:
            key: str = ROOT_KEY
        else:
            key = leaf[:h]

        if tree.set_block(key, block):
            return True

    return False

def to_distribution(access_log):
    count = Counter(access_log)
    total = len(access_log)

    return {
        value: freq / total
        for value, freq in count.items()
    }

def statistical_distance(dist1, dist2) -> float:
    keys = set(dist1.keys()) | set(dist2.keys())

    return 0.5 * sum(
        abs(dist1.get(k, 0.0) - dist2.get(k, 0.0))
        for k in keys
    )

N: int = 256
L: int = 7
Z: int = 2

pm: dict[int, str] = {}
stash: list[datablock] = []

tree: ORAMtree = ORAMtree(L, Z)

for i in range(N):
    addr: int = i
    path: str = random_leaf(L)
    data: str = str(i)

    block: datablock = datablock(addr, data)

    pm[addr] = path

    success: bool = insert_block_to_path_random(tree, block, path)

    if not success:
        stash.append(block)


tree1: ORAMtree = copy.deepcopy(tree)
tree2: ORAMtree = copy.deepcopy(tree)

pm1: dict[int, str] = copy.deepcopy(pm)
pm2: dict[int, str] = copy.deepcopy(pm)

stash1: list[datablock] = copy.deepcopy(stash)
stash2: list[datablock] = copy.deepcopy(stash)

oram_server1: server = server(tree1)
oram_client1: client = client(pm1, stash1, L, Z)

oram_server2: server = server(tree2)
oram_client2: client = client(pm2, stash2, L, Z)


# random workflow--------------------------------------------------------------
for i in range(10000):
    print(len(oram_client1.stash))
    path: str = oram_client1.get_random_data()

    blocks: list[datablock] = oram_server1.getpath(path)
    shuffle_dict: dict[str, bucket] = oram_client1.shuffle(blocks)
    oram_server1.reallocation(shuffle_dict)


# fixed address workflow--------------------------------------------------------------
for i in range(10000):
    #print(len(oram_client1.stash))
    #addr = 100
    #path = oram_client2.get_data(addr)
    path: str = oram_client2.get_random_data()

    
    blocks: list[datablock] = oram_server2.getpath(path)
    shuffle_dict: dict[str, bucket] = oram_client2.shuffle(blocks)
    oram_server2.reallocation(shuffle_dict)


bucket_distribution1 = to_distribution(oram_server1.tree.bucketlog)
bucket_distribution2 = to_distribution(oram_server2.tree.bucketlog)


print("statistical distance: bucket")
print(statistical_distance(bucket_distribution1, bucket_distribution2))
