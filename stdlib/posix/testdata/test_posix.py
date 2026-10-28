import os, sys
raw = __import__("posix" if os.name == "posix" else "nt")
print(raw.__name__ == ("posix" if os.name == "posix" else "nt"))
try:
    __import__("nt" if os.name == "posix" else "posix")
except ImportError as e:
    print("ImportError")
print(raw.getcwd() == os.getcwd(), raw.getpid() == os.getpid(), raw.error is OSError)
print(raw.stat(".").st_mode == os.stat(".").st_mode, type(raw.stat(".")).__name__)
print(raw.listdir(".") == os.listdir("."), raw.fspath("a/b") == "a/b")
print(raw.strerror(2) == os.strerror(2), raw.urandom(4).__len__())
print(isinstance(raw.environ, dict), all(isinstance(k, bytes if os.name == "posix" else str) for k in raw.environ))
for name in ("path", "walk", "makedirs", "getenv", "sep", "fsencode", "fdopen"):
    print(name, hasattr(raw, name))
print(raw.O_RDONLY, raw.O_WRONLY, raw.O_RDWR, raw.F_OK, raw.R_OK, raw.W_OK, raw.X_OK)
try: raw.stat("/definitely/not/here")
except FileNotFoundError as e: print("FileNotFoundError", e.errno)
