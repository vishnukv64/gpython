//go:build linux

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

import (
	"golang.org/x/sys/unix"
)

// NCCS is glibc's (32); the kernel's termios carries 19 control characters and
// the rest read as zero, exactly what glibc's tcgetattr returns.
const termiosNCCS = 32

var osTermiosConsts = []namedConst{{"NCCS", termiosNCCS}, {"_POSIX_VDISABLE", 0}}

type termiosState struct {
	iflag, oflag, cflag, lflag uint
	ispeed, ospeed             uint
	cc                         [termiosNCCS]byte
}

func tcGet(fd int) (*termiosState, error) {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	s := &termiosState{iflag: uint(t.Iflag), oflag: uint(t.Oflag), cflag: uint(t.Cflag), lflag: uint(t.Lflag)}
	copy(s.cc[:], t.Cc[:])
	s.ospeed = uint(t.Cflag & unix.CBAUD)
	s.ispeed = uint(t.Cflag&unix.CIBAUD) >> 16
	if s.ispeed == 0 {
		s.ispeed = s.ospeed
	}
	return s, nil
}

func tcSet(fd, when int, s *termiosState) error {
	t := unix.Termios{Iflag: uint32(s.iflag), Oflag: uint32(s.oflag), Lflag: uint32(s.lflag)}
	t.Cflag = uint32(s.cflag)&^(unix.CBAUD|unix.CIBAUD) | uint32(s.ospeed)&unix.CBAUD
	if s.ispeed != s.ospeed {
		t.Cflag |= uint32(s.ispeed<<16) & unix.CIBAUD
	}
	copy(t.Cc[:], s.cc[:])
	return unix.IoctlSetTermios(fd, unix.TCSETS+uint(when), &t)
}

func tcDrain(fd int) error { return unix.IoctlSetInt(fd, unix.TCSBRK, 1) }

func tcFlush(fd, queue int) error { return unix.IoctlSetInt(fd, unix.TCFLSH, queue) }

func tcFlow(fd, action int) error { return unix.IoctlSetInt(fd, unix.TCXONC, action) }

func tcBreak(fd, duration int) error {
	if duration <= 0 {
		return unix.IoctlSetInt(fd, unix.TCSBRK, 0)
	}
	// glibc: a duration other than 0 is not portable; TCSBRKP takes tenths of a second.
	return unix.IoctlSetInt(fd, unix.TCSBRKP, (duration+99)/100)
}
