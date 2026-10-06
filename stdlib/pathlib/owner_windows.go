// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathlib

import "os"

// ownerIDs reports nothing: windows files have no uid/gid, and CPython's
// Path.owner and Path.group are unsupported there too.
func ownerIDs(fi os.FileInfo) (uid, gid uint32, ok bool) { return 0, 0, false }
