import gc, io, os, secrets, tarfile, tempfile, timeit, unittest, random

# gc
assert gc.isenabled()
gc.disable(); assert not gc.isenabled(); gc.enable(); assert gc.isenabled()
assert isinstance(gc.collect(), int)
assert len(gc.get_count()) == 3 and len(gc.get_threshold()) == 3

# timeit
assert timeit.timeit('1+1', number=1000) >= 0
assert len(timeit.repeat('1+1', number=10, repeat=3)) == 3

# secrets / SystemRandom
assert len(secrets.token_hex(8)) == 16
assert len(secrets.token_bytes(5)) == 5
assert secrets.choice('abc') in 'abc'
assert 0 <= secrets.randbelow(10) < 10
assert secrets.compare_digest('a', 'a')
r = random.SystemRandom()
assert 0 <= r.random() < 1 and 1 <= r.randint(1, 6) <= 6
assert r.getrandbits(8) < 256

# os.path.commonprefix
print(os.path.commonprefix(['/usr/lib', '/usr/local']), repr(os.path.commonprefix([])))

# tarfile roundtrip
d = tempfile.mkdtemp()
src = os.path.join(d, 'a.txt')
with open(src, 'w') as f:
    f.write('hello')
t = os.path.join(d, 'x.tar.gz')
with tarfile.open(t, 'w:gz') as tf:
    tf.add(src, arcname='dir/a.txt')
with tarfile.open(t) as tf:
    print(tf.getnames())
    tf.extractall(os.path.join(d, 'out'))
with open(os.path.join(d, 'out', 'dir', 'a.txt')) as f:
    print(f.read())

# unittest: pass, skip, expected failure; fail/error via result objects
class T(unittest.TestCase):
    def test_ok(self):
        self.assertEqual(1 + 1, 2)
    @unittest.skip('nope')
    def test_skip(self): pass
    @unittest.expectedFailure
    def test_xfail(self): self.assertEqual(1, 2)
    def test_fail(self): self.assertEqual(1, 2)
    def test_error(self): raise ValueError('x')

class Ok(unittest.TestCase):
    def test_a(self): self.assertTrue(True)
    def test_b(self): self.assertIn(1, [1])

buf = io.StringIO()
res = unittest.TextTestRunner(stream=buf, verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(Ok))
print(res.wasSuccessful(), res.testsRun)
print(buf.getvalue().split('\nRan')[0].replace('\r', ''))

res = unittest.TextTestRunner(stream=io.StringIO()).run(unittest.defaultTestLoader.loadTestsFromTestCase(T))
print(res.testsRun, len(res.failures), len(res.errors), len(res.skipped), len(res.expectedFailures))
print(res.failures[0][1].splitlines()[-1])
print(res.errors[0][1].splitlines()[-1])
