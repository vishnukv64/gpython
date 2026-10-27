// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mpcore provides the two C modules CPython's multiprocessing sits
// on: _multiprocessing (SemLock, sem_unlink) and _posixsubprocess (fork_exec).
//
// A gpython process cannot fork (the Go runtime is multithreaded), so
// multiprocessing runs with the "spawn" start method everywhere: every worker
// is a fresh gpython process started through _posixsubprocess.fork_exec.
//
// SemLock is a real cross-process primitive.  CPython uses a named POSIX
// semaphore; the portable Go equivalent here is a counter in a small file under
// os.TempDir(), updated under flock(2).  A waiter polls with a short backoff
// (no futex without cgo), so a blocked acquire costs a wakeup every few
// milliseconds and release-to-acquire latency is up to ~2ms.
//
// Windows: multiprocessing there needs _winapi handle duplication, which is
// not provided; SemLock raises NotImplementedError.
package mpcore
