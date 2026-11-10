# sysconfig layouts relative to a prefix: posix_prefix and posix_venv, as a
# virtual environment (PEP 405) and pip --prefix use them.
import sysconfig

v = {"base": "/P", "platbase": "/P", "installed_base": "/B", "installed_platbase": "/B"}
for scheme in ("posix_prefix", "posix_venv", "posix_home"):
    print(scheme)
    paths = sysconfig.get_paths(scheme, vars=v)
    for k in sorted(paths):
        print(" ", k, paths[k])
print(sysconfig.get_path("purelib", "posix_venv", vars=v))
print(sorted(sysconfig.get_path_names()))
print("posix_venv" in sysconfig.get_scheme_names(), "venv" in sysconfig.get_scheme_names())
