import pty, os, tty, termios
m, s = pty.openpty()
print(os.isatty(m), os.isatty(s))
os.write(m, b"hi\n")
print(os.read(s, 10))
os.write(s, b"yo\n")
# The echo of "hi" and the "yo" line can arrive in separate reads (Linux).
got = b""
while not got.endswith(b"yo\r\n"):
    got += os.read(m, 64)
print(got)
print(pty.STDIN_FILENO, pty.STDOUT_FILENO, pty.STDERR_FILENO, pty.CHILD)
print(sorted(pty.__all__))
tty.setraw(s)
os.write(m, b"abc")
print(os.read(s, 3))
os.close(m); os.close(s)
