package ast

import (
	"reflect"
	"strings"
	"sync"

	pyast "github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/parser"
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/tokenizemod"
)

// Node is the Python-visible form of a compiler AST node: an instance of a
// class named as in CPython's ast module, whose fields are plain attributes.
// Orig keeps the compiler node so literal_eval can still take a parsed tree.
type Node struct {
	cls  *py.Type
	Dict py.StringDict
	Orig pyast.Ast
}

func (n *Node) Type() *py.Type         { return n.cls }
func (n *Node) GetDict() py.StringDict { return n.Dict }

var (
	classMu sync.Mutex
	classes = map[string]*py.Type{}
	// bases of the abstract node classes, by CPython name
	abstract = map[string]string{"pattern": "AST", "type_param": "AST", "type_ignore": "AST", "mod": "AST", "stmt": "AST", "expr": "AST", "expr_context": "AST",
		"boolop": "AST", "operator": "AST", "unaryop": "AST", "cmpop": "AST", "excepthandler": "AST"}
	ctxNames = map[string]string{"Load": "expr_context", "Store": "expr_context", "Del": "expr_context",
		"And": "boolop", "Or": "boolop", "Invert": "unaryop", "Not": "unaryop", "UAdd": "unaryop", "USub": "unaryop",
		"Add": "operator", "Sub": "operator", "Mult": "operator", "MatMult": "operator", "Div": "operator", "Mod": "operator",
		"Pow": "operator", "LShift": "operator", "RShift": "operator", "BitOr": "operator", "BitXor": "operator",
		"BitAnd": "operator", "FloorDiv": "operator", "Eq": "cmpop", "NotEq": "cmpop", "Lt": "cmpop", "LtE": "cmpop",
		"Gt": "cmpop", "GtE": "cmpop", "Is": "cmpop", "IsNot": "cmpop", "In": "cmpop", "NotIn": "cmpop"}
)

// classFor returns the Python class for a node name, creating it (and its
// abstract parents) on first use.  kind is "stmt", "expr", "mod" or "" and
// fields the ordered _fields.
func classFor(name, kind string, fields []string) *py.Type {
	abstractOnce.Do(makeAbstract)
	return newClass(name, kind, fields)
}

func newClass(name, kind string, fields []string) *py.Type {
	classMu.Lock()
	defer classMu.Unlock()
	if c, ok := classes[name]; ok {
		setFields(c, fields)
		return c
	}
	var base *py.Type
	switch {
	case name == "AST":
		base = py.ObjectType
	case abstract[name] != "":
		base = classes["AST"]
	default:
		parent := kind
		if p, ok := ctxNames[name]; ok {
			parent = p
		}
		if parent == "" {
			parent = "AST"
		}
		base = classes[parent]
	}
	c := base.NewType(name, name+" node", nil, nil)
	c.Dict.Set("_fields", py.Tuple{})
	attrs := py.Tuple{}
	switch {
	case kind == "stmt" || kind == "expr" || kind == "excepthandler" || kind == "pattern" || kind == "type_param",
		name == "arg" || name == "keyword" || name == "alias":
		attrs = py.Tuple{py.String("lineno"), py.String("col_offset"), py.String("end_lineno"), py.String("end_col_offset")}
	}
	c.Dict.Set("_attributes", attrs)
	classes[name] = c
	setFields(c, fields)
	return c
}

func setFields(c *py.Type, fields []string) {
	if len(fields) == 0 {
		return
	}
	t := make(py.Tuple, len(fields))
	for i, f := range fields {
		t[i] = py.String(f)
	}
	c.Dict.Set("_fields", t)
}

