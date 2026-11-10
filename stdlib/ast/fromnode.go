package ast

import (
	"reflect"

	pyast "github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/py"
)

var astIface = reflect.TypeOf((*pyast.Ast)(nil)).Elem()

// ToCompiler turns a tree of Python AST nodes back into the compiler's, for
// compile(tree).  A node that ast.parse made is rebuilt field by field from
// its current attributes, so a NodeTransformer's edits are honoured; the
// Module, Interactive and Expression wrappers may also be built by hand.  A
// node class the parser never produces (and so has no compiler counterpart
// to copy) is refused.
func ToCompiler(obj py.Object, mode py.CompileMode) (pyast.Ast, error) {
	n, err := nodeDict(obj)
	if err != nil {
		return nil, err
	}
	switch name := obj.Type().Name; name {
	case "Module", "Interactive":
		body, err := toGoValue(n.GetOrNil("body"), reflect.TypeOf([]pyast.Stmt(nil)))
		if err != nil {
			return nil, err
		}
		if name == "Interactive" {
			return &pyast.Interactive{Body: body.Interface().([]pyast.Stmt)}, nil
		}
		return &pyast.Module{Body: body.Interface().([]pyast.Stmt)}, nil
	case "Expression":
		body, err := toGoValue(n.GetOrNil("body"), reflect.TypeOf((*pyast.Expr)(nil)).Elem())
		if err != nil {
			return nil, err
		}
		return &pyast.Expression{Body: body.Interface().(pyast.Expr)}, nil
	}
	return nil, py.ExceptionNewf(py.TypeError, "expected Module, Interactive or Expression node, got %s", obj.Type().Name)
}

func nodeDict(obj py.Object) (py.StringDict, error) {
	h, ok := obj.(py.IGetDict)
	if !ok {
		return py.StringDict{}, py.ExceptionNewf(py.TypeError, "expected an AST node, got %s", obj.Type().Name)
	}
	return h.GetDict(), nil
}

