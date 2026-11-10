//go:build !darwin && !linux

package xattr

import "github.com/vishnukv64/gpython/py"

var errNotSup = py.ExceptionNewf(py.OSError, "extended attributes are not supported on this platform")

func osGet(path, name string) ([]byte, error) { return nil, errNotSup }
func osSet(path, name string, v []byte) error { return errNotSup }
func osRemove(path, name string) error        { return errNotSup }
func osList(path string) ([]string, error)    { return nil, errNotSup }
