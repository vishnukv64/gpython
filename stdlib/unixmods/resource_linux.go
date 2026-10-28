//go:build linux

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

import "golang.org/x/sys/unix"

const rlimInfinity = -1
const rlimNLimits = 16

var osResourceConsts = []namedConst{
	{"RLIMIT_MSGQUEUE", unix.RLIMIT_MSGQUEUE}, {"RLIMIT_NICE", unix.RLIMIT_NICE},
	{"RLIMIT_RTPRIO", unix.RLIMIT_RTPRIO}, {"RLIMIT_RTTIME", unix.RLIMIT_RTTIME},
	{"RLIMIT_SIGPENDING", unix.RLIMIT_SIGPENDING}, {"RUSAGE_THREAD", 1},
}
