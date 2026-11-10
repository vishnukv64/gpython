package py

import "strings"

// LookupMethod returns the plain Python function that "self.key" would bind,
// or nil when the ordinary attribute lookup could yield anything else.  It lets
// the VM call "self.key(...)" without building a BoundMethod.  It answers only
// for an instance of a Python class, for a name that is not a dunder, when
// nothing in the instance dict shadows it and __getattribute__ is the default;
// all of these mirror the order of getAttrString.
func LookupMethod(self Object, key string) *Function {
	ty, ok := self.(*Type)
	if !ok || ty.Name != "" || ty.ObjectType == nil {
		return nil
	}
	if len(key) >= 5 && strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
		return nil
	}
	t := ty.ObjectType
	if t.Lookup("__getattribute__") != Object(objectGetattribute) {
		return nil
	}
	if _, found := ty.Dict.Get(key); found {
		return nil
	}
	f, _ := t.Lookup(key).(*Function)
	return f
}
