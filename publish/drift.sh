#!/usr/bin/env bash
# Drift between the two engines since the fork: commits that changed one engine's Go code and not
# the other's — each is a fix or feature that may need porting. An instrument (Nature 9): it
# reports, it never ports anything itself. Exit 0 when nothing drifts, 1 when something does.
#
#   publish/drift.sh            # since the fork commit named in agent-one/engine/FORKED.md
#   publish/drift.sh <commit>   # since an explicit commit
set -euo pipefail
cd "$(dirname "$0")/.."
BASE=${1:-$(grep -oE '\b[0-9a-f]{7,40}\b' agent-one/engine/FORKED.md 2>/dev/null | head -1 || true)}
[ -n "$BASE" ] || { echo "no fork commit: pass one, or record it in agent-one/engine/FORKED.md" >&2; exit 2; }

one_sided() { # $1 = engine that changed, $2 = engine that did not
  git log --format='%h %s' "$BASE..HEAD" -- "$1" | while read -r sha subject; do
    if ! git show --name-only --format= "$sha" | grep -q "^$2/"; then
      printf '  %s  %s\n' "$sha" "$subject"
    fi
  done
}

a=$(one_sided isekai agent-one/engine)
b=$(one_sided agent-one/engine isekai)
if [ -z "$a$b" ]; then echo "no drift since $BASE"; exit 0; fi
[ -n "$a" ] && { echo "changed in isekai/ only (port to agent-one/engine?):"; echo "$a"; }
[ -n "$b" ] && { echo "changed in agent-one/engine/ only (port to isekai?):"; echo "$b"; }
exit 1