// exportedClasses is the module namespace: every node class by name.
func exportedClasses() py.StringDict {
	d := py.NewStringDict()
	stmts := "FunctionDef AsyncFunctionDef ClassDef Return Delete Assign AugAssign AnnAssign For AsyncFor While If With AsyncWith Raise Try Assert Import ImportFrom Global Nonlocal Expr Pass Break Continue Match TryStar TypeAlias"
	exprs := "BoolOp BinOp UnaryOp Lambda IfExp NamedExpr Dict Set ListComp SetComp DictComp GeneratorExp Yield YieldFrom Await Compare Call Constant JoinedStr Attribute Subscript Starred Name List Tuple Slice"
	for kind, names := range map[string]string{"stmt": stmts, "expr": exprs, "mod": "Module Interactive Expression",
		"": "comprehension arguments arg keyword alias withitem Load Store Del And Or Invert Not UAdd USub Add Sub Mult MatMult Div Mod Pow LShift RShift BitOr BitXor BitAnd FloorDiv Eq NotEq Lt LtE Gt GtE Is IsNot In NotIn"} {
		for _, n := range strings.Fields(names) {
			classFor(n, kind, nil)
		}
	}
	classFor("ExceptHandler", "excepthandler", nil)
	// Node kinds the parser never produces here, present so that code naming
	// them (executing, ast.unparse users) imports.
	for _, n := range strings.Fields("FormattedValue Interpolation TemplateStr Index ExtSlice Suite") {
		classFor(n, "expr", nil)
	}
	for _, n := range strings.Fields("MatchValue MatchSingleton MatchSequence MatchMapping MatchClass MatchStar MatchAs MatchOr") {
		classFor(n, "pattern", nil)
	}
	for _, n := range strings.Fields("TypeVar ParamSpec TypeVarTuple") {
		classFor(n, "type_param", nil)
	}
	classFor("TypeIgnore", "type_ignore", nil)
	classFor("match_case", "", nil)
	// The module wrappers are built by hand (ast.Module([node], [])).
	classFor("Module", "mod", []string{"body", "type_ignores"})
	classFor("Interactive", "mod", []string{"body"})
	classFor("Expression", "mod", []string{"body"})
	classMu.Lock()
	classes["AST"].Dict.Set("__init__", py.MustNewMethod("__init__", astInit, 0, "Set the fields from positional and keyword arguments."))
	classMu.Unlock()
	classMu.Lock()
	defer classMu.Unlock()
	for n, c := range classes {
		d.Set(n, c)
	}
	// compile() flag values, as in CPython 3.14.
	d.Set("PyCF_ONLY_AST", py.Int(0x400))
	d.Set("PyCF_TYPE_COMMENTS", py.Int(0x1000))
	d.Set("PyCF_ALLOW_TOP_LEVEL_AWAIT", py.Int(0x2000))
	d.Set("PyCF_OPTIMIZED_AST", py.Int(0x8400))
	return d
}

// astInit is ast.AST.__init__: positional arguments fill _fields in order,
// keywords set attributes.
func astInit(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d, ok := self.(py.IGetDict)
	if !ok {
		return py.None, nil
	}
	fields, _ := self.Type().Lookup("_fields").(py.Tuple)
	if len(args) > len(fields) {
		return nil, py.ExceptionNewf(py.TypeError, "%s constructor takes at most %d positional arguments", self.Type().Name, len(fields))
	}
	dict := d.GetDict()
	for i, a := range args {
		name, _ := py.StrAsString(fields[i])
		dict.Set(name, a)
	}
	for _, k := range kwargs.Keys() {
		dict.Set(k, kwargs.GetOrNil(k))
	}
	return py.None, nil
}

// IsNode reports whether obj is an instance of ast.AST.
func IsNode(obj py.Object) bool {
	classMu.Lock()
	base := classes["AST"]
	classMu.Unlock()
	return base != nil && obj.Type().IsSubtype(base)
}

// abstractOnce creates the abstract classes before any concrete one: package
// init order puts ast.go's registration ahead of this file.
var abstractOnce sync.Once

func makeAbstract() {
	for _, n := range []string{"AST", "pattern", "type_param", "type_ignore", "mod", "stmt", "expr", "expr_context", "boolop", "operator", "unaryop", "cmpop", "excepthandler"} {
		newClass(n, "", nil)
	}
}

