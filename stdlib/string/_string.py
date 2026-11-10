# _string: the str.format parser used by string.Formatter.


def formatter_parser(s):
    """Yield (literal_text, field_name, format_spec, conversion) tuples."""
    if not isinstance(s, str):
        raise TypeError("expected str, got %s" % type(s).__name__)
    out = []
    i, n = 0, len(s)
    lit = []
    while i < n:
        c = s[i]
        if c == '{':
            if s[i + 1:i + 2] == '{':
                lit.append('{')
                i += 2
                continue
            # a replacement field: find the matching close brace
            depth, j = 1, i + 1
            while j < n and depth:
                if s[j] == '{':
                    depth += 1
                elif s[j] == '}':
                    depth -= 1
                j += 1
            if depth:
                raise ValueError("expected '}' before end of string")
            field = s[i + 1:j - 1]
            conv = None
            spec = ''
            # name ends at the first top-level '!' or ':' (not inside [])
            k, br = 0, False
            while k < len(field):
                ch = field[k]
                if ch == '[':
                    br = True
                elif ch == ']':
                    br = False
                elif not br and ch in '!:':
                    break
                k += 1
            name = field[:k]
            rest = field[k:]
            if rest[:1] == '!':
                if len(rest) < 2:
                    raise ValueError("end of string while looking for conversion specifier")
                conv = rest[1]
                rest = rest[2:]
                if rest and rest[0] != ':':
                    raise ValueError("expected ':' after conversion specifier")
            if rest[:1] == ':':
                spec = rest[1:]
            out.append((''.join(lit), name, spec, conv))
            lit = []
            i = j
        elif c == '}':
            if s[i + 1:i + 2] == '}':
                lit.append('}')
                i += 2
                continue
            raise ValueError("Single '}' encountered in format string")
        else:
            lit.append(c)
            i += 1
    if lit:
        out.append((''.join(lit), None, None, None))
    return iter(out)


def formatter_field_name_split(name):
    """Return (first, iterator of (is_attr, key)) for a field name."""
    if not isinstance(name, str):
        raise TypeError("expected str, got %s" % type(name).__name__)
    n = len(name)
    i = 0
    while i < n and name[i] not in '.[':
        i += 1
    first = name[:i]
    if first.isdigit():
        first = int(first)
    rest = []
    while i < n:
        if name[i] == '.':
            j = i + 1
            while j < n and name[j] not in '.[':
                j += 1
            if j == i + 1:
                raise ValueError("Empty attribute in format string")
            rest.append((True, name[i + 1:j]))
            i = j
        else:
            j = name.find(']', i)
            if j < 0:
                raise ValueError("Missing ']' in format string")
            key = name[i + 1:j]
            if not key:
                raise ValueError("Empty attribute in format string")
            rest.append((False, int(key) if key.isdigit() else key))
            i = j + 1
            if i < n and name[i] not in '.[':
                raise ValueError("Only '.' or '[' may follow ']' in format field specifier")
    return first, iter(rest)
