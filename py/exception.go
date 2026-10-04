// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Exception objects

package py

import (
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
	"sync"
)

// A python Exception object
type Exception struct {
	Base            *Type
	Args            Object
	Traceback       Object
	Context         Object
	Cause           Object
	SuppressContext bool
	Dict            StringDict // anything else that we want to stuff in
}

// A python exception info block
type ExceptionInfo struct {
	Type      *Type
	Value     Object
	Traceback *Traceback
}

// Make Exception info statisfy the error interface

var (
	// Exception heirachy
	BaseException      = ObjectType.NewTypeFlags("BaseException", "Common base class for all exceptions", ExceptionNew, nil, ObjectType.Flags|TPFLAGS_BASE_EXC_SUBCLASS)
	SystemExit         = BaseException.NewType("SystemExit", "Request to exit from the interpreter.", nil, nil)
	KeyboardInterrupt  = BaseException.NewType("KeyboardInterrupt", "Program interrupted by user.", nil, nil)
	GeneratorExit      = BaseException.NewType("GeneratorExit", "Request that a generator exit.", nil, nil)
	ExceptionType      = BaseException.NewType("Exception", "Common base class for all non-exit exceptions.", nil, nil)
	StopIteration      = ExceptionType.NewType("StopIteration", "Signal the end from iterator.__next__().", nil, nil)
	ArithmeticError    = ExceptionType.NewType("ArithmeticError", "Base class for arithmetic errors.", nil, nil)
	FloatingPointError = ArithmeticError.NewType("FloatingPointError", "Floating point operation failed.", nil, nil)
	OverflowError      = ArithmeticError.NewType("OverflowError", "Result too large to be represented.", nil, nil)
	ZeroDivisionError  = ArithmeticError.NewType("ZeroDivisionError", "Second argument to a division or modulo operation was zero.", nil, nil)
	AssertionError     = ExceptionType.NewType("AssertionError", "Assertion failed.", nil, nil)
	AttributeError     = ExceptionType.NewType("AttributeError", "Attribute not found.", nil, nil)
	BufferError        = ExceptionType.NewType("BufferError", "Buffer error.", nil, nil)
	EOFError           = ExceptionType.NewType("EOFError", "Read beyond end of file.", nil, nil)
	ImportError        = ExceptionType.NewType("ImportError", "Import can't find module, or can't find name in module.", nil, nil)
	// ModuleNotFoundError is the ImportError a failed IMPORT raises, as
	// distinct from one a failed "from ... import ..." raises.  Libraries
	// catch it by name to tell "this optional dependency is absent" from
	// "this dependency is broken", so it has to exist and be an ImportError.
	ModuleNotFoundError       = ImportError.NewType("ModuleNotFoundError", "Module not found.", nil, nil)
	LookupError               = ExceptionType.NewType("LookupError", "Base class for lookup errors.", nil, nil)
	IndexError                = LookupError.NewType("IndexError", "Sequence index out of range.", nil, nil)
	KeyError                  = LookupError.NewType("KeyError", "Mapping key not found.", nil, nil)
	MemoryError               = ExceptionType.NewType("MemoryError", "Out of memory.", nil, nil)
	NameError                 = ExceptionType.NewType("NameError", "Name not found globally.", nil, nil)
	UnboundLocalError         = NameError.NewType("UnboundLocalError", "Local name referenced but not bound to a value.", nil, nil)
	OSError                   = ExceptionType.NewType("OSError", "Base class for I/O related errors.", nil, nil)
	BlockingIOError           = OSError.NewType("BlockingIOError", "I/O operation would block.", nil, nil)
	ChildProcessError         = OSError.NewType("ChildProcessError", "Child process error.", nil, nil)
	ConnectionError           = OSError.NewType("ConnectionError", "Connection error.", nil, nil)
	BrokenPipeError           = ConnectionError.NewType("BrokenPipeError", "Broken pipe.", nil, nil)
	ConnectionAbortedError    = ConnectionError.NewType("ConnectionAbortedError", "Connection aborted.", nil, nil)
	ConnectionRefusedError    = ConnectionError.NewType("ConnectionRefusedError", "Connection refused.", nil, nil)
	ConnectionResetError      = ConnectionError.NewType("ConnectionResetError", "Connection reset.", nil, nil)
	FileExistsError           = OSError.NewType("FileExistsError", "File already exists.", nil, nil)
	FileNotFoundError         = OSError.NewType("FileNotFoundError", "File not found.", nil, nil)
	InterruptedError          = OSError.NewType("InterruptedError", "Interrupted by signal.", nil, nil)
	IsADirectoryError         = OSError.NewType("IsADirectoryError", "Operation doesn't work on directories.", nil, nil)
	NotADirectoryError        = OSError.NewType("NotADirectoryError", "Operation only works on directories.", nil, nil)
	PermissionError           = OSError.NewType("PermissionError", "Not enough permissions.", nil, nil)
	ProcessLookupError        = OSError.NewType("ProcessLookupError", "Process not found.", nil, nil)
	TimeoutError              = OSError.NewType("TimeoutError", "Timeout expired.", nil, nil)
	ReferenceError            = ExceptionType.NewType("ReferenceError", "Weak ref proxy used after referent went away.", nil, nil)
	RuntimeError              = ExceptionType.NewType("RuntimeError", "Unspecified run-time error.", nil, nil)
	NotImplementedError       = RuntimeError.NewType("NotImplementedError", "Method or function hasn't been implemented yet.", nil, nil)
	SyntaxError               = ExceptionType.NewType("SyntaxError", "Invalid syntax.", nil, nil)
	IndentationError          = SyntaxError.NewType("IndentationError", "Improper indentation.", nil, nil)
	TabError                  = IndentationError.NewType("TabError", "Improper mixture of spaces and tabs.", nil, nil)
	SystemError               = ExceptionType.NewType("SystemError", "Internal error in the Gpython interpreter.\n\nPlease report this to the Gpython maintainer, along with the traceback,\nthe Gpython version, and the hardware/OS platform and version.", nil, nil)
	TypeError                 = ExceptionType.NewType("TypeError", "Inappropriate argument type.", nil, nil)
	ValueError                = ExceptionType.NewType("ValueError", "Inappropriate argument value (of correct type).", nil, nil)
	UnicodeError              = ValueError.NewType("UnicodeError", "Unicode related error.", nil, nil)
	UnicodeDecodeError        = UnicodeError.NewType("UnicodeDecodeError", "Unicode decoding error.", nil, nil)
	UnicodeEncodeError        = UnicodeError.NewType("UnicodeEncodeError", "Unicode encoding error.", nil, nil)
	UnicodeTranslateError     = UnicodeError.NewType("UnicodeTranslateError", "Unicode translation error.", nil, nil)
	Warning                   = ExceptionType.NewType("Warning", "Base class for warning categories.", nil, nil)
	DeprecationWarning        = Warning.NewType("DeprecationWarning", "Base class for warnings about deprecated features.", nil, nil)
	PendingDeprecationWarning = Warning.NewType("PendingDeprecationWarning", "Base class for warnings about features which will be deprecated\nin the future.", nil, nil)
	RuntimeWarning            = Warning.NewType("RuntimeWarning", "Base class for warnings about dubious runtime behavior.", nil, nil)
	SyntaxWarning             = Warning.NewType("SyntaxWarning", "Base class for warnings about dubious syntax.", nil, nil)
	UserWarning               = Warning.NewType("UserWarning", "Base class for warnings generated by user code.", nil, nil)
	FutureWarning             = Warning.NewType("FutureWarning", "Base class for warnings about constructs that will change semantically\nin the future.", nil, nil)
	ImportWarning             = Warning.NewType("ImportWarning", "Base class for warnings about probable mistakes in module imports", nil, nil)
	UnicodeWarning            = Warning.NewType("UnicodeWarning", "Base class for warnings about Unicode related problems, mostly\nrelated to conversion problems.", nil, nil)
	BytesWarning              = Warning.NewType("BytesWarning", "Base class for warnings about bytes and buffer related problems, mostly\nrelated to conversion from str or comparing to str.", nil, nil)
	ResourceWarning           = Warning.NewType("ResourceWarning", "Base class for warnings about resource usage.", nil, nil)
	// Singleton exceptions
	NotImplemented Object
)

