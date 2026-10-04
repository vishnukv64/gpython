// Package importlibabc implements importlib.abc: the abstract base classes of
// the import system.
//
// The module exists in CPython so that code can test and derive from the import
// protocols - "isinstance(loader, importlib.abc.Loader)" and
// "class Mine(importlib.abc.MetaPathFinder)".  pkg_resources imports it without
// naming anything in it, which is why a missing module blocked pip at all; but
// the classes must be real, because the isinstance checks are the reason the
// module is imported in the first place.
//
// The classes are plain classes here rather than ABCs: this interpreter's
// abc.ABCMeta would be usable, but none of the behaviour these classes provide
// is abstract-method enforcement - they are the shapes the import machinery
// checks a loader against - and inheriting is what a caller needs.
package importlibabc

import (
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/typing"
)

const module_doc = `Abstract base classes related to import.`

func init() {
	// Loader is the base of every loader: it has load_module, and the newer
	// exec_module/create_module pair.
	loader := py.ObjectType.NewType("importlib.abc.Loader",
		"Base class for loaders.", nil, nil)
	loader.Flags |= py.TPFLAGS_BASETYPE

	// ResourceLoader and InspectLoader are the two loader refinements.
	resourceLoader := loader.NewType("importlib.abc.ResourceLoader",
		"Base class for loaders that can read a resource.", nil, nil)
	resourceLoader.Flags |= py.TPFLAGS_BASETYPE

	inspectLoader := loader.NewType("importlib.abc.InspectLoader",
		"Base class for loaders that can inspect a module.", nil, nil)
	inspectLoader.Flags |= py.TPFLAGS_BASETYPE

	// ExecutionLoader is both, which is how CPython composes it.
	executionLoader := inspectLoader.NewType("importlib.abc.ExecutionLoader",
		"Base class for loaders that can execute a module.", nil, nil)
	executionLoader.Flags |= py.TPFLAGS_BASETYPE

	sourceLoader := executionLoader.NewType("importlib.abc.SourceLoader",
		"Base class for loaders that can read a module's source.", nil, nil)
	sourceLoader.Flags |= py.TPFLAGS_BASETYPE

	fileLoader := resourceLoader.NewType("importlib.abc.FileLoader",
		"Base class for loaders that load from a file.", nil, nil)
	fileLoader.Flags |= py.TPFLAGS_BASETYPE

	// The finders.
	finder := py.ObjectType.NewType("importlib.abc.Finder",
		"Base class for finders.", nil, nil)
	finder.Flags |= py.TPFLAGS_BASETYPE

	metaPathFinder := finder.NewType("importlib.abc.MetaPathFinder",
		"Base class for finders on sys.meta_path.", nil, nil)
	metaPathFinder.Flags |= py.TPFLAGS_BASETYPE

	pathEntryFinder := finder.NewType("importlib.abc.PathEntryFinder",
		"Base class for finders on a path entry.", nil, nil)
	pathEntryFinder.Flags |= py.TPFLAGS_BASETYPE

	// The resource protocol, which is a separate hierarchy.
	traversable := py.ObjectType.NewType("importlib.abc.Traversable",
		"An object with a subset of pathlib.Path methods for a resource.", nil, nil)
	traversable.Flags |= py.TPFLAGS_BASETYPE

	traversableResources := py.ObjectType.NewType("importlib.abc.TraversableResources",
		"An abstract base class for resource readers.", nil, nil)
	traversableResources.Flags |= py.TPFLAGS_BASETYPE

	resourceReader := py.ObjectType.NewType("importlib.abc.ResourceReader",
		"An abstract base class for reading a package's resources.", nil, nil)
	resourceReader.Flags |= py.TPFLAGS_BASETYPE

	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))
	for name, t := range map[string]*py.Type{
		"Loader":               loader,
		"ResourceLoader":       resourceLoader,
		"InspectLoader":        inspectLoader,
		"ExecutionLoader":      executionLoader,
		"SourceLoader":         sourceLoader,
		"FileLoader":           fileLoader,
		"Finder":               finder,
		"MetaPathFinder":       metaPathFinder,
		"PathEntryFinder":      pathEntryFinder,
		"Traversable":          traversable,
		"TraversableResources": traversableResources,
		"ResourceReader":       resourceReader,
		// The SAME class typing exposes: CPython's
		// "importlib.abc.Protocol is typing.Protocol" is True, and a second class
		// with that name would make the identity check fail.
		"Protocol": typing.ProtocolType,
	} {
		globals.Set(name, t)
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "importlib.abc",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
