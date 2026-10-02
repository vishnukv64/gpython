"""stat: interpreting file mode bits.

Run with:  /tmp/gpy examples/stdlib/stat_demo.py

stat exposes the S_I* constants and the S_IS* predicates, plus filemode() which
renders a mode the way `ls -l` does.

Interpreter notes:
  * os.stat() is NOT present in this interpreter, so there is no st_mode to
    read off a real file. The predicates are exercised on mode values built
    from the constants themselves, which is exactly what they operate on.
  * stat.filemode() takes an integer mode and works.
"""

import os
import stat

print("--- the permission constants ---")
print("owner read/write/execute:  %s %s %s" % (stat.S_IRUSR, stat.S_IWUSR, stat.S_IXUSR))
print("group read/write/execute:  %s %s %s" % (stat.S_IRGRP, stat.S_IWGRP, stat.S_IXGRP))
print("other read/write/execute:  %s %s %s" % (stat.S_IROTH, stat.S_IWOTH, stat.S_IXOTH))
print("the masks:")
print("  S_IRWXU (owner):  %o" % (stat.S_IRWXU,))
print("  S_IRWXG (group):  %o" % (stat.S_IRWXG,))
print("  S_IRWXO (other):  %o" % (stat.S_IRWXO,))
print("special bits: S_ISUID=%o S_ISGID=%o S_ISVTX=%o"
      % (stat.S_ISUID, stat.S_ISGID, stat.S_ISVTX))

print()
print("--- the file-type bits ---")
for name in ["S_IFREG", "S_IFDIR", "S_IFLNK", "S_IFIFO", "S_IFSOCK", "S_IFBLK", "S_IFCHR"]:
    print("  %-9s = %o" % (name, getattr(stat, name)))
print("  S_IFMT (the mask)  = %o" % (stat.S_IFMT,))

print()
print("--- filemode() renders a mode like `ls -l` ---")


def render(mode):
    return "%s  %04o" % (stat.filemode(mode), mode)


examples = [
    ("a readable file", 0o100644),
    ("an executable", 0o100755),
    ("owner only", 0o100600),
    ("a directory, 755", 0o040755),
    ("a directory, 700", 0o040700),
]
for label, mode in examples:
    print("  %-18s %s" % (label, render(mode)))

print()
print("--- building a mode from constants ---")
mode = stat.S_IFREG | stat.S_IRUSR | stat.S_IWUSR | stat.S_IRGRP | stat.S_IROTH
print("regular + rw-r--r-- = %o ->" % (mode,), stat.filemode(mode))
dir_mode = stat.S_IFDIR | stat.S_IRWXU | stat.S_IRGRP | stat.S_IXGRP
print("directory + rwxr-x--- = %o ->" % (dir_mode,), stat.filemode(dir_mode))
dir_mode = dir_mode | stat.S_IROTH | stat.S_IXOTH
print("after adding other r-x: %o ->" % (dir_mode,), stat.filemode(dir_mode))

print()
print("--- the S_IS* predicates ---")
modes = [
    ("regular file", stat.S_IFREG | 0o644),
    ("directory", stat.S_IFDIR | 0o755),
    ("symlink", stat.S_IFLNK | 0o777),
    ("fifo", stat.S_IFIFO | 0o644),
    ("socket", stat.S_IFSOCK | 0o755),
    ("block device", stat.S_IFBLK | 0o660),
    ("character device", stat.S_IFCHR | 0o660),
]
header = "  %-17s %-6s %-6s %-6s %-6s %-6s"
print(header % ("mode", "REG", "DIR", "LNK", "FIFO", "SOCK"))
for label, mode in modes:
    print(header % (label,
                    stat.S_ISREG(mode),
                    stat.S_ISDIR(mode),
                    stat.S_ISLNK(mode),
                    stat.S_ISFIFO(mode),
                    stat.S_ISSOCK(mode)))

print()
print("--- why the mode constants matter when os.stat is missing ---")
print("hasattr(os.stat):  ", hasattr(os, "stat"))
print("hasattr(os.lstat): ", hasattr(os, "lstat"))
print("But os.path carries the same information for the common questions:")
print("  os.path.isdir('/tmp'):  ", os.path.isdir("/tmp"))
print("  os.path.isfile('/tmp'): ", os.path.isfile("/tmp"))
print("  os.path.exists('/tmp'): ", os.path.exists("/tmp"))

print()
print("--- a worked example: a permission summary ---")


class Permission:
    def __init__(self, mode):
        self.mode = mode

    def owner(self):
        return (self.mode >> 6) & 0o7

    def group(self):
        return (self.mode >> 3) & 0o7

    def other(self):
        return self.mode & 0o7

    def describe(self):
        parts = []
        parts.append("owner=%d" % (self.owner(),))
        parts.append("group=%d" % (self.group(),))
        parts.append("other=%d" % (self.other(),))
        parts.append("display=%s" % (stat.filemode(self.mode),))
        kind = "dir" if stat.S_ISDIR(self.mode) else ("file" if stat.S_ISREG(self.mode) else "special")
        parts.append("kind=%s" % (kind,))
        return "  ".join(parts)

    def is_world_writable(self):
        return (self.other() & 0o2) != 0


for label, mode in [("safe file", 0o100644), ("world-writable", 0o100666),
                    ("private dir", 0o040700), ("shared dir", 0o040777)]:
    permission = Permission(mode)
    print("  %-16s %s" % (label, permission.describe()))
    if permission.is_world_writable():
        print("  %-16s    ^^ writable by anyone: a security concern" % ("",))
