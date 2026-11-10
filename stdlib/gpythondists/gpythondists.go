// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gpythondists provides 'gpython_dists': `python -c 'import gpython_dists; gpython_dists.main()'`
// records the PyPI projects that gpython implements natively (rapidfuzz, and xattr off Windows) as installed distributions in the active environment.
//
// pip decides what is installed by scanning *.dist-info directories on
// sys.path.  rapidfuzz is a C++ extension whose sdist needs CMake, and
// gpython's wheel tags (gpython314-*, py3*-none-any) match none of PyPI's
// rapidfuzz wheels, so without a record `pip install poetry` tries to build
// rapidfuzz and fails.  The record is a real dist-info (METADATA, INSTALLER,
// RECORD, REQUESTED-less) so `pip list`, `pip show` and the resolver treat
// the requirement as satisfied; pip uninstall removes it like any other.
// The code itself is the built-in module, which has priority over
// site-packages, so a compiled rapidfuzz wheel installed later is never
// imported by mistake.
package gpythondists

import "github.com/vishnukv64/gpython/py"

const codeSrc = `import os, sys, sysconfig

DISTS = {"rapidfuzz": ("RapidFuzz", "3.14.6", "Go-native subset of rapidfuzz")}
if sys.platform != "win32":
    DISTS["xattr"] = ("xattr", "1.3.0", "Go-native subset of xattr")

def install(purelib=None):
    purelib = purelib or sysconfig.get_paths()["purelib"]
    os.makedirs(purelib, exist_ok=True)
    done = []
    for mod, (name, version, summary) in DISTS.items():
        d = os.path.join(purelib, "%s-%s.dist-info" % (name.lower(), version))
        if os.path.isdir(d):
            continue
        os.makedirs(d)
        files = {
            "METADATA": "Metadata-Version: 2.1\nName: %s\nVersion: %s\nSummary: %s\n" % (name, version, summary),
            "INSTALLER": "gpython_dists\n",
        }
        for f, text in files.items():
            with open(os.path.join(d, f), "w") as fh:
                fh.write(text)
        rec = os.path.basename(d)
        with open(os.path.join(d, "RECORD"), "w") as fh:
            fh.write("".join("%s/%s,,\n" % (rec, f) for f in sorted(files)) + "%s/RECORD,,\n" % rec)
        done.append(name)
    return done

def main():
    done = install()
    print("recorded built-in distributions: " + (", ".join(done) or "none (already present)"))
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "gpython_dists",
			FileDesc: "gpython_dists.py",
			Doc:      "Record gpython's built-in PyPI replacements as installed distributions.",
		},
		CodeSrc: codeSrc,
	})
}
