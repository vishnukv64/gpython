package ast

import (
	"fmt"

	"github.com/vishnukv64/gpython/py"
)

// PEP 695 is implemented by rewriting the tree before the symbol table is
// built, so that the symbol table, the compiler and the VM need no special
// scope.  Each generic definition becomes a function that creates the type
// parameters and returns the definition, exactly what CPython compiles:
//
//	def f[T: int](x: T = d) -> T: ...
//
// becomes
//
//	def <generic parameters of f>():
//	    T = typing._gpy_typevar("T", 1, lambda: int, None)
//	    def f(x: T = d) -> T: ...      # bound to a hidden name
//	    f.__type_params__ = (T,)
//	    return f
//	f = <generic parameters of f>()
//	del <generic parameters of f>
//
// The type parameters are ordinary locals of that function, so the function,
// its annotations and (for a class) its methods see them as closure
// variables, and a class body sees them through the enclosing function.
// Bounds, constraints and defaults are wrapped in lambdas, which is what
// makes them lazily evaluated.  Decorators stay outside and are applied to
// the result.

const genericPrefix = "<generic parameters of "

// IsGenericScopeName reports whether a function name is that of a generated
// generic scope; qualified names skip such scopes.
func IsGenericScopeName(name string) bool {
	return len(name) > len(genericPrefix) && name[:len(genericPrefix)] == genericPrefix
}

type desugarer struct {
	err error
}

// DesugarTypeParams rewrites every PEP 695 construct of mod in place.
func DesugarTypeParams(mod Ast) error {
	d := &desugarer{}
	switch m := mod.(type) {
	case *Module:
		m.Body = d.stmts(m.Body)
	case *Interactive:
		m.Body = d.stmts(m.Body)
	case *Suite:
		m.Body = d.stmts(m.Body)
	}
	return d.err
}

func (d *desugarer) fail(pos Pos, format string, a ...interface{}) {
	if d.err == nil {
		d.err = py.MakeSyntaxError(py.ExceptionNewf(py.SyntaxError, format, a...), "", pos.Lineno, pos.ColOffset, "")
	}
}

func (d *desugarer) stmts(list []Stmt) []Stmt {
	var out []Stmt
	changed := false
	for _, s := range list {
		rewritten := d.stmt(s)
		if len(rewritten) != 1 || rewritten[0] != s {
			changed = true
		}
		out = append(out, rewritten...)
	}
	if !changed {
		return list
	}
	return out
}

// stmt returns the statements that replace s.
func (d *desugarer) stmt(s Stmt) []Stmt {
	switch n := s.(type) {
	case *FunctionDef:
		n.Body = d.stmts(n.Body)
		if len(n.TypeParams) == 0 {
			return []Stmt{s}
		}
		return d.generic(n, n.Name, &n.TypeParams, n.DecoratorList, &n.DecoratorList, &n.StoreName)
	case *ClassDef:
		n.Body = d.stmts(n.Body)
		if len(n.TypeParams) == 0 {
			return []Stmt{s}
		}
		return d.generic(n, n.Name, &n.TypeParams, n.DecoratorList, &n.DecoratorList, &n.StoreName)
	case *TypeAlias:
		return d.typeAlias(n)
	case *If:
		n.Body = d.stmts(n.Body)
		n.Orelse = d.stmts(n.Orelse)
	case *For:
		n.Body = d.stmts(n.Body)
		n.Orelse = d.stmts(n.Orelse)
	case *While:
		n.Body = d.stmts(n.Body)
		n.Orelse = d.stmts(n.Orelse)
	case *With:
		n.Body = d.stmts(n.Body)
	case *Try:
		n.Body = d.stmts(n.Body)
		for _, h := range n.Handlers {
			h.Body = d.stmts(h.Body)
		}
		n.Orelse = d.stmts(n.Orelse)
		n.Finalbody = d.stmts(n.Finalbody)
	case *MatchStmt:
		for _, c := range n.Cases {
			c.Body = d.stmts(c.Body)
		}
	}
	return []Stmt{s}
}

func nameExpr(pos Pos, id string, ctx ExprContext) *Name {
	return &Name{ExprBase: ExprBase{Pos: pos}, Id: Identifier(id), Ctx: ctx}
}

