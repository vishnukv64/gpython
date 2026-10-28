import os
from os import path as p
class Base:
    x = 1
    def m(self): pass
    async def am(self): pass
    class Inner:
        def deep(self): pass
class Derived(Base, object):
    @property
    def prop(self): return 1
    def n(self): pass
def func(a, b=2):
    def nested(): pass
    return a
async def afunc(): pass
