# Process start/join/exitcode under spawn semantics.
import multiprocessing as mp
import os
import sys


def work(x, y=0):
    print("child", x, y, flush=True)


def fail():
    sys.exit(3)


def boom():
    raise ValueError("boom")


def pid_of(q):
    q.put(os.getpid())


if __name__ == "__main__":
    print(mp.get_start_method())
    p = mp.Process(target=work, args=(1,), kwargs={"y": 2}, name="worker")
    print(p.name, p.is_alive(), p.exitcode)
    p.start()
    p.join()
    print(p.is_alive(), p.exitcode)

    p = mp.Process(target=fail)
    p.start()
    p.join()
    print("exit", p.exitcode)

    p = mp.Process(target=boom)
    p.start()
    p.join()
    print("raised", p.exitcode)

    q = mp.Queue()
    p = mp.Process(target=pid_of, args=(q,))
    p.start()
    child = q.get()
    p.join()
    print("real process:", child != os.getpid(), p.pid == child)
