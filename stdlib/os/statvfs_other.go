// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !darwin

package os

import "github.com/vishnukv64/gpython/py"

// No statvfs here, as in CPython on Windows.
func statvfsMethods() []*py.Method { return nil }
