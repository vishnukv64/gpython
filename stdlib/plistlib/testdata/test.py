"""plistlib: XML property lists, reading and writing."""

import io

import plistlib

data = {"name": "gpython", "count": 3, "flag": True, "ratio": 1.5, "nested": {"a": [1, 2]}}
blob = plistlib.dumps(data)
print("starts with header:", blob.startswith(b"<?xml"))
print("has plist tag:", b"<plist" in blob)

back = plistlib.loads(blob)
print("round trip:", back == data)
print("types:", type(back["count"]).__name__, type(back["flag"]).__name__, type(back["ratio"]).__name__)

buf = io.BytesIO()
plistlib.dump(data, buf)
buf.seek(0)
from_file = plistlib.load(buf)
print("via file:", from_file["name"], from_file["nested"])

raw = bytes([0, 1, 255])
withdata = {"blob": raw}
got = plistlib.loads(plistlib.dumps(withdata))
print("data round trip:", got["blob"] == raw, type(got["blob"]).__name__)

doc = b"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>ProductVersion</key>
    <string>14.5</string>
    <key>BuildVersion</key>
    <string>23F79</string>
</dict>
</plist>
"""
parsed = plistlib.loads(doc)
print("ProductVersion:", parsed["ProductVersion"])
print("BuildVersion:", parsed["BuildVersion"])
print("version split:", parsed["ProductVersion"].split("."))

try:
    plistlib.loads(b'<plist version="1.0"><dict><key>x</key></dict></plist>')
except ValueError as e:
    print("malformed value raises ValueError")

print("FMT_XML is str:", isinstance(plistlib.FMT_XML, str))

doc = "finished"
