# dis is gpython-native: there is no CPython golden for the listing itself
# (the bytecode differs by design).  This checks the API contract that holds on
# both: opname/opmap agree, instructions carry offsets and lines, labels mark
# jump targets, Bytecode/code_info/findlinestarts work, and CPython-style
# users (pdb's find_first_executable_line, modulefinder) get what they need.
import dis, io

def f(a, b=2):
    for i in range(a):
        if i > b:
            print("big", i)
    return a

ins = list(dis.get_instructions(f))
assert ins[0].opname == "RESUME" and ins[0].positions.lineno == f.__code__.co_firstlineno
real = ins[1:]
assert all(dis.opmap[i.opname] == i.opcode and dis.opname[i.opcode] == i.opname for i in real)
assert [i.offset for i in real] == sorted(i.offset for i in real)
lines = [l for _, l in dis.findlinestarts(f)]
first = f.__code__.co_firstlineno
assert lines == [first + 1, first + 2, first + 3, first + 1, first + 4], lines
targets = {i.argval for i in real if i.jump_target is not None}
assert targets and all(i.is_jump_target for i in real if i.offset in targets)
assert any(i.opname == "LOAD_CONST" and i.argval == "big" for i in real)
assert any(i.opname == "LOAD_FAST" and i.argval == "a" for i in real)
out = io.StringIO()
dis.dis(f, file=out)
text = out.getvalue()
assert "FOR_ITER" in text and "(to " in text and "RETURN_VALUE" in text
assert text == dis.Bytecode(f).dis()
assert "Name:              f" in dis.code_info(f)
assert dis.opmap["STORE_NAME"] == dis.opmap["STORE_NAME"] and dis.HAVE_ARGUMENT <= dis.opmap["STORE_NAME"]
mod = compile("import os\nx = 1\n", "<m>", "exec")
assert list(dis._find_imports(mod)) == [("os", 0, None)]
assert list(dis._find_store_names(mod)) == ["os", "x"]
print("dis ok")
