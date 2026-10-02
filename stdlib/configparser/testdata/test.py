import configparser, io
c = configparser.ConfigParser()
c.read_string("""
[DEFAULT]
d = dval
[styles]
foo = bold red
bar = italic
[sec2]
x: 1
   continued
y = 2
""")
print("sections:", c.sections())
print("has_section:", c.has_section("styles"), c.has_section("nope"))
print("items:", sorted(c.items("styles")))
print("get:", c.get("styles", "foo"))
print("default-inherited:", c.get("styles", "d"))
print("colon sep:", c.get("sec2", "x"))
print("getint:", c.getint("sec2", "y"))
print("proxy:", c["styles"]["foo"], "foo" in c["styles"])
print("options:", sorted(c.options("styles")))

doc = "finished"
