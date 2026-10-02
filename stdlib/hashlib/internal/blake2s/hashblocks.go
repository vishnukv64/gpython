// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blake2s

// hashBlocks always uses the portable implementation.
//
// The upstream package selects an assembly version on amd64 and 386 via build
// tags and golang.org/x/sys/cpu.  Only the generic path is vendored here so that
// hashlib gains BLAKE2s without pulling a new module dependency; the result is
// bit-identical, only slower.
func hashBlocks(h *[8]uint32, c *[2]uint32, flag uint32, blocks []byte) {
	hashBlocksGeneric(h, c, flag, blocks)
}
