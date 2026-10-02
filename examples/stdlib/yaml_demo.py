"""yaml: read and write YAML.

Run with:  /tmp/gpy examples/stdlib/yaml_demo.py

safe_load() parses a YAML document into dicts, lists and scalars; safe_dump()
writes one back. load_all()/dump_all() handle multi-document streams. The
"safe_" prefix means only plain data types are constructed.

Interpreter note: a malformed document raises yaml.YAMLError, as you would
expect. Because dict ordering is not insertion order in this interpreter,
dumped mappings may list their keys in any order.
"""

import yaml

print("--- scalars ---")
for document in ["42", "3.14", "true", "false", "null", "a plain string",
                 "'quoted string'", "2024-01-15"]:
    value = yaml.safe_load(document)
    print("  %-20s -> %-20r (%s)" % (repr(document), value, type(value).__name__))

print()
print("--- lists ---")
print("flow style  [1, 2, 3]:   ", yaml.safe_load("[1, 2, 3]"))
print("block style:")
print(yaml.safe_load("- one\n- two\n- three\n"))

print()
print("--- mappings ---")
DOCUMENT = """
name: gpython
version: 0.1
features:
  - f-strings
  - match statements
limits:
  async: false
  threads: false
"""
parsed = yaml.safe_load(DOCUMENT)
print("input document:")
print(DOCUMENT)
print("parsed into a dict with keys:", sorted(parsed.keys()))
print("  name:    ", parsed["name"])
print("  version: ", parsed["version"])
print("  features:", parsed["features"])
print("  limits:  ", parsed["limits"])

print()
print("--- nested access ---")
print("parsed['features'][0]:        ", parsed["features"][0])
print("parsed['limits']['async']:    ", parsed["limits"]["async"])
print("it is a real bool:            ", isinstance(parsed["limits"]["async"], bool))
print("feature count:                ", len(parsed["features"]))

print()
print("--- dumping back out ---")
simple = {"name": "gpython", "tags": ["python", "go"]}
dumped = yaml.safe_dump(simple)
print("safe_dump({'name': ..., 'tags': [...]}):")
print(dumped)
reloaded = yaml.safe_load(dumped)
print("reloading it gives the same data:", reloaded == simple)
print("(dict key order in the dump may vary run to run)")

print()
print("--- multiple documents in one stream ---")
STREAM = "---\nfirst: 1\n---\nsecond: 2\n---\n- a\n- b\n"
print("stream:")
print(STREAM)
documents = list(yaml.load_all(STREAM))
print("load_all gives %d documents:" % (len(documents),))
for index, document in enumerate(documents):
    print("  %d: %r" % (index, document))
print()
print("dump_all writes a stream back:")
print(yaml.dump_all([{"first": 1}, {"second": 2}]))

print()
print("--- errors ---")
for label, bad in [("unclosed flow sequence", "a: [1,"),
                   ("bad indentation", "a: 1\n  b: 2\n"),
                   ("a tab in the indentation", "a:\n\t- 1\n")]:
    try:
        yaml.safe_load(bad)
        print("  %-24s parsed without error" % (label,))
    except yaml.YAMLError as err:
        print("  %-24s -> YAMLError: %s" % (label, str(err).split("\n")[0]))

print()
print("--- load() and safe_load() agree on plain data ---")
plain = "key: value\nlist: [1, 2]\n"
print("safe_load:", yaml.safe_load(plain))
print("load:     ", yaml.load(plain))

print()
print("--- the module surface ---")
for name in ["safe_load", "safe_dump", "load", "dump", "load_all", "dump_all",
             "full_load", "YAMLError", "SafeLoader", "add_constructor",
             "add_representer", "CSafeLoader"]:
    print("  yaml.%-18s %s" % (name, hasattr(yaml, name)))

print()
print("--- a worked example: configuration with defaults ---")

DEFAULTS = {
    "host": "localhost",
    "port": 8080,
    "debug": False,
    "workers": 4,
}


def load_config(text, defaults):
    """Merge a YAML document over a set of defaults."""
    supplied = yaml.safe_load(text) or {}
    merged = {}
    for key in defaults:
        merged[key] = supplied.get(key, defaults[key])
    extra = [key for key in supplied if key not in defaults]
    return merged, extra


CONFIG = """
host: api.example.com
workers: 8
"""
merged, extra = load_config(CONFIG, DEFAULTS)
print("document:", repr(CONFIG))
print("merged configuration:")
for key in sorted(merged.keys()):
    print("  %-8s = %r" % (key, merged[key]))
print("keys the defaults did not know about:", extra)

print()
print("an unknown key is kept out of the merge, but reported:")
merged, extra = load_config("host: x\nverbosity: high\n", DEFAULTS)
print("  merged keys:", sorted(merged.keys()))
print("  extra keys: ", extra)

print()
print("--- validating the parsed shape ---")


def validate(document, defaults):
    """Return a list of complaints about a parsed configuration."""
    problems = []
    if not isinstance(document, dict):
        return ["the document is not a mapping"]
    for key in defaults:
        if key not in document:
            continue
        if not isinstance(document[key], type(defaults[key])):
            problems.append("%s should be %s, got %s"
                            % (key, type(defaults[key]).__name__,
                               type(document[key]).__name__))
    return problems


for label, text in [("good", "host: x\nworkers: 2\n"),
                    ("wrong type", "workers: many\n"),
                    ("not a mapping", "- one\n- two\n")]:
    document = yaml.safe_load(text)
    problems = validate(document, DEFAULTS)
    print("  %-14s -> %s" % (label, problems if problems else "ok"))