func strExpr(pos Pos, s string) Expr {
	return &Str{ExprBase: ExprBase{Pos: pos}, S: py.String(s)}
}

func numExpr(pos Pos, n int) Expr {
	return &Num{ExprBase: ExprBase{Pos: pos}, N: py.Int(n)}
}

func noneExpr(pos Pos) Expr {
	return &NameConstant{ExprBase: ExprBase{Pos: pos}, Value: py.None}
}

func thunk(pos Pos, body Expr) Expr {
	return &Lambda{ExprBase: ExprBase{Pos: pos}, Args: &Arguments{Pos: pos}, Body: body}
}

// typingCall builds typing.<fn>(args...), importing typing by __import__ so
// that nothing the program defines can shadow it.
func typingCall(pos Pos, fn string, args ...Expr) Expr {
	imp := &Call{ExprBase: ExprBase{Pos: pos}, Func: nameExpr(pos, "__import__", Load), Args: []Expr{strExpr(pos, "typing")}}
	return &Call{
		ExprBase: ExprBase{Pos: pos},
		Func:     &Attribute{ExprBase: ExprBase{Pos: pos}, Value: imp, Attr: Identifier(fn), Ctx: Load},
		Args:     args,
	}
}

// typeParamStmts returns "T = typevar(...)" for each parameter, after
// checking the list the way CPython's compiler does.
func (d *desugarer) typeParamStmts(params []*TypeParam) []Stmt {
	var out []Stmt
	seen := map[Identifier]bool{}
	sawDefault := false
	for _, p := range params {
		if seen[p.Name] {
			d.fail(p.Pos, "duplicate type parameter '%s'", p.Name)
		}
		seen[p.Name] = true
		if p.Default != nil {
			sawDefault = true
		} else if sawDefault && p.Kind == TypeVarParam {
			d.fail(p.Pos, "non-default type parameter '%s' follows default type parameter", p.Name)
		}
		var dflt Expr = noneExpr(p.Pos)
		if p.Default != nil {
			def := p.Default
			if st, ok := def.(*Starred); ok {
				// "*Ts = *tuple[int]": the default is the one item "*" yields.
				def = &Subscript{ExprBase: ExprBase{Pos: p.Pos}, Slice: &Index{Value: numExpr(p.Pos, 0)}, Ctx: Load,
					Value: &Tuple{ExprBase: ExprBase{Pos: p.Pos}, Elts: []Expr{st}, Ctx: Load}}
			}
			dflt = thunk(p.Pos, def)
		}
		var call Expr
		switch p.Kind {
		case TypeVarParam:
			kind := 0
			var bound Expr = noneExpr(p.Pos)
			if p.Bound != nil {
				kind = 1
				if p.Constraints {
					kind = 2
				}
				bound = thunk(p.Pos, p.Bound)
			}
			call = typingCall(p.Pos, "_gpy_typevar", strExpr(p.Pos, string(p.Name)), numExpr(p.Pos, kind), bound, dflt)
		case TypeVarTupleParam:
			call = typingCall(p.Pos, "_gpy_typevartuple", strExpr(p.Pos, string(p.Name)), dflt)
		case ParamSpecParam:
			call = typingCall(p.Pos, "_gpy_paramspec", strExpr(p.Pos, string(p.Name)), dflt)
		}
		out = append(out, &Assign{StmtBase: StmtBase{Pos: p.Pos}, Targets: []Expr{nameExpr(p.Pos, string(p.Name), Store)}, Value: call})
	}
	return out
}

func paramsTuple(pos Pos, params []*TypeParam) Expr {
	elts := make([]Expr, len(params))
	for i, p := range params {
		elts[i] = nameExpr(pos, string(p.Name), Load)
	}
	return &Tuple{ExprBase: ExprBase{Pos: pos}, Elts: elts, Ctx: Load}
}

const hiddenBinding = "<generic>"

