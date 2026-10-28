import os
import collections.abc as c

print(isinstance(os.environ, c.MutableMapping))
os.environ["GP_A"] = "1"
os.environ.update({"GP_B": "2"}, GP_C="3")
os.environ.update([("GP_D", "4")])
print([os.environ[k] for k in ("GP_A", "GP_B", "GP_C", "GP_D")])
print(os.getenv("GP_B"))
print(os.environ.pop("GP_A"), os.environ.pop("GP_A", "dflt"), os.getenv("GP_A"))
try:
    os.environ.pop("GP_A")
except KeyError as e:
    print("KeyError", e)
print(os.environ.setdefault("GP_E", "5"), os.environ.setdefault("GP_E", "6"))
r = os.environ | {"GP_F": "6"}
print(type(r).__name__, r["GP_F"], "GP_F" in os.environ)
r = {"GP_G": "7"} | os.environ
print(type(r).__name__, r["GP_G"], "GP_G" in os.environ)
os.environ |= {"GP_H": "8"}
print(os.environ["GP_H"], os.getenv("GP_H"))
for bad in (5, [("a",)], [("a", "b", "c")], {"a": 1}, {1: "a"}):
    try:
        os.environ.update(bad)
    except (TypeError, ValueError) as e:
        print(type(e).__name__, e)
try:
    os.environ.update({}, {})
except TypeError as e:
    print(e)
c2 = os.environ.copy()
print(type(c2).__name__, c2["GP_B"], "GP_B" in c2)
c2["GP_Z"] = "z"
print("GP_Z" in os.environ)
print(len([k for k in os.environ if k.startswith("GP_")]))
for k in [k for k in os.environ if k.startswith("GP_")]:
    del os.environ[k]
print([k for k in os.environ if k.startswith("GP_")])
