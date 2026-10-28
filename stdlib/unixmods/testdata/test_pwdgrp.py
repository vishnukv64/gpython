import pwd, grp, os
r = pwd.getpwuid(0)
print(type(r).__name__, type(r).__module__, len(r), r.pw_uid, r[2], r.pw_name == pwd.getpwnam(r.pw_name).pw_name)
print(r._fields if hasattr(r, '_fields') else 'nofields', pwd.struct_passwd.n_fields)
print(repr(r).split('(')[0], [f for f in dir(r) if f.startswith('pw_')])
g = grp.getgrgid(0)
print(type(g).__name__, len(g), g.gr_gid, g[2], isinstance(g.gr_mem, list), g.gr_name == grp.getgrnam(g.gr_name).gr_name)
print(len(pwd.getpwall()) > 0, len(grp.getgrall()) > 0, all(isinstance(x, pwd.struct_passwd) for x in pwd.getpwall()))
for f in (lambda: pwd.getpwnam('nonexistent_zz'), lambda: pwd.getpwuid(99999), lambda: pwd.getpwnam(1), lambda: pwd.getpwuid('a'), lambda: grp.getgrgid(99999), lambda: grp.getgrnam('nonex'), lambda: pwd.getpwnam('a\0b'), lambda: grp.getgrnam(1), lambda: grp.getgrgid('x')):
    try: f()
    except Exception as e: print(type(e).__name__, e)
import pickle
print(pickle.loads(pickle.dumps(r)) == r)
print(pwd.struct_passwd(tuple(r)) == r)
try: pwd.struct_passwd((1,2))
except TypeError as e: print(e)
