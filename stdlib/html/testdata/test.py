import html
import html.entities
import html.parser

# escape returns a str, and quotes the shell-unsafe characters when asked.
assert isinstance(html.escape('<a href="x">'), str)
assert html.escape('<a href="x">') == "&lt;a href=&quot;x&quot;&gt;", html.escape('<a href="x">')
assert html.escape('<a href="x">', quote=False) == '&lt;a href="x"&gt;', html.escape('<a href="x">', quote=False)
assert html.escape("'\"<>&") == "&#x27;&quot;&lt;&gt;&amp;"

# unescape resolves named, decimal and hex references, and the HTML5 rules.
assert html.unescape("&amp;&#65;&#x42;&copy;&nonsense;") == "&AB©&nonsense;"
assert html.unescape("&notit; &notin; &amp &ampx") == "¬it; ∉ & &x"
assert html.unescape("a &lt; b &gt; c") == "a < b > c"
assert html.unescape("&#0;") == "\ufffd"

# html.entities tables.
assert html.entities.html5["amp;"] == "&"
assert html.entities.html5["copy;"] == "©"
assert html.entities.name2codepoint["copy"] == 169
assert html.entities.codepoint2name[169] == "copy"
assert html.entities.name2codepoint["amp"] == 38
assert html.entities.entitydefs["copy"] == "©"
assert len(html.entities.html5) == 2231
assert len(html.entities.name2codepoint) == 252
assert len(html.entities.codepoint2name) == 252


events = []


class Recorder(html.parser.HTMLParser):
    # events is a module-level list rather than an instance attribute: this
    # interpreter does not call a Python subclass's __init__ when the base is
    # a native type, so an attribute set there would never be created.

    def handle_starttag(self, tag, attrs):
        events.append(("start", tag, attrs))

    def handle_startendtag(self, tag, attrs):
        events.append(("startend", tag, attrs))

    def handle_endtag(self, tag):
        events.append(("end", tag))

    def handle_data(self, data):
        events.append(("data", data))

    def handle_entityref(self, name):
        events.append(("eref", name))

    def handle_charref(self, name):
        events.append(("cref", name))

    def handle_comment(self, data):
        events.append(("comment", data))

    def handle_decl(self, decl):
        events.append(("decl", decl))

    def handle_pi(self, data):
        events.append(("pi", data))

    def unknown_decl(self, data):
        events.append(("unk", data))


def parse(text, **kw):
    del events[:]
    p = Recorder(**kw)
    p.feed(text)
    p.close()
    return p


# convert_charrefs=True (the default) folds references into handle_data.
p = parse("<p>a&amp;b<br/></p>")
assert p.convert_charrefs is True
assert events == [
    ("start", "p", []),
    ("data", "a&b"),
    ("startend", "br", []),
    ("end", "p"),
], events

# convert_charrefs=False routes them to handle_entityref / handle_charref.
p = parse("<p>a&amp;b&#65;<br/></p>", convert_charrefs=False)
assert events == [
    ("start", "p", []),
    ("data", "a"),
    ("eref", "amp"),
    ("data", "b"),
    ("cref", "65"),
    ("startend", "br", []),
    ("end", "p"),
], events

# Attributes, lowercased names, and the None value for a valueless attribute.
parse('<div a=1 b="2" c>t</div>')
assert events[0] == ("start", "div", [("a", "1"), ("b", "2"), ("c", None)]), events
assert events[1] == ("data", "t")
assert events[2] == ("end", "div")

# A quoted value may contain '>'.
parse("<div foo='b>c'>")
assert events[0] == ("start", "div", [("foo", "b>c")]), events

# Comments, declarations and processing instructions.
parse("<!-- c --><!DOCTYPE html><?pi x>")
assert events == [("comment", " c "), ("decl", "DOCTYPE html"), ("pi", "pi x")], events

# "</>" is ignored; "</ x>" is a bogus comment.
parse("</>")
assert events == [], events
parse("</ x>")
assert events == [("comment", " x")], events

# CDATA.
parse("<![CDATA[y]]>")
assert events == [("unk", "CDATA[y")], events

# get_starttag_text returns the last start tag verbatim.
p = parse("<DIV CLASS=x>")
assert p.get_starttag_text() == "<DIV CLASS=x>", p.get_starttag_text()
assert events[0] == ("start", "div", [("class", "x")]), events

print("html ok")