func init() {
	var err error
	NotImplemented, err = ExceptionNew(NotImplementedError, nil, NewStringDict())
	if err != nil {
		log.Fatalf("Failed to make NotImplemented")
	}
}

// Type of this object
func (e *Exception) Type() *Type {
	return e.Base
}

// Go error interface
func (e *Exception) Error() string {
	// FIXME is this really how exceptions get their message stored?
	// should it be in the dict??
	message := e.Base.Name
	if args, ok := e.Args.(Tuple); ok {
		for i, arg := range args {
			if i == 0 {
				message += ": "
			} else {
				message += ", "
			}
			repr, err := ReprAsString(arg)
			if err == nil {
				message += repr
			} else {
				message += "?"
			}
		}
	}
	// FIXME Print out special stuff for things which look like SyntaxErrors
	if e.Dict.GetOrNil("lineno") != nil {
		message = fmt.Sprintf("\n  File \"%v\", line %v, offset %v\n    %s\n\n", e.Dict.GetOrNil("filename"), e.Dict.GetOrNil("lineno"), e.Dict.GetOrNil("offset"), e.Dict.GetOrNil("line")) + message
	}
	return message
}

// Go error interface
func (e ExceptionInfo) Error() string {
	if e.Value == nil {
		return "ExceptionInfo{<nil>}"
	}
	if exception, ok := e.Value.(*Exception); ok {
		return exception.Error()
	}
	return e.Value.Type().Name
}

