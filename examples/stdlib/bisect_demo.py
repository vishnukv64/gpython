"""bisect: binary search on an already-sorted list.

Run with:  /tmp/gpy examples/stdlib/bisect_demo.py

bisect_left / bisect_right give the insertion point for a value; insort_left /
insort_right insert while keeping the list sorted. Everything is O(log n) to
find and O(n) to insert (list insertion moves elements).

Interpreter note: tuples and lists cannot be compared with < in this
interpreter, so all ordering here is done on plain numbers or on strings,
never on composite keys.
"""

import bisect

print("--- the list must already be sorted ---")
scores = [10, 20, 30, 40, 50]
print("scores:", scores)

print()
print("--- finding an insertion point for a missing value ---")
for value in [5, 25, 35, 55]:
    left = bisect.bisect_left(scores, value)
    right = bisect.bisect_right(scores, value)
    plain = bisect.bisect(scores, value)
    print("  value=%-3d left=%d right=%d bisect=%d" % (value, left, right, plain))

print()
print("--- left vs right differ on duplicates ---")
dups = [1, 2, 2, 2, 3]
print("list:        ", dups)
print("bisect_left: ", bisect.bisect_left(dups, 2), "(first index where 2 could go)")
print("bisect_right:", bisect.bisect_right(dups, 2), "(index just past the last 2)")
print("count of 2s: ", bisect.bisect_right(dups, 2) - bisect.bisect_left(dups, 2))

print()
print("--- insort keeps the list sorted ---")
pool = [1, 4, 7]
print("start:            ", pool)
bisect.insort(pool, 5)
print("after insort(5):  ", pool)
bisect.insort_right(pool, 4)
print("after insort_right(4):", pool)
bisect.insort_left(pool, 4)
print("after insort_left(4): ", pool)
print("still sorted:", pool == sorted(pool))

print()
print("--- a worked example: a sorted leaderboard with O(log n) lookup ---")


class Leaderboard:
    """Keeps scores sorted; names are looked up from a parallel list.

    CPython would store (score, name) tuples in one list. This interpreter
    cannot compare tuples with <, so the two lists are kept in step and the
    name for a position is read out by index instead.
    """

    def __init__(self):
        self.scores = []
        self.names = []

    def add(self, name, score):
        # Insert the score, then splice the matching name into the same slot.
        index = bisect.bisect_left(self.scores, score)
        self.scores.insert(index, score)
        self.names.insert(index, name)
        return self

    def ranked(self):
        """Highest score first."""
        rows = []
        for index in range(len(self.scores) - 1, -1, -1):
            rows.append((self.names[index], self.scores[index]))
        return rows

    def count_at_least(self, score):
        """How many entries have at least this score."""
        return len(self.scores) - bisect.bisect_left(self.scores, score)

    def rank_of(self, score):
        """1-based position, counting from the highest score down."""
        return len(self.scores) - bisect.bisect_left(self.scores, score)


board = Leaderboard()
for name, score in [("ann", 90), ("bob", 75), ("cat", 90), ("dan", 60), ("eve", 75)]:
    board.add(name, score)

print("names sorted by score:", board.names)
print("scores:               ", board.scores)
print("leaderboard:          ", board.ranked())
print("count score>=75:      ", board.count_at_least(75))
print("count score>=95:      ", board.count_at_least(95))
print("scores stay monotonic:", board.scores == sorted(board.scores))

print()
print("--- the family of functions ---")
print("bisect_left/bisect_right return an index;")
print("insort_left/insort_right insert in place.")
print("all six are available:", all([callable(f) for f in
      [bisect.bisect_left, bisect.bisect_right, bisect.bisect,
       bisect.insort_left, bisect.insort_right, bisect.insort]]))
