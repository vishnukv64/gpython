"""TLS/SSL wrapper for socket objects: the Python half, over the native _ssl."""
import enum
import socket as _socket

import _ssl
from _ssl import *
from _ssl import (_SSLContext, OP_ALL, OP_NO_COMPRESSION, OP_NO_TICKET,
                  OP_NO_TLSv1_1, OP_NO_TLSv1_2, OP_NO_TLSv1_3,
                  HAS_NEVER_CHECK_COMMON_NAME, OPENSSL_VERSION_NUMBER)

SSLContext = _ssl.SSLContext
SSLError = _ssl.SSLError


class TLSVersion(enum.IntEnum):
    MINIMUM_SUPPORTED = -2
    SSLv3 = 768
    TLSv1 = 769
    TLSv1_1 = 770
    TLSv1_2 = 771
    TLSv1_3 = 772
    MAXIMUM_SUPPORTED = -1


class Purpose(enum.Enum):
    SERVER_AUTH = "1.3.6.1.5.5.7.3.1"
    CLIENT_AUTH = "1.3.6.1.5.5.7.3.2"


def create_default_context(purpose=Purpose.SERVER_AUTH, *, cafile=None,
                           capath=None, cadata=None):
    if not isinstance(purpose, Purpose):
        raise TypeError(f"purpose must be Purpose, not {purpose!r}")
    if purpose == Purpose.SERVER_AUTH:
        context = SSLContext(PROTOCOL_TLS_CLIENT)
        context.verify_mode = CERT_REQUIRED
        context.check_hostname = True
    else:
        context = SSLContext(PROTOCOL_TLS_SERVER)
    if cafile or capath or cadata:
        context.load_verify_locations(cafile, capath, cadata)
    elif context.verify_mode != CERT_NONE:
        context.load_default_certs(purpose)
    return context


def _create_unverified_context(protocol=None, *, cert_reqs=CERT_NONE,
                               check_hostname=False, purpose=Purpose.SERVER_AUTH,
                               certfile=None, keyfile=None, cafile=None,
                               capath=None, cadata=None):
    context = SSLContext(PROTOCOL_TLS_CLIENT if purpose == Purpose.SERVER_AUTH
                         else PROTOCOL_TLS_SERVER)
    context.check_hostname = check_hostname
    context.verify_mode = cert_reqs
    if certfile:
        context.load_cert_chain(certfile, keyfile)
    if cafile or capath or cadata:
        context.load_verify_locations(cafile, capath, cadata)
    return context


_create_default_https_context = create_default_context


def get_default_verify_paths():
    return _ssl.get_default_verify_paths()


def match_hostname(cert, hostname):
    raise NotImplementedError("match_hostname was removed in Python 3.12")
