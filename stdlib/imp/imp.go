// Package imp implements the _imp module: the low-level hooks of the import
// system.
//
// Only the import LOCK is implemented, because that is the whole of what is
// needed here: pkg_resources imports _imp and uses acquire_lock/release_lock to
// make its namespace-package bookkeeping atomic.  Everything else CPython's
// _imp exposes is about loading builtin, frozen and extension modules, none of
// which exists in this interpreter - so those are absent rather than present
// and wrong.
//
// The lock is real.  It is reentrant, which the import machinery needs (an
// import triggered while the lock is held must not deadlock), and lock_held()
// reports the truth rather than a constant.
package imp

import (
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `This module provides the low-level hooks for the import system.

Only the import lock is implemented: acquire_lock, release_lock and lock_held.
`

// gLock is the import lock.
//
// It is REENTRANT and it BLOCKS.  Reentrancy is required because the import
// system holds it across an import and an import may trigger another: a plain
// mutex would deadlock the moment declare_namespace imported a parent package.
// Blocking is required because a second THREAD must wait rather than walk
// through the critical section - which an earlier version of this file did,
// overwriting the owner under the state mutex that was only held for the check.
type importLock struct {
	mu    sync.Mutex
	cond  *sync.Cond
	owner uint64
	depth int
}

var gLock = func() *importLock {
	l := &importLock{}
	l.cond = sync.NewCond(&l.mu)
	return l
}()

// acquire takes the lock for a holder, waiting while another one holds it.  A
// holder that already owns it only deepens the count.
func (l *importLock) acquire(id uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for l.depth > 0 && l.owner != id {
		l.cond.Wait()
	}
	l.owner = id
	l.depth++
}

// release drops one level for a holder.
//
// Releasing a lock that is not held RAISES, which is what CPython does
// ("RuntimeError: not holding the import lock") and the opposite of what I first
// wrote.  A caller that releases without holding has a bug, and silently
// succeeding hides it.
func (l *importLock) release(id uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.depth == 0 {
		return py.ExceptionNewf(py.RuntimeError, "not holding the import lock")
	}
	if l.owner != id {
		return py.ExceptionNewf(py.RuntimeError, "not holding the import lock")
	}
	l.depth--
	if l.depth == 0 {
		l.owner = 0
		l.cond.Broadcast()
	}
	return nil
}

func (l *importLock) held() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.depth > 0
}

func init() {
	methods := []*py.Method{
		py.MustNewMethod("acquire_lock", func(self py.Object, args py.Tuple) (py.Object, error) {
			gLock.acquire(py.CurrentGoroutineID())
			return py.None, nil
		}, 0, "Acquire the interpreter's import lock for the current thread."),
		py.MustNewMethod("release_lock", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := gLock.release(py.CurrentGoroutineID()); err != nil {
				return nil, err
			}
			return py.None, nil
		}, 0, "Release the interpreter's import lock."),
		py.MustNewMethod("lock_held", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.NewBool(gLock.held()), nil
		}, 0, "Return True if the import lock is currently held."),
	}

	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "_imp",
			Doc:  module_doc,
		},
		Methods: methods,
		Globals: globals,
	})
}