// toGoValue makes a Go value of type want from a Python attribute value.
func toGoValue(v py.Object, want reflect.Type) (reflect.Value, error) {
	bad := func() (reflect.Value, error) {
		return reflect.Value{}, py.ExceptionNewf(py.TypeError, "unexpected %s in an AST field of type %s", typeName(v), want.Name())
	}
	if v == nil {
		return reflect.Zero(want), nil
	}
	switch want.Kind() {
	case reflect.Interface:
		if v == py.None {
			return reflect.Zero(want), nil
		}
		if want.Implements(astIface) {
			n, ok := v.(*Node)
			if !ok {
				return bad()
			}
			a, err := fromNode(n)
			if err != nil {
				return reflect.Value{}, err
			}
			av := reflect.ValueOf(a)
			if !av.Type().Implements(want) {
				return bad()
			}
			return av, nil
		}
		// Object or Singleton: a plain Python value.
		return reflect.ValueOf(v), nil
	case reflect.Ptr:
		if v == py.None {
			return reflect.Zero(want), nil
		}
		n, ok := v.(*Node)
		if !ok {
			return bad()
		}
		a, err := fromNode(n)
		if err != nil {
			return reflect.Value{}, err
		}
		av := reflect.ValueOf(a)
		if av.Type() != want {
			return bad()
		}
		return av, nil
	case reflect.Struct:
		n, ok := v.(*Node)
		if !ok {
			return bad()
		}
		a, err := fromNode(n)
		if err != nil {
			return reflect.Value{}, err
		}
		av := reflect.ValueOf(a)
		if av.Kind() != reflect.Ptr || av.Elem().Type() != want {
			return bad()
		}
		return av.Elem(), nil
	case reflect.Slice:
		if want.Elem().Kind() == reflect.Uint8 {
			b, ok := v.(py.Bytes)
			if !ok {
				return bad()
			}
			return reflect.ValueOf([]byte(b)), nil
		}
		if v == py.None {
			return reflect.Zero(want), nil
		}
		l, ok := v.(*py.List)
		if !ok {
			return bad()
		}
		out := reflect.MakeSlice(want, 0, len(l.Items))
		for _, it := range l.Items {
			e, err := toGoValue(it, want.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			out = reflect.Append(out, e)
		}
		return out, nil
	case reflect.String:
		s, ok := v.(py.String)
		if !ok {
			if v == py.None {
				return reflect.Zero(want), nil
			}
			return bad()
		}
		return reflect.ValueOf(string(s)).Convert(want), nil
	case reflect.Bool:
		return reflect.ValueOf(v == py.True).Convert(want), nil
	case reflect.Int, reflect.Int64:
		if n, ok := v.(*Node); ok {
			// Load(), Add(), Eq() ...: the Go constants are matched by name.
			for i := 0; i < 64; i++ {
				c := reflect.New(want).Elem()
				c.SetInt(int64(i))
				if s, ok := c.Interface().(interface{ String() string }); ok && s.String() == n.cls.Name+"()" {
					return c, nil
				}
			}
			return bad()
		}
		if i, ok := v.(py.Int); ok {
			return reflect.ValueOf(int64(i)).Convert(want), nil
		}
		return bad()
	}
	return bad()
}

func typeName(v py.Object) string {
	if v == nil {
		return "nothing"
	}
	return v.Type().Name
}

// fromNode rebuilds the compiler node for one Python node.
func fromNode(n *Node) (pyast.Ast, error) {
	if n.Orig == nil {
		return nil, py.ExceptionNewf(py.TypeError, "cannot compile a hand-built %s node: only nodes made by ast.parse are supported", n.cls.Name)
	}
	if n.cls.Name == "Constant" {
		return constantNode(n)
	}
	orig := reflect.ValueOf(n.Orig).Elem()
	cp := reflect.New(orig.Type())
	cp.Elem().Set(orig)
	t := orig.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous || !f.IsExported() || f.Name == "IsAsync" || f.Name == "StoreName" {
			continue
		}
		val, ok := n.Dict.Get(snake(f.Name))
		if !ok {
			continue
		}
		if a, isArgs := n.Orig.(*pyast.Arguments); isArgs && f.Name == "Args" {
			// Python's arguments.args leaves out the positional-only ones.
			po, err := toGoValue(n.Dict.GetOrNil("posonlyargs"), reflect.TypeOf([]*pyast.Arg(nil)))
			if err != nil {
				return nil, err
			}
			rest, err := toGoValue(val, f.Type)
			if err != nil {
				return nil, err
			}
			cp.Elem().Field(i).Set(reflect.AppendSlice(po, rest))
			_ = a
			continue
		}
		c, err := toGoValue(val, f.Type)
		if err != nil {
			return nil, err
		}
		cp.Elem().Field(i).Set(c)
	}
	res := cp.Interface().(pyast.Ast)
	pos := pyast.Pos{}
	pos.Lineno = intAttr(n.Dict, "lineno", n.Orig.GetLineno())
	pos.ColOffset = intAttr(n.Dict, "col_offset", n.Orig.GetColOffset())
	pos.EndLineno = intAttr(n.Dict, "end_lineno", n.Orig.GetEndLineno())
	pos.EndColOffset = intAttr(n.Dict, "end_col_offset", n.Orig.GetEndColOffset())
	res.SetPosition(pos)
	return res, nil
}

func intAttr(d py.StringDict, name string, def int) int {
	if i, ok := d.GetOrNil(name).(py.Int); ok {
		return int(i)
	}
	return def
}

// constantNode picks the compiler's literal node for the current value.
func constantNode(n *Node) (pyast.Ast, error) {
	var res pyast.Ast
	switch v := n.Dict.GetOrNil("value").(type) {
	case py.Int, py.Float, py.Complex:
		res = &pyast.Num{N: v}
	case *py.BigInt:
		res = &pyast.Num{N: v}
	case py.String:
		res = &pyast.Str{S: v}
	case py.Bytes:
		res = &pyast.Bytes{S: v}
	default:
		if v == py.Ellipsis {
			res = &pyast.Ellipsis{}
		} else if v == py.None || v == py.True || v == py.False {
			res = &pyast.NameConstant{Value: v}
		} else {
			return nil, py.ExceptionNewf(py.TypeError, "got an invalid type in Constant: %s", typeName(v))
		}
	}
	pos := pyast.Pos{
		Lineno:       intAttr(n.Dict, "lineno", n.Orig.GetLineno()),
		ColOffset:    intAttr(n.Dict, "col_offset", n.Orig.GetColOffset()),
		EndLineno:    intAttr(n.Dict, "end_lineno", n.Orig.GetEndLineno()),
		EndColOffset: intAttr(n.Dict, "end_col_offset", n.Orig.GetEndColOffset()),
	}
	res.SetPosition(pos)
	return res, nil
}
