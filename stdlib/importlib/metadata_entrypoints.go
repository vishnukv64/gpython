// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// importlib.metadata's entry-point API.
//
// The modern shape (Python 3.10 and later) is
//
//	entry_points()            -> an EntryPoints collection
//	entry_points(group="x")   -> the entry points in that group
//	.EntryPoints.select(...)  -> the same, for the 3.10+ spelling
//
// and each entry point carries .name, .group, .value and .load().  Reading a
// distribution's entry_points.txt is what supplies them: the file records the
// groups a package declares and, under each, "name = module:object" lines.
//
// This exists for pygments, which pip vendors: pygments/plugin.py does
//
//	from importlib.metadata import entry_points
//	groups = entry_points()
//	return groups.select(group=group_name)
//
// to discover plugins, so without it pip cannot import pygments, and without
// pygments rich cannot render a traceback.
//
// A distribution whose entry points cannot be read contributes nothing rather
// than failing the whole call: one malformed package on sys.path must not make
// every lookup raise.

package importlib

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// entryPoint is one "name = module:object" line from an entry_points.txt.
type entryPoint struct {
	name  string
	group string
	value string
	// dist is the distribution the entry point came from, for repr.
	dist string
}

var entryPointType = py.NewType("importlib.metadata.EntryPoint",
	"An entry point: a named object a distribution advertises.")

func (e *entryPoint) Type() *py.Type { return entryPointType }

// entryPoints is the collection entry_points() returns.
type entryPoints struct {
	items []*entryPoint
}

var entryPointsType = py.NewType("importlib.metadata.EntryPoints",
	"A set of entry points, selectable by group.")

func (e *entryPoints) Type() *py.Type { return entryPointsType }

// splitEntryPoint splits "module:attr.sub" into (module, attr).
func splitEntryPoint(value string) (string, string) {
	if i := strings.IndexByte(value, ':'); i >= 0 {
		return strings.TrimSpace(value[:i]), strings.TrimSpace(value[i+1:])
	}
	return strings.TrimSpace(value), ""
}

// loadEntryPoint imports the module and resolves the attribute path.
//
// An entry point pointing at a module that cannot be imported is an error the
// caller sees, not a silent nil: a plugin that failed to load must not look
// like a plugin that registered nothing.
func loadEntryPoint(e *entryPoint, ctx py.Context) (py.Object, error) {
	modName, attr := splitEntryPoint(e.value)
	if ctx == nil {
		return nil, py.ExceptionNewf(py.ImportError,
			"cannot load entry point %q: no interpreter context", e.name)
	}
	mod, err := py.ImportModuleLevelObject(ctx, modName, py.NewStringDict(), py.NewStringDict(), py.Tuple{}, 0)
	if err != nil {
		return nil, err
	}
	obj := py.Object(mod)
	if attr == "" {
		return obj, nil
	}
	for _, part := range strings.Split(attr, ".") {
		obj, err = py.GetAttrString(obj, part)
		if err != nil {
			return nil, err
		}
	}
	return obj, nil
}

// parseEntryPoints reads one entry_points.txt into the entry-point list.
//
// The file is INI-shaped: "[group]" headers, then "name = value" lines, with
// "#" and ";" comments.  It is parsed here rather than through configparser so
// that importlib does not depend on a stdlib module that may itself be
// importing importlib.
func parseEntryPoints(path, distName string, out *[]*entryPoint) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	group := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';' {
			continue
		}
		if trimmed[0] == '[' {
			if end := strings.IndexByte(trimmed, ']'); end > 0 {
				group = strings.TrimSpace(trimmed[1:end])
			}
			continue
		}
		if group == "" {
			continue
		}
		eq := strings.IndexByte(trimmed, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(trimmed[:eq])
		value := strings.TrimSpace(trimmed[eq+1:])
		if name == "" || value == "" {
			continue
		}
		*out = append(*out, &entryPoint{name: name, group: group, value: value, dist: distName})
	}
}

// allEntryPoints scans sys.path for every distribution that declares entry
// points and returns them, grouped by nothing - the caller selects.
func allEntryPoints(ctx py.Context) []*entryPoint {
	var out []*entryPoint
	for _, dir := range metadataSearchDirs(ctx) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			base := e.Name()
			isDistInfo := strings.HasSuffix(base, ".dist-info")
			isEggInfo := strings.HasSuffix(base, ".egg-info")
			if !isDistInfo && !isEggInfo {
				continue
			}
			stem := strings.TrimSuffix(strings.TrimSuffix(base, ".dist-info"), ".egg-info")
			name := stem
			if i := strings.IndexByte(stem, '-'); i > 0 {
				name = stem[:i]
			}
			// entry_points.txt sits inside the .dist-info directory, and for an
			// .egg-info it may be the directory itself.
			candidates := []string{filepath.Join(dir, base, "entry_points.txt")}
			if isEggInfo {
				candidates = append(candidates, filepath.Join(dir, base+".egg-info", "entry_points.txt"))
			}
			for _, p := range candidates {
				parseEntryPoints(p, name, &out)
			}
		}
	}
	// A stable order, so two runs report plugins in the same sequence.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].group != out[j].group {
			return out[i].group < out[j].group
		}
		return out[i].name < out[j].name
	})
	return out
}

