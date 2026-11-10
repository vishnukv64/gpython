# The Python half of gpython's ast: tree walking helpers over the node
# classes pynodes.go builds.  Adapted from the NodeVisitor family in CPython's
# ast.py, which cannot be imported here (it needs _ast).

def iter_fields(node):
    for field in node._fields:
        try:
            yield field, getattr(node, field)
        except AttributeError:
            pass


def iter_child_nodes(node):
    for name, field in iter_fields(node):
        if isinstance(field, AST):
            yield field
        elif isinstance(field, list):
            for item in field:
                if isinstance(item, AST):
                    yield item


def walk(node):
    from collections import deque
    todo = deque([node])
    while todo:
        node = todo.popleft()
        todo.extend(iter_child_nodes(node))
        yield node


def get_docstring(node, clean=True):
    if not isinstance(node, (AsyncFunctionDef, FunctionDef, ClassDef, Module)):
        raise TypeError("%r can't have docstrings" % node.__class__.__name__)
    if not(node.body and isinstance(node.body[0], Expr)):
        return None
    node = node.body[0].value
    if isinstance(node, Constant) and isinstance(node.value, str):
        text = node.value
    else:
        return None
    if clean:
        import inspect
        text = inspect.cleandoc(text)
    return text


class NodeVisitor(object):
    def visit(self, node):
        method = 'visit_' + node.__class__.__name__
        visitor = getattr(self, method, self.generic_visit)
        return visitor(node)

    def generic_visit(self, node):
        for field, value in iter_fields(node):
            if isinstance(value, list):
                for item in value:
                    if isinstance(item, AST):
                        self.visit(item)
            elif isinstance(value, AST):
                self.visit(value)


class NodeTransformer(NodeVisitor):
    def generic_visit(self, node):
        for field, old_value in iter_fields(node):
            if isinstance(old_value, list):
                new_values = []
                for value in old_value:
                    if isinstance(value, AST):
                        value = self.visit(value)
                        if value is None:
                            continue
                        elif not isinstance(value, AST):
                            new_values.extend(value)
                            continue
                    new_values.append(value)
                old_value[:] = new_values
            elif isinstance(old_value, AST):
                new_node = self.visit(old_value)
                if new_node is None:
                    delattr(node, field)
                else:
                    setattr(node, field, new_node)
        return node


# unparse covers expressions (what pyclbr needs for class bases); statements
# raise NotImplementedError.  CPython's is the full _Unparser in ast.py.
_BINOP = {'Add': '+', 'Sub': '-', 'Mult': '*', 'Div': '/', 'Modulo': '%', 'Pow': '**', 'LShift': '<<',
          'RShift': '>>', 'BitOr': '|', 'BitXor': '^', 'BitAnd': '&', 'FloorDiv': '//', 'MatMult': '@'}
_CMPOP = {'Eq': '==', 'NotEq': '!=', 'Lt': '<', 'LtE': '<=', 'Gt': '>', 'GtE': '>=', 'Is': 'is',
          'IsNot': 'is not', 'In': 'in', 'NotIn': 'not in'}
_UNARY = {'Invert': '~', 'Not': 'not ', 'UAdd': '+', 'USub': '-'}


def unparse(node):
    name = node.__class__.__name__
    u = unparse
    if name == 'Name':
        return node.id
    if name == 'Constant':
        return repr(node.value)
    if name == 'Attribute':
        return u(node.value) + '.' + node.attr
    if name == 'Call':
        args = [u(a) for a in node.args] + [u(k) for k in node.keywords]
        return u(node.func) + '(' + ', '.join(args) + ')'
    if name == 'keyword':
        return (node.arg + '=' if node.arg else '**') + u(node.value)
    if name == 'Starred':
        return '*' + u(node.value)
    if name == 'Subscript':
        return u(node.value) + '[' + u(node.slice) + ']'
    if name == 'Index':
        return u(node.value)
    if name == 'Slice':
        s = (u(node.lower) if node.lower else '') + ':' + (u(node.upper) if node.upper else '')
        return s + (':' + u(node.step) if node.step else '')
    if name == 'Tuple':
        if len(node.elts) == 1:
            return '(' + u(node.elts[0]) + ',)'
        return '(' + ', '.join(u(e) for e in node.elts) + ')'
    if name == 'List':
        return '[' + ', '.join(u(e) for e in node.elts) + ']'
    if name == 'Set':
        return '{' + ', '.join(u(e) for e in node.elts) + '}'
    if name == 'Dict':
        return '{' + ', '.join((u(k) + ': ' if k is not None else '**') + u(v)
                               for k, v in zip(node.keys, node.values)) + '}'
    if name == 'BinOp':
        return '(' + u(node.left) + ' ' + _BINOP[node.op.__class__.__name__] + ' ' + u(node.right) + ')'
    if name == 'UnaryOp':
        return '(' + _UNARY[node.op.__class__.__name__] + u(node.operand) + ')'
    if name == 'BoolOp':
        op = ' and ' if node.op.__class__.__name__ == 'And' else ' or '
        return '(' + op.join(u(v) for v in node.values) + ')'
    if name == 'Compare':
        parts = [u(node.left)]
        for op, c in zip(node.ops, node.comparators):
            parts += [_CMPOP[op.__class__.__name__], u(c)]
        return '(' + ' '.join(parts) + ')'
    raise NotImplementedError("unparse does not support %s" % name)


