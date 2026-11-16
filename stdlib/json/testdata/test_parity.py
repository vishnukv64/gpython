import json

# Each line is a CPython 3.12 parity case. The golden file is CPython's output.

# indent: a negative number gives newlines with no indentation.
print(json.dumps({'a': 1}, indent=-2))
# indent=0 still turns on newlines, and separators= is honoured with it.
print(json.dumps([1, 2], indent=0, separators=(',', ': ')))
print(json.dumps({'a': 1, 'b': [1, 2]}, indent=1))
print(json.dumps({'a': 1, 'b': 2}, separators=(',', ':')))
print(json.dumps({'a': 1}, separators=(', ', ' => ')))

# float repr: whole floats keep their .0; exponent form only from 1e16 up.
print(json.dumps(1000000.0))
print(json.dumps(1e15))
print(json.dumps(1e16))
print(json.dumps(0.0001))
print(json.dumps(1e-05))
print(json.dumps(1.5e300))

# Cycles are an error, not a stack overflow.
cyc = []
cyc.append(cyc)
try:
    json.dumps(cyc)
except ValueError as e:
    print("cycle:", e)

# JSONDecodeError is a ValueError.
try:
    json.loads('{bad')
except ValueError as e:
    print("decode:", type(e).__name__, isinstance(e, ValueError))
