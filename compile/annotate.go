package compile

import (
	"strings"

	"github.com/vishnukv64/gpython/ast"
)

// Deferred annotations (PEP 649, PEP 749).
//
// Annotations are not evaluated where they are written.  Every function
// whose parameters or return are annotated gets an __annotate__ function; a
// class or module whose body has "x: T" gets one too, stored as
// __annotate_func__ (class) or __annotate__ (module).  Calling it with a
// format returns the annotations dict, evaluating on demand:
//
//	VALUE (1), VALUE_WITH_FAKE_GLOBALS (2)  the values; a NameError propagates
//	FORWARDREF (3)                          an annotation that raises
//	                                        NameError becomes a ForwardRef
//	STRING (4)                              the source text
//
// The pass runs on the tree before the symbol table is built:
//
//	def f(x: A) -> B: ...
//
// becomes a FunctionDef whose Annotate is
//
//	lambda format, /: typing._gpy_annotate(format, ("x", "return"),
//	                                       (lambda: A, lambda: B), ("A", "B"))
//
// In a class body the names the class binds are not visible to nested
// scopes, but an annotation may use them, so there the function takes the
// class namespace as a hidden keyword-only argument and such names are
// looked up in it first.

const classdictName = "__classdict__"

type annotator struct {
	classNames map[ast.Identifier]bool // names bound by the class being walked, or nil
}

// lazyAnnotations rewrites mod in place.  It does nothing for a module under
// "from __future__ import annotations", whose annotations are strings
// already, or for the embedded library, which is compiled with string
// annotations.
func lazyAnnotations(mod ast.Ast, filename string) {
	if strings.HasPrefix(filename, "<gpython-lib>") {
		return
	}
	var body *[]ast.Stmt
	switch m := mod.(type) {
	case *ast.Module:
		body = &m.Body
	default:
		return
	}
	for _, stmt := range *body {
		if imp, ok := stmt.(*ast.ImportFrom); ok && imp.Module == "__future__" {
			for _, alias := range imp.Names {
				if alias.Name == "annotations" {
					return
				}
			}
		}
	}
	a := &annotator{}
	*body = a.stmts(*body, false, false)
}

// stmts walks a statement list.  scope says the list is a module or class
// body (so "x: T" is collected into an __annotate__).
func (a *annotator) stmts(list []ast.Stmt, scope, class bool) []ast.Stmt {
	var names []string
	var exprs []ast.Expr
	var pos ast.Pos
	list = a.walk(list, scope, &names, &exprs, &pos)
	if len(names) > 0 {
		fn := a.annotateDef(pos, names, exprs, class)
		if class {
			fn.StoreName = "__annotate_func__"
		}
		list = append([]ast.Stmt{fn}, list...)
	}
	return list
}

// walk rewrites list, appending the annotations of simple "x: T" statements
// to names/exprs when collect is set.
func (a *annotator) walk(list []ast.Stmt, collect bool, names *[]string, exprs *[]ast.Expr, pos *ast.Pos) []ast.Stmt {
	out := make([]ast.Stmt, 0, len(list))
	for _, s := range list {
		switch n := s.(type) {
		case *ast.AnnAssign:
			name, ok := n.Target.(*ast.Name)
			if !collect || !ok {
				out = append(out, s)
				continue
			}
			if len(*names) == 0 {
				*pos = ast.Pos{Lineno: n.GetLineno(), ColOffset: n.GetColOffset()}
			}
			*names = append(*names, string(name.Id))
			*exprs = append(*exprs, n.Annotation)
			if n.Value != nil {
				out = append(out, &ast.Assign{StmtBase: n.StmtBase, Targets: []ast.Expr{n.Target}, Value: n.Value})
			}
			continue
		case *ast.FunctionDef:
			n.Body = a.stmts(n.Body, false, false)
			a.annotateFunction(n)
		case *ast.ClassDef:
			outer := a.classNames
			a.classNames = classBoundNames(n.Body)
			n.Body = a.stmts(n.Body, false, true)
			a.classNames = outer
		case *ast.If:
			n.Body = a.walk(n.Body, collect, names, exprs, pos)
			n.Orelse = a.walk(n.Orelse, collect, names, exprs, pos)
		case *ast.For:
			n.Body = a.walk(n.Body, collect, names, exprs, pos)
			n.Orelse = a.walk(n.Orelse, collect, names, exprs, pos)
		case *ast.While:
			n.Body = a.walk(n.Body, collect, names, exprs, pos)
			n.Orelse = a.walk(n.Orelse, collect, names, exprs, pos)
		case *ast.With:
			n.Body = a.walk(n.Body, collect, names, exprs, pos)
		case *ast.Try:
			n.Body = a.walk(n.Body, collect, names, exprs, pos)
			for _, h := range n.Handlers {
				h.Body = a.walk(h.Body, collect, names, exprs, pos)
			}
			n.Orelse = a.walk(n.Orelse, collect, names, exprs, pos)
			n.Finalbody = a.walk(n.Finalbody, collect, names, exprs, pos)
		case *ast.MatchStmt:
			for _, c := range n.Cases {
				c.Body = a.walk(c.Body, collect, names, exprs, pos)
			}
		}
		out = append(out, s)
	}
	return out
}

