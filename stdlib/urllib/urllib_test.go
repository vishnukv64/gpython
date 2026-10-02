// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package urllibparse_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestUrlopen serves the responses urlopen.py expects, then runs the script
// against that server.  The server is real - the point is that urlopen makes
// a genuine request and raises when it cannot, rather than fabricating a
// response.
func TestUrlopen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello world\nsecond line\n"))
		case "/echo":
			buf := make([]byte, r.ContentLength)
			r.Body.Read(buf)
			w.WriteHeader(http.StatusOK)
			w.Write(buf)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	os.Setenv("GPY_URLOPEN_BASE", srv.URL)
	defer os.Unsetenv("GPY_URLOPEN_BASE")

	pytest.RunScript(t, "./testdata/urlopen.py")
}
