# The behaviour of shutil, checked through the interpreter.
#
# This script uses only the os surface this interpreter actually has: there is
# no os.chmod, os.symlink, os.islink or os.stat here, so the executable-bit and
# symlink cases are covered from the Go test, which can create such things.
import os
import shutil
import tempfile

# ---- Error and SameFileError really are OSError subclasses, which is how
# callers catch them.
assert issubclass(shutil.Error, OSError), shutil.Error.__mro__
assert issubclass(shutil.SameFileError, shutil.Error), shutil.SameFileError.__mro__

# ---- which finds a binary that is definitely on PATH
found = shutil.which('ls')
assert found is not None, "which('ls') found nothing on PATH"
assert os.path.isabs(found), found
assert os.path.exists(found), found
assert not os.path.isdir(found), found

# A command that is definitely not there answers None rather than raising.
assert shutil.which('definitely-not-a-command-xyzzy') is None

# An explicit path argument overrides PATH.
assert shutil.which('ls', path='/bin') is not None or shutil.which('ls', path='/usr/bin') is not None
assert shutil.which('ls', path='/definitely-not-here') is None
# An empty PATH matches nothing at all, unlike PATH=':'.
assert shutil.which('ls', path='') is None
# A directory is never a result, even though /usr contains bin/.
assert shutil.which('bin', path='/usr') is None, shutil.which('bin', path='/usr')

d = tempfile.mkdtemp(prefix='shutil-test-')
try:
    # ---- get_terminal_size always answers a 2-tuple of ints and honours the
    # fallback.
    size = shutil.get_terminal_size()
    assert len(size) == 2, size
    assert size.columns >= 1 and size.lines >= 1, size
    assert size[0] == size.columns and size[1] == size.lines, size
    fb = shutil.get_terminal_size((123, 45))
    assert fb.columns >= 1 and fb.lines >= 1, fb

    # ---- copyfileobj copies every byte
    src = os.path.join(d, 'a.txt')
    f = open(src, 'w')
    f.write('hello world')
    f.close()
    dstObj = os.path.join(d, 'b.txt')
    with open(src, 'r') as fin, open(dstObj, 'w') as fout:
        shutil.copyfileobj(fin, fout)
    got = open(dstObj).read()
    assert got == 'hello world', got

    # ---- copy/copy2 return the destination path and copy the contents
    dst2 = os.path.join(d, 'c.txt')
    res = shutil.copy(src, dst2)
    assert res == dst2, res
    assert open(dst2).read() == 'hello world'
    dst3 = os.path.join(d, 'd.txt')
    res2 = shutil.copy2(src, dst3)
    assert res2 == dst3, res2
    assert open(dst3).read() == 'hello world'

    # copy into an existing directory puts the file inside it
    into = os.path.join(d, 'into')
    os.mkdir(into)
    res3 = shutil.copy(src, into)
    assert res3 == os.path.join(into, 'a.txt'), res3
    assert os.path.exists(res3)

    # ---- copyfile refuses to copy a file onto itself
    try:
        shutil.copyfile(src, src)
    except shutil.SameFileError:
        pass
    else:
        raise AssertionError('copyfile onto itself did not raise SameFileError')
    # and the source survived
    assert open(src).read() == 'hello world'

    # ---- a missing source is a FileNotFoundError, by name
    try:
        shutil.copy(os.path.join(d, 'nope'), dst2)
    except FileNotFoundError:
        pass
    else:
        raise AssertionError('copy of a missing file did not raise FileNotFoundError')

    # ---- copytree builds a real tree
    tree = os.path.join(d, 'tree')
    os.mkdir(tree)
    os.mkdir(os.path.join(tree, 'sub'))
    f = open(os.path.join(tree, 'file.txt'), 'w')
    f.write('tree file')
    f.close()
    f = open(os.path.join(tree, 'sub', 'nested.txt'), 'w')
    f.write('nested')
    f.close()
    copyOfTree = os.path.join(d, 'tree-copy')
    out = shutil.copytree(tree, copyOfTree)
    assert out == copyOfTree, out
    assert open(os.path.join(copyOfTree, 'file.txt')).read() == 'tree file'
    assert open(os.path.join(copyOfTree, 'sub', 'nested.txt')).read() == 'nested'
    # copying over an existing destination is refused without dirs_exist_ok
    try:
        shutil.copytree(tree, copyOfTree)
    except FileExistsError:
        pass
    else:
        raise AssertionError('copytree onto an existing directory did not raise')

    # ---- move renames a file
    toMove = os.path.join(d, 'move-me.txt')
    f = open(toMove, 'w')
    f.write('move me')
    f.close()
    moved = shutil.move(toMove, os.path.join(d, 'moved.txt'))
    assert not os.path.exists(toMove), 'move left the source behind'
    assert open(moved).read() == 'move me'

    # and moves a whole tree
    movedTree = shutil.move(tree, os.path.join(d, 'tree-moved'))
    assert not os.path.exists(tree), 'tree move left the source behind'
    assert open(os.path.join(movedTree, 'sub', 'nested.txt')).read() == 'nested'

    # ---- rmtree really removes a tree
    doomed = os.path.join(d, 'doomed')
    os.mkdir(doomed)
    os.mkdir(os.path.join(doomed, 'inner'))
    f = open(os.path.join(doomed, 'inner', 'x.txt'), 'w')
    f.write('x')
    f.close()
    shutil.rmtree(doomed)
    assert not os.path.exists(doomed), 'rmtree left the tree behind'

    # a missing path raises, and ignore_errors silences it
    try:
        shutil.rmtree(os.path.join(d, 'not-here'))
    except OSError:
        pass
    else:
        raise AssertionError('rmtree of a missing path did not raise')
    shutil.rmtree(os.path.join(d, 'not-here'), ignore_errors=True)

    # ---- disk_usage answers real, non-negative numbers
    usage = shutil.disk_usage(d)
    assert usage.total > 0, usage
    assert usage.free >= 0 and usage.used >= 0, usage
    assert usage.total == usage.used + usage.free, (usage.used, usage.free, usage.total)
    assert usage[0] == usage.total and usage[1] == usage.used and usage[2] == usage.free

    # make_archive is deliberately absent, so attribute access fails loudly
    # rather than calling something that is not there.
    assert not hasattr(shutil, 'make_archive'), 'make_archive should be absent'
    try:
        shutil.make_archive
    except AttributeError:
        pass
    else:
        raise AssertionError('shutil.make_archive unexpectedly exists')
finally:
    shutil.rmtree(d, ignore_errors=True)

print("shutil ok")
