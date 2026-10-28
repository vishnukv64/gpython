"""Disassembler of gpython bytecode.

gpython compiles to its own instruction set (opcodes from vm/opcodes.go: one
opcode byte, then a 16-bit little-endian argument for opcodes from
HAVE_ARGUMENT up, widened by EXTENDED_ARG), not to CPython's, so CPython's
dis.py cannot run here.  This module offers dis's public API over gpython's
code objects: opname, opmap, dis, distb, disassemble, get_instructions,
Bytecode, findlabels, findlinestarts, code_info, show_code.

Two honest differences from CPython's dis:
  * the listing is of gpython's opcodes, so it does not match CPython's;
  * gpython has no function prologue, but code written for CPython 3.11+
    finds a function's first executable line as "the instruction after
    RESUME".  get_instructions therefore yields a pseudo RESUME at offset 0
    whose opcode is RESUME (256, outside the byte range); it takes no space
    in co_code.
"""

import collections
import io
import sys
import types

__all__ = ["code_info", "dis", "disassemble", "distb", "disco", "findlinestarts",
           "findlabels", "show_code", "get_instructions", "Instruction",
           "Bytecode", "opname", "opmap", "cmp_op", "hasconst", "hasname",
           "hasjrel", "hasjabs", "haslocal", "hasfree", "hascompare",
           "HAVE_ARGUMENT", "EXTENDED_ARG", "stack_effect"]

opmap = {}
opname = ['<%r>' % (op,) for op in range(256)]
for _name, _num in _vm_ops:
    opmap[_name] = _num
    opname[_num] = _name
del _name, _num
RESUME = 256

cmp_op = ('<', '<=', '==', '!=', '>', '>=', 'in', 'not in', 'is', 'is not',
          'exception match', 'BAD')


def _ops(*names):
    return [opmap[n] for n in names if n in opmap]


hasconst = _ops('LOAD_CONST')
hasname = _ops('STORE_NAME', 'DELETE_NAME', 'STORE_ATTR', 'DELETE_ATTR', 'STORE_GLOBAL',
               'DELETE_GLOBAL', 'LOAD_NAME', 'LOAD_ATTR', 'IMPORT_NAME', 'IMPORT_FROM',
               'LOAD_GLOBAL')
hasjrel = _ops('FOR_ITER', 'JUMP_FORWARD', 'SETUP_LOOP', 'SETUP_EXCEPT', 'SETUP_FINALLY',
               'SETUP_WITH', 'SETUP_ASYNC_WITH')
hasjabs = _ops('JUMP_IF_FALSE_OR_POP', 'JUMP_IF_TRUE_OR_POP', 'JUMP_ABSOLUTE',
               'POP_JUMP_IF_FALSE', 'POP_JUMP_IF_TRUE', 'CONTINUE_LOOP')
haslocal = _ops('LOAD_FAST', 'STORE_FAST', 'DELETE_FAST')
hasfree = _ops('LOAD_CLOSURE', 'LOAD_DEREF', 'STORE_DEREF', 'DELETE_DEREF', 'LOAD_CLASSDEREF')
hascompare = _ops('COMPARE_OP')

COMPILER_FLAG_NAMES = {1: "OPTIMIZED", 2: "NEWLOCALS", 4: "VARARGS", 8: "VARKEYWORDS", 16: "NESTED",
                       32: "GENERATOR", 64: "NOFREE", 128: "COROUTINE", 256: "ITERABLE_COROUTINE",
                       512: "ASYNC_GENERATOR"}

Positions = collections.namedtuple("Positions", ["lineno", "end_lineno", "col_offset", "end_col_offset"],
                                   defaults=[None] * 4)

_Instruction = collections.namedtuple("_Instruction",
    "opname opcode arg argval argrepr offset start_offset starts_line line_number "
    "label is_jump_target positions cache_info")


class Instruction(_Instruction):
    """Details for a bytecode operation.

    opname/opcode/arg are the instruction; argval its resolved argument and
    argrepr a readable form of it; offset its position in co_code (a jump
    target is an offset too); starts_line is True when it begins a source
    line, line_number that line; positions has lineno only (gpython keeps
    no columns).
    """
    __slots__ = ()

    @property
    def oparg(self):
        return self.arg

    @property
    def baseopname(self):
        return self.opname

    @property
    def baseopcode(self):
        return self.opcode

    @property
    def jump_target(self):
        return self.argval if self.opcode in hasjrel or self.opcode in hasjabs else None

    def _disassemble(self, lineno_width=3, mark_as_current=False, offset_width=4):
        fields = []
        if lineno_width:
            if self.starts_line:
                fields.append(repr(self.line_number).rjust(lineno_width))
            else:
                fields.append(' ' * lineno_width)
        fields.append('-->' if mark_as_current else '   ')
        fields.append('>>' if self.is_jump_target else '  ')
        fields.append(repr(self.offset).rjust(offset_width))
        fields.append(self.opname.ljust(20))
        if self.arg is not None:
            fields.append(repr(self.arg).rjust(5))
            if self.argrepr:
                fields.append('(' + self.argrepr + ')')
        return ' '.join(fields).rstrip()


