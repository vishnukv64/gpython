import importlib.util as u

for name in ("json", "os", "sys", "math", "no_such_module_xyz"):
    s = u.find_spec(name)
    print(name, None if s is None else s.name)
s = u.find_spec("sys")
print(s.origin, s.loader.__name__, s.loader.is_package("sys"), s.loader.get_source("sys"), s.loader.get_code("sys"))
