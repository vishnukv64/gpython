# Copyright 2022 The go-python Authors.  All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import time

now = time.time()
now = time.time_ns()
now = time.clock()

def notimplemented(fn, *args, **kwargs):
    try:
        fn(*args, **kwargs)
        print("error for %s(%s, %s)" % (fn,args,kwargs))
    except NotImplementedError:
        pass

notimplemented(time.clock_gettime)
notimplemented(time.clock_settime)

print("# sleep")
time.sleep(0.1)
try:
    time.sleep(-1)
    print("no error sleep(-1)")
except ValueError as e:
    print("caught error: %s" % (e,))
    pass
try:
    time.sleep("1")
    print("no error sleep('1')")
except TypeError as e:
    print("caught error: %s" % (e,))
    pass

notimplemented(time.gmtime)
notimplemented(time.localtime)
notimplemented(time.asctime)
notimplemented(time.ctime)
notimplemented(time.mktime, 1)
notimplemented(time.strftime)
notimplemented(time.strptime)
notimplemented(time.tzset)
# These three are real clocks now, so they return a float rather than
# raising.  Each must advance (or hold still) rather than go backwards.
_a = time.perf_counter()
_b = time.perf_counter()
print("perf_counter: float=%s monotonic=%s" % (isinstance(_a, float), _b >= _a))
print("monotonic: float=%s" % isinstance(time.monotonic(), float))
# process_time is CPU time, so a sleep must NOT move it.
_p = time.process_time()
time.sleep(0.02)
print("process_time: float=%s ignores sleep=%s"
      % (isinstance(_p, float), time.process_time() - _p < 0.02))
notimplemented(time.get_clock_info)

print("OK")