// annotateFunction moves the annotations of a def into its Annotate.
func (a *annotator) annotateFunction(fn *ast.FunctionDef) {
	if fn.Args == nil {
		return
	}
	var names []string
	var exprs []ast.Expr
	take := func(args ...*ast.Arg) {
		for _, arg := range args {
			if arg != nil && arg.Annotation != nil {
				names = append(names, string(arg.Arg))
				exprs = append(exprs, arg.Annotation)
				arg.Annotation = nil
			}
		}
	}
	take(fn.Args.Args...)
	take(fn.Args.Vararg)
	take(fn.Args.Kwonlyargs...)
	take(fn.Args.Kwarg)
	if fn.Returns != nil {
		names = append(names, "return")
		exprs = append(exprs, fn.Returns)
		fn.Returns = nil
	}
	if len(names) == 0 {
		return
	}
	pos := ast.Pos{Lineno: fn.GetLineno(), ColOffset: fn.GetColOffset()}
	args, body := a.annotateParts(pos, names, exprs, a.classNames != nil)
	fn.Annotate = &ast.Lambda{ExprBase: ast.ExprBase{Pos: pos}, Args: args, Body: body}
}

func (a *annotator) annotateDef(pos ast.Pos, names []string, exprs []ast.Expr, class bool) *ast.FunctionDef {
	args, body := a.annotateParts(pos, names, exprs, class)
	return &ast.FunctionDef{
		StmtBase: ast.StmtBase{Pos: pos},
		Name:     "__annotate__",
		Args:     args,
		Body:     []ast.Stmt{&ast.Return{StmtBase: ast.StmtBase{Pos: pos}, Value: body}},
	}
}

// annotateParts builds the parameters and the body expression of an
// __annotate__ function.  inClass adds the hidden class-namespace argument.
func (a *annotator) annotateParts(pos ast.Pos, names []string, exprs []ast.Expr, inClass bool) (*ast.Arguments, ast.Expr) {
	eb := ast.ExprBase{Pos: pos}
	format := &ast.Arg{Pos: pos, Arg: "format"}
	args := &ast.Arguments{Pos: pos, Args: []*ast.Arg{format}, Posonlyargs: []*ast.Arg{format}}
	if inClass {
		locals := &ast.Call{ExprBase: eb, Func: ast.NameExpr(pos, "locals", ast.Load)}
		args.Kwonlyargs = []*ast.Arg{{Pos: pos, Arg: classdictName, Default: locals}}
		args.KwDefaults = []ast.Expr{locals}
	}
	var nameElts, thunks, srcs []ast.Expr
	for i, e := range exprs {
		src, err := annotationSource(e)
		if str, ok := e.(*ast.Str); ok {
			// A string annotation is its own source text.
			src, err = string(str.S), nil
		}
		if err != nil {
			src = ""
		}
		if inClass && a.classNames != nil {
			e = a.rewriteClassNames(e)
		}
		nameElts = append(nameElts, ast.StrExpr(pos, names[i]))
		srcs = append(srcs, ast.StrExpr(pos, src))
		thunks = append(thunks, &ast.Lambda{ExprBase: eb, Args: &ast.Arguments{Pos: pos}, Body: e})
	}
	tuple := func(elts []ast.Expr) ast.Expr { return &ast.Tuple{ExprBase: eb, Elts: elts, Ctx: ast.Load} }
	return args, ast.TypingCall(pos, "_gpy_annotate", ast.NameExpr(pos, "format", ast.Load), tuple(nameElts), tuple(thunks), tuple(srcs), fwdRef(pos))
}

// fwdRef is lambda: __import__("annotationlib").ForwardRef
func fwdRef(pos ast.Pos) ast.Expr {
	eb := ast.ExprBase{Pos: pos}
	imp := &ast.Call{ExprBase: eb, Func: ast.NameExpr(pos, "__import__", ast.Load), Args: []ast.Expr{ast.StrExpr(pos, "annotationlib")}}
	return &ast.Lambda{ExprBase: eb, Args: &ast.Arguments{Pos: pos}, Body: &ast.Attribute{ExprBase: eb, Value: imp, Attr: "ForwardRef", Ctx: ast.Load}}
}

