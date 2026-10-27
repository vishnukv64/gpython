//go:build !windows && !gpython_simwin

package socket

func noteFileno(fd int, s *sock) {}

func ignoredSockopt(level, optname int) bool { return false }
