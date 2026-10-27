# Lock / Value / Array / Event / Semaphore shared across processes.
import multiprocessing as mp


def bump(v, lock, n):
    for _ in range(n):
        with lock:
            v.value += 1


def fill(a):
    for i in range(len(a)):
        a[i] = i * i


def setter(ev, sem):
    sem.acquire()
    ev.set()


def holder(lock, ev):
    lock.acquire()
    ev.set()


if __name__ == "__main__":
    v = mp.Value("i", 0)
    lock = mp.Lock()
    ps = [mp.Process(target=bump, args=(v, lock, 100)) for _ in range(3)]
    for p in ps:
        p.start()
    for p in ps:
        p.join()
    print("value", v.value)

    d = mp.Value("d", 1.5, lock=False)
    print(d.value)

    a = mp.Array("i", 6)
    p = mp.Process(target=fill, args=(a,))
    p.start()
    p.join()
    print("array", list(a))

    ev = mp.Event()
    sem = mp.Semaphore(1)
    p = mp.Process(target=setter, args=(ev, sem))
    p.start()
    print("event", ev.wait(30))
    p.join()
    print("sem taken by child:", not sem.acquire(False))

    lk = mp.Lock()
    ev2 = mp.Event()
    p = mp.Process(target=holder, args=(lk, ev2))
    p.start()
    ev2.wait(30)
    p.join()
    print("held by dead child:", lk.acquire(False))
