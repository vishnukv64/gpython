class BaseExceptionGroup(BaseException):
    """A combination of multiple unrelated exceptions."""

    def __new__(cls, message, exceptions):
        if not isinstance(message, str):
            raise TypeError("argument 1 must be str, not " + type(message).__name__)
        if isinstance(exceptions, (str, bytes, dict, set, frozenset)) or not hasattr(exceptions, "__getitem__"):
            raise TypeError("second argument (exceptions) must be a sequence")
        excs = tuple(exceptions)
        if not excs:
            raise ValueError("second argument (exceptions) must be a non-empty sequence")
        for i, e in enumerate(excs):
            if not isinstance(e, BaseException):
                raise ValueError("Item %d of second argument (exceptions) is not an exception" % i)
        if cls is BaseExceptionGroup:
            if all(isinstance(e, Exception) for e in excs):
                cls = ExceptionGroup
        elif issubclass(cls, Exception):
            for e in excs:
                if not isinstance(e, Exception):
                    raise TypeError("Cannot nest BaseExceptions in an ExceptionGroup")
        self = BaseException.__new__(cls, message, exceptions)
        self._message = message
        self._exceptions = excs
        return self

    @property
    def message(self):
        return self._message

    @property
    def exceptions(self):
        return self._exceptions

    def __str__(self):
        n = len(self._exceptions)
        return "%s (%d sub-exception%s)" % (self._message, n, "" if n == 1 else "s")

    def __repr__(self):
        return "%s(%r, %r)" % (type(self).__name__, self._message, self.args[1])

    def derive(self, excs):
        return BaseExceptionGroup(self._message, excs)

    def _copy_meta(self, new):
        new.__cause__ = self.__cause__
        new.__context__ = self.__context__
        new.__traceback__ = self.__traceback__
        for name in ("__notes__",):
            if hasattr(self, name):
                setattr(new, name, getattr(self, name))
        return new

    def _split(self, cond):
        # returns (match, rest) as exception-or-None
        if cond(self):
            return self, None
        matched, rest = [], []
        for e in self._exceptions:
            if isinstance(e, BaseExceptionGroup):
                m, r = e._split(cond)
            elif cond(e):
                m, r = e, None
            else:
                m, r = None, e
            if m is not None:
                matched.append(m)
            if r is not None:
                rest.append(r)
        mg = rg = None
        if matched:
            mg = self._copy_meta(self.derive(matched))
        if rest:
            rg = self._copy_meta(self.derive(rest))
        return mg, rg

    @staticmethod
    def _cond(condition):
        if isinstance(condition, type) and issubclass(condition, BaseException):
            return lambda e: isinstance(e, condition)
        if isinstance(condition, tuple) and all(isinstance(c, type) and issubclass(c, BaseException) for c in condition):
            return lambda e: isinstance(e, condition)
        if callable(condition) and not isinstance(condition, type):
            return condition
        raise TypeError("expected an exception type, a tuple of exception types, or a callable (other than a class)")

    def subgroup(self, condition):
        return self._split(BaseExceptionGroup._cond(condition))[0]

    def split(self, condition):
        return self._split(BaseExceptionGroup._cond(condition))

    def __class_getitem__(cls, item):
        return cls


class ExceptionGroup(BaseExceptionGroup, Exception):
    pass