// Dump a traceback for exc to w
func (exc *ExceptionInfo) TracebackDump(w io.Writer) {
	if exc == nil {
		fmt.Fprintf(w, "Traceback <nil>\n")
		return
	}
	fmt.Fprintf(w, "Traceback (most recent call last):\n")
	exc.Traceback.TracebackDump(w)
	// The final line is "Name: str(value)", rendered the PYTHON way.  Go's
	// "%v" renders the Go struct instead, which produced
	// "IndexError: 'whatever'" - the Go value carried quotes that CPython's
	// str() does not - and it is the last line of every uncaught traceback.
	name := "Exception"
	if exc.Type != nil {
		name = exc.Type.Name
	}
	if s, err := StrAsString(exc.Value); err == nil && s != "" {
		fmt.Fprintf(w, "%s: %s\n", name, s)
		return
	}
	fmt.Fprintf(w, "%s\n", name)
}

// Test for being set
func (exc *ExceptionInfo) IsSet() bool {
	return exc.Type != nil
}

// exceptionNew
func exceptionNew(metatype *Type, args Tuple) *Exception {
	return &Exception{
		Base: metatype,
		Args: args.Copy(),
		Dict: NewStringDict(),
	}
}

// ExceptionNew
func ExceptionNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() != 0 {
		// FIXME this causes an initialization loop
		// return nil, ExceptionNewf(TypeError, "%s does not take keyword arguments", metatype.Name)
		return nil, fmt.Errorf("TypeError: %s does not take keyword arguments", metatype.Name)
	}
	return exceptionNew(metatype, args), nil
}

// baseExceptionInit registers BaseException.__init__, which stores its
// positional arguments in .args.  It exists so that a subclass whose own
// __init__ chains with super().__init__(...) finds a real initializer in the
// MRO; without it the lookup reached object.__init__, which accepts no
// arguments, and "super().__init__(*a)" failed.
func init() {
	BaseException.Dict.Set("__init__", MustNewMethod("__init__", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		if kwargs.Len() != 0 {
			return nil, ExceptionNewf(TypeError, "%s does not take keyword arguments", self.Type().Name)
		}
		if e, ok := self.(*Exception); ok {
			e.Args = args
			return None, nil
		}
		// An instance of a python subclass of Exception is a *Type here
		// rather than a *Exception, so .args is kept in its dict.  CPython
		// stores the arguments tuple on every exception instance.
		if d, ok := self.(IGetDict); ok && !reflect.ValueOf(d).IsNil() {
			dict := d.GetDict()
			if !dict.IsNil() {
				dict.Set("args", args)
			}
		}
		return None, nil
	}, 0, "Initialize self.  See help(type(self)) for accurate signature."))
}

