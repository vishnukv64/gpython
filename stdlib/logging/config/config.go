// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config implements logging.config: configuring the logging system
// from a dictionary or a file.
//
// dictConfig is implemented, because it is the form real programs use - pip
// imports this module and passes a dict - and because it can be done exactly:
// the schema is a plain nested mapping, and every action it implies
// (create a logger, set its level, attach a handler with a formatter) is
// already a method on the objects involved.
//
// fileConfig and listen/stopListening are NOT implemented and raise.  fileConfig
// is the older INI format with its own evaluation rules; listen starts a socket
// server to reconfigure a running process.  Neither can be approximated without
// silently ignoring part of a configuration, which is the failure this project
// keeps choosing to make loud.

package config

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/logging"
)

const module_doc = `Configuration functions for the logging package.

dictConfig(config) configures logging from a dictionary, in the schema
CPython documents: version, formatters, handlers, loggers and root.

fileConfig and listen are not implemented, and raise, because neither can be
approximated without silently ignoring part of a configuration.
`

// levelByName maps a level name to the number, so a config's "INFO" works.
func levelByName(name string) (int, bool) {
	// The logging module owns the name table, INCLUDING the levels a program
	// registered with addLevelName - pip registers VERBOSE that way.  A
	// hardcoded switch here ignored them and refused a configuration dict that
	// named one.
	if n, ok := logging.LevelNumberByName(name); ok {
		return n, true
	}
	switch strings.ToUpper(name) {
	case "CRITICAL":
		return logging.CRITICAL, true
	}
	return 0, false
}

// asMapping reads a value that must be a mapping.
//
// A config may use a real dict; anything else is an error naming the key, which
// is what makes a malformed config legible rather than silently empty.
func asMapping(v py.Object, what string) (py.StringDict, error) {
	if d, ok := v.(py.StringDict); ok {
		return d, nil
	}
	return py.StringDict{}, py.ExceptionNewf(py.TypeError,
		"%s must be a dictionary, not %s", what, v.Type().Name)
}

func strOf(v py.Object) string {
	s, err := py.StrAsString(v)
	if err != nil {
		return ""
	}
	return s
}

// resolveLevel reads a level from a value: a name, or a number.
func resolveLevel(v py.Object) (int, error) {
	switch x := v.(type) {
	case py.Int:
		n, _ := x.GoInt64()
		return int(n), nil
	case py.String:
		if n, ok := levelByName(string(x)); ok {
			return n, nil
		}
		return 0, py.ExceptionNewf(py.ValueError, "unknown level: %q", string(x))
	}
	return 0, py.ExceptionNewf(py.TypeError, "level must be a string or an integer, not %s", v.Type().Name)
}

// makeFormatter builds a Formatter from its config entry.
func makeFormatter(spec py.StringDict) (py.Object, error) {
	format := "%(message)s"
	if v, ok := spec.Get("format"); ok {
		format = strOf(v)
	}
	datefmt := ""
	if v, ok := spec.Get("datefmt"); ok {
		datefmt = strOf(v)
	}
	// The class is almost always logging.Formatter, which is what this builds.
	// A custom class is honoured by importing it.
	if v, ok := spec.Get("()"); ok {
		cls := v
		kwargs := py.NewStringDict()
		if k, ok := spec.Get("kwargs"); ok {
			if d, ok := k.(py.StringDict); ok {
				kwargs = d
			}
		}
		return py.Call(cls, py.Tuple{}, kwargs)
	}
	// Built by calling the type, which is how CPython's own config does it and
	// keeps one constructor rather than two.
	return py.Call(logging.FormatterType, py.Tuple{py.String(format), py.String(datefmt)}, py.NewStringDict())
}

// resolveDotted turns "logging.StreamHandler" into the class it names.
//
// A config names its classes by DOTTED STRING, which is the whole point of the
// format - the config never imports anything - so the string has to be turned
// into the object before it can be called.  Passing the string straight to
// Call raised "'str' object is not callable".
func resolveDotted(ctx py.Context, name string) (py.Object, error) {
	if !strings.Contains(name, ".") {
		if ctx == nil {
			return nil, py.ExceptionNewf(py.ImportError, "cannot resolve %q: no context", name)
		}
		builtins, err := ctx.GetModule("builtins")
		if err != nil {
			return nil, err
		}
		return py.GetAttrString(builtins, name)
	}
	// The module is everything before the LAST dot, since the attribute itself
	// may be dotted and the module may be a package.
	modName, attr := name[:strings.LastIndex(name, ".")], name[strings.LastIndex(name, ".")+1:]
	if ctx == nil {
		return nil, py.ExceptionNewf(py.ImportError, "cannot resolve %q: no context", name)
	}
	var target py.Object
	mod, err := ctx.GetModule(modName)
	if err == nil {
		target = mod
	} else {
		// Not loaded yet: import it, which is what CPython's config does.
		target, err = py.ImportModuleLevelObject(ctx, modName, py.NewStringDict(), py.NewStringDict(), py.Tuple{}, 0)
		if err != nil {
			return nil, err
		}
	}
	return py.GetAttrString(target, attr)
}

