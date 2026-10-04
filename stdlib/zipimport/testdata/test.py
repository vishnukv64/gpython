"""zipimport: reading a module and data out of an archive.

The archive is a fixture built with Go's archive/zip, because gpython's zipfile
can READ a zip but not write one - and the module under test here is zipimport.
"""

import os
import zipimport

archive = os.path.join(os.path.dirname(os.path.abspath(__file__)), "probe.zip")
print("archive exists:", os.path.exists(archive))

zi = zipimport.zipimporter(archive)
print("archive basename:", os.path.basename(zi.archive))
print("prefix:", repr(zi.prefix))
print("is_package mod:", zi.is_package("mod"))
print("is_package pkg:", zi.is_package("pkg"))

try:
    zi.is_package("nope")
except zipimport.ZipImportError as e:
    print("missing raises:", e)

print("source is str:", isinstance(zi.get_source("mod"), str))
print("source:", repr(zi.get_source("mod")))
print("data is bytes:", isinstance(zi.get_data("data.txt"), bytes))
print("get_data:", repr(zi.get_data("data.txt")))

# ZipImportError is an ImportError, so either catches it.
print("is ImportError:", issubclass(zipimport.ZipImportError, ImportError))

# find_module was removed in 3.10, and CPython 3.14 no longer has it.
print("has find_module:", hasattr(zi, "find_module"))
print("has load_module:", hasattr(zi, "load_module"))

# A path that is not a zip raises rather than pretending.
try:
    zipimport.zipimporter(os.path.join(os.path.dirname(archive), "not-a-zip.zip"))
except zipimport.ZipImportError as e:
    print("not a zip raises:", "not a Zip file" in str(e))

doc = "finished"
