"""PEP 701: f-strings are parsed by the grammar, not by quote matching."""

d = {"k": 1, "j": [1, 2]}
x = 5
w = 7
print(f"{d["k"]}")
print(f"{d["j"][0]:{w}}|")
print(f"{f"{f"{1+1}"}"}")
print(f"{f"{f"{f"{f"{x}"}"}"}"}")
print(f"{'a' # comment with a } brace and a " quote
}")
print(f"{
  1 +
  2
}")
print(f"{"\n".join(["a", "b"])}")
print(f"{'\\'}")
print(f"{x=}", f"{x = :>5}", f"{x=!r:^{x+1}}", f"{d["k"]=}")
print(f"{x:{w}}", f"{x!r:>{w}}", f"{x:{'>'}{w}}")
print(f"{{}}{x}{{")
print(f'{"a" if x else "b"}')
print(f"{ {1: 2}[1] }")
print(f"{(y:=3)} {y}")
print(f"""{
"""x"""}""")
print(f"{x!s}", f"{x!a}", rf"\n{x}", f"{'{'}", f"{'}'}")
print(f"{x:%Y}" if 0 else "ok")
print(f"{[i for i in "abc"]}")
print(f"{(lambda a: a * 2)(x)}")
print(f"{x != 3} {x == 5} {x <= 5}")
print(f"{'a' 'b'}", f"{"a" "b"}")
print(f"{3.5:{w}.{2}}|")

for src in (
    'f"{}"',
    'f"{ }"',
    'f"{x"',
    'f"{x!}"',
    'f"{x!z}"',
    'f"}"',
    'f"{x!r:{}}"',
    'f"{# c\n}"',
    'f"{x:{y:{z}}}"',
    'f"{x!r!s}"',
    'f"{lambda x: 1}"',
    'f"{1 +}"',
):
    try:
        compile(src, "<s>", "eval")
        print("ok", repr(src))
    except SyntaxError as e:
        print("SyntaxError", repr(src), e.msg)
