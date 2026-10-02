"""Chained access into the structures json and yaml produce.

Loading a document gives plain dicts and lists, so the ordinary container
operations compose: obj["a"]["b"]["c"], obj.get("a").get("b"), mixing the two,
and the mutating forms - setdefault, update, pop - all chain as they do in
CPython.  This pins that behaviour, because it is what real code does with a
parsed document and it would be easy to lose.

Every expectation here was checked against CPython 3.14.
"""

import json
import yaml

print("--- json: chained subscript ---")
d = json.loads('{"a": {"b": {"c": 42}}}')
print(d["a"]["b"]["c"])

print("--- json: chained get ---")
print(d.get("a").get("b").get("c"))

print("--- json: subscript then get, and the reverse ---")
print(d.get("a")["b"]["c"], d["a"].get("b").get("c"))

print("--- json: a default at the end of a chain ---")
print(d.get("nope", {}).get("x", "DEFAULT"))

print("--- json: through a list ---")
j = json.loads('[{"a": {"b": [1, 2]}}]')
print(j[0]["a"]["b"][1])

print("--- json: keys and items of a nested map ---")
print(sorted(d["a"]["b"].keys()))
print(list(d["a"]["b"].items()))
print("c" in d["a"]["b"])

print("--- json: roundtrip keeps the chain ---")
r = json.loads(json.dumps({"x": {"y": {"z": 1}}}))
print(r["x"]["y"]["z"])

print("--- json: null in the middle ---")
n = json.loads('{"a": null}')
print(n.get("a"))
print((n.get("a") or {}).get("b", "DEFAULT"))

print("--- json: building a nested structure by chaining setdefault ---")
e = json.loads("{}")
e.setdefault("a", {}).setdefault("b", {})["c"] = 1
print(e)

print("--- json: mutating a nested map ---")
m = json.loads('{"a": {"b": 1}}')
m["a"].update({"b": 2, "c": 3})
print(m)
print(m["a"].pop("c"), m)

print("--- yaml: chained subscript and get ---")
y = yaml.safe_load("a:\n  b:\n    c: 42\n")
print(y["a"]["b"]["c"], y.get("a").get("b").get("c"))

print("--- yaml: through a list ---")
yl = yaml.safe_load("list:\n  - x: 1\n")
print(yl["list"][0]["x"])

print("--- yaml: flow style nests too ---")
yf = yaml.safe_load("a: {b: {c: 9}}")
print(yf["a"]["b"]["c"])

print("--- yaml: safe_dump roundtrip keeps the chain ---")
yd = yaml.safe_load(yaml.safe_dump({"p": {"q": {"r": 5}}}))
print(yd["p"]["q"]["r"])

print("done")
