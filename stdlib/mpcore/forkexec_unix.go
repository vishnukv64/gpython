//go:build !windows && !plan9 && !js && !wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mpcore

import (
	"os"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

func strList(o py.Object) ([]string, error) {
	if o == py.None {
		return nil, nil
	}
	items, err := py.SequenceList(o)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items.Items))
	for _, it := range items.Items {
		switch v := it.(type) {
		case py.String:
			out = append(out, string(v))
		case py.Bytes:
			out = append(out, string(v))
		default:
			return nil, py.ExceptionNewf(py.TypeError, "expected str or bytes, not %s", it.Type().Name)
		}
	}
	return out, nil
}

// registerForkExec defines _posixsubprocess.fork_exec.  It serves the way
// multiprocessing uses it: start a program with stdio redirected and an
// explicit set of descriptors kept open.  Go's ForkExec closes everything
// else by close-on-exec, so fds_to_keep are the only ones inherited.
func registerForkExec() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_posixsubprocess", Doc: "A POSIX helper for the subprocess module."},
		Globals: py.NewStringDictFrom(py.DictEntry{Key: "fork_exec", Value: py.MustNewMethod("fork_exec", func(self py.Object, args py.Tuple) (py.Object, error) {
			// (args, executable_list, close_fds, fds_to_keep, cwd, env_list,
			//  p2cread, p2cwrite, c2pread, c2pwrite, errread, errwrite,
			//  errpipe_read, errpipe_write, restore_signals, call_setsid,
			//  pgid_to_set, gid, extra_groups, uid, child_umask, preexec_fn)
			if len(args) < 14 {
				return nil, py.ExceptionNewf(py.TypeError, "fork_exec() takes at least 14 arguments (%d given)", len(args))
			}
			argv, err := strList(args[0])
			if err != nil {
				return nil, err
			}
			exes, err := strList(args[1])
			if err != nil {
				return nil, err
			}
			keep, err := py.SequenceList(args[3])
			if err != nil {
				return nil, err
			}
			cwd, _ := strList(py.Tuple{args[4]})
			env, err := strList(args[5])
			if err != nil {
				return nil, err
			}
			if args[5] == py.None {
				env = os.Environ()
			}
			ints := make([]int, 6)
			for i := range ints {
				if ints[i], err = py.IndexInt(args[6+i]); err != nil {
					return nil, err
				}
			}
			// Child fd i must be parent fd files[i]; -1 closes it.
			files := []uintptr{0, 1, 2}
			for i, fd := range []int{ints[0], ints[3], ints[5]} { // p2cread, c2pwrite, errwrite
				if fd != -1 {
					files[i] = uintptr(fd)
				}
			}
			for _, k := range keep.Items {
				fd, err := py.IndexInt(k)
				if err != nil {
					return nil, err
				}
				for len(files) <= fd {
					files = append(files, ^uintptr(0))
				}
				files[fd] = uintptr(fd)
			}
			attr := &syscall.ProcAttr{Env: env, Files: files}
			if len(cwd) == 1 {
				attr.Dir = cwd[0]
			}
			var pid int
			for _, exe := range exes {
				if pid, err = syscall.ForkExec(exe, argv, attr); err == nil {
					return py.Int(pid), nil
				}
			}
			return nil, py.OSErrorFrom(err, "")
		}, 0, "Start a program in a new process; return its pid.")}),
	})
}

// registerShm defines _posixshmem.  There is no shm_open without cgo, so a
// shared-memory segment is a file under os.TempDir(); mapping it MAP_SHARED
// from two processes shares the pages exactly as /dev/shm would.
func registerShm() {
	name := func(o py.Object) (string, error) {
		n, err := py.StrAsString(o)
		if err != nil {
			return "", err
		}
		return semPath(n) + ".shm", nil
	}
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_posixshmem", Doc: "POSIX shared memory (file-backed)."},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "shm_open", Value: py.MustNewMethod("shm_open", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				var p, flags, mode py.Object = nil, nil, py.Int(0o777)
				if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:shm_open", []string{"path", "flags", "mode"}, &p, &flags, &mode); err != nil {
					return nil, err
				}
				path, err := name(p)
				if err != nil {
					return nil, err
				}
				fl, err := py.IndexInt(flags)
				if err != nil {
					return nil, err
				}
				m, err := py.IndexInt(mode)
				if err != nil {
					return nil, err
				}
				fd, err := syscall.Open(path, fl|syscall.O_CLOEXEC, uint32(m))
				if err != nil {
					return nil, py.OSErrorFrom(err, path)
				}
				return py.Int(fd), nil
			}, 0, "Open a shared-memory segment; return its fd.")},
			py.DictEntry{Key: "shm_unlink", Value: py.MustNewMethod("shm_unlink", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				var p py.Object
				if err := py.ParseTupleAndKeywords(args, kwargs, "O:shm_unlink", []string{"path"}, &p); err != nil {
					return nil, err
				}
				path, err := name(p)
				if err != nil {
					return nil, err
				}
				if err := os.Remove(path); err != nil {
					return nil, py.OSErrorFrom(err, path)
				}
				return py.None, nil
			}, 0, "Remove a shared-memory segment.")},
		),
	})
}
