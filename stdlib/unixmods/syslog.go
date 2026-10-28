// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods

import (
	"log/syslog"
	"runtime"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

var syslogConsts = []namedConst{
	{"LOG_EMERG", 0}, {"LOG_ALERT", 1}, {"LOG_CRIT", 2}, {"LOG_ERR", 3}, {"LOG_WARNING", 4}, {"LOG_NOTICE", 5},
	{"LOG_INFO", 6}, {"LOG_DEBUG", 7},
	{"LOG_PID", 1}, {"LOG_CONS", 2}, {"LOG_ODELAY", 4}, {"LOG_NDELAY", 8}, {"LOG_NOWAIT", 0x10}, {"LOG_PERROR", 0x20},
	{"LOG_KERN", 0}, {"LOG_USER", 8}, {"LOG_MAIL", 16}, {"LOG_DAEMON", 24}, {"LOG_AUTH", 32}, {"LOG_SYSLOG", 40},
	{"LOG_LPR", 48}, {"LOG_NEWS", 56}, {"LOG_UUCP", 64}, {"LOG_CRON", 72}, {"LOG_AUTHPRIV", 80}, {"LOG_FTP", 88},
	{"LOG_LOCAL0", 128}, {"LOG_LOCAL1", 136}, {"LOG_LOCAL2", 144}, {"LOG_LOCAL3", 152}, {"LOG_LOCAL4", 160},
	{"LOG_LOCAL5", 168}, {"LOG_LOCAL6", 176}, {"LOG_LOCAL7", 184},
}

var darwinSyslogConsts = []namedConst{
	{"LOG_NETINFO", 96}, {"LOG_REMOTEAUTH", 104}, {"LOG_INSTALL", 112}, {"LOG_RAS", 120}, {"LOG_LAUNCHD", 192},
}

// syslog state: one writer per (ident, facility), dropped on closelog.  Like
// libc's syslog(3) a failure to reach the daemon is silent.
var (
	slMu      sync.Mutex
	slWriters = map[[2]any]*syslog.Writer{}
)

func slSend(ident string, facility, level int, msg string) {
	slMu.Lock()
	defer slMu.Unlock()
	key := [2]any{ident, facility}
	w := slWriters[key]
	if w == nil {
		var err error
		if w, err = syslog.New(syslog.Priority(facility), ident); err != nil {
			return
		}
		slWriters[key] = w
	}
	switch level {
	case 0:
		_ = w.Emerg(msg)
	case 1:
		_ = w.Alert(msg)
	case 2:
		_ = w.Crit(msg)
	case 3:
		_ = w.Err(msg)
	case 4:
		_ = w.Warning(msg)
	case 5:
		_ = w.Notice(msg)
	case 6:
		_ = w.Info(msg)
	default:
		_ = w.Debug(msg)
	}
}

func slClose() {
	slMu.Lock()
	defer slMu.Unlock()
	for k, w := range slWriters {
		_ = w.Close()
		delete(slWriters, k)
	}
}

const syslogSrc = `
import sys as _sys, operator as _operator
_ident = None
_option = 0
_facility = LOG_USER
_mask = 0xff
_opened = False
def LOG_MASK(pri, /):
    "Calculates the mask for the individual priority pri."
    return 1 << pri
def LOG_UPTO(pri, /):
    "Calculates the mask for all priorities up to and including pri."
    return (1 << (pri + 1)) - 1
def openlog(ident=None, logoption=0, facility=LOG_USER):
    "Set logging options of subsequent syslog() calls."
    global _ident, _option, _facility, _opened
    if ident is not None and not isinstance(ident, str):
        raise TypeError("openlog() argument 'ident' must be str or None, not %s" % type(ident).__name__)
    if ident is None:
        a = getattr(_sys, 'argv', None)
        ident = a[0].rsplit('/', 1)[-1] if a and a[0] else 'python'
    _ident, _option, _facility, _opened = ident, _operator.index(logoption), _operator.index(facility), True
def syslog(*args):
    "syslog([priority=LOG_INFO,] message)\nSend the string message to the system logger."
    if len(args) == 1:
        priority, message = LOG_INFO, args[0]
    elif len(args) == 2:
        priority, message = _operator.index(args[0]), args[1]
    else:
        raise TypeError("syslog expected at least 1 argument, got %d" % len(args) if not args else "syslog expected at most 2 arguments, got %d" % len(args))
    if not isinstance(message, str):
        raise TypeError("syslog() argument 'message' must be str, not %s" % type(message).__name__)
    if '\0' in message:
        raise ValueError("embedded null character")
    if not _opened:
        openlog()
    level = priority & 7
    if not (_mask & (1 << level)):
        return
    fac = priority & 0x3f8 or _facility
    if _option & LOG_PERROR:
        print("%s: %s" % (_ident, message), file=_sys.stderr)
    _send(_ident, fac, level, message)
def closelog():
    "Reset the syslog module values and call the system library closelog()."
    global _opened, _ident, _option, _facility
    _opened = False; _ident = None; _option = 0; _facility = LOG_USER
    _close()
def setlogmask(maskpri, /):
    "Set the log mask for the current process."
    global _mask
    maskpri = _operator.index(maskpri)
    old = _mask
    if maskpri:
        _mask = maskpri & 0xff
    return old
`

func init() {
	g := py.NewStringDict()
	cs := syslogConsts
	if runtime.GOOS == "darwin" {
		cs = append(append([]namedConst{}, cs...), darwinSyslogConsts...)
	}
	for _, c := range cs {
		g.Set(c.name, py.Int(c.val))
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "syslog", Doc: "Unix syslog library interface"},
		Globals: g,
		Methods: []*py.Method{
			py.MustNewMethod("_send", func(self py.Object, args py.Tuple) (py.Object, error) {
				slSend(string(args[0].(py.String)), intArg(args[1]), intArg(args[2]), string(args[3].(py.String)))
				return py.None, nil
			}, 0, ""),
			py.MustNewMethod("_close", func(self py.Object, args py.Tuple) (py.Object, error) {
				slClose()
				return py.None, nil
			}, 0, ""),
		},
		CodeSrc: syslogSrc,
	})
}
