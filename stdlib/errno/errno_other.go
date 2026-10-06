// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !darwin

package errno

// platformErrors is empty here: every name this system defines is already in
// the shared table in errno.go.
var platformErrors []errnoEntry
