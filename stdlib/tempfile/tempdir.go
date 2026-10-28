package tempfile

import (
	"os"

	"github.com/vishnukv64/gpython/py"
)

// temporaryDirectory is tempfile.TemporaryDirectory: a directory created on
// construction and removed by cleanup() or on leaving a with block.
// ponytail: no finalizer, so an unreferenced one is not cleaned up
// implicitly; CPython does that through weakref.finalize.
type temporaryDirectory struct {
	name         string
	ignoreErrors bool
	delete       bool
	done         bool
}

var temporaryDirectoryType = py.NewTypeX("tempfile.TemporaryDirectory",
	"Create and return a temporary directory.",
	func(_ *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var suffix, prefix, dir py.Object = py.None, py.None, py.None
		var ignore, del py.Object = py.False, py.True
		if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOp$p:TemporaryDirectory",
			[]string{"suffix", "prefix", "dir", "ignore_cleanup_errors", "delete"},
			&suffix, &prefix, &dir, &ignore, &del); err != nil {
			return nil, err
		}
		name, err := mkdtemp(nil, py.Tuple{suffix, prefix, dir}, py.NewStringDict())
		if err != nil {
			return nil, err
		}
		ig, err := py.ObjectIsTrue(ignore)
		if err != nil {
			return nil, err
		}
		dl, err := py.ObjectIsTrue(del)
		if err != nil {
			return nil, err
		}
		return &temporaryDirectory{name: string(name.(py.String)), ignoreErrors: ig, delete: dl}, nil
	}, nil)

func (*temporaryDirectory) Type() *py.Type { return temporaryDirectoryType }

func (d *temporaryDirectory) cleanup() error {
	if d.done {
		return nil
	}
	d.done = true
	if err := os.RemoveAll(d.name); err != nil && !d.ignoreErrors {
		return py.OSErrorFrom(err, d.name)
	}
	return nil
}

func init() {
	dict := temporaryDirectoryType.Dict
	dict.Set("name", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(self.(*temporaryDirectory).name), nil
	}})
	dict.Set("cleanup", py.MustNewMethod("cleanup", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, self.(*temporaryDirectory).cleanup()
	}, 0, "Remove the directory and its contents."))
	dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String(self.(*temporaryDirectory).name), nil
	}, 0, "Return the directory name."))
	dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*temporaryDirectory)
		if d.delete {
			if err := d.cleanup(); err != nil {
				return nil, err
			}
		}
		return py.False, nil
	}, 0, "Remove the directory when delete is true."))
	dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		r, err := py.ReprAsString(py.String(self.(*temporaryDirectory).name))
		return py.String("<TemporaryDirectory " + r + ">"), err
	}, 0, "Return repr(self)."))
}