// snake turns a compiler field name into CPython's.
func snake(s string) string {
	switch s {
	case "AsName":
		return "asname"
	case "ExprType":
		return "type"
	}
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pyName is CPython's node name for a compiler node.
func pyName(n pyast.Ast) string {
	v := reflect.ValueOf(n).Elem()
	name := v.Type().Name()
	async := false
	if f := v.FieldByName("IsAsync"); f.IsValid() {
		async = f.Bool()
	}
	switch name {
	case "ExprStmt":
		return "Expr"
	case "Num", "Str", "Bytes", "NameConstant", "Ellipsis":
		return "Constant"
	case "FString":
		return "JoinedStr"
	case "MatchStmt":
		return "Match"
	case "FunctionDef", "For", "With":
		if async {
			return "Async" + name
		}
	case "Comprehension":
		return "comprehension"
	case "Arguments":
		return "arguments"
	case "Arg":
		return "arg"
	case "Keyword":
		return "keyword"
	case "Alias":
		return "alias"
	case "WithItem":
		return "withitem"
	case "ExceptHandler":
		return "ExceptHandler"
	}
	return name
}

// toPy converts a compiler AST value to Python objects.
func toPy(v reflect.Value, ends map[int]int) py.Object {
	switch v.Kind() {
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return py.None
		}
		if v.Kind() == reflect.Interface {
			return toPy(v.Elem(), ends)
		}
		if n, ok := v.Interface().(pyast.Ast); ok {
			return nodeFor(n, ends)
		}
		if o, ok := v.Interface().(py.Object); ok {
			return o
		}
		return py.None
	case reflect.Struct: // Comprehension is held by value
		if v.CanAddr() {
			if n, ok := v.Addr().Interface().(pyast.Ast); ok {
				return nodeFor(n, ends)
			}
		}
		cp := reflect.New(v.Type())
		cp.Elem().Set(v)
		return toPy(cp, ends)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return py.Bytes(v.Bytes())
		}
		l := py.NewListSized(v.Len())
		for i := 0; i < v.Len(); i++ {
			l.Items[i] = toPy(v.Index(i), ends)
		}
		return l
	case reflect.String:
		return py.String(v.String())
	case reflect.Bool:
		return py.NewBool(v.Bool())
	case reflect.Int, reflect.Int64:
		if s, ok := v.Interface().(interface{ String() string }); ok { // Load(), Add(), ...
			name := strings.TrimSuffix(s.String(), "()")
			return &Node{cls: classFor(name, "expr_context", nil), Dict: py.NewStringDict()}
		}
		return py.Int(v.Int())
	}
	return py.None
}

func nodeFor(n pyast.Ast, ends map[int]int) py.Object {
	v := reflect.ValueOf(n).Elem()
	t := v.Type()
	name := pyName(n)
	node := &Node{Dict: py.NewStringDict(), Orig: n}
	var fields []string
	kind := ""
	switch n.(type) {
	case pyast.Mod:
		kind = "mod"
	case pyast.Stmt:
		kind = "stmt"
	case pyast.Expr, pyast.Slicer:
		kind = "expr"
	}
	switch name {
	case "ExceptHandler":
		kind = "excepthandler"
	case "comprehension", "arguments", "arg", "keyword", "alias", "withitem":
		kind = ""
	}
	if name == "Constant" {
		fields = []string{"value", "kind"}
		var val py.Object = py.None
		switch c := n.(type) {
		case *pyast.Num:
			val = c.N
		case *pyast.Str:
			val = c.S
		case *pyast.Bytes:
			val = c.S
		case *pyast.NameConstant:
			val = c.Value
		case *pyast.Ellipsis:
			val = py.Ellipsis
		}
		node.Dict.Set("value", val)
		node.Dict.Set("kind", py.None)
	} else {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous || !f.IsExported() || f.Name == "IsAsync" || f.Name == "StoreName" {
				continue
			}
			py_ := snake(f.Name)
			fields = append(fields, py_)
			fv := v.Field(i)
			if a, ok := n.(*pyast.Arguments); ok && f.Name == "Args" {
				// pyast keeps the positional-only parameters at the head of
				// Args too; CPython's arguments.args does not.
				fv = fv.Slice(len(a.Posonlyargs), fv.Len())
			}
			node.Dict.Set(py_, toPy(fv, ends))
		}
	}
	if name == "Module" {
		fields = append(fields, "type_ignores")
		node.Dict.Set("type_ignores", py.NewList())
	}
	node.cls = classFor(name, kind, fields)
	node.Dict.Set("lineno", py.Int(n.GetLineno()))
	node.Dict.Set("col_offset", py.Int(n.GetColOffset()))
	node.Dict.Set("end_lineno", py.None)
	node.Dict.Set("end_col_offset", py.None)
	if n.GetEndLineno() != 0 {
		node.Dict.Set("end_lineno", py.Int(n.GetEndLineno()))
		node.Dict.Set("end_col_offset", py.Int(n.GetEndColOffset()))
	} else if name == "FunctionDef" || name == "AsyncFunctionDef" || name == "ClassDef" {
		if e, ok := ends[n.GetLineno()]; ok {
			node.Dict.Set("end_lineno", py.Int(e))
		}
	}
	return node
}

// parsedTree converts a parsed tree, taking def/class end lines from src.
func parsedTree(n pyast.Ast, src string) py.Object {
	return nodeFor(n, tokenizemod.BlockEnds(src))
}

// ParseTree is ast.parse for compile(..., PyCF_ONLY_AST).
func ParseTree(src string, mode py.CompileMode) (py.Object, error) {
	node, err := parser.ParseString(src, mode)
	if err != nil {
		return nil, err
	}
	return parsedTree(node, src), nil
}
