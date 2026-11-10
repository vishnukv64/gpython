# A venv made by EnvBuilder records pyvenv.cfg and the site-packages layout.
import os, sys, tempfile, venv

with tempfile.TemporaryDirectory() as d:
    env = os.path.join(d, "env")
    venv.create(env, with_pip=False)
    cfg = {}
    with open(os.path.join(env, "pyvenv.cfg")) as f:
        for line in f:
            k, _, val = line.partition("=")
            cfg[k.strip()] = val.strip()
    print(sorted(cfg)[:3], cfg["include-system-site-packages"])
    # A Windows venv is Lib\site-packages and Scripts (CPython's nt_venv).
    if os.name == "nt":
        site, scripts = os.path.join(env, "Lib", "site-packages"), os.path.join(env, "Scripts")
    else:
        site = os.path.join(env, "lib", "python%d.%d" % sys.version_info[:2], "site-packages")
        scripts = os.path.join(env, "bin")
    print(os.path.isdir(site))
    print(sorted(os.listdir(scripts))[:1])
