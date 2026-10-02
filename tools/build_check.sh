#!/bin/bash
# Build gpython from a pristine HEAD checkout with only this task's files overlaid.
#
# Peers are editing py/ in the shared working tree, so a build there is not a
# verdict on this code.  This harness takes HEAD, copies in stdlib/pathlib,
# stdlib/socket and stdlib/unicodedata plus the three registration lines in
# stdlib/stdlib.go, and builds that.
set -e
A=/Users/vishnukv/facets/codebases/gpython
W=/tmp/vkbuild

rm -rf "$W"
mkdir -p "$W"
git -C "$A" archive HEAD | tar -x -C "$W"

for pkg in pathlib socket unicodedata; do
    mkdir -p "$W/stdlib/$pkg"
    cp "$A"/stdlib/$pkg/*.go "$W/stdlib/$pkg/"
done

# Insert the three registration lines after the final existing blank import,
# which is how the module list is kept sorted in stdlib.go.
python3 - "$W/stdlib/stdlib.go" <<'PY'
import sys, re
path = sys.argv[1]
src = open(path).read()
if 'stdlib/pathlib' not in src:
    lines = src.split('\n')
    # Find the last blank import of a stdlib package and insert after it.
    last = max(i for i, l in enumerate(lines)
               if re.match(r'\s*_ "github\.com/vishnukv64/gpython/stdlib/', l))
    add = ['\t_ "github.com/vishnukv64/gpython/stdlib/pathlib"',
           '\t_ "github.com/vishnukv64/gpython/stdlib/socket"',
           '\t_ "github.com/vishnukv64/gpython/stdlib/unicodedata"']
    lines[last + 1:last + 1] = add
    open(path, 'w').write('\n'.join(lines))
    print("inserted registration lines after line", last + 1)
else:
    print("registration lines already present")
PY

cd "$W"
gofmt -l stdlib/pathlib stdlib/socket stdlib/unicodedata
go build -o /tmp/gpyvk .
echo "built /tmp/gpyvk from HEAD + task files"
