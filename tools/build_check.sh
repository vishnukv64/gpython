#!/bin/bash
# Build gpython from a pristine HEAD checkout with only this task's files overlaid.
#
# Peers are editing py/ and the stdlib registration list in the shared working
# tree, so a build there is not a verdict on this code - and their codemods have
# also been rewriting files under stdlib/ mid-edit.  This harness therefore
# takes HEAD and overlays ONLY the task's three packages, from this task's
# authoritative copy in $SRC.
set -e
A=/Users/vishnukv/facets/codebases/gpython
SRC=/Users/vishnukv/.praxis/agent/sessions/01a0f728-4f0e-7b37-8ac9-65155b22cc91/subagents/workspaces/task-d7bc688b-a719-5f0a-9cba-b7af94d0c39c/workspace
W=/tmp/vkbuild

rm -rf "$W"
mkdir -p "$W"
git -C "$A" archive HEAD | tar -x -C "$W"

for pkg in pathlib socket unicodedata; do
    mkdir -p "$W/stdlib/$pkg"
    cp "$SRC"/stdlib/$pkg/*.go "$W/stdlib/$pkg/"
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
