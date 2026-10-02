"""json: encoding and decoding JSON.

Run with:  /tmp/gpy examples/stdlib/json_demo.py
"""

import json

print("--- dumps: Python values to JSON text ---")
print("dict:      ", json.dumps({"name": "gpython", "version": 3}))
print("list:      ", json.dumps([1, 2, 3]))
print("scalars:   ", json.dumps(42), json.dumps(3.5), json.dumps("text"))
print("True/None: ", json.dumps(True), json.dumps(False), json.dumps(None))

print()
print("--- the type mapping ---")
mapping = {
    "str": "a string",
    "int": 7,
    "float": 1.5,
    "bool": True,
    "none": None,
    "list": [1, 2],
    "dict": {"nested": "value"},
}
encoder_input = json.dumps(mapping)
print("input:      ", "the mapping above (str/int/float/bool/None/list/dict)")
print("encoded:    ", encoder_input)

print()
print("--- indent and sort_keys ---")
data = {"b": [1, 2], "a": {"x": 1}}
print("compact:      ", json.dumps(data))
print("sorted keys:  ", json.dumps(data, sort_keys=True))
print("indented:")
print(json.dumps(data, indent=2))
print("indent + sort_keys:")
print(json.dumps(data, indent=4, sort_keys=True))

print()
print("--- custom separators ---")
print("compact separators:", json.dumps([1, 2], separators=(",", ":")))
print("spaced:            ", json.dumps([1, 2], separators=(" | ", " = ")))

print()
print("--- loads: JSON text back to Python values ---")
print("object:  ", json.loads('{"a": 1, "b": [2, 3]}'))
print("array:   ", json.loads("[1, 2, 3]"))
print("string:  ", json.loads('"hello"'))
print("number:  ", json.loads("42"), json.loads("1.5"))
print("boolean: ", json.loads("true"), json.loads("false"))
print("null:    ", json.loads("null"))

print()
print("--- a full round trip preserves the data ---")
original = {
    "name": "widget",
    "tags": ["a", "b"],
    "price": 9.99,
    "available": True,
    "details": {"weight": 1, "colour": None},
}
encoded = json.dumps(original, sort_keys=True)
decoded = json.loads(encoded)
print("encoded:      ", encoded)
print("decoded:      ", decoded)
print("equal to start:", decoded == original)

print()
print("--- unicode escapes ---")
print("default ascii:", json.dumps("café"))
print("decoding:     ", json.loads('"caf\\u00e9"'))
print("non-ascii keys survive the round trip:", json.loads(json.dumps({"café": 1})))

print()
print("--- decoding errors ---")
for bad in ['{bad}', '{', '[1,2', 'undefined']:
    try:
        json.loads(bad)
        print("%-10s decoded unexpectedly" % bad)
    except json.JSONDecodeError as err:
        print("%-10s -> JSONDecodeError: %s" % (bad, err))
    except Exception as err:
        print("%-10s -> %s: %s" % (bad, type(err), err))

print()
print("--- unsupported Python values ---")
try:
    json.dumps({"when": object()})
except TypeError as err:
    print("dumping an object() raises TypeError:", err)

print()
print("--- a worked example: a tiny persistence layer ---")
STORE_FILE = "/tmp/gpy_json_store.json"


class Store:
    def __init__(self, path):
        self.path = path
        self.records = {}

    def load(self):
        if not __import__("os").path.exists(self.path):
            self.records = {}
            return self
        with open(self.path) as handle:
            text = handle.read()
        if not text.strip():
            self.records = {}
            return self
        self.records = json.loads(text)
        return self

    def save(self):
        with open(self.path, "w") as handle:
            handle.write(json.dumps(self.records, indent=2, sort_keys=True))
        return self

    def put(self, key, value):
        self.records[key] = value
        return self

    def get(self, key, default=None):
        return self.records.get(key, default)


store = Store(STORE_FILE).load()
store.put("users", ["ann", "bob"])
store.put("settings", {"theme": "dark", "font": 12})
store.save()

reloaded = Store(STORE_FILE).load()
print("reloaded users:    ", reloaded.get("users"))
print("reloaded settings: ", reloaded.get("settings"))
print("missing key:       ", reloaded.get("nope", "absent"))

with open(STORE_FILE) as handle:
    print("file contents:", handle.read().strip().replace("\n", " "))
