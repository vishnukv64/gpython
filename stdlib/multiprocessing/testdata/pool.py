# Pool(2) over real processes.
import multiprocessing as mp


def square(x):
    return x * x


def add(a, b):
    return a + b


def bad(x):
    if x == 3:
        raise ValueError("bad 3")
    return x


if __name__ == "__main__":
    with mp.Pool(2) as pool:
        print(pool.map(square, range(10)))
        print(pool.starmap(add, [(1, 2), (3, 4)]))
        print(list(pool.imap(square, range(5))))
        print(sorted(pool.imap_unordered(square, range(5))))
        print(pool.apply(square, (7,)))
        print(pool.apply_async(square, (8,)).get(timeout=30))
        try:
            pool.map(bad, range(5))
        except ValueError as e:
            print("error:", e)
