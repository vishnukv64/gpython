package string

import (
	"reflect"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// Template strings (PEP 750): the Template and Interpolation types that a
// t"..." literal produces, as CPython's _string module / templateobject.c.
// string.templatelib (embedded, CPython's own) obtains them with type(t"").
// The compiler lowers t"..." to _gpy_template(strings, interpolations).

var (
	// InterpolationType is string.templatelib.Interpolation.
	InterpolationType = py.NewTypeX("string.templatelib.Interpolation", "Interpolation object", interpolationNew, nil)
	// TemplateObjType is string.templatelib.Template.
	TemplateObjType = py.NewTypeX("string.templatelib.Template", "Template object", tmplNew, nil)
)

func init() {
	InterpolationType.Dict.Set("__match_args__", py.Tuple{py.String("value"), py.String("expression"), py.String("conversion"), py.String("format_spec")})
}

// Interpolation is the result of one {field} of a t-string.
type Interpolation struct {
	value      py.Object
	expression py.String
	conversion py.Object // None or 's', 'r', 'a'
	formatSpec py.String
}

// TemplateObj is a t-string value.
type TemplateObj struct {
	strings        py.Tuple // always len(interpolations)+1
	interpolations py.Tuple
}

func (i *Interpolation) Type() *py.Type { return InterpolationType }
func (t *TemplateObj) Type() *py.Type   { return TemplateObjType }

func defaultAttr(self py.Object, name string) (py.Object, error) {
	return py.ObjectGetAttribute(self, name)
}

func (i *Interpolation) M__getattribute__(name string) (py.Object, error) {
	switch name {
	case "value":
		return i.value, nil
	case "expression":
		return i.expression, nil
	case "conversion":
		return i.conversion, nil
	case "format_spec":
		return i.formatSpec, nil
	}
	return defaultAttr(i, name)
}

func (i *Interpolation) M__repr__() (py.Object, error) {
	v, err := py.Repr(i.value)
	if err != nil {
		return nil, err
	}
	e, _ := py.Repr(i.expression)
	c, _ := py.Repr(i.conversion)
	f, _ := py.Repr(i.formatSpec)
	return py.String("Interpolation(" + string(v.(py.String)) + ", " + string(e.(py.String)) + ", " + string(c.(py.String)) + ", " + string(f.(py.String)) + ")"), nil
}

func interpolationNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var value, expr, conv, spec py.Object
	expr, conv, spec = py.String(""), py.None, py.String("")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOO:Interpolation", []string{"value", "expression", "conversion", "format_spec"}, &value, &expr, &conv, &spec); err != nil {
		return nil, err
	}
	e, ok := expr.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "Interpolation() argument 'expression' must be str, not %s", expr.Type().Name)
	}
	f, ok := spec.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "Interpolation() argument 'format_spec' must be str, not %s", spec.Type().Name)
	}
	if conv != py.None {
		s, ok := conv.(py.String)
		if !ok || (s != "s" && s != "r" && s != "a") {
			return nil, py.ExceptionNewf(py.ValueError, "Interpolation() argument 'conversion' must be one of 's', 'a' or 'r'")
		}
	}
	return &Interpolation{value: value, expression: e, conversion: conv, formatSpec: f}, nil
}

func newTemplate(strs []string, interps []py.Object) *TemplateObj {
	s := make(py.Tuple, len(strs))
	for i, x := range strs {
		s[i] = py.String(x)
	}
	return &TemplateObj{strings: s, interpolations: py.Tuple(interps)}
}

func (t *TemplateObj) M__getattribute__(name string) (py.Object, error) {
	switch name {
	case "strings":
		return t.strings, nil
	case "interpolations":
		return t.interpolations, nil
	case "values":
		vals := make(py.Tuple, len(t.interpolations))
		for i, it := range t.interpolations {
			vals[i] = it.(*Interpolation).value
		}
		return vals, nil
	}
	return defaultAttr(t, name)
}

func (t *TemplateObj) M__repr__() (py.Object, error) {
	s, err := py.Repr(t.strings)
	if err != nil {
		return nil, err
	}
	i, err := py.Repr(t.interpolations)
	if err != nil {
		return nil, err
	}
	return py.String("Template(strings=" + string(s.(py.String)) + ", interpolations=" + string(i.(py.String)) + ")"), nil
}

// M__iter__ yields the non-empty strings and the interpolations, in order.
func (t *TemplateObj) M__iter__() (py.Object, error) {
	var items py.Tuple
	for i, s := range t.strings {
		if s.(py.String) != "" {
			items = append(items, s)
		}
		if i < len(t.interpolations) {
			items = append(items, t.interpolations[i])
		}
	}
	return py.NewIterator(items), nil
}

func (t *TemplateObj) M__add__(other py.Object) (py.Object, error) {
	o, ok := other.(*TemplateObj)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "can only concatenate string.templatelib.Template (not \"%s\") to string.templatelib.Template", other.Type().Name)
	}
	strs := make(py.Tuple, 0, len(t.strings)+len(o.strings)-1)
	strs = append(strs, t.strings[:len(t.strings)-1]...)
	strs = append(strs, t.strings[len(t.strings)-1].(py.String)+o.strings[0].(py.String))
	strs = append(strs, o.strings[1:]...)
	interps := append(append(py.Tuple{}, t.interpolations...), o.interpolations...)
	return &TemplateObj{strings: strs, interpolations: interps}, nil
}

func tmplNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if kwargs.Len() > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "Template.__new__ only accepts *args arguments")
	}
	var strs []string
	var interps []py.Object
	var cur strings.Builder
	for _, a := range args {
		switch v := a.(type) {
		case py.String:
			cur.WriteString(string(v))
		case *Interpolation:
			strs = append(strs, cur.String())
			cur.Reset()
			interps = append(interps, v)
		default:
			return nil, py.ExceptionNewf(py.TypeError, "Template.__new__ *args need to be of type 'str' or 'Interpolation', got %s", a.Type().Name)
		}
	}
	strs = append(strs, cur.String())
	return newTemplate(strs, interps), nil
}

// gpyTemplate is the lowering target of a t-string literal: strings is the
// tuple of literal parts and interps the tuple of Interpolation objects.
func gpyTemplate(self py.Object, args py.Tuple) (py.Object, error) {
	var strs, interps py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "_gpy_template", 2, 2, &strs, &interps); err != nil {
		return nil, err
	}
	return &TemplateObj{strings: strs.(py.Tuple), interpolations: interps.(py.Tuple)}, nil
}

// gpyInterp builds the Interpolation of one field: (value, expression,
// conversion, format_spec).
func gpyInterp(self py.Object, args py.Tuple) (py.Object, error) {
	var v, e, c, f py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "_gpy_interp", 4, 4, &v, &e, &c, &f); err != nil {
		return nil, err
	}
	return &Interpolation{value: v, expression: e.(py.String), conversion: c, formatSpec: f.(py.String)}, nil
}

func (i *Interpolation) M__hash__() (py.Object, error) { return py.Int(identity(i)), nil }
func (t *TemplateObj) M__hash__() (py.Object, error)   { return py.Int(identity(t)), nil }

func (i *Interpolation) M__eq__(o py.Object) (py.Object, error) {
	return py.NewBool(py.Object(i) == o), nil
}
func (t *TemplateObj) M__eq__(o py.Object) (py.Object, error) {
	return py.NewBool(py.Object(t) == o), nil
}

func identity(o py.Object) int64 {
	return int64(reflect.ValueOf(o).Pointer())
}
