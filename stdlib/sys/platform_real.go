//go:build !gpython_simwin

package sys

// simWin is true only in the scratch builds that pretend to be Windows to
// check that Python-level imports resolve (go build -tags gpython_simwin).
const simWin = false