// selectEntryPoints filters by group and, when given, by name.
func selectEntryPoints(all []*entryPoint, group string, names []string) *entryPoints {
	res := &entryPoints{}
	for _, ep := range all {
		if group != "" && ep.group != group {
			continue
		}
		if len(names) > 0 {
			found := false
			for _, n := range names {
				if n == ep.name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		res.items = append(res.items, ep)
	}
	return res
}

func init() {
	entryPointType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*entryPoint); ok {
				return py.String(e.name), nil
			}
			return py.None, nil
		},
		Doc: "The name of this entry point.",
	})
	entryPointType.Dict.Set("group", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*entryPoint); ok {
				return py.String(e.group), nil
			}
			return py.None, nil
		},
		Doc: "The group this entry point belongs to.",
	})
	entryPointType.Dict.Set("value", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*entryPoint); ok {
				return py.String(e.value), nil
			}
			return py.None, nil
		},
		Doc: "The object reference, as written in entry_points.txt.",
	})
	entryPointType.Dict.Set("load", py.MustNewMethod("load", func(self py.Object, args py.Tuple) (py.Object, error) {
		e, ok := self.(*entryPoint)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not an EntryPoint")
		}
		return loadEntryPoint(e, currentContext(self))
	}, 0, "Import and return the object this entry point refers to."))
	entryPointType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		e, ok := self.(*entryPoint)
		if !ok {
			return py.String("<EntryPoint>"), nil
		}
		return py.String("EntryPoint(name='" + e.name + "', value='" + e.value +
			"', group='" + e.group + "')"), nil
	}, 0, "Return repr(self)."))
	entryPointType.Dict.Set("__eq__", py.MustNewMethod("__eq__", func(self py.Object, args py.Tuple) (py.Object, error) {
		e, ok := self.(*entryPoint)
		if !ok || len(args) < 1 {
			return py.False, nil
		}
		o, ok := args[0].(*entryPoint)
		if !ok {
			return py.False, nil
		}
		return py.NewBool(e.name == o.name && e.group == o.group && e.value == o.value), nil
	}, 0, "Two entry points are equal when name, group and value match."))
	entryPointType.Dict.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		e, ok := self.(*entryPoint)
		if !ok {
			return py.Int(0), nil
		}
		return py.Int(int64(py.MemoryHash([]byte(e.name + "\x00" + e.group + "\x00" + e.value)))), nil
	}, 0, "Return hash(self)."))

	// The group of entry points, with select() - the 3.10+ spelling pygments
	// uses when it is available.
	entryPointsType.Dict.Set("select", py.MustNewMethod("select", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		eps, ok := self.(*entryPoints)
		if !ok {
			return &entryPoints{}, nil
		}
		group := ""
		var names []string
		if v, ok := kwargs.Get("group"); ok && v != py.None {
			group, _ = py.StrAsString(v)
		}
		if v, ok := kwargs.Get("name"); ok && v != py.None {
			if s, err := py.StrAsString(v); err == nil {
				names = append(names, s)
			}
		}
		if v, ok := kwargs.Get("pattern"); ok && v != py.None {
			// A pattern is matched with fnmatch-like globbing in CPython; this
			// refuses rather than returning a wrong subset.
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"EntryPoints.select(pattern=...) is not supported")
		}
		return selectEntryPoints(eps.items, group, names), nil
	}, 0, "Return the entry points in a group, optionally restricted by name."))

	entryPointsType.Dict.Set("get", py.MustNewMethod("names_get_placeholder", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Kept for the pre-3.10 spelling some code still uses: a mapping lookup
		// by group name, returning a LIST.
		eps, ok := self.(*entryPoints)
		if !ok || len(args) < 1 {
			return py.NewList(), nil
		}
		group, _ := py.StrAsString(args[0])
		sel := selectEntryPoints(eps.items, group, nil)
		out := py.NewList()
		for _, ep := range sel.items {
			out.Append(ep)
		}
		return out, nil
	}, 0, "The entry points in a group, as a list - the pre-3.10 spelling."))

	entryPointsType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if eps, ok := self.(*entryPoints); ok {
			return py.Int(len(eps.items)), nil
		}
		return py.Int(0), nil
	}, 0, "The number of entry points."))

	entryPointsType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		out := py.NewList()
		if eps, ok := self.(*entryPoints); ok {
			for _, ep := range eps.items {
				out.Append(ep)
			}
		}
		return py.Iter(out)
	}, 0, "Iterate over the entry points."))

	entryPointsType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		eps, ok := self.(*entryPoints)
		if !ok || len(args) < 1 {
			return nil, py.ExceptionNewf(py.IndexError, "entry points index out of range")
		}
		n, ok := args[0].(py.Int)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "entry points index must be an integer")
		}
		i, _ := n.GoInt64()
		if i < 0 || int(i) >= len(eps.items) {
			return nil, py.ExceptionNewf(py.IndexError, "entry points index out of range")
		}
		return eps.items[i], nil
	}, 0, "The entry point at an index."))

	entryPointsType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		eps, ok := self.(*entryPoints)
		if !ok {
			return py.String("EntryPoints()"), nil
		}
		parts := make([]string, 0, len(eps.items))
		for _, ep := range eps.items {
			parts = append(parts, "EntryPoint(name='"+ep.name+"', value='"+ep.value+"', group='"+ep.group+"')")
		}
		return py.String("EntryPoints([" + strings.Join(parts, ", ") + "])"), nil
	}, 0, "Return repr(self)."))
}

// metadataEntryPoints implements the module-level entry_points().
func metadataEntryPoints(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	ctx := currentContext(self)
	all := allEntryPoints(ctx)
	// The 3.10+ signature accepts **params, most usefully group=, and returns
	// only those.  Calling it with no arguments returns everything.
	group := ""
	var names []string
	if v, ok := kwargs.Get("group"); ok && v != py.None {
		group, _ = py.StrAsString(v)
	}
	if v, ok := kwargs.Get("name"); ok && v != py.None {
		if s, err := py.StrAsString(v); err == nil {
			names = append(names, s)
		}
	}
	return selectEntryPoints(all, group, names), nil
}
