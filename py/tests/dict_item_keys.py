d = {'items': {'a': 1}, 'keys': 1, 'get': 2, 'x': 3}
print(list(d.items()), list(d.keys()), d.get('x'), d['items'])
class D(dict): pass
e = D(items=1, values=2)
print(list(e.items()), list(e.values()))
