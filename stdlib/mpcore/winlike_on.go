//go:build windows || gpython_simwin

package mpcore

// winLike: the module follows the win32 layout (closesocket/recv/send exist).
const winLike = true
