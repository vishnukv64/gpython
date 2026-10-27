import signal, os

got = []
def h(n, frame):
    got.append(signal.Signals(n).name)

print(signal.getsignal(signal.SIGUSR1) == signal.SIG_DFL)
old = signal.signal(signal.SIGUSR1, h)
print(old == signal.SIG_DFL, signal.getsignal(signal.SIGUSR1) is h)
os.kill(os.getpid(), signal.SIGUSR1)
x = 0
for i in range(1000):
    x += i
print(got)

print(signal.signal(signal.SIGUSR2, signal.SIG_IGN) == signal.SIG_DFL)
print(signal.getsignal(signal.SIGUSR2) == signal.SIG_IGN)
os.kill(os.getpid(), signal.SIGUSR2)

print(repr(signal.SIGINT), repr(signal.SIG_IGN))
print(signal.getsignal(signal.SIGINT) is signal.default_int_handler)
try:
    signal.raise_signal(signal.SIGINT)
    for i in range(100000):
        pass
    print("no KeyboardInterrupt")
except KeyboardInterrupt:
    print("KeyboardInterrupt")

try:
    signal.signal(signal.SIGKILL, h)
except OSError:
    print("OSError")
try:
    signal.signal(9999, h)
except ValueError as e:
    print(e)
print(signal.SIGKILL in signal.valid_signals())
