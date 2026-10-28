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