// classBoundNames collects the names a class body binds at its own level.
func classBoundNames(body []ast.Stmt) map[ast.Identifier]bool {
	names := map[ast.Identifier]bool{}
	var target func(e ast.Expr)
	target = func(e ast.Expr) {
		switch t := e.(type) {
		case *ast.Name:
			names[t.Id] = true
		case *ast.Tuple:
			for _, x := range t.Elts {
				target(x)
			}
		case *ast.List:
			for _, x := range t.Elts {
				target(x)
			}
		case *ast.Starred:
			target(t.Value)
		}
	}
	var walk func(list []ast.Stmt)
	walk = func(list []ast.Stmt) {
		for _, s := range list {
			switch n := s.(type) {
			case *ast.Assign:
				for _, t := range n.Targets {
					target(t)
				}
			case *ast.AnnAssign:
				target(n.Target)
			case *ast.AugAssign:
				target(n.Target)
			case *ast.FunctionDef:
				names[n.Binding()] = true
			case *ast.ClassDef:
				names[n.Binding()] = true
			case *ast.Import:
				for _, al := range n.Names {
					if al.AsName != "" {
						names[al.AsName] = true
					} else {
						names[ast.Identifier(strings.SplitN(string(al.Name), ".", 2)[0])] = true
					}
				}
			case *ast.ImportFrom:
				for _, al := range n.Names {
					if al.AsName != "" {
						names[al.AsName] = true
					} else {
						names[al.Name] = true
					}
				}
			case *ast.For:
				target(n.Target)
				walk(n.Body)
				walk(n.Orelse)
			case *ast.While:
				walk(n.Body)
				walk(n.Orelse)
			case *ast.If:
				walk(n.Body)
				walk(n.Orelse)
			case *ast.With:
				for _, it := range n.Items {
					if it.OptionalVars != nil {
						target(it.OptionalVars)
					}
				}
				walk(n.Body)
			case *ast.Try:
				walk(n.Body)
				for _, h := range n.Handlers {
					if h.Name != "" {
						names[h.Name] = true
					}
					walk(h.Body)
				}
				walk(n.Orelse)
				walk(n.Finalbody)
			}
		}
	}
	walk(body)
	return names
}

// rewriteClassNames replaces each load of a name the class binds with
// (__classdict__["n"] if "n" in __classdict__ else n), so the name resolves
// in the class namespace first and otherwise as it would from outside.
func (a *annotator) rewriteClassNames(e ast.Expr) ast.Expr {
	var rw func(e ast.Expr) ast.Expr
	rwList := func(l []ast.Expr) {
		for i := range l {
			l[i] = rw(l[i])
		}
	}
	rw = func(e ast.Expr) ast.Expr {
		switch n := e.(type) {
		case *ast.Name:
			if n.Ctx != ast.Load || !a.classNames[n.Id] {
				return e
			}
			pos := n.Pos
			eb := ast.ExprBase{Pos: pos}
			dict := func() ast.Expr { return ast.NameExpr(pos, classdictName, ast.Load) }
			key := func() ast.Expr { return ast.StrExpr(pos, string(n.Id)) }
			return &ast.IfExp{
				ExprBase: eb,
				Test:     &ast.Compare{ExprBase: eb, Left: key(), Ops: []ast.CmpOp{ast.In}, Comparators: []ast.Expr{dict()}},
				Body:     &ast.Subscript{ExprBase: eb, Value: dict(), Slice: &ast.Index{Value: key()}, Ctx: ast.Load},
				Orelse:   e,
			}
		case *ast.Attribute:
			n.Value = rw(n.Value)
		case *ast.Subscript:
			n.Value = rw(n.Value)
			switch s := n.Slice.(type) {
			case *ast.Index:
				s.Value = rw(s.Value)
			case *ast.Slice:
				if s.Lower != nil {
					s.Lower = rw(s.Lower)
				}
				if s.Upper != nil {
					s.Upper = rw(s.Upper)
				}
				if s.Step != nil {
					s.Step = rw(s.Step)
				}
			case *ast.ExtSlice:
				for _, d := range s.Dims {
					if idx, ok := d.(*ast.Index); ok {
						idx.Value = rw(idx.Value)
					}
				}
			}
		case *ast.Tuple:
			rwList(n.Elts)
		case *ast.List:
			rwList(n.Elts)
		case *ast.Starred:
			n.Value = rw(n.Value)
		case *ast.BinOp:
			n.Left, n.Right = rw(n.Left), rw(n.Right)
		case *ast.UnaryOp:
			n.Operand = rw(n.Operand)
		case *ast.BoolOp:
			rwList(n.Values)
		case *ast.IfExp:
			n.Test, n.Body, n.Orelse = rw(n.Test), rw(n.Body), rw(n.Orelse)
		case *ast.Compare:
			n.Left = rw(n.Left)
			rwList(n.Comparators)
		case *ast.Call:
			n.Func = rw(n.Func)
			rwList(n.Args)
			for _, k := range n.Keywords {
				k.Value = rw(k.Value)
			}
		case *ast.Dict:
			for i, k := range n.Keys {
				if k != nil {
					n.Keys[i] = rw(k)
				}
			}
			rwList(n.Values)
		}
		return e
	}
	return rw(e)
}