// generic wraps a generic function or class definition.
func (d *desugarer) generic(def Stmt, name Identifier, params *[]*TypeParam, decorators []Expr, decoratorList *[]Expr, store *Identifier) []Stmt {
	pos := Pos{Lineno: def.GetLineno(), ColOffset: def.GetColOffset()}
	tparams := *params
	*params = nil
	*decoratorList = nil
	*store = hiddenBinding

	body := d.typeParamStmts(tparams)
	switch n := def.(type) {
	case *ClassDef:
		n.Bases = append(n.Bases, typingCall(pos, "_gpy_generic", paramsTuple(pos, tparams)))
		n.Body = append([]Stmt{&Assign{StmtBase: StmtBase{Pos: pos}, Targets: []Expr{nameExpr(pos, "__type_params__", Store)}, Value: paramsTuple(pos, tparams)}}, n.Body...)
		body = append(body, def)
	case *FunctionDef:
		body = append(body, def)
		body = append(body, &Assign{
			StmtBase: StmtBase{Pos: pos},
			Targets:  []Expr{&Attribute{ExprBase: ExprBase{Pos: pos}, Value: nameExpr(pos, hiddenBinding, Load), Attr: "__type_params__", Ctx: Store}},
			Value:    paramsTuple(pos, tparams),
		})
	}
	body = append(body, &Return{StmtBase: StmtBase{Pos: pos}, Value: nameExpr(pos, hiddenBinding, Load)})

	scope := genericPrefix + string(name) + ">"
	fn := &FunctionDef{StmtBase: StmtBase{Pos: pos}, Name: Identifier(scope), Args: &Arguments{Pos: pos}, Body: body}
	var call Expr = &Call{ExprBase: ExprBase{Pos: pos}, Func: nameExpr(pos, scope, Load)}
	for i := len(decorators) - 1; i >= 0; i-- {
		call = &Call{ExprBase: ExprBase{Pos: pos}, Func: decorators[i], Args: []Expr{call}}
	}
	return []Stmt{
		fn,
		&Assign{StmtBase: StmtBase{Pos: pos}, Targets: []Expr{nameExpr(pos, string(name), Store)}, Value: call},
		&Delete{StmtBase: StmtBase{Pos: pos}, Targets: []Expr{nameExpr(pos, scope, Del)}},
	}
}

// typeAlias rewrites "type X[T] = value".
func (d *desugarer) typeAlias(n *TypeAlias) []Stmt {
	pos := Pos{Lineno: n.GetLineno(), ColOffset: n.GetColOffset()}
	name := string(n.Name.Id)
	mk := func(params Expr) Expr {
		return typingCall(pos, "_gpy_alias", strExpr(pos, name), thunk(pos, n.Value), params, nameExpr(pos, "__name__", Load))
	}
	if len(n.TypeParams) == 0 {
		return []Stmt{&Assign{StmtBase: StmtBase{Pos: pos}, Targets: []Expr{nameExpr(pos, name, Store)}, Value: mk(&Tuple{ExprBase: ExprBase{Pos: pos}, Ctx: Load})}}
	}
	body := d.typeParamStmts(n.TypeParams)
	body = append(body, &Return{StmtBase: StmtBase{Pos: pos}, Value: mk(paramsTuple(pos, n.TypeParams))})
	scope := fmt.Sprintf("%s%s>", genericPrefix, name)
	return []Stmt{
		&FunctionDef{StmtBase: StmtBase{Pos: pos}, Name: Identifier(scope), Args: &Arguments{Pos: pos}, Body: body},
		&Assign{StmtBase: StmtBase{Pos: pos}, Targets: []Expr{nameExpr(pos, name, Store)}, Value: &Call{ExprBase: ExprBase{Pos: pos}, Func: nameExpr(pos, scope, Load)}},
		&Delete{StmtBase: StmtBase{Pos: pos}, Targets: []Expr{nameExpr(pos, scope, Del)}},
	}
}

// NameExpr, StrExpr and TypingCall build the small trees the lowering passes
// (here and in the compiler's annotation pass) are made of.
func NameExpr(pos Pos, id string, ctx ExprContext) Expr { return nameExpr(pos, id, ctx) }
func StrExpr(pos Pos, s string) Expr                    { return strExpr(pos, s) }
func TypingCall(pos Pos, fn string, args ...Expr) Expr  { return typingCall(pos, fn, args...) }
