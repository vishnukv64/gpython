package py

// WinOSError is the OSError CPython raises for a Win32 error code on any
// platform: "[WinError N] msg", winerror set, and the PEP 3151 subclass the
// code maps to.  The _winapi/_overlapped emulation uses it so that asyncio's
// "except ... exc.winerror" paths behave identically under a simulated win32.
func WinOSError(code int, msg string) error {
	class, errno := OSError, 22 // EINVAL, the CRT's fallback
	switch code {
	case 109, 231, 232: // ERROR_BROKEN_PIPE, ERROR_PIPE_BUSY, ERROR_NO_DATA
		class, errno = BrokenPipeError, 32
	case 1225, 10061: // ERROR_CONNECTION_REFUSED, WSAECONNREFUSED
		class, errno = ConnectionRefusedError, 107
	case 1236, 10053: // ERROR_CONNECTION_ABORTED, WSAECONNABORTED
		class, errno = ConnectionAbortedError, 106
	case 64, 10054: // ERROR_NETNAME_DELETED, WSAECONNRESET
		class, errno = ConnectionResetError, 108
	case 2, 3: // ERROR_FILE_NOT_FOUND, ERROR_PATH_NOT_FOUND
		class, errno = FileNotFoundError, 2
	case 5: // ERROR_ACCESS_DENIED
		class, errno = PermissionError, 13
	}
	e := ExceptionNewf(class, "[WinError %d] %s", code, msg)
	SetErrno(e, errno, msg)
	e.Dict.Set("winerror", Int(code))
	return e
}
