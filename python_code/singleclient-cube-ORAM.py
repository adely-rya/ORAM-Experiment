from __future__ import annotations
from collections import Counter
import random
from typing import Optional
import copy
from PRF import random_int
import math

#random.seed(10)

PMPosition = Optional[str]

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
        for k, v in self.cube.items():
            result += f"{k}->{v} \n"
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
    def __init__(self, pm: dict[int, PMPosition], stash: list[datablock], Bit: int, Z: int, PL: int, window: int) -> None:
        self.pm: dict[int, PMPosition] = pm
        self.stash: list[datablock] = stash

        self.counter: int  = 0

        self.Bit: int = Bit
        self.Z: int = Z
        self.PL: int = PL

        self.accessblock: int = 0
        self.pathlist: list[str] = []

        self.window: int = window #どれぐらいの共通アクセスを気にするのか
        self.seed = 123 #PRF用のシード　本当は共通鍵
    
    def get_data(self, addr: int, target_step: int | None = None) -> list[str]:

        self.accessblock = addr

        block_position = self.pm[addr]

        root: str = "0" * self.Bit

        if (block_position is None) or (block_position == root):
            block_position = format(random.randrange(0,2**self.Bit), f"0{self.Bit}b")

        target: str = block_position
        

        def hamming(a: str, b: str) -> int:
            return sum(x != y for x, y in zip(a, b))

        def flip_point(point: str, bit: int) -> str:
            point_list = list(point)
            point_list[bit] = "1" if point_list[bit] == "0" else "0"
            return "".join(point_list)

        distance = hamming(root, target)

        # target に到達できる step 候補を作る
        # ハイパーキューブでは偶奇制約があるので、
        # target_step と distance の偶奇は一致する必要がある
        if distance == 0:
            possible_target_steps = [0]
        else:
            possible_target_steps = [
                step for step in range(distance, self.PL + 1)
                if step % 2 == distance % 2
            ]

        if not possible_target_steps:
            raise ValueError("指定された PL では target を通るパスを作れません")

        max_retry = 1000

        for _ in range(max_retry):

            if target_step is None:
                selected_target_step = random.choice(possible_target_steps)
            else:
                selected_target_step = target_step

                if selected_target_step not in possible_target_steps:
                    raise ValueError("target_step が距離・偶奇制約を満たしていません")

            path: list[str] = []
            visited: set[str] = set()

            current_point = root
            path.append(current_point)
            visited.add(current_point)

            success = True

            # =========================
            # target に到達するまで
            # =========================
            while len(path) - 1 < selected_target_step:

                current_step = len(path) - 1
                remaining_to_target = selected_target_step - current_step

                candidates: list[str] = []

                for bit in range(self.Bit):
                    next_point = flip_point(current_point, bit)

                    if next_point in visited:
                        continue

                    next_remaining = remaining_to_target - 1
                    dist_to_target = hamming(next_point, target)

                    # 残りステップ数で target に届かないなら除外
                    if dist_to_target > next_remaining:
                        continue

                    # 偶奇が合わないなら除外
                    if dist_to_target % 2 != next_remaining % 2:
                        continue

                    # target_step より前に target に着くのは禁止
                    if next_point == target and next_remaining != 0:
                        continue

                    candidates.append(next_point)

                if not candidates:
                    success = False
                    break

                current_point = random.choice(candidates)
                path.append(current_point)
                visited.add(current_point)

            if not success:
                continue

            # 念のため確認
            if current_point != target:
                continue

            # =========================
            # target 到達後
            # =========================
            while len(path) - 1 < self.PL:

                candidates: list[str] = []

                for bit in range(self.Bit):
                    next_point = flip_point(current_point, bit)

                    if next_point in visited:
                        continue

                    candidates.append(next_point)

                if not candidates:
                    success = False
                    break

                current_point = random.choice(candidates)
                path.append(current_point)
                visited.add(current_point)

            if not success:
                continue

            self.pathlist = path
            return path

        raise ValueError("条件を満たす simple path の生成に失敗しました")

    def get_random_data(self) -> list[str]:
        keys_list: list[int] = list(self.pm)
        random_key: int = random.choice(keys_list)
        #print(f"block address = {random_key}")
        return self.get_data(random_key)

    def shuffle(self, blocks: list[datablock]) -> dict[str, bucket]:
        if not self.pathlist:
            raise ValueError("get_data or get_random_data must be called before shuffle")

        shuffled: dict[str, bucket] = {}

        all_blocks: list[datablock] = []


        for position in self.pathlist:
            shuffled[position] = bucket(self.Z)

        for block in blocks:
            all_blocks.append(block)

        for block in self.stash:
            all_blocks.append(block)        
        
        key_list = list(shuffled.keys())
        new_stash: list[datablock] = []

        for block in all_blocks:
            available_keys = [
                key for key in key_list
                if len(shuffled[key].value) < self.Z
            ]

            if not available_keys:
                new_stash.append(block)
                self.pm[block.addr] = None
                continue

            if self.accessblock == block.addr:
                sorted_list = sorted(available_keys,key = lambda x: x.count("1"))
                key = sorted_list[0]
            else:
                key = random.choice(available_keys)
            shuffled[key].setblock(block)
            self.pm[block.addr] = key

        self.stash = new_stash

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