# From CPython's ast.py.
def copy_location(new_node, old_node):
    """Copy source location (lineno, col_offset, end_lineno, end_col_offset)."""
    for attr in 'lineno', 'col_offset', 'end_lineno', 'end_col_offset':
        if attr in old_node._attributes and attr in new_node._attributes:
            value = getattr(old_node, attr, None)
            if value is not None or (
                hasattr(old_node, attr) and attr.startswith("end_")
            ):
                setattr(new_node, attr, value)
    return new_node


def fix_missing_locations(node):
    """Fill in missing lineno/col_offset attributes of a tree."""
    def _fix(node, lineno, col_offset, end_lineno, end_col_offset):
        if 'lineno' in node._attributes:
            if not hasattr(node, 'lineno'):
                node.lineno = lineno
            else:
                lineno = node.lineno
        if 'end_lineno' in node._attributes:
            if getattr(node, 'end_lineno', None) is None:
                node.end_lineno = end_lineno
            else:
                end_lineno = node.end_lineno
        if 'col_offset' in node._attributes:
            if not hasattr(node, 'col_offset'):
                node.col_offset = col_offset
            else:
                col_offset = node.col_offset
        if 'end_col_offset' in node._attributes:
            if getattr(node, 'end_col_offset', None) is None:
                node.end_col_offset = end_col_offset
            else:
                end_col_offset = node.end_col_offset
        for child in iter_child_nodes(node):
            _fix(child, lineno, col_offset, end_lineno, end_col_offset)
    _fix(node, 1, 0, 1, 0)
    return node


def increment_lineno(node, n=1):
    """Increment the line number and end line number of each node in the tree."""
    for child in walk(node):
        if 'lineno' in child._attributes:
            child.lineno = getattr(child, 'lineno', 0) + n
        if (
            "end_lineno" in child._attributes
            and (end_lineno := getattr(child, "end_lineno", 0)) is not None
        ):
            child.end_lineno = end_lineno + n
    return node


def dump(node, annotate_fields=True, include_attributes=False, *, indent=None,
         show_empty=False):
    """Return a formatted dump of the tree in node."""
    def _format(node, level=0):
        if indent is not None:
            level += 1
            prefix = '\n' + indent * level
            sep = ',\n' + indent * level
        else:
            prefix = ''
            sep = ', '
        if isinstance(node, AST):
            cls = type(node)
            args = []
            args_buffer = []
            allsimple = True
            keywords = annotate_fields
            for name in node._fields:
                try:
                    value = getattr(node, name)
                except AttributeError:
                    keywords = True
                    continue
                # An optional field left at None is omitted, as CPython does by
                # giving each such field a class-level None.
                if value is None and not (isinstance(node, Constant) and name == 'value'):
                    keywords = True
                    continue
                if not show_empty and isinstance(value, list) and not value:
                    if not keywords:
                        args_buffer.append(repr(value))
                    continue
                if args_buffer:
                    args.extend(args_buffer)
                    args_buffer = []
                value, simple = _format(value, level)
                allsimple = allsimple and simple
                if keywords:
                    args.append('%s=%s' % (name, value))
                else:
                    args.append(value)
            if include_attributes and node._attributes:
                for name in node._attributes:
                    try:
                        value = getattr(node, name)
                    except AttributeError:
                        continue
                    if value is None and getattr(cls, name, ...) is None:
                        continue
                    value, simple = _format(value, level)
                    allsimple = allsimple and simple
                    args.append('%s=%s' % (name, value))
            if allsimple and len(args) <= 3:
                return '%s(%s)' % (node.__class__.__name__, ', '.join(args)), not args
            return '%s(%s%s)' % (node.__class__.__name__, prefix, sep.join(args)), False
        elif isinstance(node, list):
            if not node:
                return '[]', True
            return '[%s%s]' % (prefix, sep.join(_format(x, level)[0] for x in node)), False
        return repr(node), True

    if not isinstance(node, AST):
        raise TypeError('expected AST, got %r' % node.__class__.__name__)
    if indent is not None and not isinstance(indent, str):
        indent = ' ' * indent
    return _format(node)[0]
