from itertools import product

print(list(product()), list(product([])), list(product([1, 2], [])), list(product([1, 2], [3])))
print(list(product([], repeat=2)), list(product(repeat=0)), list(product("ab", repeat=2)))