def plot_bucket_access_distribution(
    random_log: list[str],
    biased_log: list[str],
    bit: int,
    output_path: str = "cube_oram_bucket_access_distribution.png",
) -> None:
    import matplotlib.pyplot as plt

    random_distribution = to_distribution(random_log)
    biased_distribution = to_distribution(biased_log)
    bucket_addresses = [format(i, f"0{bit}b") for i in range(2**bit)]
    x_values = list(range(len(bucket_addresses)))

    random_values = [
        random_distribution.get(address, 0.0)
        for address in bucket_addresses
    ]
    biased_values = [
        biased_distribution.get(address, 0.0)
        for address in bucket_addresses
    ]
    diff_values = [
        biased_value - random_value
        for random_value, biased_value in zip(random_values, biased_values)
    ]

    fig, (ax_distribution, ax_diff) = plt.subplots(
        2,
        1,
        figsize=(14, 8),
        sharex=True,
        gridspec_kw={"height_ratios": [2, 1]},
    )

    bar_width = 0.42
    ax_distribution.bar(
        [x - bar_width / 2 for x in x_values],
        random_values,
        width=bar_width,
        label="random access",
        alpha=0.8,
    )
    ax_distribution.bar(
        [x + bar_width / 2 for x in x_values],
        biased_values,
        width=bar_width,
        label="biased access",
        alpha=0.8,
    )
    ax_distribution.set_ylabel("Access probability")
    ax_distribution.set_title("Cube ORAM bucket access distribution")
    ax_distribution.legend()
    ax_distribution.grid(axis="y", linestyle="--", linewidth=0.5, alpha=0.6)

    diff_colors = [
        "tab:red" if value >= 0 else "tab:blue"
        for value in diff_values
    ]
    ax_diff.bar(x_values, diff_values, color=diff_colors, alpha=0.85)
    ax_diff.axhline(0, color="black", linewidth=0.8)
    ax_diff.set_ylabel("Biased - random")
    ax_diff.set_xlabel("Bucket address")
    ax_diff.grid(axis="y", linestyle="--", linewidth=0.5, alpha=0.6)

    tick_step = max(1, len(bucket_addresses) // 32)
    tick_positions = x_values[::tick_step]
    ax_diff.set_xticks(tick_positions)
    ax_diff.set_xticklabels(
        [bucket_addresses[i] for i in tick_positions],
        rotation=90,
        fontsize=8,
    )

    fig.tight_layout()
    fig.savefig(output_path, dpi=200)
    plt.close(fig)

    top_differences = sorted(
        zip(bucket_addresses, random_values, biased_values, diff_values),
        key=lambda item: abs(item[3]),
        reverse=True,
    )[:10]

    print(f"saved bucket access distribution graph: {output_path}")
    print("top bucket access differences:")
    for address, random_value, biased_value, diff_value in top_differences:
        print(
            f"{address}: random={random_value:.8f}, "
            f"biased={biased_value:.8f}, diff={diff_value:+.8f}"
        )

N: int = 128
Bit: int = 7
Z: int = 4
PL: int = 10

Window: int = 100

pm: dict[int, PMPosition] = {}
stash: list[datablock] = []

cube = ORAMcube(Bit,Z,PL)


for i in range(1, N + 1):
    addr: int = i
    position: str = format(random.randrange(2**Bit), f"0{Bit}b")
    data: str = str(addr)

    block: datablock = datablock(addr,data)

    success: bool = cube.set_block(position,block)

    if success:
        pm[addr] = position
    else:
        stash.append(block)
        pm[addr] = None


#print(cube)
#print(stash)

#print(pm)

cube1: ORAMcube = copy.deepcopy(cube)
cube2: ORAMcube = copy.deepcopy(cube)

pm1: dict[int, PMPosition] = copy.deepcopy(pm)
pm2: dict[int, PMPosition] = copy.deepcopy(pm)

stash1: list[datablock] = copy.deepcopy(stash)
stash2: list[datablock] = copy.deepcopy(stash)

oram_server1: server = server(cube1)
oram_client1: client = client(pm1,stash1,Bit,Z,PL,Window)

oram_server2: server = server(cube2)
oram_client2: client = client(pm2,stash2,Bit,Z,PL,Window)

# random workflow--------------------------------------------------------------

for i in range(100000):
    oram_client1.counter = oram_server1.give_counter()
    pathlist: list[str] = oram_client1.get_random_data()
    #print(pathlist)
    datalist: list[datablock] = oram_server1.getpath(pathlist)
    #print(datalist)
    shuffled: dict[str,bucket] = oram_client1.shuffle(datalist)
    #print(shuffled)
    oram_server1.reallocation(shuffled)

# fixed address workflow--------------------------------------------------------------

for i in range(100000):
    oram_client2.counter = oram_server2.give_counter()
    
    if i > -1:
        pathlist: list[str] = oram_client2.get_data(10)
    else:
        pathlist: list[str] = oram_client2.get_random_data()
    datalist: list[datablock] = oram_server2.getpath(pathlist)
    #print(datalist)
    shuffled: dict[str,bucket] = oram_client2.shuffle(datalist)
    #print(shuffled)
    oram_server2.reallocation(shuffled)



bucket_distribution1 = to_distribution(oram_server1.cube.bucket_log)
bucket_distribution2 = to_distribution(oram_server2.cube.bucket_log)

print("statistical distance: bucket")
print(statistical_distance(bucket_distribution1, bucket_distribution2))

plot_bucket_access_distribution(
    oram_server1.cube.bucket_log,
    oram_server2.cube.bucket_log,
    Bit,
)
