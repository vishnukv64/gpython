# The spawn contract: the child re-imports this file as __mp_main__, so code
# outside the __main__ guard runs once per process.
import multiprocessing as mp
import os

print("module level in", __name__, flush=True)


def f():
    print("child sees", __name__, flush=True)


if __name__ == "__main__":
    p = mp.Process(target=f)
    p.start()
    p.join()
    print("done", p.exitcode)
