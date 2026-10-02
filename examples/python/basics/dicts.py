"""Dictionaries: creation, lookup, iteration, and what is available here.

Run with:  /tmp/gpy examples/python/basics/dicts.py

This interpreter implements only four dict methods: get, keys, values, items.
update / pop / popitem / setdefault / copy / clear / fromkeys are missing, so
this file shows the supported spelling of each job.
"""

print("--- creating dictionaries ---")
print("literal:        ", {"a": 1, "b": 2})
print("empty:          ", {})
print("from pairs:     ", dict([("x", 1), ("y", 2)]))
d = {}
d["name"] = "gpython"
d["version"] = 3
print("built by assign:", d)
print("mixed keys:     ", {1: "int", "one": 2, (1, 2): "tuple"})
print("nested:         ", {"outer": {"inner": [1, 2]}})
print("dict comp:      ", {n: n * n for n in range(4)})
print("from two lists: ", dict(zip(["a", "b"], [1, 2])))

print()
print("--- lookup ---")
scores = {"alice": 90, "bob": 75}
print("dict:           ", scores)
print("scores['alice']:", scores["alice"])
print("get present:    ", scores.get("bob"))
print("get missing:    ", scores.get("carol"))
print("get with default:", scores.get("carol", 0))
print("membership:     ", "alice" in scores, "carol" in scores)
print("len:            ", len(scores))

print()
print("--- updating values ---")
scores["alice"] = 95
print("reassign:       ", scores)
scores["carol"] = 88
print("add new key:    ", scores)
del scores["bob"]
print("del key:        ", scores)

# dict.update() is not implemented; assign key by key, or rebuild with {**a}.
extra = {"dave": 70, "erin": 81}
for k, v in extra.items():
    scores[k] = v
print("update by loop: ", scores)
merged = {**scores, "frank": 60}
print("merge with {**} ", sorted(merged))
print("original intact:", sorted(scores))

print()
print("--- iteration ---")
inventory = {"apples": 5, "pears": 2, "plums": 9}
print("keys():         ", sorted(inventory.keys()))
print("values():       ", sorted(inventory.values()))
print("items():        ", inventory.items())
print("bare for loop:  ", [k for k in inventory])

total = 0
for fruit, qty in inventory.items():
    total += qty
print("sum of values:  ", total)
print("sum(values()):  ", sum(inventory.values()))

# Sorting dict entries requires a key function: this interpreter has no
# tuple <, so sorted(d.items()) raises TypeError.
entries = list(inventory.items())
entries.sort(key=lambda pair: pair[0])
print("sorted by key:  ", entries)
print("sorted by value:", sorted(entries, key=lambda pair: pair[1]))

print()
print("--- nested data ---")
catalogue = {
    "books": [
        {"title": "Dune", "year": 1965},
        {"title": "Neuromancer", "year": 1984},
    ],
    "count": 2,
}
print("nested lookup:  ", catalogue["books"][0]["title"])
print("walking:        ", [b["title"] for b in catalogue["books"]])
catalogue["books"][1]["year"] = 1984
catalogue["count"] = len(catalogue["books"])
print("after edits:    ", catalogue["count"], catalogue["books"][1])

print()
print("--- the missing methods, spelled out ---")
stock = {"a": 1, "b": 2}

# pop -> get then del
value = stock.get("a")
del stock["a"]
print("pop via get/del:", value, "leaves", sorted(stock))

# setdefault -> get with default, then assign if absent
if stock.get("c") is None:
    stock["c"] = 0
print("setdefault:     ", sorted(stock))

# clear -> rebind or delete every key
for key in list(stock.keys()):
    del stock[key]
print("cleared:        ", stock)

# copy -> dict() around items()
original = {"k": "v"}
duplicate = dict(original.items())
duplicate["k2"] = "v2"
print("copy:           ", original, duplicate)

# fromkeys -> comprehension

print("fromkeys:       ", {k: 0 for k in ["x", "y"]})

print()
print("--- counting words, end to end ---")
text = "the quick brown fox jumps over the lazy dog the fox"
counts = {}
for word in text.split():
    counts[word] = counts.get(word, 0) + 1
top = list(counts.items())
top.sort(key=lambda pair: pair[1], reverse=True)
print("word counts:    ", top[:3])
print("distinct words: ", len(counts))
