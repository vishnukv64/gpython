"""uuid: universally unique identifiers.

Run with:  /tmp/gpy examples/stdlib/uuid_demo.py

uuid4() is random, uuid1() is time-based, and uuid3()/uuid5() hash a namespace
and a name so the same input always yields the same id.

Interpreter note: UUID objects stringify correctly, but they have no .hex,
.int or .version attributes here, so the parts are read from the string form.
"""

import uuid

print("--- the versions available ---")
print("uuid1: time-based    ", uuid.uuid1())
print("uuid4: random        ", uuid.uuid4())
print("uuid3: MD5 of a name ", uuid.uuid3(uuid.NAMESPACE_DNS, "example.com"))
print("uuid5: SHA-1 of a name", uuid.uuid5(uuid.NAMESPACE_DNS, "example.com"))

print()
print("--- the standard namespaces are fixed ---")
for name in ["NAMESPACE_DNS", "NAMESPACE_URL", "NAMESPACE_OID", "NAMESPACE_X500"]:
    print("  %-16s %s" % (name, getattr(uuid, name)))

print()
print("--- a string is 36 characters: 8-4-4-4-12 ---")
for label, function in [("uuid1", uuid.uuid1), ("uuid4", uuid.uuid4)]:
    value = str(function())
    parts = value.split("-")
    print("  %-6s %s  length=%d parts=%d" % (label, value, len(value), len(parts)))
    print("         part lengths: %s" % ([len(part) for part in parts],))

print()
print("--- name-based ids are deterministic ---")
first = uuid.uuid5(uuid.NAMESPACE_DNS, "example.com")
second = uuid.uuid5(uuid.NAMESPACE_DNS, "example.com")
print("uuid5 of the same name twice:", first == second)
print("a different name gives a different id:",
      uuid.uuid5(uuid.NAMESPACE_DNS, "example.org") != first)
print("uuid3 and uuid5 differ (MD5 vs SHA-1):",
      uuid.uuid3(uuid.NAMESPACE_DNS, "example.com") != first)
print("different namespaces differ:",
      uuid.uuid5(uuid.NAMESPACE_URL, "example.com") != first)

print()
print("--- random ids collide with negligible probability ---")
random_ids = [str(uuid.uuid4()) for _ in range(5)]
for value in random_ids:
    print("  ", value)
print("all distinct:", len(set(random_ids)) == len(random_ids))

print()
print("--- building one from an explicit string ---")
literal = uuid.UUID("12345678-1234-5678-1234-567812345678")
print("UUID(...):   ", literal)
print("str() of it: ", str(literal))
print("it is not the nil uuid:", literal != uuid.UUID("00000000-0000-0000-0000-000000000000"))
print("the nil uuid is:", uuid.UUID("00000000-0000-0000-0000-000000000000"))

print()
print("--- the version nibble lives in the 13th hex digit ---")
for label, value in [("uuid1", uuid.uuid1()), ("uuid4", uuid.uuid4()),
                     ("uuid3", uuid.uuid3(uuid.NAMESPACE_DNS, "x")),
                     ("uuid5", uuid.uuid5(uuid.NAMESPACE_DNS, "x"))]:
    text = str(value).replace("-", "")
    print("  %-6s %s  version nibble=%s" % (label, value, text[12]))

print()
print("--- the module surface ---")
for name in ["uuid1", "uuid3", "uuid4", "uuid5", "UUID", "NAMESPACE_DNS",
             "NAMESPACE_URL", "NAMESPACE_OID", "NAMESPACE_X500",
             "RESERVED_NCS", "RESERVED_MICROSOFT", "RESERVED_FUTURE", "RFC_4122"]:
    print("  uuid.%-20s %s" % (name, hasattr(uuid, name)))

print()
print("--- a worked example: stable ids for records across runs ---")


class IdRegistry:
    """Derives a stable UUID for a record from its natural key."""

    def __init__(self, namespace):
        self.namespace = namespace
        self.seen = []

    def id_for(self, *parts):
        key = "|".join([str(part) for part in parts])
        value = uuid.uuid5(self.namespace, key)
        self.seen.append((key, value))
        return value

    def report(self):
        for key, value in self.seen:
            print("  %-24s -> %s" % (key, value))


registry = IdRegistry(uuid.NAMESPACE_URL)
customer = registry.id_for("customer", "ann@example.com")
order = registry.id_for("order", "A-1001")
print("customer id:", customer)
print("order id:   ", order)
print("deriving the customer id again gives the same value:",
      registry.id_for("customer", "ann@example.com") == customer)
print("the two ids differ:", customer != order)
print()
registry.report()

print()
print("--- and a short-form renderer, since .hex is absent ---")


def short(value):
    """First 8 hex digits of a UUID, derived from its string form."""
    return str(value).replace("-", "")[:8]


for value in [uuid.uuid4(), uuid.uuid5(uuid.NAMESPACE_DNS, "example.com")]:
    print("  %s -> %s" % (value, short(value)))
