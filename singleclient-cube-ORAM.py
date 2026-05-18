from __future__ import annotations
from collections import Counter
import random
from typing import Optional
import copy
import logging

random.seed(10)

class ORAMcube:
    def __init__(self,Bit: int,Z: int,PL: int) -> None:
        self.Bit: int = Bit
        self.Z: int = Z
        self.PL: int = PL
        self.cube: dict[str,bucket] = {}

        self.root: str = format(0,f"0{self.Bit}b")

        self.bucket_log: list[str] = []

        for i in range(2 ** self.Bit):
            key: str = format(i,f"0{self.Bit}b")
            self.cube[key] = bucket(self.Z)
        
    def __repr__(self) -> str:
        result: str = ""
        for k,v in self.cube.items():
            result += f"{k}→{v} \n"
        return result
    
    def set_block(self, key: str,block: datablock) -> bool:
        if self.cube[key].setblock(block):
            return True
        else:
            return False
    
    def set_bucket(self,key: str,bucket: bucket) -> None:
        self.cube[key] = bucket
    
    def get_bucket(self,key: str) -> bucket:
        self.bucket_log.append(key)
        return self.cube[key]

        

class bucket:
    def __init__(self, Z: int) -> None:
        self.Z: int = Z
        self.value: list[datablock] = []

    def __repr__(self) -> str:
        return "[" + ",".join(repr(i) for i in self.value) + "]"

    def setblock(self, block: datablock) -> bool:
        if len(self.value) < self.Z:
            self.value.append(block)
            return True
        else:
            return False


class datablock:
    def __init__(self, addr: int, data: str) -> None:
        self.addr: int = addr
        self.data: str = data

    def __repr__(self) -> str:
        return f"addr:{self.addr} data:{self.data}"


class server:
    def __init__(self,cube: ORAMcube) -> None:
        self.cube = cube
        self.counter = 0
    
    def give_counter(self):
        counter = self.counter
        self.counter += 1
        return counter
    
    def getpath(self,pathlist: list[str]) -> list[datablock]:
        datalist: list[datablock] = list()

        for i in pathlist:
            blocks:bucket = self.cube.get_bucket(i)
            for j in blocks.value:
                datalist.append(j)

        return datalist

    def reallocation(self,shuffled: dict[str,bucket]) -> None:
        for k,v in shuffled.items():
            self.cube.set_bucket(k,v)

    
class client:
    def __init__(self, pm: dict[int, str], stash: list[datablock], Bit: int, Z: int, PL: int) -> None:
        self.pm: dict[int, str] = pm
        self.stash: list[datablock] = stash

        self.counter: int  = 0

        self.Bit: int = Bit
        self.Z: int = Z
        self.PL: int = PL

        self.accessblock: int = 0
        self.pathlist: list[str] = []
    
    def get_data(self, addr: int) -> list[str]:
        path: list[str] = list()
        self.accessblock = addr

        block_position = self.pm[addr]
        #print(f"target position = {block_position}")
        
        if block_position == "S":
            block_position = random.randint(0,len(self.pm))
        
        distance: int = 0
        flip_list: list[int] = []

        for i in range(self.Bit):
            if block_position[i] == "1":
                distance += 1
                flip_list.append(i)
        
        random.shuffle(flip_list)

        dif: int = self.PL - distance


        path: list[str] = []
        visited: set[str] = set()

        last_bit = ["0" for _ in range(self.Bit)]

        for i in flip_list:
            path.append("".join(last_bit))
            visited.add("".join(last_bit))
            last_bit[i] = "1"

        current_point = "".join(last_bit)
        path.append(current_point)
        visited.add(current_point)


        for i in range(random.randint(0, dif)):
            candidates: list[int] = []

            for bit in range(self.Bit):
                next_bit = last_bit.copy()

                if next_bit[bit] == "0":
                    next_bit[bit] = "1"
                else:
                    next_bit[bit] = "0"

                next_point = "".join(next_bit)

                if next_point not in visited:
                    candidates.append(bit)

            if not candidates:
                print("次の点が見つかりません")
                raise ValueError

            flipbit = random.choice(candidates)

            if last_bit[flipbit] == "0":
                last_bit[flipbit] = "1"
            else:
                last_bit[flipbit] = "0"

            current_point = "".join(last_bit)

            visited.add(current_point)
            path.append(current_point)

        
        half_path: list[str] = []

        last_bit = ["0" for _ in range(self.Bit)]

        for i in range(0, self.PL - len(path)):
            candidates: list[int] = []

            for bit in range(self.Bit):
                next_bit = last_bit.copy()

                if next_bit[bit] == "0":
                    next_bit[bit] = "1"
                else:
                    next_bit[bit] = "0"

                next_point = "".join(next_bit)

                if next_point not in visited:
                    candidates.append(bit)

            if not candidates:
                print("次の点が見つかりません")
                raise ValueError

            flipbit = random.choice(candidates)

            if last_bit[flipbit] == "0":
                last_bit[flipbit] = "1"
            else:
                last_bit[flipbit] = "0"

            current_point = "".join(last_bit)

            visited.add(current_point)
            half_path.append(current_point)
                
        half_path.reverse()

        half_path.extend(path)
        path = half_path

        self.pathlist = path
        return path

    def get_random_data(self) -> list[str]:
        keys_list: list[int] = list(self.pm)
        random_key: int = random.choice(keys_list)
        #print(f"block address = {random_key}")
        return self.get_data(random_key)

    def shuffle(self, blocks: list[datablock]) -> dict[str, bucket]:

        shuffled: dict[str, bucket] = {}

        all_blocks: list[datablock] = []

        for block_data in blocks:
            all_blocks.append(block_data)

        for block_data in self.stash:
            all_blocks.append(block_data)

        self.stash = []

        for position in self.pathlist:
            shuffled[position] = bucket(self.Z)

        random.shuffle(all_blocks)

        for block_data in all_blocks:
            candidate_positions: list[str] = self.pathlist.copy()
            random.shuffle(candidate_positions)

            placed = False

            for position in candidate_positions:
                if shuffled[position].setblock(block_data):
                    self.pm[block_data.addr] = position
                    placed = True
                    break

            if not placed:
                self.stash.append(block_data)


        return shuffled

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

