# gpython stub: its code objects are not CPython bytecode, so there is nothing
# to disassemble.  The module exists so that importing it (setuptools.depends
# does) works; the names it reads at call time fail loudly.
opmap = {"STORE_NAME": 90, "STORE_GLOBAL": 97, "LOAD_CONST": 100}
opname = {v: k for k, v in opmap.items()}
HAVE_ARGUMENT = 43


def _unsupported(*args, **kwargs):
    raise NotImplementedError("dis: gpython code objects cannot be disassembled")


dis = distb = disassemble = get_instructions = findlabels = Bytecode = _unsupported
