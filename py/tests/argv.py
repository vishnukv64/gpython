"""sys.argv always has at least one entry, and argv[0] names the program."""

import sys

# The VALUE of argv[0] depends on how the program was started - a script path,
# "-c", a module name, or the test binary - so only its shape is asserted here.
# The exact forms are covered by pip, which reads argv[0] to name itself in its
# own usage line, and by the -c and -m probes.
print("has at least one entry:", len(sys.argv) >= 1)
print("argv[0] is a string:", isinstance(sys.argv[0], str))
print("argv is a list:", isinstance(sys.argv, list))

doc = "finished"
