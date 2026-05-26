from __future__ import annotations

from collections import Counter
import random


BIT: int = 8
PL: int = 12
ACCESS_COUNT: int = 100000
SEED: int | None = 10
GRAPH_PATH: str = "cube_oram_path_selection_distribution.png"


def hamming(a: str, b: str) -> int:
    return sum(x != y for x, y in zip(a, b))


def flip_point(point: str, bit: int) -> str:
    point_list = list(point)
    point_list[bit] = "1" if point_list[bit] == "0" else "0"
    return "".join(point_list)


def random_bucket(bit: int) -> str:
    return format(random.randrange(2**bit), f"0{bit}b")


def select_path(bit: int, pl: int, target: str, target_step: int | None = None) -> list[str]:
    root = "0" * bit
    distance = hamming(root, target)

    if distance == 0:
        possible_target_steps = [0]
    else:
        possible_target_steps = [
            step for step in range(distance, pl + 1)
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

        while len(path) - 1 < selected_target_step:
            current_step = len(path) - 1
            remaining_to_target = selected_target_step - current_step
            candidates: list[str] = []

            for bit_index in range(bit):
                next_point = flip_point(current_point, bit_index)

                if next_point in visited:
                    continue

                next_remaining = remaining_to_target - 1
                dist_to_target = hamming(next_point, target)

                if dist_to_target > next_remaining:
                    continue

                if dist_to_target % 2 != next_remaining % 2:
                    continue

                if next_point == target and next_remaining != 0:
                    continue

                candidates.append(next_point)

            if not candidates:
                success = False
                break

            current_point = random.choice(candidates)
            path.append(current_point)
            visited.add(current_point)

        if not success or current_point != target:
            continue

        while len(path) - 1 < pl:
            candidates = [
                flip_point(current_point, bit_index)
                for bit_index in range(bit)
                if flip_point(current_point, bit_index) not in visited
            ]

            if not candidates:
                success = False
                break

            current_point = random.choice(candidates)
            path.append(current_point)
            visited.add(current_point)

        if success:
            return path

    raise ValueError("条件を満たす simple path の生成に失敗しました")


def print_bucket_table(
    bucket_counts: Counter[str],
    bit: int,
    pl: int,
    total_accesses: int,
) -> None:
    total_bucket_calls = total_accesses * (pl + 1)

    print(f"BIT={bit}, PL={pl}, accesses={total_accesses}")
    print()
    print("bucket,count,probability")

    for bucket in [format(i, f"0{bit}b") for i in range(2**bit)]:
        count = bucket_counts[bucket]
        probability = count / total_bucket_calls
        print(f"{bucket},{count},{probability:.8f}")


def plot_bucket_distribution(
    bucket_counts: Counter[str],
    bit: int,
    pl: int,
    total_accesses: int,
    output_path: str,
) -> None:
    import matplotlib.pyplot as plt

    buckets = [format(i, f"0{bit}b") for i in range(2**bit)]
    total_bucket_calls = total_accesses * (pl + 1)
    probabilities = [
        bucket_counts[bucket] / total_bucket_calls
        for bucket in buckets
    ]

    plt.figure(figsize=(14, 6))
    plt.bar(buckets, probabilities)
    plt.title("Cube ORAM path selection bucket distribution")
    plt.xlabel("Bucket")
    plt.ylabel("Probability")
    plt.xticks(rotation=90, fontsize=8)
    plt.grid(axis="y", linestyle="--", linewidth=0.5, alpha=0.6)
    plt.tight_layout()
    plt.savefig(output_path, dpi=200)
    plt.close()


def main() -> None:
    if SEED is not None:
        random.seed(SEED)

    bucket_counts: Counter[str] = Counter()

    for _ in range(ACCESS_COUNT):
        target = random_bucket(BIT)
        path = select_path(BIT, PL, target)
        bucket_counts.update(path)

    print_bucket_table(bucket_counts, BIT, PL, ACCESS_COUNT)
    plot_bucket_distribution(bucket_counts, BIT, PL, ACCESS_COUNT, GRAPH_PATH)
    print()
    print(f"saved graph: {GRAPH_PATH}")


if __name__ == "__main__":
    main()
