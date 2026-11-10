from collections.abc import MutableSequence
class Item:
    def __init__(self, t): self.t = t
class CL(MutableSequence, list): pass
class P(list): pass
class Q(Item, list):
    def __init__(self, v):
        Item.__init__(self, 1); list.__init__(self, v)
class R(CL):
    def __init__(self, v): list.__init__(self, v)
class S(Item, CL):
    def __init__(self, v):
        Item.__init__(self, 1); list.__init__(self, v)
for K in (P, Q, R, S):
    try:
        x = K([1, 2]); print(K.__name__, list.__len__(x))
    except Exception as e:
        print(K.__name__, "ERR", e)