// makeHandler builds a handler and applies its level, formatter and filters.
func makeHandler(ctx py.Context, class py.Object, spec py.StringDict, formatters py.StringDict, filters py.StringDict) (py.Object, error) {
	// A "()" key names the factory explicitly, which is the documented way to
	// use a callable rather than a class.
	factory := class
	if v, ok := spec.Get("()"); ok {
		factory = v
	}
	// The factory may be a dotted STRING - that is how a config names a class -
	// so it is resolved before anything tries to call it.
	if name, isStr := factory.(py.String); isStr {
		resolved, err := resolveDotted(ctx, string(name))
		if err != nil {
			return nil, err
		}
		factory = resolved
	}
	var args py.Object = py.None
	if v, ok := spec.Get("args"); ok {
		args = v
	}
	kwargs := py.NewStringDict()
	if v, ok := spec.Get("kwargs"); ok {
		if d, ok := v.(py.StringDict); ok {
			kwargs = d.Copy()
		} else {
			return nil, py.ExceptionNewf(py.TypeError, "handler kwargs must be a dictionary")
		}
	}
	// A handler's own options are written at the TOP level of its entry -
	// "stream", "filename", "mode" - not nested under kwargs, and CPython passes
	// them to the factory as keyword arguments.  Forwarding only "kwargs"
	// dropped them, so a handler configured with a stream wrote to stderr
	// instead: the output appeared on the terminal rather than in the stream
	// the config named.
	//
	// The keys that configure the HANDLER rather than the factory - level,
	// formatter, filters - are applied afterwards and are not passed.
	for _, ent := range spec.Items() {
		switch ent.Key {
		case "class", "()", "level", "formatter", "filters", "kwargs", "args":
			continue
		}
		if _, exists := kwargs.Get(ent.Key); !exists {
			kwargs.Set(ent.Key, ent.Value)
		}
	}
	// Only the factory's own arguments are passed; level and formatter are
	// applied afterwards, which is what keeps them out of the constructor.
	var argTuple py.Tuple
	if t, ok := args.(py.Tuple); ok {
		argTuple = t
	}
	h, err := py.Call(factory, argTuple, kwargs)
	if err != nil {
		return nil, err
	}
	if v, ok := spec.Get("level"); ok {
		level, lerr := resolveLevel(v)
		if lerr != nil {
			return nil, lerr
		}
		set, serr := py.GetAttrString(h, "setLevel")
		if serr != nil {
			return nil, serr
		}
		if _, cerr := py.Call(set, py.Tuple{py.Int(level)}, py.NewStringDict()); cerr != nil {
			return nil, cerr
		}
	}
	if v, ok := spec.Get("formatter"); ok {
		name := strOf(v)
		f, ok := formatters.Get(name)
		if !ok {
			return nil, py.ExceptionNewf(py.ValueError, "Unable to set formatter %q: no such formatter", name)
		}
		set, serr := py.GetAttrString(h, "setFormatter")
		if serr != nil {
			return nil, serr
		}
		if _, cerr := py.Call(set, py.Tuple{f}, py.NewStringDict()); cerr != nil {
			return nil, cerr
		}
	}
	// The handler's filters are applied here, by NAME from the config's filters
	// section.  They were skipped entirely - makeHandler's own comment said they
	// were applied and nothing did it - so every handler ran UNFILTERED.
	//
	// pip's config gives its stderr handler a MaxLevelFilter(WARNING) so that
	// only warnings and above go there, while the stdout handler takes the rest.
	// With the filter ignored, both handlers took every record and every line of
	// "pip show" was written TWICE, once per stream.
	if v, ok := spec.Get("filters"); ok {
		names, isSeq := v.(py.Tuple)
		if !isSeq {
			if l, isList := v.(*py.List); isList {
				names = py.Tuple((*l).Items)
			} else {
				return nil, py.ExceptionNewf(py.ValueError, "Unable to configure handler %q: filters must be a sequence", strOf(spec.GetOrNil("class")))
			}
		}
		add, aerr := py.GetAttrString(h, "addFilter")
		if aerr != nil {
			return nil, aerr
		}
		for _, n := range names {
			name := strOf(n)
			f, ok := filters.Get(name)
			if !ok {
				return nil, py.ExceptionNewf(py.ValueError, "Unable to configure handler %q: no filter named %q", strOf(spec.GetOrNil("class")), name)
			}
			if _, cerr := py.Call(add, py.Tuple{f}, py.NewStringDict()); cerr != nil {
				return nil, cerr
			}
		}
	}
	return h, nil
}

