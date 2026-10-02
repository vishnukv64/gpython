"""pprint: pretty-print Python data structures.

Run with:  /tmp/gpy examples/stdlib/pprint_demo.py

pformat() returns the string, pprint() writes it, and PrettyPrinter lets you
configure the layout.

Interpreter note: dict output is NOT deterministic -- two dictionaries with
equal contents may print their keys in a different order on different runs,
because this interpreter's dict is not insertion-ordered. Every dict shown
below is therefore either a single key or printed with sorted keys. `sort_dicts`
is accepted; `sort_dicts=False` may be non-deterministic.

WARNING: pprint.pformat() on a self-referencing container hangs forever when
`width` is 20 or less -- at the default width it prints a `<Recursion on ...>`
marker correctly. See examples/KNOWN_ISSUES.md.
"""

import pprint


def _self_ref_dict():
    """A dict that contains itself -- isrecursive() can spot it."""
    mapping = {}
    mapping["self"] = mapping
    return mapping


print("--- pformat returns a string ---")
simple = [1, 2, 3, "four", 5.0]
print("as a list:      ", simple)
print("pformat of it:  ", pprint.pformat(simple))

print()
print("--- pprint writes to stdout ---")
print("about to pprint the same list:")
pprint.pprint(simple)

print()
print("--- long structures are wrapped to fit the width ---")
wide = [list(range(12))]
print("default width (80):")
print(pprint.pformat(wide))
print()
print("width=40:")
print(pprint.pformat(wide, width=40))
print()
print("width=200:")
print(pprint.pformat(wide, width=200))

print()
print("--- indent controls the nesting offset ---")
nested = [[["deep", "values", "here"], ["more", "of", "them"], ["and", "again", "too"]]]
print("indent=1 (the default):")
print(pprint.pformat(nested, width=30))
print()
print("indent=4:")
print(pprint.pformat(nested, width=30, indent=4))

print()
print("--- depth limits how far down it goes ---")
deep = {"a": {"b": {"c": {"d": 1}}}}
print("depth=None (full):")
print(pprint.pformat(deep, width=20))
print("depth=2:")
print(pprint.pformat(deep, width=20, depth=2))

print()
print("--- isreadable and isrecursive ---")
print("isreadable([1, 2]):            ", pprint.isreadable([1, 2]))
print("isreadable({'a': 1}):         ", pprint.isreadable({"a": 1}))
print("isreadable('text'):           ", pprint.isreadable("text"))
print("isrecursive([1, 2]):          ", pprint.isrecursive([1, 2]))
recursive = []
recursive.append(recursive)
print("isrecursive on a self-referencing list:", pprint.isrecursive(recursive))
print("isrecursive on a self-referencing dict:", pprint.isrecursive(_self_ref_dict()))
print()
print("a self-referencing container prints, with a <Recursion on ...> marker:")
print("  list (default width): ", pprint.pformat(recursive))
print("  dict (default width): ", pprint.pformat(_self_ref_dict()))
print()
print("WARNING: narrow a self-referencing container and pformat() never returns.")
print("pformat(recursive, width=20) HANGS; width=21 is fine. The demo therefore")
print("only ever formats these at the default width. See examples/KNOWN_ISSUES.md.")

print()
print("--- PrettyPrinter holds the settings ---")
printer = pprint.PrettyPrinter(indent=2, width=30, depth=None)
print(printer.pformat(["a long-ish string here", "and another", "and a third"]))
print("default printer:", pprint.PrettyPrinter().pformat([1, 2, 3]))

print()
print("--- dict key order is not guaranteed; sort when it matters ---")


def stable(mapping):
    """Render a dict with its keys sorted, so the output is reproducible."""
    items = sorted(mapping.items(), key=lambda pair: pair[0])
    body = ", ".join(["%r: %r" % (key, value) for key, value in items])
    return "{" + body + "}"


config = {"host": "localhost", "port": 8080, "debug": True, "retries": 3}
print("raw pprint (order may vary):", pprint.pformat(config, width=80))
print("sorted rendering:           ", stable(config))

print()
print("--- a worked example: a debug dump for a request object ---")


class Request:
    def __init__(self):
        self.method = "POST"
        self.path = "/api/v1/items"
        self.headers = {"content-type": "application/json", "accept": "*/*"}
        self.body = {"name": "widget", "sizes": [1, 2, 3], "nested": [{"k": "v"}]}
        self.user = "ann"


request = Request()
print("method/path:", request.method, request.path)
print("headers (sorted):", stable(request.headers))
print("body:")
print(pprint.pformat(request.body, width=60, indent=2))
print()
print("a whole-object dump, rendered in a reproducible order:")
for attribute in ["method", "path", "user"]:
    print("  %-8s = %r" % (attribute, getattr(request, attribute)))
print("  %-8s = %s" % ("headers", stable(request.headers)))
print("  %-8s = %s" % ("body", stable(request.body)))