// ExceptionNewf - make a new exception with fmt parameters
func ExceptionNewf(metatype *Type, format string, a ...interface{}) *Exception {
	message := fmt.Sprintf(format, a...)
	return &Exception{
		Base: metatype,
		Args: Tuple{String(message)},
		Dict: NewStringDict(),
	}
}

/*
	if py.ExceptionClassCheck(exc) {
		t = exc.(*py.Type)
		value = py.Call(exc, nil, nil)
		if value == nil {
			return exitException
		}
		if !py.ExceptionInstanceCheck(value) {
			PyErr_Format(PyExc_TypeError, "calling %s should have returned an instance of BaseException, not %s", t.Name, value.Type().Name)
			return exitException
		}
	} else if t = py.ExceptionInstanceCheck(exc); t != nil {
		value = exc
	} else {
		// Not something you can raise.  You get an exception
		// anyway, just not what you specified :-)
		PyErr_SetString(PyExc_TypeError, "exceptions must derive from BaseException")
		return exitException
	}
*/

// Coerce an object into an exception instance one way or another
func MakeException(r interface{}) *Exception {
	switch x := r.(type) {
	case *Exception:
		return x
	case *Type:
		if x.Flags&TPFLAGS_BASE_EXC_SUBCLASS != 0 {
			return exceptionNew(x, nil)
		}
		// An INSTANCE of a python class is represented as a *Type with an
		// empty Name, so "raise MyErr('x')" arrives here rather than as an
		// *Exception.  Its class is the one that must derive from
		// BaseException, and it already holds the arguments.
		if x.Name == "" {
			if cls := x.Type(); cls != nil && cls.Flags&TPFLAGS_BASE_EXC_SUBCLASS != 0 {
				// Carry the arguments the instance was built with, so
				// str(e) and e.args behave as they do for a builtin.
				args := Tuple{}
				if v, ok := x.Dict.Get("args"); ok {
					if t, ok := v.(Tuple); ok {
						args = t
					}
				}
				return exceptionNew(cls, args)
			}
		}
		return ExceptionNewf(TypeError, "exceptions must derive from BaseException")
	case error:
		return exceptionNew(SystemError, Tuple{String(x.Error())})
	case string:
		return exceptionNew(SystemError, Tuple{String(x)})
	default:
		return exceptionNew(SystemError, Tuple{String(fmt.Sprintf("Unknown error %#v", r))})
	}
}

// First calls MakeException then adds the extra details in to make it a SyntaxError
func MakeSyntaxError(r interface{}, filename string, lineno int, offset int, line string) *Exception {
	// FIXME add more stuff to make it a SyntaxError!
	// see Python/errors.c PyErr_SyntaxLocationObject
	e := MakeException(r)
	e.Dict.Set("filename", String(filename))
	e.Dict.Set("lineno", Int(lineno))
	e.Dict.Set("offset", Int(offset))
	e.Dict.Set("line", String(line))
	return e
}

/*
#define PyType_HasFeature(t,f)  (((t)->tp_flags & (f)) != 0)

#define PyType_FastSubclass(t,f)  PyType_HasFeature(t,f)

#define PyType_Check(op) \
    PyType_FastSubclass(Py_TYPE(op), Py_TPFLAGS_TYPE_SUBCLASS)

#define PyType_CheckExact(op) (Py_TYPE(op) == &PyType_Type)

#define PyExceptionClass_Check(x)                                       \
    (PyType_Check((x)) &&                                               \
     PyType_FastSubclass((PyTypeObject*)(x), Py_TPFLAGS_BASE_EXC_SUBCLASS))

#define PyExceptionInstance_Check(x)                    \
    PyType_FastSubclass((x)->ob_type, Py_TPFLAGS_BASE_EXC_SUBCLASS)

#define PyExceptionClass_Name(x) \
     ((char *)(((PyTypeObject*)(x))->tp_name))

#define PyExceptionInstance_Class(x) ((PyObject*)((x)->ob_type))
*/

