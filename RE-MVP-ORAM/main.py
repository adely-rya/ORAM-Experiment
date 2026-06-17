import math


def stirling2_table(n_max):
    S = [[0.0] * (n_max + 1) for _ in range(n_max + 1)]
    S[0][0] = 1.0

    for n in range(1, n_max + 1):
        for k in range(1, n + 1):
            S[n][k] = k * S[n - 1][k] + S[n - 1][k - 1]

    return S


def falling_factorial(n, k):
    result = 1.0
    for i in range(k):
        result *= (n - i)
    return result


def harmonic_number(n, alpha):
    return sum(i ** (-alpha) for i in range(1, n + 1))


def random_k_distribution(L, c):
    """
    c clients が 2^L 個の leaf を独立一様に選ぶときの
    distinct leaf 数 K の分布

        Pr[K=k]
        = ( (2^L)_k * S(c,k) ) / (2^L)^c
    """
    m = 2 ** L
    S = stirling2_table(c)

    dist = [0.0] * (c + 1)

    for k in range(1, c + 1):
        dist[k] = (
            falling_factorial(m, k)
            * S[c][k]
            / (m ** c)
        )

    return dist


def zipf_k_distribution(L, c, alpha):
    """
    アドレス総数 N = 2^(L-1)

    rank j が

        [2^d , 2^(d+1))

    に属するとき、そのアドレスは

        m = 2^((L-1)-d)

    個の leaf 候補を持つと仮定。

    Pr[K=k]
      = Σ_d P(rank∈bucket d)
            · Pr[K=k | m=2^((L-1)-d)]
    """

    N = 2 ** (L - 1)

    H = harmonic_number(N, alpha)
    S = stirling2_table(c)

    dist = [0.0] * (c + 1)

    for d in range(0, L):

        start = 2 ** d
        end = min(2 ** (d + 1), N + 1)

        if start > N:
            break

        rank_prob = sum(
            (j ** (-alpha)) / H
            for j in range(start, end)
        )

        m = 2 ** max((L - 1) - d, 0)

        for k in range(1, min(c, m) + 1):

            prob_k_given_d = (
                falling_factorial(m, k)
                * S[c][k]
                / (m ** c)
            )

            dist[k] += rank_prob * prob_k_given_d

    return dist


def total_variation_distance(p, q):
    n = max(len(p), len(q))

    tvd = 0.0

    for i in range(n):
        pi = p[i] if i < len(p) else 0.0
        qi = q[i] if i < len(q) else 0.0

        tvd += abs(pi - qi)

    return 0.5 * tvd


def print_distribution(name, dist):
    print(f"\n{name}")
    print("-" * 50)

    for k, p in enumerate(dist):
        if p > 1e-12:
            print(f"K={k:2d} : {p:.12f}")

    print(f"sum = {sum(dist):.12f}")


if __name__ == "__main__":

    L = 10
    c = 40

    # 80/20 に近い Zipf
    alpha = 1.1

    random_dist = random_k_distribution(L, c)
    zipf_dist = zipf_k_distribution(L, c, alpha)

    tvd = total_variation_distance(
        random_dist,
        zipf_dist
    )

    print(f"L = {L}")
    print(f"clients = {c}")
    print(f"addresses = {2 ** (L - 1)}")
    print(f"leaves = {2 ** L}")
    print(f"alpha = {alpha}")
    print()

    print_distribution("Random", random_dist)
    print_distribution("Zipf", zipf_dist)

    print("\n" + "=" * 50)
    print(f"TVD(Random, Zipf) = {tvd:.12f}")