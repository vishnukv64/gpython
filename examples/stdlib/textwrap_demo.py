"""textwrap: wrap and indent blocks of text.

Run with:  /tmp/gpy examples/stdlib/textwrap_demo.py

wrap() returns a list of lines, fill() returns one string, and dedent(),
indent() and shorten() reformat a whole block.

Interpreter note: shorten() takes its width as a *positional* argument --
the keyword form raises `TypeError: "shorten() missing 1 required positional
argument: 'width'"`.
"""

import textwrap

PROSE = ("The quick brown fox jumps over the lazy dog, and then it keeps on "
         "running well past the point where a single line of output would "
         "comfortably hold it all.")

print("--- the source text ---")
print(PROSE)

print()
print("--- wrap() returns a list of lines ---")
for width in [30, 50, 79]:
    lines = textwrap.wrap(PROSE, width=width)
    print("width=%d -> %d lines" % (width, len(lines)))
    for line in lines:
        print("  |" + line + "|")
    print("  no line exceeds the width:",
          all([len(line) <= width for line in lines]))
    print()

print("--- fill() returns the joined string ---")
print(textwrap.fill(PROSE, width=40))
print()
print("the same text re-wrapped at width=40 is the join of wrap():",
      textwrap.fill(PROSE, width=40) == "\n".join(textwrap.wrap(PROSE, width=40)))

print()
print("--- break_long_words=False keeps long tokens intact ---")
long_token = "a_very_long_identifier_that_cannot_be_broken_at_all"
print("default (breaks the token):")
for line in textwrap.wrap(long_token, width=20):
    print("  |" + line + "|")
print("break_long_words=False (keeps it whole):")
for line in textwrap.wrap(long_token, width=20, break_long_words=False):
    print("  |" + line + "|")

print()
print("--- dedent() strips the common leading whitespace ---")
indented = """
        def example():
            return 1

        class Thing:
            pass
    """
print("raw block:")
print(indented)
print("dedented:")
print(textwrap.dedent(indented))
print("the common prefix was removed from every non-blank line")

print()
print("--- indent() adds a prefix to chosen lines ---")
block = "first\nsecond\nthird\n"
print("plain:            ", repr(block))
print("indent('> '):     ", repr(textwrap.indent(block, "> ")))
print("indent('    '):   ", repr(textwrap.indent(block, "    ")))
numbered = textwrap.indent(block, "1. ")
print("for a numbered list:", repr(numbered))

print()
print("--- shorten() collapses the middle ---")
for limit in [100, 40, 20]:
    print("  limit=%-4d -> %r" % (limit, textwrap.shorten(PROSE, limit)))
print("note: shorten() collapses the middle to '[...]' and, at a tiny limit,")
print("returns just the placeholder rather than failing:")
print("  limit=5 ->", repr(textwrap.shorten(PROSE, 5)))

print()
print("--- TextWrapper holds the settings ---")
wrapper = textwrap.TextWrapper(width=35, initial_indent="  ", subsequent_indent="  ")
print(wrapper.fill("A paragraph with an indent applied to every line it produces."))
print()
print("wrap() with a first-line indent only:")
wrapper = textwrap.TextWrapper(width=30, initial_indent="* ")
for line in wrapper.wrap("A bullet point that runs on for a while."):
    print(line)

print()
print("--- a worked example: a terminal-width report ---")


class Report:
    def __init__(self, title, width):
        self.title = title
        self.width = width

    def rule(self, character="-"):
        return character * self.width

    def render(self, rows):
        lines = []
        lines.append(self.rule("="))
        lines.append(self.title.center(self.width) if False else self.title)
        lines.append(self.rule("="))
        for label, description in rows:
            lines.append(label.upper())
            for line in textwrap.wrap(description, width=self.width - 4):
                lines.append("    " + line)
            lines.append("")
        lines.append(self.rule("-"))
        return lines


rows = [
    ("usage", "Call the tool with a path, and optionally a --verbose flag "
              "to see every step it takes."),
    ("errors", "A missing file produces a FileNotFoundError; a malformed "
               "document produces a ValueError."),
]
report = Report("gpython tool reference", 46)
for line in report.render(rows):
    print(line)

print("--- a help-text renderer with a hanging indent ---")


def usage(option, description, width=56):
    """Render an option and its description, wrapped and hanging-indented."""
    prefix = "  %-16s" % (option,)
    body = textwrap.wrap(description, width=width - len(prefix),
                         initial_indent=prefix,
                         subsequent_indent=" " * len(prefix))
    return "\n".join(body)


print(usage("--verbose", "print every stage as it runs, including the "
                         "intermediate values passed between them"))
print(usage("--output PATH", "write the result to PATH instead of stdout, "
                             "creating parent directories as needed"))
