// Copyright 2025 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import (
	"unsafe"

	lib "modernc.org/sqlite/lib"

	"github.com/vishnukv64/gpython/py"
)

type backup struct {
	src, dst *Db
	p        uintptr
}

func (b *backup) Type() *py.Type { return backupType }

type rawBlob struct {
	db *Db
	p  uintptr
}

func (b *rawBlob) Type() *py.Type { return blobType }

func (b *rawBlob) live() error {
	if b.p == 0 {
		return py.ExceptionNewf(errProgramming, "Cannot operate on a closed blob.")
	}
	return nil
}

func registerBackupBlob(def func(*py.Type, string, interface{})) {
	type A = py.Tuple
	type O = py.Object
	// backup_init(dst, dst_name, src_name)
	def(dbType, "backup_init", func(self O, a A) (O, error) {
		src := self.(*Db)
		dst, ok := a[0].(*Db)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "target must be a Connection")
		}
		if err := src.open(); err != nil {
			return nil, err
		}
		if err := dst.open(); err != nil {
			return nil, err
		}
		dn, sn := dst.cstr(string(a[1].(py.String))), src.cstr(string(a[2].(py.String)))
		defer dst.free(dn)
		defer src.free(sn)
		p := lib.Xsqlite3_backup_init(dst.tls, dst.db, dn, src.db, sn)
		if p == 0 {
			return nil, dst.sqliteError(lib.Xsqlite3_errcode(dst.tls, dst.db))
		}
		return &backup{src: src, dst: dst, p: p}, nil
	})
	def(backupType, "step", func(self O, a A) (O, error) {
		b := self.(*backup)
		n, err := goInt(a[0])
		if err != nil {
			return nil, err
		}
		return py.Int(lib.Xsqlite3_backup_step(b.dst.tls, b.p, int32(n))), nil
	})
	def(backupType, "remaining", func(self O, a A) (O, error) {
		b := self.(*backup)
		return py.Int(lib.Xsqlite3_backup_remaining(b.dst.tls, b.p)), nil
	})
	def(backupType, "pagecount", func(self O, a A) (O, error) {
		b := self.(*backup)
		return py.Int(lib.Xsqlite3_backup_pagecount(b.dst.tls, b.p)), nil
	})
	// finish(rc): rc is the last step result; anything but DONE raises.
	def(backupType, "finish", func(self O, a A) (O, error) {
		b := self.(*backup)
		rc, err := goInt(a[0])
		if err != nil {
			return nil, err
		}
		lib.Xsqlite3_backup_finish(b.dst.tls, b.p)
		if rc != lib.SQLITE_DONE {
			return nil, b.dst.sqliteError(int32(rc))
		}
		return py.None, nil
	})
	def(dbType, "sleep", func(self O, a A) (O, error) {
		n, err := goInt(a[0])
		if err != nil {
			return nil, err
		}
		lib.Xsqlite3_sleep(self.(*Db).tls, int32(n))
		return py.None, nil
	})

	// blob_open(table, column, row, readonly, dbname)
	def(dbType, "blob_open", func(self O, a A) (O, error) {
		d := self.(*Db)
		if err := d.open(); err != nil {
			return nil, err
		}
		row, err := goInt(a[2])
		if err != nil {
			return nil, err
		}
		tb, col, nm := d.cstr(string(a[0].(py.String))), d.cstr(string(a[1].(py.String))), d.cstr(string(a[4].(py.String)))
		defer d.free(tb)
		defer d.free(col)
		defer d.free(nm)
		pp := d.malloc(int(ptrSize))
		defer d.free(pp)
		var flags int32
		if !toBool(a[3]) {
			flags = 1
		}
		rc := lib.Xsqlite3_blob_open(d.tls, d.db, nm, tb, col, row, flags, pp)
		if rc != lib.SQLITE_OK {
			if p := *(*uintptr)(unsafe.Pointer(pp)); p != 0 {
				lib.Xsqlite3_blob_close(d.tls, p)
			}
			return nil, d.sqliteError(rc)
		}
		return &rawBlob{db: d, p: *(*uintptr)(unsafe.Pointer(pp))}, nil
	})
	def(blobType, "close", func(self O, a A) (O, error) {
		b := self.(*rawBlob)
		if b.p != 0 {
			lib.Xsqlite3_blob_close(b.db.tls, b.p)
			b.p = 0
		}
		return py.None, nil
	})
	def(blobType, "closed", func(self O, a A) (O, error) { return py.NewBool(self.(*rawBlob).p == 0), nil })
	def(blobType, "size", func(self O, a A) (O, error) {
		b := self.(*rawBlob)
		if err := b.live(); err != nil {
			return nil, err
		}
		return py.Int(lib.Xsqlite3_blob_bytes(b.db.tls, b.p)), nil
	})
	def(blobType, "read", func(self O, a A) (O, error) {
		b := self.(*rawBlob)
		if err := b.live(); err != nil {
			return nil, err
		}
		n, _ := goInt(a[0])
		off, _ := goInt(a[1])
		out := make(py.Bytes, n)
		if n > 0 {
			p := b.db.malloc(int(n))
			defer b.db.free(p)
			if rc := lib.Xsqlite3_blob_read(b.db.tls, b.p, p, int32(n), int32(off)); rc != lib.SQLITE_OK {
				return nil, b.db.sqliteError(rc)
			}
			copy(out, unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n)))
		}
		return out, nil
	})
	def(blobType, "write", func(self O, a A) (O, error) {
		b := self.(*rawBlob)
		if err := b.live(); err != nil {
			return nil, err
		}
		data, err := py.BytesFromObject(a[0])
		if err != nil {
			return nil, err
		}
		off, _ := goInt(a[1])
		if len(data) > 0 {
			p := b.db.malloc(len(data))
			defer b.db.free(p)
			copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
			if rc := lib.Xsqlite3_blob_write(b.db.tls, b.p, p, int32(len(data)), int32(off)); rc != lib.SQLITE_OK {
				return nil, b.db.sqliteError(rc)
			}
		}
		return py.None, nil
	})
}
