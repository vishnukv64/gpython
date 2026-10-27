# concurrent.futures.ProcessPoolExecutor.
from concurrent.futures import ProcessPoolExecutor, as_completed


def square(x):
    return x * x


def fail(x):
    raise RuntimeError("no %d" % x)


if __name__ == "__main__":
    with ProcessPoolExecutor(max_workers=2) as ex:
        print(list(ex.map(square, range(8))))
        f = ex.submit(square, 12)
        print(f.result())
        print(sorted(x.result() for x in as_completed([ex.submit(square, i) for i in range(4)])))
        try:
            ex.submit(fail, 5).result()
        except RuntimeError as e:
            print("error:", e)