def _get_code_object(x):
    """Helper to handle methods, functions, generators, strings and raw code objects"""
    if hasattr(x, '__func__'):
        x = x.__func__
    if hasattr(x, '__code__'):
        x = x.__code__
    elif hasattr(x, 'gi_code'):
        x = x.gi_code
    elif hasattr(x, 'ag_code'):
        x = x.ag_code
    elif hasattr(x, 'cr_code'):
        x = x.cr_code
    if isinstance(x, str):
        x = compile(x, "<disassembly>", "exec")
    if hasattr(x, 'co_code'):
        return x
    raise TypeError("don't know how to disassemble %s objects" % type(x).__name__)


def _unpack_opargs(code):
    """Yield (offset, start_offset, op, arg) for each instruction, EXTENDED_ARG
    prefixes included (with their own argument); arg is None below HAVE_ARGUMENT."""
    extended_arg = 0
    i = 0
    n = len(code)
    start = 0
    while i < n:
        op = code[i]
        offset = i
        i += 1
        if op >= HAVE_ARGUMENT:
            arg = code[i] | code[i + 1] << 8 | extended_arg
            i += 2
            extended_arg = (arg << 16) if op == EXTENDED_ARG else 0
        else:
            arg = None
            extended_arg = 0
        yield (offset, start, op, arg)
        start = i if op != EXTENDED_ARG else start


def _get_const_info(const_index, const_list):
    argval = const_list[const_index]
    return argval, repr(argval)


def _get_name_info(name_index, name_list):
    argval = name_list[name_index]
    return argval, argval


def findlabels(code):
    """Detect all offsets in a byte code which are jump targets."""
    labels = []
    for offset, _, op, arg in _unpack_opargs(code):
        if arg is not None:
            if op in hasjrel:
                label = offset + 3 + arg
            elif op in hasjabs:
                label = arg
            else:
                continue
            if label not in labels:
                labels.append(label)
    return labels


def _line_table(code):
    """[(offset, line)]: the line each run of instructions belongs to, from
    co_lnotab, whose line deltas are signed (a loop exit goes back to its header)."""
    lnotab = code.co_lnotab
    line = code.co_firstlineno
    addr = 0
    table = [(0, line)]
    for i in range(0, len(lnotab), 2):
        addr += lnotab[i]
        delta = lnotab[i + 1]
        line += delta - 256 if delta >= 128 else delta
        if table[-1][0] == addr:
            table[-1] = (addr, line)
        elif table[-1][1] != line:
            table.append((addr, line))
    return table


def findlinestarts(code):
    """Find the offsets in a byte code which are start of lines in the source.

    Generate pairs (offset, lineno)."""
    code = _get_code_object(code)
    if not len(code.co_code):
        return
    yield from _line_table(code)


def _get_instructions_bytes(code, varname_from_oparg=None, names=None, co_consts=None,
                            linestarts=None, line_offset=0, cells=None, free=None):
    labels = set(findlabels(code))
    for offset, start, op, arg in _unpack_opargs(code):
        starts_line = linestarts.get(offset) if linestarts else None
        argval = None
        argrepr = ''
        if arg is not None:
            argval = arg
            if op in hasconst:
                argval, argrepr = _get_const_info(arg, co_consts)
            elif op in hasname:
                argval, argrepr = _get_name_info(arg, names)
            elif op in hasjrel:
                argval = offset + 3 + arg
                argrepr = "to " + repr(argval)
            elif op in hasjabs:
                argval = arg
                argrepr = "to " + repr(argval)
            elif op in haslocal:
                argval, argrepr = _get_name_info(arg, varname_from_oparg)
            elif op in hascompare:
                argval = cmp_op[arg] if arg < len(cmp_op) else arg
                argrepr = str(argval)
            elif op in hasfree:
                argval, argrepr = _get_name_info(arg, cells)
        yield (offset, start, op, arg, argval, argrepr, starts_line, offset in labels)


def get_instructions(x, *, first_line=None, show_caches=False, adaptive=False):
    """Iterator for the opcodes in methods, functions or code

    Generates a series of Instruction named tuples giving the details of
    each operations in the supplied code, preceded by the pseudo RESUME
    described in the module docstring.
    """
    co = _get_code_object(x)
    line_offset = 0
    if first_line is not None:
        line_offset = first_line - co.co_firstlineno
    return _get_instructions(co, line_offset)