// Checks that the object passed in is a class and is an exception
func ExceptionClassCheck(err Object) bool {
	if t, ok := err.(*Type); ok {
		// FIXME not telling instances and classes apart
		// properly! This could be an instance of something
		// here
		return t.Flags&TPFLAGS_BASE_EXC_SUBCLASS != 0
	}
	return false
}

// Check to see if err matches exc
//
// exc can be a tuple
//
// Used in except statements
func ExceptionGivenMatches(err, exc Object) bool {
	if err == nil || exc == nil {
		// maybe caused by "import exceptions" that failed early on
		return false
	}

	// Test the tuple case recursively
	if excTuple, ok := exc.(Tuple); ok {
		for i := range excTuple {
			if ExceptionGivenMatches(err, excTuple[i]) {
				return true
			}
		}
		return false
	}

	// err might be an instance, so check its class.
	if exception, ok := err.(*Exception); ok {
		err = exception.Type()
	}

	if ExceptionClassCheck(err) && ExceptionClassCheck(exc) {
		res := false
		// PyObject *exception, *value, *tb;
		// PyErr_Fetch(&exception, &value, &tb);

		// PyObject_IsSubclass() can recurse and therefore is
		// not safe (see test_bad_getattr in test.pickletester).
		res = err.(*Type).IsSubtype(exc.(*Type))
		// This function must not fail, so print the error here
		// if (res == -1) {
		// 	PyErr_WriteUnraisable(err);
		// 	res = false
		// }
		// PyErr_Restore(exception, value, tb);
		return res
	}

	return err == exc
}

// IsException matches the result of recover to an exception
//
// # For use to catch a single python exception from go code
//
// It can be an instance or the class itself
func IsException(exception *Type, r interface{}) bool {
	var t *Type
	switch ex := r.(type) {
	case ExceptionInfo:
		t = ex.Type
	case *Exception:
		t = ex.Type()
	case *Type:
		t = ex
	default:
		return false
	}
	// Exact instance or subclass match
	if t == exception {
		return true
	}
	// Can't be a subclass of exception
	if t.Flags&TPFLAGS_BASE_EXC_SUBCLASS == 0 {
		return false
	}
	// Now the full match
	return t.IsSubtype(exception)
}

// FIXME prototype __getattr__ before we do introspection!
// M__getattr__ answers for an attribute the exception does not have.
//
// It used to return the ARGUMENT TUPLE for every name, which is why
// "e.with_traceback(None)" raised "'tuple' object is not callable": the
// lookup succeeded and produced a tuple where a method was expected.  An
// unknown attribute is now an AttributeError, as it is in CPython.
func (e *Exception) M__getattr__(name string) (Object, error) {
	return nil, ExceptionNewf(AttributeError, "'%s' object has no attribute '%s'", e.Base.Name, name)
}

// with_traceback(tb) sets the traceback and returns self.
//
// A traceback is a *ExceptionInfo here rather than a Python object, so the
// argument is accepted and ignored: the interpreter builds its own traceback
// when the exception propagates.  The call is idiomatically used to CLEAR a
// traceback - "e.with_traceback(None)" - and it must at least not fail.
func (e *Exception) M__with_traceback(args Tuple) (Object, error) {
	_ = args
	return e, nil
}

// gCurrentException is the exception the interpreter is currently handling,
// which sys.exc_info() reports.  The VM records it when a raise happens.
//
// It is GOROUTINE-local, not a plain package variable.  Two interpreter
// contexts running in two goroutines each handle their own exceptions, and a
// shared variable was a data race on every raise - the race detector reports
// it - as well as reporting the wrong exception when they interleaved.
//
// Go has no goroutine-local storage, so the value is kept in a mutex-guarded
// map keyed by the goroutine id; see vm.goroutineID for how the id is
// obtained and why it is confined to one function.
//
// Known difference: CPython clears the state when the except block ends,
// whereas here it survives until the next raise.  Code that reads exc_info()
// INSIDE a handler - which is what it is for - sees the right exception.
var gCurrentException = &excSlot{exc: map[uint64]*Exception{}}

type excSlot struct {
	mu  sync.Mutex
	exc map[uint64]*Exception
}

