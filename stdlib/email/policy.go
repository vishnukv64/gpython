// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import "github.com/vishnukv64/gpython/py"

// policySrc is email.policy.  The Go parser and Message take a policy and pass
// it through but do not consult it, so these classes carry the settings
// (max_line_length, linesep, cte_type, utf8 ...) and the clone/+ protocol that
// callers subclass and combine (packaging.metadata subclasses EmailPolicy;
// installer parses with compat32); they do not change how a message is parsed
// or folded.
const policySrc = `"""This module provides the email policy objects (settings carriers)."""

__all__ = ["Compat32", "compat32", "Policy", "EmailPolicy", "default", "strict", "SMTP", "HTTP"]


class _PolicyBase:
    """Immutable settings with clone() and + , as in email._policybase."""

    raise_on_defect = False
    linesep = "\n"
    cte_type = "8bit"
    max_line_length = 78
    mangle_from_ = False
    message_factory = None
    verify_generated_headers = True

    def __init__(self, **kw):
        for name, value in kw.items():
            if hasattr(self, name):
                super().__setattr__(name, value)
            else:
                raise TypeError("{!r} is an invalid keyword argument for {}".format(name, self.__class__.__name__))

    def __repr__(self):
        args = ["{}={!r}".format(k, v) for k, v in self.__dict__.items()]
        return "{}({})".format(self.__class__.__name__, ", ".join(args))

    def clone(self, **kw):
        newpolicy = self.__class__.__new__(self.__class__)
        for attr, value in self.__dict__.items():
            object.__setattr__(newpolicy, attr, value)
        for attr, value in kw.items():
            if not hasattr(self, attr):
                raise TypeError("{!r} is an invalid keyword argument for {}".format(attr, self.__class__.__name__))
            object.__setattr__(newpolicy, attr, value)
        return newpolicy

    def __setattr__(self, name, value):
        if hasattr(self, name):
            msg = "{!r} object attribute {!r} is read-only"
        else:
            msg = "{!r} object has no attribute {!r}"
        raise AttributeError(msg.format(self.__class__.__name__, name))

    def __add__(self, other):
        return self.clone(**other.__dict__)


class Policy(_PolicyBase):
    """Controls for how messages are parsed, handled and serialized."""

    def handle_defect(self, obj, defect):
        if self.raise_on_defect:
            raise defect
        self.register_defect(obj, defect)

    def register_defect(self, obj, defect):
        obj.defects.append(defect)

    def header_max_count(self, name):
        return None

    def header_source_parse(self, sourcelines):
        raise NotImplementedError

    def header_store_parse(self, name, value):
        raise NotImplementedError

    def header_fetch_parse(self, name, value):
        raise NotImplementedError

    def fold(self, name, value):
        raise NotImplementedError

    def fold_binary(self, name, value):
        raise NotImplementedError


class Compat32(Policy):
    """Backward-compatible policy: header values are plain strings."""

    mangle_from_ = True

    def _sanitize_header(self, name, value):
        return value

    def header_source_parse(self, sourcelines):
        name, value = sourcelines[0].split(":", 1)
        value = value.lstrip(" \t") + "".join(sourcelines[1:])
        return (name, value.rstrip("\r\n"))

    def header_store_parse(self, name, value):
        return (name, value)

    def header_fetch_parse(self, name, value):
        return self._sanitize_header(name, value)

    def fold(self, name, value):
        return name + ": " + value + self.linesep

    def fold_binary(self, name, value):
        return self.fold(name, value).encode("ascii", "surrogateescape")


compat32 = Compat32()


class EmailPolicy(Policy):
    """Policy with the modern header API (settings only here)."""

    message_factory = None
    utf8 = False
    refold_source = "long"
    header_factory = None
    content_manager = None

    def header_max_count(self, name):
        return 1 if name.lower() in ("subject", "date", "from", "to", "cc", "bcc", "sender", "reply-to",
                                     "message-id", "in-reply-to", "resent-date", "resent-from") else None

    def header_source_parse(self, sourcelines):
        name, value = sourcelines[0].split(":", 1)
        value = value.lstrip(" \t") + "".join(sourcelines[1:])
        return (name, value.rstrip("\r\n"))

    def header_store_parse(self, name, value):
        return (name, value)

    def header_fetch_parse(self, name, value):
        return value

    def fold(self, name, value):
        return name + ": " + value + self.linesep

    def fold_binary(self, name, value):
        return self.fold(name, value).encode("ascii", "surrogateescape")


default = EmailPolicy()
SMTP = default.clone(linesep="\r\n")
SMTPUTF8 = SMTP.clone(utf8=True)
HTTP = default.clone(linesep="\r\n", max_line_length=None)
strict = default.clone(raise_on_defect=True)
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "email.policy",
			FileDesc: "policy.py",
			Doc:      "This module provides the email policy objects (settings carriers).",
		},
		CodeSrc: policySrc,
	})
}
