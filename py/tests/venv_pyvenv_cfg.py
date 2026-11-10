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
    print(os.path.isdir(os.path.join(env, "lib", "python%d.%d" % sys.version_info[:2], "site-packages")))
    print(sorted(os.listdir(os.path.join(env, "bin")))[:1])
