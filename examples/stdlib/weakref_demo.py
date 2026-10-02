"""weakref: references that do not keep an object alive.

Run with:  /tmp/gpy examples/stdlib/weakref_demo.py

A weak reference lets you point at an object without preventing it from being
collected. weakref.ref() and weakref.proxy() are solid; WeakValueDictionary and
WeakSet work with caveats; WeakKeyDictionary and WeakSet.add() do not work at
all, because they hash the object and this interpreter's plain instances are
unhashable.

Interpreter notes (all verified by running this file):
  * Instances of a plain class are unhashable: `hash(obj)` raises
    `TypeError: unhashable type: 'Widget'`. An explicit __hash__ method does
    not help -- `hash()` calls it, but WeakSet.add()/WeakKeyDictionary still
    reject the object.
  * WeakValueDictionary keys on the *string* key, not the object, so it works:
    __setitem__, __getitem__, get, __contains__, keys, values and len are all
    sound. Only items() is missing.
  * WeakSet.add(obj) raises `TypeError: unhashable type: 'Widget'` and
    WeakKeyDictionary[obj] = ... raises the same, so neither can hold anything.
  * weakref.getweakrefcount() and getweakrefs() always report 0 and [],
    even after refs have been created.
  * There is no gc module, so collection cannot be forced or observed.
"""

import weakref


class Widget:
    def __init__(self, name):
        self.name = name

    def __repr__(self):
        return "Widget(%r)" % (self.name,)


print("--- a weak reference to an object ---")
widget = Widget("first")
reference = weakref.ref(widget)
print("weakref.ref(widget):        ", reference)
print("calling it returns the object:", reference())
print("it is the same object:      ", reference() is widget)
print("the reference is callable:  ", callable(reference))
print("two refs to the same object both resolve:",
      weakref.ref(widget)() is widget)

print()
print("--- but the counts do not reflect them ---")
print("getweakrefcount(widget):    ", weakref.getweakrefcount(widget))
print("getweakrefs(widget):        ", weakref.getweakrefs(widget))
print("(CPython would report the number of live references here)")

print()
print("--- a proxy forwards attribute access ---")
proxy = weakref.proxy(widget)
print("proxy.name:             ", proxy.name)
print("proxy is not the object:", proxy is not widget)
print("repr through the proxy: ", repr(proxy))

print()
print("--- plain instances are unhashable ---")
plain = Widget("plain")
try:
    hash(plain)
except TypeError as err:
    print("hash(widget) -> TypeError:", err)
print("this matters for the weak containers below, which hash their members")

print()
print("--- WeakValueDictionary keys on the string key, so it works ---")
cache = weakref.WeakValueDictionary()
print("constructed:          ", cache)
cache["first"] = widget
print("len after one add:    ", len(cache))
print("'first' in cache:     ", "first" in cache)
print("cache['first']:       ", cache["first"])
print("cache.get('first'):   ", cache.get("first"))
print("cache.get('missing'): ", cache.get("missing"))
print("keys():               ", list(cache.keys()))
print("values():             ", list(cache.values()))
print("it has items():       ", hasattr(cache, "items"))
print("it has update():      ", hasattr(cache, "update"))

print()
print("--- WeakSet.add hashes the member, so it cannot hold a plain object ---")
collection = weakref.WeakSet()
print("constructed:      ", collection)
try:
    collection.add(widget)
    print("added; len:", len(collection))
except TypeError as err:
    print("collection.add(widget) -> TypeError:", err)
print("len(collection):   ", len(collection), "(still empty)")
print("widget in collection:", widget in collection)
try:
    list(collection)
except TypeError as err:
    print("list(collection) -> TypeError:", err)

print()
print("--- WeakKeyDictionary hashes the key, so it cannot hold a plain object ---")
by_key = weakref.WeakKeyDictionary()
print("constructed:", by_key)
try:
    by_key[widget] = "metadata"
    print("assigned; len:", len(by_key))
except TypeError as err:
    print("by_key[widget] = ... -> TypeError:", err)
print("len(by_key):", len(by_key), "(still empty)")
try:
    print(by_key[widget])
except KeyError as err:
    print("by_key[widget] -> KeyError (the key was never stored)")

print()
print("--- an object with an explicit __hash__ is still rejected ---")


class Hashable:
    def __init__(self, name):
        self.name = name

    def __hash__(self):
        return id(self)

    def __eq__(self, other):
        return self is other


hashable = Hashable("hashable")
print("hash(hashable) works:", isinstance(hash(hashable), int))
try:
    weakref.WeakSet().add(hashable)
    print("WeakSet.add accepted it")
except TypeError as err:
    print("WeakSet.add(hashable) -> TypeError:", err)
try:
    weakref.WeakKeyDictionary()[hashable] = 1
    print("WeakKeyDictionary accepted it")
except TypeError as err:
    print("WeakKeyDictionary[hashable] = 1 -> TypeError:", err)
print("(so __hash__ satisfies hash() but not the weak containers)")

print()
print("--- the module surface ---")
for name in ["ref", "proxy", "getweakrefcount", "getweakrefs",
             "WeakValueDictionary", "WeakKeyDictionary", "WeakSet",
             "WeakMethod", "finalize", "ReferenceType", "ProxyType", "CallableProxyType"]:
    print("  weakref.%-22s %s" % (name, hasattr(weakref, name)))

print()
print("--- there is no gc module, so collection cannot be observed ---")
try:
    import gc
    print("gc imported")
except ModuleNotFoundError as err:
    print("import gc -> ModuleNotFoundError:", err)

print()
print("--- a worked example: a cache that does not keep its values alive ---")


class WidgetCache:
    """Caches widgets by name, holding only weak references to them.

    WeakValueDictionary is exactly the right tool: the key is a plain string,
    so no hashing of the widget is needed, and an entry vanishes once the
    caller drops its own reference to the value.
    """

    def __init__(self):
        self.entries = weakref.WeakValueDictionary()
        self.hits = 0
        self.misses = 0

    def get_or_create(self, name):
        existing = self.entries.get(name)
        if existing is not None:
            self.hits = self.hits + 1
            return existing
        self.misses = self.misses + 1
        created = Widget(name)
        self.entries[name] = created
        return created

    def report(self):
        return ("cache holds %d entries; %d hit(s), %d miss(es); keys=%s"
                % (len(self.entries), self.hits, self.misses,
                   sorted(self.entries.keys())))


cache = WidgetCache()
print("get_or_create('alpha'):", cache.get_or_create("alpha"))
print("get_or_create('beta'): ", cache.get_or_create("beta"))
print("get_or_create('alpha') again:",
      cache.get_or_create("alpha"), "(this one is a cache hit)")
print(cache.report())
print()
print("the widgets are alive because the call above still holds them;")
print("drop those references and the entries would go away on their own.")
