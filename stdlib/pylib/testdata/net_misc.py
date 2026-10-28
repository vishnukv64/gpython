import ftplib
import imaplib
import io
import poplib
import smtplib
import socket
import socketserver
import sys
import threading
import warnings
import webbrowser
from wsgiref import headers as wh
from wsgiref import util as wu
from wsgiref import validate as wv
from wsgiref.handlers import BaseHandler, SimpleHandler

warnings.simplefilter("ignore")
import nturl2path
import sre_compile
import sre_constants
import sre_parse

# protocol modules: constants and pure helpers, no network
print(smtplib.SMTP_PORT, smtplib.SMTP_SSL_PORT, repr(smtplib.CRLF))
print(smtplib.quoteaddr("<a@b.c>"), repr(smtplib.quotedata("a\n.b\n")))
print(smtplib.SMTPResponseException.__mro__[1].__name__)
# Internaldate2tuple answers in local time; compare as epoch seconds so the
# golden does not depend on the machine's timezone.
import time
print(imaplib.IMAP4_PORT, imaplib.IMAP4_SSL_PORT, int(time.mktime(imaplib.Internaldate2tuple(b'INTERNALDATE "01-Jan-2020 10:00:00 +0000"'))))
print(poplib.POP3_PORT, poplib.POP3_SSL_PORT, poplib.error_proto.__name__)
print(ftplib.FTP_PORT, ftplib.MAXLINE, ftplib.error_perm.__mro__[1].__name__)
print(ftplib.parse150("150 Opening data connection for x (123 bytes)"))
print(socket._GLOBAL_DEFAULT_TIMEOUT is socket._GLOBAL_DEFAULT_TIMEOUT)

# socketserver: localhost echo
class Echo(socketserver.StreamRequestHandler):
    def handle(self):
        self.wfile.write(b"echo:" + self.rfile.readline())


with socketserver.TCPServer(("127.0.0.1", 0), Echo) as srv:
    port = srv.server_address[1]
    t = threading.Thread(target=srv.handle_request)
    t.start()
    with socket.create_connection(("127.0.0.1", port), timeout=10) as c:
        c.sendall(b"ping\n")
        print(c.makefile("rb").readline())
    t.join()

# wsgiref: util, headers, validate and a handler, all in process
env = {"wsgi.url_scheme": "http", "HTTP_HOST": "example.com:8080",
       "SCRIPT_NAME": "/app", "PATH_INFO": "/a b", "QUERY_STRING": "x=1"}
print(wu.request_uri(env), wu.application_uri(env))
print(wu.shift_path_info(dict(env, PATH_INFO="/one/two")))
print(wu.guess_scheme({"HTTPS": "on"}), wu.is_hop_by_hop("Connection"))
h = wh.Headers([("Content-Type", "text/plain")])
h.add_header("X-Test", "v", a="b")
print(repr(str(h)), h["content-type"], h.get_all("x-test"))


def app(environ, start_response):
    start_response("200 OK", [("Content-Type", "text/plain")])
    return [b"hello wsgi"]


out, err = io.BytesIO(), io.StringIO()
e = {}
wu.setup_testing_defaults(e)
SimpleHandler(io.BytesIO(), out, err, e).run(wv.validator(app))
print([l for l in out.getvalue().split(b"\r\n") if not l.startswith(b"Date:")])
try:
    wv.validator(lambda e, s: None)(e, lambda *a: None)
except AssertionError as ex:
    print("AssertionError", ex)

# webbrowser with a registered fake controller
calls = []


class Fake(webbrowser.BaseBrowser):
    def open(self, url, new=0, autoraise=True):
        calls.append((url, new))
        return True


webbrowser.register("fake", None, Fake("fake"), preferred=True)
print(webbrowser.get("fake").name, webbrowser.open("http://x.test", new=2), webbrowser.open_new_tab("http://y.test"), calls)
try:
    webbrowser.get("no-such-browser-xyz")
except webbrowser.Error as ex:
    print("Error", ex)

# nturl2path
print(nturl2path.url2pathname("///C|/foo/bar"), nturl2path.pathname2url("C:\\foo\\bar"))

# sre_*: parser and constants (re is not the SRE engine here, so no compile())
p = sre_parse.parse(r"a(b|c)+\d", 0)
print(p)
print(p.getwidth(), p.state.groups)
print(sre_constants.MAXREPEAT, sre_constants.MAGIC, sre_constants.OPCODES[3])
print(sre_constants.error.__mro__[1].__name__)
print(sre_compile.isstring("x"), sre_compile._code(p, 0)[:3])
