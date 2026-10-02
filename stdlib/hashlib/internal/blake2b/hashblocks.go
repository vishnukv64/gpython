// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blake2b

// hashBlocks always uses the portable implementation.
//
// The upstream package selects an assembly version on amd64 via build tags and
// golang.org/x/sys/cpu.  Only the generic path is vendored here so that hashlib
// gains BLAKE2b without pulling a new module dependency; the result is
// bit-identical, only slower.
func hashBlocks(h *[8]uint64, c *[2]uint64, flag uint64, blocks []byte) {
	hashBlocksGeneric(h, c, flag, blocks)
}
