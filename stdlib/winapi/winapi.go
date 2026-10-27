//go:build windows || gpython_simwin

// Package winapi provides _winapi and msvcrt, the C modules the embedded
// CPython Lib imports when sys.platform is "win32" (multiprocessing,
// asyncio.windows_*).  Only the subset those packages call is implemented;
// the functions are in winapi_windows.go.  Under -tags gpython_simwin on a
// non-Windows host just the constants exist, enough for imports to resolve.
//
// A gpython file descriptor on Windows is the HANDLE itself (os.File.Fd), so
// msvcrt.get_osfhandle and open_osfhandle are the identity.
package winapi

import "github.com/vishnukv64/gpython/py"

func c(name string, v int64) py.DictEntry { return py.DictEntry{Key: name, Value: py.Int(v)} }

func init() {
	entries := []py.DictEntry{
		c("NULL", 0), c("INFINITE", 0xFFFFFFFF), c("INVALID_HANDLE_VALUE", -1),
		c("WAIT_OBJECT_0", 0), c("WAIT_ABANDONED_0", 0x80), c("WAIT_TIMEOUT", 0x102),
		c("STILL_ACTIVE", 259), c("SYNCHRONIZE", 0x100000), c("PROCESS_DUP_HANDLE", 0x40),
		c("DUPLICATE_CLOSE_SOURCE", 1), c("DUPLICATE_SAME_ACCESS", 2),
		c("GENERIC_READ", 0x80000000), c("GENERIC_WRITE", 0x40000000),
		c("FILE_GENERIC_READ", 0x120089), c("FILE_GENERIC_WRITE", 0x120116),
		c("OPEN_EXISTING", 3), c("FILE_FLAG_OVERLAPPED", 0x40000000),
		c("FILE_FLAG_FIRST_PIPE_INSTANCE", 0x00080000), c("FILE_MAP_READ", 4),
		c("PAGE_READWRITE", 4),
		c("PIPE_ACCESS_INBOUND", 1), c("PIPE_ACCESS_DUPLEX", 3), c("PIPE_WAIT", 0),
		c("PIPE_TYPE_MESSAGE", 4), c("PIPE_READMODE_MESSAGE", 2),
		c("PIPE_UNLIMITED_INSTANCES", 255), c("NMPWAIT_WAIT_FOREVER", 0xFFFFFFFF),
		c("ERROR_ACCESS_DENIED", 5), c("ERROR_ALREADY_EXISTS", 183),
		c("ERROR_BROKEN_PIPE", 109), c("ERROR_IO_PENDING", 997), c("ERROR_MORE_DATA", 234),
		c("ERROR_NETNAME_DELETED", 64), c("ERROR_NO_DATA", 232),
		c("ERROR_OPERATION_ABORTED", 995), c("ERROR_PIPE_BUSY", 231),
		c("ERROR_SEM_TIMEOUT", 121),
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_winapi", Doc: "Windows API subset for multiprocessing and asyncio."},
		Globals: py.NewStringDictFrom(append(entries, platformFuncs()...)...),
	})
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "msvcrt", Doc: "Windows CRT subset: descriptors are handles."},
		Globals: py.NewStringDictFrom(
			fn("get_osfhandle", func(a py.Tuple) (py.Object, error) { return identity(a) }),
			fn("open_osfhandle", func(a py.Tuple) (py.Object, error) { return identity(a) }),
		),
	})
}

func identity(a py.Tuple) (py.Object, error) {
	if len(a) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "expected a handle")
	}
	n, err := py.IndexInt(a[0])
	return py.Int(n), err
}

// fn wraps a positional-only function as a module attribute.
func fn(name string, f func(py.Tuple) (py.Object, error)) py.DictEntry {
	return py.DictEntry{Key: name, Value: py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) { return f(args) }, 0, name)}
}

// kfn wraps a function that also takes keyword arguments.
func kfn(name string, f func(py.Tuple, py.StringDict) (py.Object, error)) py.DictEntry {
	return py.DictEntry{Key: name, Value: py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) { return f(args, kwargs) }, 0, name)}
}