// SetCurrentException records the exception now being handled.  The VM calls
// it; passing nil clears the state.
func SetCurrentException(e *Exception) {
	gCurrentException.mu.Lock()
	defer gCurrentException.mu.Unlock()
	id := CurrentGoroutineID()
	if e == nil {
		delete(gCurrentException.exc, id)
		return
	}
	gCurrentException.exc[id] = e
	// This is the raise path - the vm calls SetCurrentException when it raises -
	// so it is where a traceback is captured.  CPython sets __traceback__ at
	// raise for the same reason: it is the only moment the frames that led here
	// are still on the stack.  SetExceptionTraceback does not overwrite a
	// traceback an exception already carries, so a re-raise or a chain keeps
	// the place the exception originally came from.
	SetExceptionTraceback(e, CaptureTraceback())
}

// CurrentException returns the exception now being handled, or nil.
func CurrentException() *Exception {
	gCurrentException.mu.Lock()
	defer gCurrentException.mu.Unlock()
	return gCurrentException.exc[CurrentGoroutineID()]
}

// CurrentGoroutineID is set by the vm package at init to the same
// goroutine-id lookup it uses for the current frame.  It is a function
// variable to avoid a dependency from py on vm.
var CurrentGoroutineID = func() uint64 { return 0 }

func (e *Exception) M__str__() (Object, error) {
	args, ok := e.Args.(Tuple)
	if !ok || len(args) == 0 {
		// "str(ValueError())" is "" - CPython gives an argument-less
		// exception the empty string.  Indexing [0] unguarded panicked with
		// "index out of range [0] with length 0", which repr() already
		// guarded against.
		return String(""), nil
	}
	if len(args) == 1 {
		// KeyError is the one builtin exception whose message is the REPR of
		// its argument, not str(): str(KeyError('k')) is "'k'", with the
		// quotes, because a KeyError is reporting a key and the quotes make
		// a string key distinguishable from a non-string one.  CPython does
		// this in KeyError_str.
		if e.Base == KeyError {
			return Repr(args[0])
		}
		// str(ValueError(5)) is "5": the single argument is str()'d, not
		// returned as it stands.
		return Str(args[0])
	}
	// Several arguments stringify as the repr of the whole tuple, so
	// str(ValueError("a", "b")) is "('a', 'b')".
	return args.M__repr__()
}

func (e *Exception) M__repr__() (Object, error) {
	typ := e.Base.Name
	args := e.Args.(Tuple)
	if len(args) == 0 {
		return String(fmt.Sprintf("%s()", typ)), nil
	}
	// The representation is "Name(arg, arg)" - the tuple's own repr WITHOUT the
	// trailing comma a one-element tuple carries.  repr(IndexError('x')) is
	// "IndexError('x')" in CPython, not "IndexError('x',)"; the comma was
	// leaking in from the tuple machinery, and it appeared in every traceback
	// that reported a single-argument exception.
	parts := make([]string, 0, len(args))
	for _, a := range args {
		s, err := ReprAsString(a)
		if err != nil {
			return nil, err
		}
		parts = append(parts, s)
	}
	return String(typ + "(" + strings.Join(parts, ", ") + ")"), nil
}

// Check Interfaces
var (
	_ error = (*ExceptionInfo)(nil)

	_ error     = (*Exception)(nil)
	_ I__str__  = (*Exception)(nil)
	_ I__repr__ = (*Exception)(nil)
)

// GeneratorExitInstance is a fresh GeneratorExit, for closing a generator.
//
// A new instance each time: the exception carries its own traceback, and one
// shared instance would accumulate the frames of every close ever performed.
func GeneratorExitInstance() *Exception {
	return exceptionNew(GeneratorExit, Tuple{})
}