def _get_instructions(co, line_offset=0):
    table = _line_table(co)
    linestarts = {off: line for off, line in findlinestarts(co)}
    cells = co.co_cellvars + co.co_freevars
    yield Instruction('RESUME', RESUME, 0, 0, '', 0, 0, False, co.co_firstlineno + line_offset,
                      None, False, Positions(co.co_firstlineno + line_offset), None)
    cur = table[0][1] if table else co.co_firstlineno
    ti = 0
    for (offset, start, op, arg, argval, argrepr, starts_line, is_target) in _get_instructions_bytes(
            co.co_code, co.co_varnames, co.co_names, co.co_consts, linestarts, line_offset, cells):
        while ti < len(table) and table[ti][0] <= offset:
            cur = table[ti][1]
            ti += 1
        line = cur + line_offset
        yield Instruction(opname[op], op, arg, argval, argrepr, offset, start,
                          starts_line is not None, line, None, is_target, Positions(line), None)


def _disassemble_recursive(co, *, file=None, depth=None, **kwargs):
    disassemble(co, file=file, **kwargs)
    if depth is None or depth > 0:
        if depth is not None:
            depth = depth - 1
        for x in co.co_consts:
            if hasattr(x, 'co_code'):
                print(file=file)
                print("Disassembly of %r:" % (x,), file=file)
                _disassemble_recursive(x, file=file, depth=depth, **kwargs)


def disassemble(co, lasti=-1, *, file=None, show_caches=False, adaptive=False, **kwargs):
    """Disassemble a code object."""
    co = _get_code_object(co)
    instrs = [i for i in get_instructions(co) if i.opcode != RESUME]
    maxlineno = max([i.line_number for i in instrs] or [0])
    lineno_width = len(str(maxlineno)) if maxlineno >= 1000 else 3
    offset_width = len(str(max(len(co.co_code) - 1, 0))) if len(co.co_code) >= 10000 else 4
    first = True
    for instr in instrs:
        if instr.starts_line and not first:
            print(file=file)
        first = False
        print(instr._disassemble(lineno_width, instr.offset == lasti, offset_width), file=file)


def dis(x=None, *, file=None, depth=None, show_caches=False, adaptive=False, **kwargs):
    """Disassemble classes, methods, functions, and other compiled objects.

    With no argument, disassemble the last traceback."""
    if x is None:
        distb(file=file)
        return
    if hasattr(x, '__func__'):
        x = x.__func__
    if hasattr(x, '__code__'):
        x = x.__code__
    if hasattr(x, '__dict__'):
        items = sorted(x.__dict__.items())
        for name, x1 in items:
            if isinstance(x1, (types.FunctionType, types.CodeType, classmethod, staticmethod)):
                print("Disassembly of %s:" % name, file=file)
                try:
                    dis(x1, file=file, depth=depth)
                except TypeError as msg:
                    print("Sorry:", msg, file=file)
                print(file=file)
    elif hasattr(x, 'co_code'):
        _disassemble_recursive(x, file=file, depth=depth)
    elif isinstance(x, (bytes, bytearray)):
        for offset, start, op, arg in _unpack_opargs(x):
            print(repr(offset).rjust(4), opname[op].ljust(20), '' if arg is None else repr(arg).rjust(5), file=file)
    elif isinstance(x, str):
        _disassemble_recursive(compile(x, "<disassembly>", "exec"), file=file, depth=depth)
    else:
        raise TypeError("don't know how to disassemble %s objects" % type(x).__name__)


def distb(tb=None, *, file=None, show_caches=False, adaptive=False):
    """Disassemble a traceback (default: last traceback)."""
    if tb is None:
        try:
            if hasattr(sys, 'last_exc'):
                tb = sys.last_exc.__traceback__
            else:
                tb = sys.last_traceback
        except AttributeError:
            raise RuntimeError("no last traceback to disassemble") from None
        while tb.tb_next:
            tb = tb.tb_next
    disassemble(tb.tb_frame.f_code, tb.tb_lasti, file=file)


disco = disassemble


