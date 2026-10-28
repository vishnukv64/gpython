// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods

import (
	"golang.org/x/sys/unix"

	"github.com/vishnukv64/gpython/py"
)

var resourceConsts = []namedConst{
	{"RLIMIT_CPU", unix.RLIMIT_CPU}, {"RLIMIT_FSIZE", unix.RLIMIT_FSIZE}, {"RLIMIT_DATA", unix.RLIMIT_DATA},
	{"RLIMIT_STACK", unix.RLIMIT_STACK}, {"RLIMIT_CORE", unix.RLIMIT_CORE}, {"RLIMIT_AS", unix.RLIMIT_AS},
	{"RLIMIT_RSS", unix.RLIMIT_RSS}, {"RLIMIT_MEMLOCK", unix.RLIMIT_MEMLOCK}, {"RLIMIT_NPROC", unix.RLIMIT_NPROC},
	{"RLIMIT_NOFILE", unix.RLIMIT_NOFILE}, {"RUSAGE_SELF", unix.RUSAGE_SELF}, {"RUSAGE_CHILDREN", unix.RUSAGE_CHILDREN},
}

const resourceSrc = structSeqSrc + `
error = OSError
struct_rusage = _structseq('struct_rusage', 'resource',
    ['ru_utime', 'ru_stime', 'ru_maxrss', 'ru_ixrss', 'ru_idrss', 'ru_isrss', 'ru_minflt', 'ru_majflt',
     'ru_nswap', 'ru_inblock', 'ru_oublock', 'ru_msgsnd', 'ru_msgrcv', 'ru_nsignals', 'ru_nvcsw', 'ru_nivcsw'],
    "struct_rusage: Result from getrusage.\n\nThis object may be accessed either as a tuple of\n    (utime,stime,maxrss,ixrss,idrss,isrss,minflt,majflt,\n    nswap,inblock,oublock,msgsnd,msgrcv,nsignals,nvcsw,nivcsw)\nor via the attributes ru_utime, ru_stime, ru_maxrss, and so on.")
def _int(v):
    import operator
    return operator.index(v)
def getrlimit(resource, /):
    resource = _int(resource)
    r = _getrlimit(resource)
    if r is None:
        raise ValueError("invalid resource specified")
    return r
def setrlimit(resource, limits, /):
    resource = _int(resource)
    limits = tuple(limits)
    if len(limits) != 2:
        raise ValueError("expected a tuple of 2 integers")
    soft, hard = limits
    soft = _int(soft); hard = _int(hard)
    if not -2**63 <= soft < 2**64 or not -2**63 <= hard < 2**64:
        raise OverflowError("Python int too large to convert to C long")
    errno_ = _setrlimit(resource, soft, hard)
    if errno_ == 22:
        raise ValueError("current limit exceeds maximum limit")
    if errno_ == 1:
        raise ValueError("not allowed to raise maximum limit")
    if errno_ == -1:
        raise ValueError("invalid resource specified")
    if errno_:
        raise OSError(errno_, __import__('os').strerror(errno_) if hasattr(__import__('os'), 'strerror') else '')
def getrusage(who, /):
    who = _int(who)
    r = _getrusage(who)
    if r is None:
        raise ValueError("invalid who parameter")
    return struct_rusage(r)
def getpagesize():
    return _pagesize()
del _structseq, _operator
`

func init() {
	g := py.NewStringDict()
	for _, c := range append(append([]namedConst{}, resourceConsts...), osResourceConsts...) {
		g.Set(c.name, py.Int(c.val))
	}
	g.Set("RLIM_INFINITY", py.Int(rlimInfinity))
	validRes := func(r int) bool {
		return r >= 0 && r < rlimNLimits
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "resource", Doc: "Resource usage information."},
		Globals: g,
		Methods: []*py.Method{
			py.MustNewMethod("_getrlimit", func(self py.Object, args py.Tuple) (py.Object, error) {
				res := intArg(args[0])
				if !validRes(res) {
					return py.None, nil
				}
				var rl unix.Rlimit
				if err := unix.Getrlimit(res, &rl); err != nil {
					return nil, py.OSErrorFrom(err, "")
				}
				return py.Tuple{py.Int(int64(rl.Cur)), py.Int(int64(rl.Max))}, nil
			}, 0, ""),
			py.MustNewMethod("_setrlimit", func(self py.Object, args py.Tuple) (py.Object, error) {
				res := intArg(args[0])
				if !validRes(res) {
					return py.Int(-1), nil
				}
				rl := unix.Rlimit{Cur: uint64(args[1].(py.Int)), Max: uint64(args[2].(py.Int))}
				if err := unix.Setrlimit(res, &rl); err != nil {
					if en, ok := err.(unix.Errno); ok {
						return py.Int(int(en)), nil
					}
					return nil, py.OSErrorFrom(err, "")
				}
				return py.Int(0), nil
			}, 0, ""),
			py.MustNewMethod("_getrusage", func(self py.Object, args py.Tuple) (py.Object, error) {
				var ru unix.Rusage
				if err := unix.Getrusage(intArg(args[0]), &ru); err != nil {
					if err == unix.EINVAL {
						return py.None, nil
					}
					return nil, py.OSErrorFrom(err, "")
				}
				tv := func(t unix.Timeval) py.Object { return py.Float(float64(t.Sec) + float64(t.Usec)*1e-6) }
				return py.Tuple{tv(ru.Utime), tv(ru.Stime), py.Int(ru.Maxrss), py.Int(ru.Ixrss), py.Int(ru.Idrss),
					py.Int(ru.Isrss), py.Int(ru.Minflt), py.Int(ru.Majflt), py.Int(ru.Nswap), py.Int(ru.Inblock),
					py.Int(ru.Oublock), py.Int(ru.Msgsnd), py.Int(ru.Msgrcv), py.Int(ru.Nsignals), py.Int(ru.Nvcsw),
					py.Int(ru.Nivcsw)}, nil
			}, 0, ""),
			py.MustNewMethod("_pagesize", func(self py.Object, args py.Tuple) (py.Object, error) {
				return py.Int(unix.Getpagesize()), nil
			}, 0, ""),
		},
		CodeSrc: resourceSrc,
	})
}
