//go:build darwin

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

const rlimInfinity = 1<<63 - 1
const rlimNLimits = 9

var osResourceConsts []namedConst
