import io, os, re, sys, pdb, bdb

HERE = os.path.dirname(os.path.abspath(__file__))

def target(n):
    total = 0
    for i in range(n):
        total += helper(i)
    result = total * 2
    return result

def helper(x):
    y = x + 1
    return y * y

def run(label, commands, fn=target, *args):
    out = io.StringIO()
    p = pdb.Pdb(stdin=io.StringIO(commands), stdout=out, nosigint=True, readrc=False)
    p.use_rawinput = False
    try:
        r = p.runcall(fn, *args)
    except bdb.BdbQuit:
        r = "<quit>"
    text = out.getvalue().replace(HERE + os.sep, "").replace(HERE, "")
    text = re.sub(r"[^\s(]*bdb\.py\(\d+\)runcall\(\)", "BDB()", text)
    text = re.sub(r"[^\s(]*pdbgold\.py", "pdb_gold.py", text)
    # the library file is not on disk in gpython, so its source line is not shown
    text = text.replace("-> res = func(*args, **kwds)" + chr(10), "")
    print("==", label, repr(r))
    print(text, end="" if text.endswith("\n") else "\n")

run("step/next/p", "s\nn\np n\np total\nn\nn\nn\np total\nc\n", target, 3)
run("break/continue", "b helper\nc\np x\nc\np x\nbt\nc\nc\n", target, 3)
run("break lineno + cond", "b 14, x == 1\nc\np x, y\nu\np i\nd\nc\n", target, 3)
run("where/up/down", "b helper\nc\nw\nu\nl\nd\nq\n", target, 2)
run("return/until", "s\ns\ns\nr\nunt\nc\n", target, 2)
run("args/pp", "b helper\nc\nargs\npp {'a': [1, 2], 'b': x}\nwhatis x\nc\nc\n", target, 2)
run("tbreak/disable/enable", "tbreak helper\nb 10\nb\ndisable 2\nc\nc\n", target, 2)
run("display/undisplay", "b helper\nc\ndisplay x\nn\nundisplay\nn\nc\nc\n", target, 2)
run("quit", "s\nq\n", target, 2)
run("error cmds", "frobnicate\np undefined_name\np 1/0\nc\n", target, 1)
run("list", "l 5, 9\nll helper\nc\n", target, 1)
