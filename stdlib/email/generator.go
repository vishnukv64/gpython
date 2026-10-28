// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import "github.com/vishnukv64/gpython/py"

// generatorSrc is email.generator for the native Message, which already knows
// how to flatten itself; CPython's generator.py walks policy and header
// internals this Message does not have.
const generatorSrc = `"""Classes to generate plain text from a message object tree."""

__all__ = ['Generator', 'DecodedGenerator', 'BytesGenerator']

import re

_FROM_RE = re.compile('^From ', re.MULTILINE)
_FROM_RE_BYTES = re.compile(b'^From ', re.MULTILINE)


class Generator:
    """Generates output from a Message object tree."""

    def __init__(self, outfp, mangle_from_=None, maxheaderlen=None, *, policy=None):
        if mangle_from_ is None:
            mangle_from_ = True
        self._fp = outfp
        self._mangle_from_ = mangle_from_
        self.maxheaderlen = maxheaderlen
        self.policy = policy

    def write(self, s):
        self._fp.write(s)

    def _text(self, msg, unixfrom):
        text = msg.as_string()
        if unixfrom:
            ufrom = msg.get_unixfrom() if hasattr(msg, 'get_unixfrom') else None
            text = (ufrom or 'From nobody Thu Jan  1 00:00:00 1970') + '\n' + text
        if self._mangle_from_:
            text = _FROM_RE.sub('>From ', text)
        return text

    def flatten(self, msg, unixfrom=False, linesep=None):
        text = self._text(msg, unixfrom)
        if linesep is not None and linesep != '\n':
            text = text.replace('\n', linesep)
        self.write(text)

    def clone(self, fp):
        return self.__class__(fp, self._mangle_from_, self.maxheaderlen, policy=self.policy)


class BytesGenerator(Generator):
    """Generates a bytes version of a Message object tree."""

    def write(self, s):
        self._fp.write(s.encode('ascii', 'surrogateescape'))


class DecodedGenerator(Generator):
    """Generates a text representation of a message, decoding payloads."""

    def __init__(self, outfp, mangle_from_=None, maxheaderlen=None, fmt=None, *, policy=None):
        Generator.__init__(self, outfp, mangle_from_, maxheaderlen, policy=policy)
        self._fmt = fmt
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "email.generator",
			FileDesc: "generator.py",
			Doc:      "Classes to generate plain text from a message object tree.",
		},
		CodeSrc: generatorSrc,
	})
}
