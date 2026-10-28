package typing

import (
	"github.com/vishnukv64/gpython/py"
)

// A protocol class is one that derives from Protocol directly; those marked
// with @runtime_checkable answer isinstance() structurally: the object must
// have every member the protocol (and its protocol bases) declares.

const runtimeProtocolFlag = "_is_runtime_protocol"

func isProtocol(t *py.Type) bool {
	for _, b := range t.Bases {
		if b == py.Object(ProtocolType) {
			return true
		}
	}
	return false
}

// protocolSkip is typing._SPECIAL_NAMES plus the bookkeeping this module adds.
var protocolSkip = map[string]bool{
	"__abstractmethods__": true, "__annotations__": true, "__annotate__": true,
	"__annotate_func__": true, "__annotations_cache__": true, "__dict__": true,
	"__doc__": true, "__init__": true, "__module__": true, "__new__": true,
	"__slots__": true, "__subclasshook__": true, "__weakref__": true,
	"__class_getitem__": true, "__match_args__": true, "__static_attributes__": true,
	"__firstlineno__": true, "__orig_bases__": true, "__parameters__": true,
	"__qualname__": true, "__protocol_attrs__": true, "__non_callable_proto_members__": true,
	"_is_protocol": true, runtimeProtocolFlag: true, "__hash__": true,
}

// protocolMembers lists the names a protocol requires, in no useful order.
func protocolMembers(t *py.Type) []string {
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] && !protocolSkip[n] && !(len(n) > 5 && n[:5] == "_abc_") {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, b := range t.Mro {
		bt := b.(*py.Type)
		if bt == ProtocolType || bt == GenericType || bt == py.ObjectType || !isProtocol(bt) {
			continue
		}
		for _, k := range bt.Dict.Keys() {
			add(k)
		}
		if ann, ok := bt.Dict.Get("__annotations__"); ok {
			if d, ok := ann.(py.StringDict); ok {
				for _, k := range d.Keys() {
					add(k)
				}
			}
		}
	}
	return out
}

func protocolIsInstance(obj py.Object, class *py.Type) bool {
	if class == nil || class.Dict.IsNil() {
		return false
	}
	if _, ok := class.Dict.Get(runtimeProtocolFlag); !ok || !isProtocol(class) {
		return false
	}
	for _, name := range protocolMembers(class) {
		if _, err := py.GetAttrString(obj, name); err != nil {
			return false
		}
	}
	return true
}

func runtimeCheckable(self py.Object, args py.Tuple) (py.Object, error) {
	var cls py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "runtime_checkable", 1, 1, &cls); err != nil {
		return nil, err
	}
	t, ok := cls.(*py.Type)
	if !ok || !isProtocol(t) {
		return nil, py.ExceptionNewf(py.TypeError, "@runtime_checkable can be only applied to protocol classes, got %v", cls)
	}
	t.Dict.Set(runtimeProtocolFlag, py.True)
	return cls, nil
}

func init() {
	py.ABCHooks = append(py.ABCHooks, protocolIsInstance)
}
