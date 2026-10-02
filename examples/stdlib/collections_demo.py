"""collections: specialised container types.

Run with:  /tmp/gpy examples/stdlib/collections_demo.py

Covers Counter, deque, OrderedDict, defaultdict, namedtuple, ChainMap and the
UserDict / UserList / UserString wrappers.

Method coverage is uneven in this interpreter:
  * Counter:  get/keys/values/items/most_common/update/elements/total
  * deque:    append/appendleft/extend/extendleft/pop/popleft/clear/len/index
  * OrderedDict: get/keys/values/items/popitem
  * defaultdict: acts like a dict, auto-creating values from its factory
  * namedtuple: field access, indexing, iteration, tuple()
  * ChainMap: indexing and get (a .keys() is NOT implemented)
  * UserDict: get/keys ; UserList: append ; UserString: only len/indexing
"""

import collections

print("=== Counter ===")
text = "abracadabra"
counter = collections.Counter(text)
print("counting %r:" % text)
print("  most_common(3):   ", counter.most_common(3))
print("  count of 'a':     ", counter["a"])
print("  count of missing: ", counter["z"], "(absent keys read as 0)")
print("  keys():           ", sorted(counter.keys()))
print("  values():         ", sorted(counter.values()))
print("  len():            ", len(counter), "distinct characters")

counter.update("aaa")
print("after update('aaa'):")
print("  count of 'a':     ", counter["a"])
print("  total():          ", counter.total())

print()
print("counting words instead of letters:")
words = collections.Counter()
for word in "the cat sat on the mat".split():
    words[word] += 1
print("  most_common(2):   ", words.most_common(2))
print("  sorted by count:  ", sorted(words.keys()))

print()
print("=== deque: a double-ended queue ===")
dq = collections.deque([2, 3, 4])
print("start:              ", list(dq))
dq.append(5)
dq.appendleft(1)
print("append/appendleft:  ", list(dq))
dq.extend([6, 7])
dq.extendleft([0])
print("extend family:      ", list(dq))
print("pop/popleft:        ", dq.pop(), dq.popleft(), "->", list(dq))
print("len:                ", len(dq))
print("indexing dq[0]:     ", dq[0], "and dq[-1]:", dq[-1])

# A bounded deque drops items from the far end when it overflows.
bounded = collections.deque([1, 2, 3], 3)
bounded.append(4)
print("bounded deque(...,3):", list(bounded), "-> the oldest value fell off")

print()
print("a queue-like use: process items left to right")
queue = collections.deque(["job1", "job2", "job3"])
while len(queue) > 0:
    print("  processing", queue.popleft(), "remaining:", list(queue))

print()
print("=== OrderedDict ===")
od = collections.OrderedDict([("banana", 3), ("apple", 1), ("cherry", 2)])
print("insertion order:    ", list(od.keys()))
print("values in order:    ", list(od.values()))
print("items in order:     ", list(od.items()))
print("lookup by key:      ", od["apple"], od.get("durian"), od.get("durian", 0))
print("popitem (last):     ", od.popitem())
print("after popitem:      ", list(od.keys()))

print()
print("=== defaultdict ===")
# The factory runs only for keys that are missing.
counts = collections.defaultdict(int)
for letter in "mississippi":
    counts[letter] += 1
print("defaultdict(int):   ", dict(counts.items()) if hasattr(counts, "items") else counts)

grouped = collections.defaultdict(list)
for name, dept in [("ann", "eng"), ("bob", "ops"), ("cy", "eng")]:
    grouped[dept].append(name)
print("defaultdict(list):  ", dict(grouped.items()) if hasattr(grouped, "items") else grouped)
print("reading a missing key creates it:", grouped["sales"])

print()
print("Note: dict(...) on a defaultdict works through items(); a defaultdict")
print("does not have .copy(), .update() or .pop() in this interpreter.")

print()
print("=== namedtuple ===")
Point = collections.namedtuple("Point", ["x", "y"])
origin = Point(0, 0)
p = Point(3, 4)
print("Point(3, 4):        ", p)
print("field access p.x:   ", p.x, "and p.y:", p.y)
print("indexing p[0]:      ", p[0], "and p[1]:", p[1])
print("unpacking:          ", end="")
px, py = p
print(px, py)
print("iteration:          ", [c for c in p])
print("as a plain tuple:   ", tuple(p))
# NOTE: namedtuple == namedtuple is not implemented (TypeError: unsupported
# operand type(s) for ==). Compare tuples instead.
print("equality via tuple(): ", tuple(Point(1, 2)) == tuple(Point(1, 2)))
print("_fields is not exposed here: ", hasattr(Point, "_fields"))

print()
print("namedtuples make readable records:")
Employee = collections.namedtuple("Employee", ["name", "dept", "salary"])
staff = [Employee("ann", "eng", 120), Employee("bob", "ops", 95), Employee("cy", "eng", 110)]
print("  all:            ", staff)
print("  by salary desc: ", sorted(staff, key=lambda e: -e.salary))
print("  eng only:       ", [e.name for e in staff if e.dept == "eng"])
print("  total payroll:  ", sum(e.salary for e in staff))

print()
print("=== ChainMap ===")
defaults = {"colour": "blue", "size": 2}
overrides = {"colour": "red"}
chain = collections.ChainMap(overrides, defaults)
print("overrides shadow the defaults:")
print("  chain['colour']:  ", chain["colour"], "(from the first map)")
print("  chain['size']:    ", chain["size"], "(falls through to the second)")
print("  missing lookup:   ", chain.get("weight", "not found"))
print("  .keys() is not implemented on ChainMap here:", hasattr(chain, "keys"))

print()
print("a layered configuration, the classic use:")
base_config = {"host": "localhost", "port": 8080, "debug": False}
env_config = {"port": 9090}
resolved = collections.ChainMap(env_config, base_config)
for key in ["host", "port", "debug"]:
    print("  %-6s = %s" % (key, resolved[key]))

print()
print("=== UserDict, UserList, UserString ===")
# These are thin wrappers meant to be subclassed. Only a few operations are
# wired up in this interpreter.

user_dict = collections.UserDict({"a": 1, "b": 2})
print("UserDict:")
print("  lookup:   ", user_dict["a"])
print("  len():    ", len(user_dict))
print("  keys():   ", sorted(user_dict.keys()))
print("  get():    ", user_dict.get("zz", "default"))

user_list = collections.UserList([10, 20, 30])
print("UserList:")
print("  indexing: ", user_list[0], user_list[-1])
print("  len():    ", len(user_list))
user_list.append(40)
print("  append:   ", list(user_list))

user_string = collections.UserString("hello")
print("UserString:")
print("  len():    ", len(user_string))
print("  indexing: ", user_string[0])
# The usual string methods are NOT proxied on UserString here:
print("  .upper() is not proxied:", hasattr(user_string, "upper"))

print()
print("=== a worked example: word frequencies with Counter and defaultdict ===")
documents = [
    "the quick brown fox",
    "the lazy dog sleeps",
    "the fox and the dog",
]
global_counts = collections.Counter()
per_document = collections.defaultdict(set)
for index, doc in enumerate(documents):
    for word in doc.split():
        global_counts[word] += 1
        per_document[word].add(index)

print("three most common words:", global_counts.most_common(3))
print("words in every document:", end=" ")
everywhere = sorted([w for w in global_counts if len(per_document[w]) == len(documents)])
print(everywhere)
print("documents per word:")
for word in ["the", "fox", "dog"]:
    print("  %-4s appears in documents %s" % (word, sorted(per_document[word])))
