// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package platform provides python's 'platform' module.
//
// The values are the host's own.  system() and machine() are runtime.GOOS and
// runtime.GOARCH mapped onto the spellings CPython uses; release() and
// version() come from the host's own `uname`, which is the only place the
// kernel release is exposed (Go has no syscall for it); and the python_* family
// reports this interpreter, kept in step with the sys module's version_info.
//
// What is not here is said plainly: libc_ver() reports empty strings, because a
// static Go binary carries no libc soname/version and CPython's answer comes
// from parsing the executable.  win32_ver/java_ver/ios_ver/android_ver return
// the empty result, which is what CPython returns when the platform is not the
// one they describe - and none of them is macOS.
package platform

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `This module tries to retrieve as much platform-identifying data as possible.
It makes this information available via function APIs.

It is intended to be a cross-platform module, but the values here are the
host's own.`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "platform",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("system", system, 0, "Returns the system/OS name, such as 'Linux', 'Darwin'."),
			py.MustNewMethod("machine", machine, 0, "Returns the machine type, e.g. 'x86_64'."),
			py.MustNewMethod("release", release, 0, "Returns the system's release, e.g. '2.2.0'."),
			py.MustNewMethod("version", version, 0, "Returns the system's release version."),
			py.MustNewMethod("uname", unameFn, 0, "Fairly portable uname interface.  Returns a tuple."),
			py.MustNewMethod("python_version", pythonVersion, 0, "Returns the Python version as string 'major.minor.patchlevel'."),
			py.MustNewMethod("python_version_tuple", pythonVersionTuple, 0, "Returns the Python version as tuple (major, minor, patchlevel)."),
			py.MustNewMethod("python_build", pythonBuild, 0, "Returns a tuple (buildno, builddate) of the Python build."),
			py.MustNewMethod("python_compiler", pythonCompiler, 0, "Returns a string identifying the compiler used for compiling Python."),
			py.MustNewMethod("python_implementation", pythonImplementation, 0, "Returns a string identifying the Python implementation."),
			py.MustNewMethod("python_branch", pythonBranch, 0, "Returns a string identifying the Python implementation SCM branch."),
			py.MustNewMethod("python_revision", pythonRevision, 0, "Returns a string identifying the Python implementation SCM revision."),
			py.MustNewMethod("platform", platformFn, 0, "Returns a single string identifying the underlying platform."),
			py.MustNewMethod("libc_ver", libcVer, 0, "Tries to determine the libc version."),
			py.MustNewMethod("mac_ver", macVer, 0, "Get macOS version information."),
			py.MustNewMethod("win32_ver", win32Ver, 0, "Get additional version information from the Windows Registry."),
			py.MustNewMethod("java_ver", javaVer, 0, "Version interface for Jython."),
			py.MustNewMethod("ios_ver", iosVer, 0, "Get iOS version information."),
			py.MustNewMethod("android_ver", androidVer, 0, "Get Android version information."),
		},
	})
}

// noArgs rejects the arguments a CPython function of no arguments rejects.
func noArgs(name string, args py.Tuple) error {
	if len(args) != 0 {
		return py.ExceptionNewf(py.TypeError, "%s() takes no arguments", name)
	}
	return nil
}

func system(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("system", args); err != nil {
		return nil, err
	}
	return py.String(systemName()), nil
}

// systemName maps Go's GOOS onto the spelling Python uses.
func systemName() string {
	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "dragonfly":
		return "DragonFly"
	case "solaris":
		return "SunOS"
	case "js":
		return "Emscripten"
	}
	return runtime.GOOS
}

func machine(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("machine", args); err != nil {
		return nil, err
	}
	return py.String(machineName()), nil
}

func machineName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	case "arm64":
		// Apple reports its own name for the architecture; the rest of the
		// world spells it aarch64.
		if runtime.GOOS == "darwin" {
			return "arm64"
		}
		return "aarch64"
	}
	return runtime.GOARCH
}

