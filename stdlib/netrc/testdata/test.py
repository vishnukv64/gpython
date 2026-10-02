import netrc
import os
import tempfile

d = tempfile.mkdtemp()


def write(name, content):
    p = os.path.join(d, name)
    with open(p, "w") as f:
        f.write(content)
    return p


p = write("n",
          "machine example.com login u password p\n"
          "machine other.com login u2 account a2 password p2\n"
          "macdef init\ncd /tmp\n\n"
          "default login du password dp\n")

n = netrc.netrc(p)

# authenticators returns the (login, account, password) triple.
assert n.authenticators("example.com") == ("u", "", "p"), n.authenticators("example.com")
assert n.authenticators("other.com") == ("u2", "a2", "p2"), n.authenticators("other.com")
# An unknown host falls back to the default entry.
assert n.authenticators("nope.com") == ("du", "", "dp")
assert n.authenticators("x") == ("du", "", "dp")

# hosts holds every machine and the default.
assert sorted(n.hosts.keys()) == ["default", "example.com", "other.com"], sorted(n.hosts.keys())
assert n.hosts["example.com"] == ("u", "", "p")

# macros holds the macdef bodies, each as a list of lines.
assert sorted(n.macros.keys()) == ["init"]
assert n.macros["init"] == ["cd /tmp\n"], n.macros["init"]

# A netrc with no default returns None for an unknown host.
p2 = write("n2", "machine only.com login u password p\n")
n2 = netrc.netrc(p2)
assert n2.authenticators("only.com") == ("u", "", "p")
assert n2.authenticators("missing.com") is None

# A malformed file raises NetrcParseError, whose type is the one exported.
bad = write("bad", "machine\n")
try:
    netrc.netrc(bad)
except netrc.NetrcParseError:
    pass
else:
    raise AssertionError("malformed netrc did not raise NetrcParseError")

# A missing file raises FileNotFoundError.
try:
    netrc.netrc(os.path.join(d, "does-not-exist"))
except FileNotFoundError:
    pass
else:
    raise AssertionError("missing netrc did not raise FileNotFoundError")

# A bad follower token raises and the message never quotes a password.
b2 = write("b2", "machine a.com foo u\n")
try:
    netrc.netrc(b2)
except netrc.NetrcParseError as e:
    assert "password" not in str(e).lower(), str(e)
else:
    raise AssertionError("bad follower token did not raise NetrcParseError")

# $NETRC is honoured when no file is passed.
env = write("envnetrc", "machine env.com login eu password ep\n")
old = os.environ.get("NETRC")
os.environ["NETRC"] = env
try:
    ne = netrc.netrc()
    assert ne.authenticators("env.com") == ("eu", "", "ep")
finally:
    if old is None:
        del os.environ["NETRC"]
    else:
        os.environ["NETRC"] = old

print("netrc ok")
