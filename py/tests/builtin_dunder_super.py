class L(list):
    def __len__(self): return list.__len__(self) + 100
    def __getitem__(self, i): return list.__getitem__(self, i) * 2
    def __iter__(self): return list.__iter__(self)
    def __contains__(self, x): return list.__contains__(self, x)
    def __add__(self, o): return list.__add__(self, o)
a = L([1, 2, 3])
print(len(a), a[0], list(a), 2 in a, a + [4])
class T(tuple):
    def __len__(self): return tuple.__len__(self) + 1
    def __add__(self, o): return tuple.__add__(self, o)
t = T((1, 2))
print(len(t), t + (3,))
class S(str):
    def __len__(self): return str.__len__(self) * 2
print(len(S("abc")))
