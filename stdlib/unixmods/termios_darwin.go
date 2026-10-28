// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

import (
	"time"

	"golang.org/x/sys/unix"
)

const termiosNCCS = 20

var osTermiosConsts = []namedConst{{"NCCS", termiosNCCS}, {"_POSIX_VDISABLE", 0xff}}

type termiosState struct {
	iflag, oflag, cflag, lflag uint
	ispeed, ospeed             uint
	cc                         [termiosNCCS]byte
}

func tcGet(fd int) (*termiosState, error) {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return nil, err
	}
	s := &termiosState{iflag: uint(t.Iflag), oflag: uint(t.Oflag), cflag: uint(t.Cflag), lflag: uint(t.Lflag),
		ispeed: uint(t.Ispeed), ospeed: uint(t.Ospeed)}
	copy(s.cc[:], t.Cc[:])
	return s, nil
}

func tcSet(fd, when int, s *termiosState) error {
	t := unix.Termios{Iflag: uint64(s.iflag), Oflag: uint64(s.oflag), Cflag: uint64(s.cflag), Lflag: uint64(s.lflag),
		Ispeed: uint64(s.ispeed), Ospeed: uint64(s.ospeed)}
	copy(t.Cc[:], s.cc[:])
	return unix.IoctlSetTermios(fd, unix.TIOCSETA+uint(when), &t)
}

func tcDrain(fd int) error { return unix.IoctlSetInt(fd, unix.TIOCDRAIN, 0) }

func tcFlush(fd, queue int) error { return unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, queue) }

func tcFlow(fd, action int) error {
	switch action {
	case 0: // TCOOFF
		return unix.IoctlSetInt(fd, unix.TIOCSTOP, 0)
	case 1: // TCOON
		return unix.IoctlSetInt(fd, unix.TIOCSTART, 0)
	}
	s, err := tcGet(fd)
	if err != nil {
		return err
	}
	c := s.cc[unix.VSTOP]
	if action == 3 { // TCION
		c = s.cc[unix.VSTART]
	}
	_, err = unix.Write(fd, []byte{c})
	return err
}

func tcBreak(fd, duration int) error {
	if err := unix.IoctlSetInt(fd, unix.TIOCSBRK, 0); err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)
	return unix.IoctlSetInt(fd, unix.TIOCCBRK, 0)
}
