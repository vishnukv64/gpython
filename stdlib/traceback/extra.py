# The parts of CPython's traceback API that are plain Python over frames and
# tracebacks: FrameSummary, StackSummary and the functions built on them.

BUILTIN_EXCEPTION_LIMIT = object()


def walk_stack(f):
    """Walk a stack yielding the frame and line number for each frame."""
    if f is None:
        import sys
        f = sys._getframe().f_back.f_back
    while f is not None:
        yield f, f.f_lineno
        f = f.f_back


class FrameSummary:
    """Information about a single frame from a traceback."""

    def __init__(self, filename, lineno, name, *, lookup_line=True,
                 locals=None, line=None, end_lineno=None, colno=None,
                 end_colno=None, **kwargs):
        self.filename = filename
        self.lineno = lineno
        self.end_lineno = lineno if end_lineno is None else end_lineno
        self.colno = colno
        self.end_colno = end_colno
        self.name = name
        self._code = kwargs.get("_code")
        self._line = line
        if lookup_line:
            self.line
        self.locals = {k: repr(v) for k, v in locals.items()} if locals else None

    def __eq__(self, other):
        if isinstance(other, FrameSummary):
            return (self.filename == other.filename and
                    self.lineno == other.lineno and
                    self.name == other.name and
                    self.locals == other.locals)
        if isinstance(other, tuple):
            return (self.filename, self.lineno, self.name, self.line) == other
        return NotImplemented

    def __getitem__(self, pos):
        return (self.filename, self.lineno, self.name, self.line)[pos]

    def __iter__(self):
        return iter([self.filename, self.lineno, self.name, self.line])

    def __repr__(self):
        return "<FrameSummary file {filename}, line {lineno} in {name}>".format(
            filename=self.filename, lineno=self.lineno, name=self.name)

    def __len__(self):
        return 4

    @property
    def _original_line(self):
        self.line
        return self._line

    @property
    def line(self):
        if self._line is None:
            if self.lineno is None:
                return None
            import linecache
            self._line = linecache.getline(self.filename, self.lineno)
        return self._line.strip()


class StackSummary(list):
    """A list of FrameSummary objects, representing a stack of frames."""

    @classmethod
    def extract(klass, frame_gen, *, limit=None, lookup_lines=True,
                capture_locals=False):
        if limit is None:
            import sys
            limit = getattr(sys, 'tracebacklimit', None)
            if limit is not None and limit < 0:
                limit = 0
        if limit is not None:
            if limit >= 0:
                frame_gen = _islice(frame_gen, limit)
            else:
                from collections import deque
                frame_gen = iter(deque(frame_gen, maxlen=-limit))

        result = klass()
        fnames = set()
        for f, lineno in frame_gen:
            co = f.f_code
            filename = co.co_filename
            name = co.co_name
            fnames.add(filename)
            if capture_locals:
                f_locals = f.f_locals
            else:
                f_locals = None
            result.append(FrameSummary(
                filename, lineno, name, lookup_line=False, locals=f_locals))
        if lookup_lines:
            for f in result:
                f.line
        return result

    @classmethod
    def from_list(klass, a_list):
        result = StackSummary()
        for frame in a_list:
            if isinstance(frame, FrameSummary):
                result.append(frame)
            else:
                filename, lineno, name, line = frame
                result.append(FrameSummary(filename, lineno, name, line=line))
        return result

    def format_frame_summary(self, frame_summary, **kwargs):
        row = ['  File "{}", line {}, in {}\n'.format(
            frame_summary.filename, frame_summary.lineno, frame_summary.name)]
        if frame_summary.line:
            row.append('    {}\n'.format(frame_summary.line.strip()))
        if frame_summary.locals:
            for name, value in sorted(frame_summary.locals.items()):
                row.append('    {name} = {value}\n'.format(name=name, value=value))
        return ''.join(row)

    def format(self, **kwargs):
        result = []
        last_file = None
        last_line = None
        last_name = None
        count = 0
        for frame_summary in self:
            formatted_frame = self.format_frame_summary(frame_summary)
            if formatted_frame is None:
                continue
            if (last_file is None or last_file != frame_summary.filename or
                    last_line is None or last_line != frame_summary.lineno or
                    last_name is None or last_name != frame_summary.name):
                if count > _RECURSIVE_CUTOFF:
                    count -= _RECURSIVE_CUTOFF
                    result.append(
                        f'  [Previous line repeated {count} more '
                        f'time{"s" if count > 1 else ""}]\n'
                    )
                last_file = frame_summary.filename
                last_line = frame_summary.lineno
                last_name = frame_summary.name
                count = 0
            count += 1
            if count > _RECURSIVE_CUTOFF:
                continue
            result.append(formatted_frame)

        if count > _RECURSIVE_CUTOFF:
            count -= _RECURSIVE_CUTOFF
            result.append(
                f'  [Previous line repeated {count} more '
                f'time{"s" if count > 1 else ""}]\n'
            )
        return result


_RECURSIVE_CUTOFF = 3


def _islice(it, n):
    for i, x in enumerate(it):
        if i >= n:
            return
        yield x


def extract_tb(tb, limit=None):
    """Return a StackSummary of the traceback's entries."""
    return StackSummary.extract(walk_tb(tb), limit=limit)


def extract_stack(f=None, limit=None):
    """Extract the raw traceback from the current stack frame."""
    if f is None:
        import sys
        f = sys._getframe().f_back
    stack = StackSummary.extract(walk_stack(f), limit=limit)
    stack.reverse()
    return stack


def format_list(extracted_list):
    return StackSummary.from_list(extracted_list).format()


def print_list(extracted_list, file=None):
    if file is None:
        import sys
        file = sys.stderr
    for item in StackSummary.from_list(extracted_list).format():
        print(item, file=file, end="")


def format_tb(tb, limit=None):
    return extract_tb(tb, limit=limit).format()


def print_tb(tb, limit=None, file=None):
    print_list(extract_tb(tb, limit=limit), file=file)


def print_stack(f=None, limit=None, file=None):
    if f is None:
        import sys
        f = sys._getframe().f_back
    print_list(extract_stack(f, limit=limit), file=file)


def clear_frames(tb):
    """Clear all references to local variables in the frames of a traceback."""
    while tb is not None:
        try:
            tb.tb_frame.clear()
        except RuntimeError:
            pass
        tb = tb.tb_next


def print_last(limit=None, file=None, chain=True):
    import sys
    if not hasattr(sys, "last_exc") and not hasattr(sys, "last_type"):
        raise ValueError("no last exception")
    if hasattr(sys, "last_exc"):
        print_exception(sys.last_exc, limit, file, chain)
    else:
        print_exception(sys.last_type, sys.last_value, sys.last_traceback,
                        limit, file, chain)


def format_stack(f=None, limit=None):
    """Shorthand for 'format_list(extract_stack(f, limit))'."""
    if f is None:
        import sys
        f = sys._getframe().f_back
    return format_list(extract_stack(f, limit=limit))