// hostUname reads the kernel release and version from the host's uname.  Go
// exposes no uname syscall, so the platform's own command is the source; when
// it is unavailable the empty string is returned rather than a wrong value.
func hostUname() (release, version string) {
	out, err := exec.Command("uname", "-rv").Output()
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func release(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("release", args); err != nil {
		return nil, err
	}
	rel, _ := hostUname()
	return py.String(rel), nil
}

func version(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("version", args); err != nil {
		return nil, err
	}
	_, ver := hostUname()
	return py.String(ver), nil
}

// unameFn is the six-field uname result, in CPython's order:
// system, node, release, version, machine, processor.
func unameFn(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("uname", args); err != nil {
		return nil, err
	}
	rel, ver := hostUname()
	return py.Tuple{
		py.String(systemName()),
		py.String(hostName()),
		py.String(rel),
		py.String(ver),
		py.String(machineName()),
		py.String(""),
	}, nil
}

// hostName is the host's network name, which is uname's node field.
func hostName() string {
	out, err := exec.Command("uname", "-n").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func pythonVersion(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_version", args); err != nil {
		return nil, err
	}
	return py.String("3.10.0"), nil
}

func pythonVersionTuple(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_version_tuple", args); err != nil {
		return nil, err
	}
	return py.Tuple{py.Int(3), py.Int(4), py.Int(0)}, nil
}

func pythonBuild(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_build", args); err != nil {
		return nil, err
	}
	return py.Tuple{py.String(""), py.String("")}, nil
}

func pythonCompiler(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_compiler", args); err != nil {
		return nil, err
	}
	// The interpreter is this Go program, so the compiler that built it is the
	// one Go reports.
	return py.String("gc " + strings.TrimPrefix(runtime.Version(), "go")), nil
}

func pythonImplementation(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_implementation", args); err != nil {
		return nil, err
	}
	return py.String("gpython"), nil
}

func pythonBranch(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_branch", args); err != nil {
		return nil, err
	}
	return py.String(""), nil
}

func pythonRevision(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("python_revision", args); err != nil {
		return nil, err
	}
	return py.String(""), nil
}

// platformFn is the one-line description, composed as CPython composes it.
func platformFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	aliased := py.Object(py.False)
	terse := py.Object(py.False)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:platform", []string{"aliased", "terse"}, &aliased, &terse); err != nil {
		return nil, err
	}
	rel, _ := hostUname()
	return py.String(systemName() + "-" + rel + "-" + machineName()), nil
}

// libcVer reports (lib, version) of the C library.  A Go binary's libc version
// is not recoverable the way CPython recovers it - by parsing the executable -
// so empty strings are reported rather than a guess.
func libcVer(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	executable := py.Object(py.None)
	lib := py.Object(py.String(""))
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:libc_ver", []string{"executable", "lib"}, &executable, &lib); err != nil {
		return nil, err
	}
	return py.Tuple{py.String(""), py.String("")}, nil
}

// macVer is (release, versioninfo, machine).  Only Darwin has a product
// version; elsewhere CPython returns the empty triple.
func macVer(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	rel := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:mac_ver", []string{"release"}, &rel); err != nil {
		return nil, err
	}
	if runtime.GOOS != "darwin" {
		return py.Tuple{py.String(""), py.Tuple{}, py.String("")}, nil
	}
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return py.Tuple{py.String(""), py.Tuple{}, py.String("")}, nil
	}
	return py.Tuple{py.String(strings.TrimSpace(string(out))), py.Tuple{}, py.String(machineName())}, nil
}

// The remaining *_ver functions describe platforms this interpreter does not run
// on.  CPython returns the empty result when the platform is not the one being
// asked about, and so does this.

func win32Ver(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("win32_ver", args); err != nil {
		return nil, err
	}
	return py.Tuple{py.String(""), py.String(""), py.String(""), py.String("")}, nil
}

func javaVer(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("java_ver", args); err != nil {
		return nil, err
	}
	return py.Tuple{py.String(""), py.String(""), py.String("")}, nil
}

func iosVer(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	system := py.Object(py.String("iOS"))
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:ios_ver", []string{"system"}, &system); err != nil {
		return nil, err
	}
	return py.Tuple{py.String(""), py.Tuple{}, py.String("")}, nil
}

func androidVer(self py.Object, args py.Tuple) (py.Object, error) {
	if err := noArgs("android_ver", args); err != nil {
		return nil, err
	}
	return py.Tuple{py.String(""), py.Int(0), py.String("")}, nil
}
