eg = ExceptionGroup("boom", [ValueError(1), TypeError(2), ExceptionGroup("in", [KeyError(3), ValueError(4)])])
print(eg, repr(eg), type(eg).__name__)
print(eg.args)
m, r = eg.split(ValueError)
print(repr(m)); print(repr(r))
print(repr(eg.subgroup(KeyError)))
print(isinstance(eg, Exception), isinstance(BaseExceptionGroup("x", [KeyboardInterrupt()]), Exception))
print(type(BaseExceptionGroup("x", [ValueError()])).__name__)
try:
    raise eg
except ExceptionGroup as e:
    print("caught", len(e.exceptions), e.message)
try: BaseExceptionGroup("x", [])
except ValueError as e: print(e)
try: ExceptionGroup("x", [KeyboardInterrupt()])
except TypeError as e: print(e)
