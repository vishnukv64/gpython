// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods

import (
	"bufio"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// readColon returns the colon-separated records of an /etc database that have
// at least min fields.  A missing file is an empty database, as for getpwent.
func readColon(path string, min int) [][]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		if p := strings.Split(line, ":"); len(p) >= min {
			out = append(out, p)
		}
	}
	return out
}

func atoi(s string) py.Object {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return py.Int(0)
	}
	return py.Int(n)
}

func pwTuple(p []string) py.Object {
	return py.Tuple{py.String(p[0]), py.String(p[1]), atoi(p[2]), atoi(p[3]), py.String(p[4]), py.String(p[5]), py.String(p[6])}
}

func grTuple(p []string) py.Object {
	var mem []py.Object
	if len(p) > 3 && p[3] != "" {
		for _, m := range strings.Split(p[3], ",") {
			mem = append(mem, py.String(m))
		}
	}
	return py.Tuple{py.String(p[0]), py.String(p[1]), atoi(p[2]), py.NewListFromItems(mem)}
}

// pwLookup finds a user by name (str) or uid (int): /etc/passwd first, then
// os/user, which asks the system database where the build can (cgo) - on macOS
// most accounts live in Open Directory, not in /etc/passwd.
func pwLookup(key py.Object) py.Object {
	name, byName := key.(py.String)
	for _, p := range readColon("/etc/passwd", 7) {
		if (byName && p[0] == string(name)) || (!byName && p[2] == strconv.FormatInt(int64(key.(py.Int)), 10)) {
			return pwTuple(p)
		}
	}
	var u *user.User
	var err error
	if byName {
		u, err = user.Lookup(string(name))
	} else {
		u, err = user.LookupId(strconv.FormatInt(int64(key.(py.Int)), 10))
	}
	if err != nil {
		return py.None
	}
	return py.Tuple{py.String(u.Username), py.String("*"), atoi(u.Uid), atoi(u.Gid), py.String(u.Name), py.String(u.HomeDir), py.String("")}
}

func grLookup(key py.Object) py.Object {
	name, byName := key.(py.String)
	for _, p := range readColon("/etc/group", 3) {
		if (byName && p[0] == string(name)) || (!byName && p[2] == strconv.FormatInt(int64(key.(py.Int)), 10)) {
			return grTuple(p)
		}
	}
	var g *user.Group
	var err error
	if byName {
		g, err = user.LookupGroup(string(name))
	} else {
		g, err = user.LookupGroupId(strconv.FormatInt(int64(key.(py.Int)), 10))
	}
	if err != nil {
		return py.None
	}
	return py.Tuple{py.String(g.Name), py.String("*"), atoi(g.Gid), py.NewList()}
}

func all(path string, min int, f func([]string) py.Object) py.Object {
	l := py.NewList()
	for _, p := range readColon(path, min) {
		l.Items = append(l.Items, f(p))
	}
	return l
}

const pwdSrc = structSeqSrc + `
struct_passwd = _structseq('struct_passwd', 'pwd',
    ['pw_name', 'pw_passwd', 'pw_uid', 'pw_gid', 'pw_gecos', 'pw_dir', 'pw_shell'],
    "pwd.struct_passwd: Results from getpw*() routines.\n\nThis object may be accessed either as a tuple of\n  (pw_name,pw_passwd,pw_uid,pw_gid,pw_gecos,pw_dir,pw_shell)\nor via the object attributes as named in the above tuple.")
def _chkstr(fn, name):
    if not isinstance(name, str):
        raise TypeError("%s() argument must be str, not %s" % (fn, type(name).__name__))
    if '\0' in name:
        raise ValueError("embedded null byte")
def getpwnam(name, /):
    "Return the password database entry for the given user name."
    _chkstr('getpwnam', name)
    r = _pwlookup(name)
    if r is None:
        raise KeyError("getpwnam(): name not found: %r" % (name,))
    return struct_passwd(r)
def getpwuid(uid, /):
    "Return the password database entry for the given numeric user ID."
    import operator
    try:
        k = operator.index(uid)
    except TypeError:
        raise TypeError("uid should be integer, not %s" % type(uid).__name__) from None
    r = _pwlookup(k) if 0 <= k < 2**32 else None
    if r is None:
        raise KeyError("getpwuid(): uid not found: %s" % k)
    return struct_passwd(r)
def getpwall():
    "Return a list of all available password database entries, in arbitrary order."
    return [struct_passwd(r) for r in _pwall()]
del _structseq, _operator
`

const grpSrc = structSeqSrc + `
struct_group = _structseq('struct_group', 'grp',
    ['gr_name', 'gr_passwd', 'gr_gid', 'gr_mem'],
    "grp.struct_group: Results from getgr*() routines.\n\nThis object may be accessed either as a tuple of\n  (gr_name,gr_passwd,gr_gid,gr_mem)\nor via the object attributes as named in the above tuple.")
def getgrnam(name, /):
    "Return the group database entry for the given group name."
    if not isinstance(name, str):
        raise TypeError("getgrnam() argument 'name' must be str, not %s" % type(name).__name__)
    if '\0' in name:
        raise ValueError("embedded null byte")
    r = _grlookup(name)
    if r is None:
        raise KeyError("getgrnam(): name not found: %r" % (name,))
    return struct_group(r)
def getgrgid(id):
    "Return the group database entry for the given numeric group ID."
    import operator
    try:
        k = operator.index(id)
    except TypeError:
        raise TypeError("gid should be integer, not %s" % type(id).__name__) from None
    r = _grlookup(k) if 0 <= k < 2**32 else None
    if r is None:
        raise KeyError("getgrgid(): gid not found: %s" % k)
    return struct_group(r)
def getgrall():
    "Return a list of all available group entries, in arbitrary order."
    return [struct_group(r) for r in _grall()]
del _structseq, _operator
`

func lookupMethod(name string, f func(py.Object) py.Object) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		return f(args[0]), nil
	}, 0, "")
}

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "pwd", Doc: "This module provides access to the Unix password database.\nIt is available on all Unix versions."},
		Methods: []*py.Method{
			lookupMethod("_pwlookup", pwLookup),
			py.MustNewMethod("_pwall", func(self py.Object, args py.Tuple) (py.Object, error) {
				return all("/etc/passwd", 7, pwTuple), nil
			}, 0, ""),
		},
		CodeSrc: pwdSrc,
	})
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "grp", Doc: "Access to the Unix group database."},
		Methods: []*py.Method{
			lookupMethod("_grlookup", grLookup),
			py.MustNewMethod("_grall", func(self py.Object, args py.Tuple) (py.Object, error) {
				return all("/etc/group", 3, grTuple), nil
			}, 0, ""),
		},
		CodeSrc: grpSrc,
	})
}