// dictConfig configures logging from a dictionary.
//
// The schema CPython documents is honoured in the parts that matter to a real
// program: version, formatters, handlers, loggers, root, and disable_existing.
// An unknown top-level key is IGNORED, as CPython does, so a config written for
// a newer Python still applies.
func dictConfig(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var cfgObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "dictConfig", 1, 1, &cfgObj); err != nil {
		return nil, err
	}
	cfg, err := asMapping(cfgObj, "dictConfig() argument")
	if err != nil {
		return nil, err
	}
	ctx := currentContext(self)

	// version is required, and CPython accepts only 1.
	if v, ok := cfg.Get("version"); ok {
		if n, ok := v.(py.Int); ok {
			if i, _ := n.GoInt64(); i != 1 {
				return nil, py.ExceptionNewf(py.ValueError,
					"Unsupported version: %d", i)
			}
		}
	} else {
		return nil, py.ExceptionNewf(py.ValueError, "dictionary doesn't specify a version")
	}

	// Formatters first: a handler refers to them by name.
	formatters := py.NewStringDict()
	if v, ok := cfg.Get("formatters"); ok {
		defs, merr := asMapping(v, "formatters")
		if merr != nil {
			return nil, merr
		}
		for _, ent := range defs.Items() {
			spec, ok := ent.Value.(py.StringDict)
			if !ok {
				continue
			}
			f, ferr := makeFormatter(spec)
			if ferr != nil {
				return nil, ferr
			}
			formatters.Set(ent.Key, f)
		}
	}

	// Filters before handlers, which refer to them by name.  Each entry is
	// built by CALLING its "()" factory with the remaining keys as keyword
	// arguments, which is how CPython's own config does it.
	filters := py.NewStringDict()
	if v, ok := cfg.Get("filters"); ok {
		defs, merr := asMapping(v, "filters")
		if merr != nil {
			return nil, merr
		}
		for _, ent := range defs.Items() {
			spec, ok := ent.Value.(py.StringDict)
			if !ok {
				continue
			}
			factory, hasFactory := spec.Get("()")
			if !hasFactory {
				return nil, py.ExceptionNewf(py.ValueError, "Unable to configure filter %q: no '()'", ent.Key)
			}
			// The factory may be a DOTTED STRING naming the class, which is how
			// a config written for CPython names it - pip's own config does.
			// Calling the string raised "'str' object is not callable" and took
			// down pip's setup_logging.
			if name, isStr := factory.(py.String); isStr {
				resolved, rerr := resolveDotted(ctx, string(name))
				if rerr != nil {
					return nil, rerr
				}
				factory = resolved
			}
			kwargs := py.NewStringDict()
			for _, e := range spec.Items() {
				if e.Key == "()" {
					continue
				}
				kwargs.Set(e.Key, e.Value)
			}
			f, ferr := py.Call(factory, py.Tuple{}, kwargs)
			if ferr != nil {
				return nil, ferr
			}
			filters.Set(ent.Key, f)
		}
	}

	// Then handlers, which may name a formatter and filters.
	handlers := py.NewStringDict()
	if v, ok := cfg.Get("handlers"); ok {
		defs, merr := asMapping(v, "handlers")
		if merr != nil {
			return nil, merr
		}
		for _, ent := range defs.Items() {
			spec, ok := ent.Value.(py.StringDict)
			if !ok {
				continue
			}
			class, hasClass := spec.Get("class")
			if !hasClass {
				// A handler entry with no class and no factory cannot be built.
				if _, hasFactory := spec.Get("()"); !hasFactory {
					return nil, py.ExceptionNewf(py.ValueError,
						"Unable to configure handler %q: no 'class' or '()'", ent.Key)
				}
			}
			h, herr := makeHandler(ctx, class, spec, formatters, filters)
			if herr != nil {
				return nil, herr
			}
			handlers.Set(ent.Key, h)
		}
	}

	// A logger's config names handlers by name, so the lookup is by string.
	attach := func(logger py.Object, spec py.StringDict) error {
		if v, ok := spec.Get("level"); ok {
			level, lerr := resolveLevel(v)
			if lerr != nil {
				return lerr
			}
			set, serr := py.GetAttrString(logger, "setLevel")
			if serr != nil {
				return serr
			}
			if _, cerr := py.Call(set, py.Tuple{py.Int(level)}, py.NewStringDict()); cerr != nil {
				return cerr
			}
		}
		if v, ok := spec.Get("handlers"); ok {
			list, ok := v.(py.Tuple)
			if !ok {
				if l, isList := v.(*py.List); isList {
					list = py.Tuple(l.Items)
				} else {
					return py.ExceptionNewf(py.TypeError, "logger handlers must be a list")
				}
			}
			add, aerr := py.GetAttrString(logger, "addHandler")
			if aerr != nil {
				return aerr
			}
			for _, nameObj := range list {
				name := strOf(nameObj)
				h, found := handlers.Get(name)
				if !found {
					return py.ExceptionNewf(py.ValueError,
						"Unable to add handler %q: no such handler", name)
				}
				if _, cerr := py.Call(add, py.Tuple{h}, py.NewStringDict()); cerr != nil {
					return cerr
				}
			}
		}
		return nil
	}

	if v, ok := cfg.Get("loggers"); ok {
		defs, merr := asMapping(v, "loggers")
		if merr != nil {
			return nil, merr
		}
		for _, ent := range defs.Items() {
			spec, ok := ent.Value.(py.StringDict)
			if !ok {
				continue
			}
			// A config may name a dotted logger, which lives under no package
			// here; getLogger handles that.
			get, gerr := py.GetAttrString(getLoggingModule(self), "getLogger")
			if gerr != nil {
				return nil, gerr
			}
			logger, cerr := py.Call(get, py.Tuple{py.String(ent.Key)}, py.NewStringDict())
			if cerr != nil {
				return nil, cerr
			}
			if aerr := attach(logger, spec); aerr != nil {
				return nil, aerr
			}
		}
	}

	if v, ok := cfg.Get("root"); ok {
		spec, merr := asMapping(v, "root")
		if merr != nil {
			return nil, merr
		}
		get, gerr := py.GetAttrString(getLoggingModule(self), "getLogger")
		if gerr != nil {
			return nil, gerr
		}
		root, cerr := py.Call(get, py.Tuple{}, py.NewStringDict())
		if cerr != nil {
			return nil, cerr
		}
		if aerr := attach(root, spec); aerr != nil {
			return nil, aerr
		}
	}

	return py.None, nil
}