// errnoValues is what OSError exposes as .errno and .strerror.
//
// CPython takes these from the two-argument form OSError(errno, strerror);
// the interpreter's own errors carry only a message, so a handler that reads
// .errno (which is how code recognises a missing file or a closed pipe)
// would otherwise see AttributeError.  Nothing here sets them yet; they are
// present so the attribute exists and reads as None, as it does in CPython
// for an OSError built from a message alone.
func init() {
	// with_traceback is registered on BaseException, so every exception
	// inherits it through the ordinary lookup.  It has to be a real entry in
	// the type dict rather than only a Go method on *Exception: the
	// reflection shortcut in GetAttrString binds M__name for an object whose
	// own dict has nothing, and an instance of a python class is a *Type with
	// an empty Name, which that shortcut skips.
	// args is the argument tuple the exception was raised with, and it is a
	// real attribute - code reads "e.args[0]" to get at the message.  It was
	// reachable before only by accident, because M__getattr__ answered every
	// name with the argument tuple; now that unknown names raise, it has to
	// be registered.
	BaseException.Dict.Set("args", &Property{
		Fget: func(self Object) (Object, error) {
			if e, ok := self.(*Exception); ok {
				return e.Args, nil
			}
			// An instance of a python subclass of Exception is a *Type here
			// rather than a *Exception, so its fields live in its dict.
			if d, ok := self.(IGetDict); ok {
				if v, ok := d.GetDict().Get("args"); ok {
					return v, nil
				}
			}
			return None, nil
		},
		Fset: func(self, value Object) error {
			if e, ok := self.(*Exception); ok {
				e.Args = value
				return nil
			}
			if d, ok := self.(IGetDict); ok {
				dict := d.GetDict()
				dict.Set("args", value)
				return nil
			}
			return nil
		},
	})

	BaseException.Dict.Set("with_traceback", MustNewMethod("with_traceback", func(self Object, args Tuple) (Object, error) {
		return self.(*Exception).M__with_traceback(args)
	}, 1, `with_traceback(tb) -> set the traceback and return self.

A traceback is not a Python object here, so tb is accepted and ignored: the
interpreter builds its own when the exception propagates.  Idiomatic code
uses this to CLEAR a traceback with None.`))

	setErrno := func(self Object) (Object, error) {
		if e, ok := self.(*Exception); ok {
			if v, ok := e.Dict.Get("errno"); ok {
				return v, nil
			}
		}
		return None, nil
	}
	setStrerror := func(self Object) (Object, error) {
		if e, ok := self.(*Exception); ok {
			if v, ok := e.Dict.Get("strerror"); ok {
				return v, nil
			}
		}
		return None, nil
	}
	// SystemExit.code is the status the program asks to exit with: the first
	// argument, or None when there is none.  It is a plain attribute - pip
	// catches SystemExit and reads .code - and it was missing, so
	// "SystemExit(3).code" raised AttributeError.
	SystemExit.Dict.Set("code", &Property{Fget: func(self Object) (Object, error) {
		// The arguments live in the Exception's own Args field, not in its
		// Dict - reading the Dict found nothing and every code was None.
		// One argument IS the code; several give the whole argument tuple,
		// which is CPython's rule ("SystemExit(1, 2).code" is (1, 2)).
		if e, ok := self.(*Exception); ok {
			if t, ok := e.Args.(Tuple); ok {
				switch len(t) {
				case 0:
					// fall through to None
				case 1:
					return t[0], nil
				default:
					return t, nil
				}
			}
		}
		return None, nil
	}})

	OSError.Dict.Set("errno", &Property{Fget: setErrno})
	OSError.Dict.Set("strerror", &Property{Fget: setStrerror})
	OSError.Dict.Set("filename", &Property{Fget: func(self Object) (Object, error) {
		if e, ok := self.(*Exception); ok {
			if v, ok := e.Dict.Get("filename"); ok {
				return v, nil
			}
		}
		return None, nil
	}})

	// SetErrno records an errno on an OSError, which is what the operating
	// system wrappers in the standard library use.
	SetErrno = func(e *Exception, errno int, strerror string) {
		if e.Dict.IsNil() {
			e.Dict = NewStringDict()
		}
		e.Dict.Set("errno", Int(errno))
		e.Dict.Set("strerror", String(strerror))
	}
}

// SetErrno attaches an errno and its message to an OSError.  It is a
// variable so that the standard library can call it without this package
// knowing anything about errno.
var SetErrno func(e *Exception, errno int, strerror string)
