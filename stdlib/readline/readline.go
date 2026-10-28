// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package readline is a minimal "readline" module: the history list, the
// history files and the completer settings that rlcompleter and site-style
// scripts use, kept as data.  There is no line editor behind it - gpython's
// REPL edits input with liner, which does not take a completer or hooks from
// Python - so input() is unaffected by what is set here, and
// get_line_buffer() is always empty.
package readline

import "github.com/vishnukv64/gpython/py"

const src = `
backend = "readline"
_READLINE_VERSION = 0x0802
_READLINE_RUNTIME_VERSION = 0x0802
_READLINE_LIBRARY_VERSION = "8.2"

_history = []
_history_length = -1
_completer = None
_delims = " \t\n` + "`" + `~!@#$%^&*()-=+[{]}\|;:'\",<>/?"
_hooks = {'startup': None, 'pre_input': None, 'display': None}
_begidx = 0
_endidx = 0
_completion_type = 0

def parse_and_bind(string, /):
    "Execute the init line provided in the string argument."
    if not isinstance(string, str):
        raise TypeError("parse_and_bind() argument 1 must be str, not %s" % type(string).__name__)
def read_init_file(filename=None, /):
    "Execute a readline initialization file."
    if filename is not None:
        open(filename).close()
def get_line_buffer():
    "Return the current contents of the line buffer."
    return ""
def insert_text(string, /):
    "Insert text into the line buffer at the cursor position."
def redisplay():
    "Change what's displayed on the screen to reflect the current contents of the line buffer."

def _trim():
    if _history_length >= 0:
        del _history[:max(0, len(_history) - _history_length)]
def read_history_file(filename=None, /):
    "Load a readline history file."
    import os
    if filename is None:
        filename = os.path.expanduser("~/.history")
    with open(filename, encoding="utf-8") as f:
        _history[:] = [l.rstrip("\n") for l in f]
    _trim()
def write_history_file(filename=None, /):
    "Save a readline history file."
    import os
    if filename is None:
        filename = os.path.expanduser("~/.history")
    _trim()
    with open(filename, "w", encoding="utf-8") as f:
        for l in _history:
            f.write(l + "\n")
def append_history_file(nelements, filename=None, /):
    "Append the last nelements items of the history list to file."
    import os
    if filename is None:
        filename = os.path.expanduser("~/.history")
    with open(filename, "a", encoding="utf-8") as f:
        for l in _history[max(0, len(_history) - nelements):]:
            f.write(l + "\n")
def set_history_length(length, /):
    "Set the maximal number of lines which will be written to the history file."
    global _history_length
    _history_length = length
def get_history_length():
    "Return the maximum number of lines that will be written to the history file."
    return _history_length
def clear_history():
    "Clear the current readline history."
    del _history[:]
def get_current_history_length():
    "Return the number of lines currently in the history."
    return len(_history)
def get_history_item(index, /):
    "Return the current contents of history item at one-based index."
    if 1 <= index <= len(_history):
        return _history[index - 1]
    return None
def remove_history_item(pos, /):
    "Remove history item given by its zero-based position."
    if not 0 <= pos < len(_history):
        raise ValueError("No history item at position %d" % pos)
    del _history[pos]
def replace_history_item(pos, line, /):
    "Replaces history item given by its position with contents of line."
    if not isinstance(line, str):
        raise TypeError("replace_history_item() argument 2 must be str, not %s" % type(line).__name__)
    if not 0 <= pos < len(_history):
        raise ValueError("No history item at position %d" % pos)
    _history[pos] = line
def add_history(string, /):
    "Add an item to the history buffer."
    if not isinstance(string, str):
        raise TypeError("add_history() argument 1 must be str, not %s" % type(string).__name__)
    _history.append(string)
    _trim()
def set_auto_history(enabled, /):
    "Enables or disables automatic history."
def set_startup_hook(function=None, /):
    "Set or remove the function invoked by the rl_startup_hook callback."
    _hooks['startup'] = function
def set_pre_input_hook(function=None, /):
    "Set or remove the function invoked by the rl_pre_input_hook callback."
    _hooks['pre_input'] = function
def set_completion_display_matches_hook(function=None, /):
    "Set or remove the completion display function."
    _hooks['display'] = function
def set_completer(function=None, /):
    "Set or remove the completer function."
    global _completer
    _completer = function
def get_completer():
    "Get the current completer function."
    return _completer
def get_completion_type():
    "Get the type of completion being attempted."
    return _completion_type
def get_begidx():
    "Get the beginning index of the completion scope."
    return _begidx
def get_endidx():
    "Get the ending index of the completion scope."
    return _endidx
def set_completer_delims(string, /):
    "Set the word delimiters for completion."
    global _delims
    _delims = string
def get_completer_delims():
    "Get the word delimiters for completion."
    return _delims
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "readline", Doc: "Importing this module enables command line editing using libedit/GNU readline."},
		CodeSrc: src,
	})
}
