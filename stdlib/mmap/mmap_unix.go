//go:build !windows

package mmap

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// mmap.mmap(fileno, length, flags=MAP_SHARED, prot=PROT_READ|PROT_WRITE,
//
//	access=ACCESS_DEFAULT, offset=0)
//
// access and (flags, prot) are two spellings of the same request; CPython on
// POSIX lets either drive.  flags/prot are accepted but access wins, because
// that is the spelling pip's cachecontrol uses.
func parseArgs(args py.Tuple, kwargs py.StringDict) (r mapArgs, err error) {
	var filenoObj, lengthObj py.Object
	var flagsObj py.Object = py.Int(syscall.MAP_SHARED)
	var protObj py.Object = py.Int(syscall.PROT_READ | syscall.PROT_WRITE)
	var accessObj py.Object = py.Int(ACCESS_DEFAULT)
	var offsetObj py.Object = py.Int(0)
	if err = py.ParseTupleAndKeywords(args, kwargs, "OO|OOOO:mmap.mmap",
		[]string{"fileno", "length", "flags", "prot", "access", "offset"},
		&filenoObj, &lengthObj, &flagsObj, &protObj, &accessObj, &offsetObj); err != nil {
		return
	}
	if r.fileno, err = py.IndexInt(filenoObj); err != nil {
		return
	}
	if r.length, err = py.IndexInt(lengthObj); err != nil {
		return
	}
	if r.access, err = py.IndexInt(accessObj); err != nil {
		return
	}
	r.offset, err = py.MakeGoInt64(offsetObj)
	return
}

func fileSize(fileno int) (int64, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(fileno, &st); err != nil {
		return 0, errnoError(err)
	}
	return st.Size, nil
}

func mapMem(fileno int, offset int64, size, access int, tag string) ([]byte, error) {
	prot, flags := syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED
	switch access {
	case ACCESS_READ:
		prot = syscall.PROT_READ
	case ACCESS_COPY:
		flags = syscall.MAP_PRIVATE
	}
	if fileno == -1 {
		flags |= syscall.MAP_ANON
	}
	data, err := syscall.Mmap(fileno, offset, size, prot, flags)
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "[Errno %d] %s", err.(syscall.Errno), err.Error())
	}
	return data, nil
}

func unmapMem(d []byte) error { return syscall.Munmap(d) }

func platformGlobals(globals py.StringDict) {
	globals.Set("MAP_SHARED", py.Int(syscall.MAP_SHARED))
	globals.Set("MAP_PRIVATE", py.Int(syscall.MAP_PRIVATE))
	globals.Set("MAP_ANONYMOUS", py.Int(syscall.MAP_ANON))
	globals.Set("MAP_ANON", py.Int(syscall.MAP_ANON))
	globals.Set("PROT_READ", py.Int(syscall.PROT_READ))
	globals.Set("PROT_WRITE", py.Int(syscall.PROT_WRITE))
	globals.Set("PROT_EXEC", py.Int(syscall.PROT_EXEC))
	globals.Set("PAGESIZE", py.Int(syscall.Getpagesize()))
	globals.Set("ALLOCATIONGRANULARITY", py.Int(syscall.Getpagesize()))
}

// errnoError renders a syscall failure as CPython does: "[Errno 9] Bad file
// descriptor".  Go's errno text is the same strerror text with its first
// letter lowercased, so capitalising it restores CPython's wording.
func errnoError(err error) error {
	en, ok := err.(syscall.Errno)
	if !ok {
		return py.ExceptionNewf(py.OSError, "%s", err.Error())
	}
	msg := en.Error()
	if msg != "" && msg[0] >= 'a' && msg[0] <= 'z' {
		msg = string(msg[0]-'a'+'A') + msg[1:]
	}
	return py.ExceptionNewf(py.OSError, "[Errno %d] %s", int(en), msg)
}
