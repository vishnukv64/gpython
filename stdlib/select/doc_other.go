// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(darwin || linux)

// Package selectmod is empty here: syscall has no Select on this platform, so
// "import select" is a ModuleNotFoundError rather than a broken build.  The
// package still has to exist, because stdlib imports it on every platform.
package selectmod
