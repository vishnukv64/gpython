"""subprocess: run external programs.

Run with:  /tmp/gpy examples/stdlib/subprocess_demo.py

Covers run(), check_output(), call(), Popen(), getoutput() and
getstatusoutput(). Commands used here are the portable echo/true/false, so the
demo behaves the same on any POSIX system.

Interpreter notes:
  * CompletedProcess is returned, but its stdout/stderr are only populated when
    you pass stdout=subprocess.PIPE. `capture_output=True` returns an object
    whose .stdout is not filled in.
  * check_call()/check_output() on a failing command raise CalledProcessError
    with the message "Command returned non-zero exit status N."
"""

import subprocess

print("--- run(): the modern entry point ---")
completed = subprocess.run(["echo", "hello from a subprocess"])
print("returned type:  ", type(completed).__name__)
print("returncode:     ", completed.returncode)
print("the output above went straight to ours (stdout was not redirected)")

print()
print("--- capturing output ---")
captured = subprocess.run(["echo", "captured text"], stdout=subprocess.PIPE)
print("with stdout=PIPE, .stdout is bytes:", repr(captured.stdout))
text_captured = subprocess.run(["echo", "captured text"], stdout=subprocess.PIPE, text=True)
print("with text=True, .stdout is str:   ", repr(text_captured.stdout))
print()
print("--- capture_output=True does not fill .stdout here ---")
capped = subprocess.run(["echo", "hidden"], capture_output=True)
print("type:  ", type(capped).__name__)
print("stdout:", repr(capped.stdout))
print("(use stdout=PIPE instead, as above)")

print()
print("--- check_output() returns the output directly ---")
raw = subprocess.check_output(["echo", "direct output"])
print("bytes: ", repr(raw))
text = subprocess.check_output(["echo", "direct output"], text=True)
print("text:  ", repr(text))

print()
print("--- call() just gives you the exit status ---")
status = subprocess.call(["true"])
print("call(['true'])  ->", status)
status = subprocess.call(["false"])
print("call(['false']) ->", status, "(no exception is raised)")

print()
print("--- failures ---")
try:
    subprocess.check_output(["false"])
except subprocess.CalledProcessError as err:
    print("check_output(['false']) -> CalledProcessError:", err)
try:
    subprocess.check_call(["false"])
except subprocess.CalledProcessError as err:
    print("check_call(['false'])   -> CalledProcessError:", err)
print("CalledProcessError is a SubprocessError:", issubclass(subprocess.CalledProcessError, subprocess.SubprocessError))
try:
    subprocess.run(["definitely-not-a-real-command"])
except FileNotFoundError as err:
    print("running a missing program -> FileNotFoundError:", err)

print()
print("--- shell-style helpers return strings, not objects ---")
out = subprocess.getoutput("echo via the shell")
print("getoutput:       ", repr(out))
status, out = subprocess.getstatusoutput("exit 3")
print("getstatusoutput for a failing command:", status, repr(out))
status, out = subprocess.getstatusoutput("echo ok")
print("getstatusoutput for a passing command:", status, repr(out))

print()
print("--- Popen gives you the process handle ---")
process = subprocess.Popen(["echo", "from Popen"])
code = process.wait()
print("wait() returned:", code)

print()
print("--- constants and helpers ---")
print("PIPE:   ", subprocess.PIPE)
print("STDOUT: ", subprocess.STDOUT)
print("DEVNULL:", subprocess.DEVNULL)
print("list2cmdline(['a b', 'c']):", subprocess.list2cmdline(["a b", "c"]))
print("(quoting matters on Windows; on POSIX the list form is used directly)")

print()
print("--- a worked example: a pipeline of small tools ---")


def word_count(text):
    """Feed text to `wc -w` and read back the count."""
    process = subprocess.Popen(["wc", "-w"], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, text=True)
    out, _ = process.communicate(text)
    return int(out.strip())


def count_lines(text):
    process = subprocess.Popen(["wc", "-l"], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, text=True)
    out, _ = process.communicate(text)
    return int(out.strip())


SAMPLE = "one two three\nfour five\nsix\n"
print("sample text:", repr(SAMPLE))
print("wc -w count:", word_count(SAMPLE))
print("wc -l count:", count_lines(SAMPLE))
print("checked in Python: words =", len(SAMPLE.split()), " lines =", len(SAMPLE.strip().split("\n")))

print()
print("--- and a health check: did the command succeed? ---")


def health_check(command):
    result = subprocess.run(command, stdout=subprocess.PIPE, text=True)
    if result.returncode == 0:
        return "OK   %s -> %r" % (" ".join(command), result.stdout.strip())
    return "FAIL %s -> exit %d" % (" ".join(command), result.returncode)


for command in [["echo", "alive"], ["true"], ["false"]]:
    print("  " + health_check(command))
