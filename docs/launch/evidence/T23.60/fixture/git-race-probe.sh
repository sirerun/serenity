#!/bin/zsh
# Scratch probe for the per-write Git failure (see per-write-run-failure.txt). It did not reproduce it.
# usage: git-race-probe.sh <new scratch dir> <commits>. Run six copies in parallel in different directories.
# Per-write commits shaped like the fixture's (two files per fact under sharded directories), HEAD checked after each.
r=$1; n=$2; rm -rf "$r"; git init -q "$r"; cd "$r"; git config user.name t; git config user.email t@t; git commit -q --allow-empty -m base
fails=0
for i in $(seq 1 $n); do
  s=$(printf '%02x' $((i % 256))); d=brain/sources/$s/$(printf '%064d' $i); mkdir -p $d
  echo "fact $i $RANDOM $RANDOM $RANDOM $RANDOM" > $d/bytes; echo "id: $i" > $d/meta.yaml
  git add -A -- brain >/dev/null 2>&1 || { echo "add failed at $i"; fails=$((fails+1)); break; }
  git commit -q -m sync 2>/dev/null || { echo "commit failed at $i"; fails=$((fails+1)); break; }
  git rev-parse --verify -q 'HEAD^{commit}' >/dev/null 2>&1 || { echo "HEAD unreadable right after commit $i"; fails=$((fails+1)); break; }
done
echo "done commits=$i fails=$fails fsck=$(git fsck --no-dangling 2>&1 | head -1 | cut -c1-80)"