N: int = 128
Bit: int = 8
Z: int = 4
PL: int = 10

pm: dict[int,str] = {}
stash: list[datablock] = []

cube = ORAMcube(Bit,Z,PL)


for i in range(N):
    addr: int = i
    position: str = format(random.randrange(2**Bit), f"0{Bit}b")
    data: str = str(i)

    block: datablock = datablock(addr,data)

    success: bool = cube.set_block(position,block)

    if success:
        pm[addr] = position
    else:
        stash.append(block)
        pm[addr] = "S"


print(cube)
print(stash)

print(pm)

cube1: ORAMcube = copy.deepcopy(cube)
cube2: ORAMcube = copy.deepcopy(cube)

pm1: dict[int, str] = copy.deepcopy(pm)
pm2: dict[int, str] = copy.deepcopy(pm)

stash1: list[datablock] = copy.deepcopy(stash)
stash2: list[datablock] = copy.deepcopy(stash)

oram_server1: server = server(cube1)
oram_client1: client = client(pm1,stash1,Bit,Z,PL)

oram_server2: server = server(cube2)
oram_client2: client = client(pm2,stash2,Bit,Z,PL)


# random workflow--------------------------------------------------------------

for i in range(10000):
    oram_client1.counter = oram_server1.give_counter()
    pathlist: list[str] = oram_client1.get_random_data()
    #print(pathlist)
    datalist: list[datablock] = oram_server1.getpath(pathlist)
    #print(datalist)
    shuffled: dict[str,bucket] = oram_client1.shuffle(datalist)
    #print(shuffled)
    oram_server1.reallocation(shuffled)

# fixed address workflow--------------------------------------------------------------

for i in range(10000):
    oram_client2.counter = oram_server2.give_counter()
    pathlist: list[str] = oram_client2.get_data(i % 10)
    #pathlist: list[str] = oram_client2.get_random_data()
    #print(pathlist)
    datalist: list[datablock] = oram_server2.getpath(pathlist)
    #print(datalist)
    shuffled: dict[str,bucket] = oram_client2.shuffle(datalist)
    #print(shuffled)
    oram_server2.reallocation(shuffled)


bucket_distribution1 = to_distribution(oram_server1.cube.bucket_log)
bucket_distribution2 = to_distribution(oram_server2.cube.bucket_log)

print("statistical distance: bucket")
print(statistical_distance(bucket_distribution1, bucket_distribution2))