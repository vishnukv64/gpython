# Queue and Pipe between real processes.
import multiprocessing as mp


def producer(q, n):
    for i in range(n):
        q.put(i * 10)
    q.put(None)


def echo(conn):
    while True:
        msg = conn.recv()
        if msg is None:
            break
        conn.send(("echo", msg))
    conn.close()


def one_way(w):
    w.send_bytes(b"raw bytes")
    w.close()


if __name__ == "__main__":
    q = mp.Queue()
    p = mp.Process(target=producer, args=(q, 4))
    p.start()
    got = []
    while True:
        x = q.get()
        if x is None:
            break
        got.append(x)
    p.join()
    print("queue", got, p.exitcode)

    parent, child = mp.Pipe()
    p = mp.Process(target=echo, args=(child,))
    p.start()
    for m in [1, "two", [3, 4], {"five": 5}]:
        parent.send(m)
        print(parent.recv())
    parent.send(None)
    p.join()
    print("pipe", p.exitcode)

    r, w = mp.Pipe(duplex=False)
    p = mp.Process(target=one_way, args=(w,))
    p.start()
    print(r.recv_bytes())
    p.join()

    sq = mp.SimpleQueue()
    sq.put("simple")
    print(sq.get())
