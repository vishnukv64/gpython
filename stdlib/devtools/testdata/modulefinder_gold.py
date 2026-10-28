import modulefinder, sys, os
here = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mf")
mf = modulefinder.ModuleFinder(path=[here] + sys.path)
mf.run_script(os.path.join(here, "main_mod.py"))
print(sorted(n for n in mf.modules if n in ("main_mod", "helper_mod", "os", "json", "re", "__main__")))
print(sorted(n for n in mf.badmodules if n == "nonexistent_zzz"))
print(sorted(mf.modules["helper_mod"].globalnames), sorted(mf.modules["__main__"].globalnames))