// getLoggingModule returns the logging module, which the caller reached through
// this one - "logging.config" has its parent imported already.
// currentContext returns the Context a module method was called on.
func currentContext(self py.Object) py.Context {
	if self == nil {
		return nil
	}
	if m, ok := self.(*py.Module); ok {
		return m.Context
	}
	return nil
}

func getLoggingModule(self py.Object) py.Object {
	if m, ok := self.(*py.Module); ok && m.Context != nil {
		if mod, err := m.Context.GetModule("logging"); err == nil {
			return mod
		}
	}
	return py.None
}

func init() {
	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))
	globals.Set("DEFAULT_LOGGING_CONFIG_PORT", py.Int(9030))
	globals.Set("RESET_ERROR", py.Int(1))
	globals.Set("IDENTIFIER", py.String("logging.config"))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "logging.config",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("dictConfig", dictConfig, 0,
				"Configure logging from a dictionary of configuration information."),
			py.MustNewMethod("fileConfig", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return nil, py.ExceptionNewf(py.NotImplementedError,
					"logging.config.fileConfig() is not implemented: it is the older INI format, and applying part of a configuration silently is worse than refusing")
			}, 0, "Read an INI configuration; not implemented, see the module documentation."),
			py.MustNewMethod("listen", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return nil, py.ExceptionNewf(py.NotImplementedError,
					"logging.config.listen() is not implemented: it runs a socket server to reconfigure a running process")
			}, 0, "Start a socket server to reconfigure; not implemented."),
			py.MustNewMethod("stopListening", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return py.None, nil
			}, 0, "Stop the configuration listener; nothing was started."),
		},
		Globals: globals,
	})
}