def _format_code_info(co):
    lines = []
    lines.append("Name:              %s" % co.co_name)
    lines.append("Filename:          %s" % co.co_filename)
    lines.append("Argument count:    %s" % co.co_argcount)
    lines.append("Positional-only arguments: %s" % co.co_posonlyargcount)
    lines.append("Kw-only arguments: %s" % co.co_kwonlyargcount)
    lines.append("Number of locals:  %s" % co.co_nlocals)
    lines.append("Stack size:        %s" % co.co_stacksize)
    lines.append("Flags:             %s" % pretty_flags(co.co_flags))
    if co.co_consts:
        lines.append("Constants:")
        for i_c in enumerate(co.co_consts):
            lines.append("%4d: %r" % i_c)
    if co.co_names:
        lines.append("Names:")
        for i_n in enumerate(co.co_names):
            lines.append("%4d: %s" % i_n)
    if co.co_varnames:
        lines.append("Variable names:")
        for i_n in enumerate(co.co_varnames):
            lines.append("%4d: %s" % i_n)
    if co.co_freevars:
        lines.append("Free variables:")
        for i_n in enumerate(co.co_freevars):
            lines.append("%4d: %s" % i_n)
    if co.co_cellvars:
        lines.append("Cell variables:")
        for i_n in enumerate(co.co_cellvars):
            lines.append("%4d: %s" % i_n)
    return "\n".join(lines)


def pretty_flags(flags):
    """Return pretty representation of code flags."""
    names = []
    for i in range(32):
        flag = 1 << i
        if flags & flag:
            names.append(COMPILER_FLAG_NAMES.get(flag, hex(flag)))
            flags ^= flag
            if not flags:
                break
    else:
        names.append(hex(flags))
    return ", ".join(names)


def code_info(x):
    """Formatted details of methods, functions, or code."""
    return _format_code_info(_get_code_object(x))


def show_code(co, *, file=None):
    """Print details of methods, functions, or code to *file*."""
    print(code_info(co), file=file)


def stack_effect(opcode, oparg=None, *, jump=None):
    raise NotImplementedError("dis.stack_effect: gpython does not track per-opcode stack effects")


class Bytecode:
    """The bytecode operations of a piece of code

    Instantiate this with a function, method, other compiled object, string of
    code, or a code object (as returned by compile()).

    Iterating over this yields the bytecode operations as Instruction instances.
    """
    def __init__(self, x, *, first_line=None, current_offset=None, show_caches=False,
                 adaptive=False, show_offsets=False, show_positions=False):
        self.codeobj = co = _get_code_object(x)
        if first_line is None:
            self.first_line = co.co_firstlineno
            self._line_offset = 0
        else:
            self.first_line = first_line
            self._line_offset = first_line - co.co_firstlineno
        self._original_object = x
        self.current_offset = current_offset

    def __iter__(self):
        return _get_instructions(self.codeobj, self._line_offset)

    def __repr__(self):
        return "{}({!r})".format(self.__class__.__name__, self._original_object)

    @classmethod
    def from_traceback(cls, tb, *, show_caches=False, adaptive=False):
        """ Construct a Bytecode from the given traceback """
        while tb.tb_next:
            tb = tb.tb_next
        return cls(tb.tb_frame.f_code, current_offset=tb.tb_lasti)

    def info(self):
        """Return formatted information about the code object."""
        return _format_code_info(self.codeobj)

    def dis(self):
        """Return a formatted view of the bytecode operations."""
        with io.StringIO() as output:
            lasti = self.current_offset if self.current_offset is not None else -1
            disassemble(self.codeobj, lasti, file=output)
            return output.getvalue()


def _find_imports(co):
    """Find import statements in the code

    Generate triplets (name, level, fromlist) where
    name is the imported module and level, fromlist are
    the corresponding args to __import__.
    """
    IMPORT_NAME = opmap['IMPORT_NAME']
    LOAD_CONST = opmap['LOAD_CONST']
    consts = co.co_consts
    names = co.co_names
    opargs = [(op, arg) for _, _, op, arg in _unpack_opargs(co.co_code) if op != EXTENDED_ARG]
    for i, (op, oparg) in enumerate(opargs):
        if op == IMPORT_NAME and i >= 2:
            from_op = opargs[i - 1]
            level_op = opargs[i - 2]
            if from_op[0] == LOAD_CONST and level_op[0] == LOAD_CONST:
                yield (names[oparg], consts[level_op[1]], consts[from_op[1]])


def _find_store_names(co):
    """Find names of variables which are written in the code

    Generate sequence of strings
    """
    STORE_OPS = {opmap['STORE_NAME'], opmap['STORE_GLOBAL']}
    names = co.co_names
    for _, _, op, arg in _unpack_opargs(co.co_code):
        if op in STORE_OPS:
            yield names[arg]


def _test():
    """Simple test program to disassemble a file."""
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('infile', nargs='?', default='-')
    args = parser.parse_args()
    if args.infile == '-':
        source = sys.stdin.buffer.read()
        fn = "<stdin>"
    else:
        with open(args.infile, 'rb') as infile:
            source = infile.read()
        fn = args.infile
    code = compile(source, fn, "exec")
    dis(code)


if __name__ == "__main__":
    _test()
