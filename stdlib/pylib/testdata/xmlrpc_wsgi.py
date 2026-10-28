import xml.etree.ElementTree as ET
import xmlrpc.client as c

print(c.dumps((1, "a<b", 2.5, True, [1, 2], {"k": "v"}, b"by", None), "m.method", allow_none=True))
print(c.dumps(c.Fault(4, "bad"), methodresponse=True))
print(c.dumps(("r",), methodresponse=True))
print(c.loads(c.dumps((1, "a<b", 2.5, True, [1, 2], {"k": "v"}, None), "m.method", allow_none=True)))
resp, meth = c.loads(c.dumps(("ok",), methodresponse=True))
print(resp, meth)
try:
    c.loads(c.dumps(c.Fault(7, "boom"), methodresponse=True))
except c.Fault as f:
    print("Fault", f.faultCode, f.faultString)
try:
    c.loads("<methodCall><methodName>x</methodName>")
except Exception as e:
    print(type(e).__name__.split(".")[-1])
print(c.escape("a&b<c"), c.MAXINT, repr(c.Fault(1, "x")), c.boolean(1), c.Binary(b"hi").data)
print(c.DateTime("20240102T03:04:05").value, c.ProtocolError("u", 500, "m", {}).errcode)

root = ET.fromstring('<a x="1" y="2"><b>t&amp;u</b><!--c--><c/>tail</a>')
print(root.tag, sorted(root.attrib.items()), [(e.tag, e.text) for e in root], root[1].tail)
ns = ET.fromstring('<r xmlns:p="urn:x"><p:e p:a="1"/></r>')
print(ns[0].tag, ns[0].attrib)
try:
    ET.fromstring("<a><b></a>")
except ET.ParseError as e:
    print("ParseError")
