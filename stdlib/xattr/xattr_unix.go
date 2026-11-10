//go:build darwin || linux

package xattr

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
	"golang.org/x/sys/unix"
)

func oserr(err error, path string) error {
	if errno, ok := err.(unix.Errno); ok {
		return py.ExceptionNewf(py.OSError, "[Errno %d] %s: %q", int(errno), errno.Error(), path)
	}
	return err
}

func osGet(path, name string) ([]byte, error) {
	n, err := unix.Getxattr(path, name, nil)
	if err != nil {
		return nil, oserr(err, path)
	}
	buf := make([]byte, n)
	if n, err = unix.Getxattr(path, name, buf); err != nil {
		return nil, oserr(err, path)
	}
	return buf[:n], nil
}

func osSet(path, name string, v []byte) error {
	return oserr(unix.Setxattr(path, name, v, 0), path)
}

func osRemove(path, name string) error {
	return oserr(unix.Removexattr(path, name), path)
}

func osList(path string) ([]string, error) {
	n, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, oserr(err, path)
	}
	buf := make([]byte, n)
	if n, err = unix.Listxattr(path, buf); err != nil {
		return nil, oserr(err, path)
	}
	var out []string
	for _, s := range strings.Split(string(buf[:n]), "\x00") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}
