package mmap

import (
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/vishnukv64/gpython/py"
)

// lastError is what _winapi.GetLastError reports after a mapping was created:
// multiprocessing's Arena asks it whether a tagged mapping already existed
// (ERROR_ALREADY_EXISTS).  Go may move a goroutine between OS threads, so the
// thread's own last-error cannot be read back later.
var lastError atomic.Uint32

// LastError returns the status of the most recent mapping created.
func LastError() uint32 { return lastError.Load() }

// mmap.mmap(fileno, length, tagname=None, access=ACCESS_DEFAULT, offset=0)
func parseArgs(args py.Tuple, kwargs py.StringDict) (r mapArgs, err error) {
	var filenoObj, lengthObj py.Object
	var tagObj py.Object = py.None
	var accessObj py.Object = py.Int(ACCESS_DEFAULT)
	var offsetObj py.Object = py.Int(0)
	if err = py.ParseTupleAndKeywords(args, kwargs, "OO|OOO:mmap.mmap",
		[]string{"fileno", "length", "tagname", "access", "offset"},
		&filenoObj, &lengthObj, &tagObj, &accessObj, &offsetObj); err != nil {
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
	if tagObj != py.None {
		if r.tag, err = py.StrAsString(tagObj); err != nil {
			return
		}
	}
	r.offset, err = py.MakeGoInt64(offsetObj)
	return
}

func winErr(err error) error {
	if en, ok := err.(windows.Errno); ok {
		return py.WinOSError(int(en), en.Error())
	}
	return py.ExceptionNewf(py.OSError, "%s", err.Error())
}

// The fileno of a file is its HANDLE (os.File.Fd on Windows).
func fileSize(fileno int) (int64, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(fileno), &info); err != nil {
		return 0, winErr(err)
	}
	return int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow), nil
}

func mapMem(fileno int, offset int64, size, access int, tag string) ([]byte, error) {
	prot, view := uint32(windows.PAGE_READWRITE), uint32(windows.FILE_MAP_WRITE)
	switch access {
	case ACCESS_READ:
		prot, view = windows.PAGE_READONLY, windows.FILE_MAP_READ
	case ACCESS_COPY:
		prot, view = windows.PAGE_WRITECOPY, windows.FILE_MAP_COPY
	}
	var name *uint16
	if tag != "" {
		var err error
		if name, err = windows.UTF16PtrFromString(tag); err != nil {
			return nil, py.ExceptionNewf(py.ValueError, "embedded null character")
		}
	}
	file := windows.InvalidHandle
	if fileno != -1 {
		file = windows.Handle(fileno)
	}
	end := uint64(offset) + uint64(size)
	h, err := windows.CreateFileMapping(file, nil, prot, uint32(end>>32), uint32(end), name)
	lastError.Store(0)
	if err == windows.ERROR_ALREADY_EXISTS {
		lastError.Store(uint32(windows.ERROR_ALREADY_EXISTS))
		err = nil
	}
	if err != nil {
		return nil, winErr(err)
	}
	// The view keeps the mapping object alive, so the handle can go now.
	defer windows.CloseHandle(h)
	addr, err := windows.MapViewOfFile(h, view, uint32(uint64(offset)>>32), uint32(offset), uintptr(size))
	if err != nil {
		return nil, winErr(err)
	}
	// addr is OS memory, not Go memory; convert without a vet-flagged
	// uintptr -> unsafe.Pointer cast.
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&addr)), size), nil
}

func unmapMem(d []byte) error {
	return windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&d[0])))
}

func platformGlobals(globals py.StringDict) {
	globals.Set("PAGESIZE", py.Int(4096))
	globals.Set("ALLOCATIONGRANULARITY", py.Int(65536))
}
