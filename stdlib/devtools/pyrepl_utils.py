# gpython's stand-in for _pyrepl.utils, which pdb imports for syntax
# colouring.  CPython's file is written with PEP 695 "type" statements, which
# gpython's parser does not accept yet, so it cannot be copied.  Colouring is
# only used when output is a colour terminal; asking for it raises.

def _unsupported(*args, **kwargs):
    raise NotImplementedError(
        "syntax colouring needs CPython's _pyrepl.utils, which gpython "
        "cannot parse yet (PEP 695 type statements)")


gen_colors = disp_str = _unsupported
