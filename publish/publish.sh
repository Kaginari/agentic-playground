#!/usr/bin/env bash
# Export one distribution from this world (the source of truth) into its own self-contained
# repository, stamped with the release templates, and optionally push it.
#
#   publish/publish.sh isekai     [--push] [--tag vX.Y.Z]
#   publish/publish.sh agent-one  [--push] [--tag vX.Y.Z]
#
# Without --push it only builds the export in publish/out/<dist>/ and runs every check there:
# module rewrite, gofmt, vet, tests, selftest, goreleaser check, and (agent-one) the vocabulary
# leak check. --push commits the export on top of the remote's history (never rewrites it) and
# pushes main; --tag also tags and pushes the tag, which triggers the release workflow.
set -euo pipefail

DIST=${1:?usage: publish.sh isekai|agent-one [--push] [--tag vX.Y.Z]}; shift
PUSH=0; TAG=""
while [ $# -gt 0 ]; do
  case "$1" in
    --push) PUSH=1 ;;
    --tag) TAG=${2:?--tag needs a version}; shift ;;
    *) echo "unknown flag $1" >&2; exit 2 ;;
  esac
  shift
done

SRC=$(cd "$(dirname "$0")/.." && pwd)
OWNER=Kaginari
case "$DIST" in
  isekai)    REPO=isekai;    TITLE=Isekai;    ENGINE="$SRC/isekai";           OLD_MODULE=github.com/Kaginari/agentic-playground/isekai ;;
  agent-one) REPO=agent-one; TITLE=Agent-One; ENGINE="$SRC/agent-one/engine"; OLD_MODULE=github.com/Kaginari/agent-one ;;
  *) echo "unknown distribution $DIST" >&2; exit 2 ;;
esac
NEW_MODULE=github.com/$OWNER/$REPO
OUT="$SRC/publish/out/$DIST"
GO=${GO:-$(command -v go || echo "$HOME/.local/go-current/bin/go")}

say() { printf '\n== %s\n' "$*"; }

say "clone $OWNER/$REPO"
rm -rf "$OUT" && mkdir -p "$(dirname "$OUT")"
gh repo clone "$OWNER/$REPO" "$OUT" -- --quiet
find "$OUT" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +

say "engine → repository root (module $NEW_MODULE)"
[ -d "$ENGINE" ] || { echo "no engine at $ENGINE" >&2; exit 1; }
rsync -a --exclude bin/ --exclude 'cmd/*' --exclude FORKED.md "$ENGINE/" "$OUT/"
mkdir -p "$OUT/cmd" && rsync -a "$ENGINE/cmd/$DIST/" "$OUT/cmd/$DIST/"
if [ "$OLD_MODULE" != "$NEW_MODULE" ]; then
  grep -rl --include='*.go' --include=go.mod "$OLD_MODULE" "$OUT" | xargs -r sed -i "s#$OLD_MODULE#$NEW_MODULE#g"
fi

say "law, docs, portraits"
if [ "$DIST" = isekai ]; then
  mkdir -p "$OUT/docs/canon" "$OUT/portraits"
  cp "$SRC/.isekai/isekai.md" "$OUT/ISEKAI.md"
  rsync -a --exclude agents/ "$SRC/.isekai/canon/" "$OUT/docs/canon/"
  cp "$SRC/.isekai/portraits/"*.png "$OUT/portraits/"
else
  cp "$SRC/agent-one/AGENT-ONE.md" "$OUT/AGENT-ONE.md"
  rsync -a "$SRC/agent-one/portraits/" "$OUT/portraits/"
fi
cp "$SRC/.isekai/LICENSE" "$OUT/LICENSE"
mkdir -p "$OUT/docs/screens" && cp "$SRC/docs/screens/$DIST-"*.png "$OUT/docs/screens/"

say "bench"
mkdir -p "$OUT/bench"
rsync -a --exclude jobs/ --exclude __pycache__/ --exclude '.smoke-config.yaml' --exclude '.fakevllm-*.log' --exclude runs.jsonl \
  --exclude RESULTS.md "$SRC/bench/" "$OUT/bench/"
# this distribution's launches only (validation launches of the shared task travel with both)
python3 - "$SRC/bench/runs.jsonl" "$OUT/bench/runs.jsonl" "$DIST" <<'EOF'
import json, sys
src, dst, dist = sys.argv[1:]
keep = [l for l in open(src) if l.strip() and json.loads(l).get("agent") in (dist, "oracle", "nop", "harbor_agent")]
open(dst, "w").writelines(keep)
EOF
PYTHONDONTWRITEBYTECODE=1 python3 - "$OUT/bench" <<'EOF'
import json, sys
sys.path.insert(0, sys.argv[1])
import record
runs = [json.loads(x) for x in open(sys.argv[1] + "/runs.jsonl") if x.strip()]
open(sys.argv[1] + "/RESULTS.md", "w").write(record.render(runs))
EOF

say "release templates"
rsync -a "$SRC/publish/template/" "$OUT/"
[ -f "$SRC/publish/$DIST/CHANGELOG.md" ] && cp "$SRC/publish/$DIST/CHANGELOG.md" "$OUT/CHANGELOG.md"
[ -f "$SRC/publish/$DIST/README.md" ] && cp "$SRC/publish/$DIST/README.md" "$OUT/README.md"
OWNER_LC=$(echo "$OWNER" | tr '[:upper:]' '[:lower:]')
grep -rlI -e __DIST__ -e __OWNER__ -e __REPO__ -e __TITLE__ -e __OWNER_LC__ -e __DATE__ "$OUT" --exclude-dir=.git \
  | xargs sed -i "s#__DIST__#$DIST#g; s#__OWNER_LC__#$OWNER_LC#g; s#__OWNER__#$OWNER#g; s#__REPO__#$REPO#g; s#__TITLE__#$TITLE#g; s#__DATE__#$(date +%Y-%m-%d)#g"
chmod +x "$OUT/install.sh" "$OUT/bench/smoke.sh" "$OUT/bench/record.py"
cat > "$OUT/.gitignore" <<'EOF'
bin/
dist/
bench/jobs/
bench/.smoke-config.yaml
bench/.fakevllm-*.log
__pycache__/
EOF

say "checks"
(
  set -euo pipefail
  cd "$OUT"
  "$GO" mod tidy
  unformatted=$(gofmt -l .); [ -z "$unformatted" ] || { echo "gofmt: $unformatted" >&2; exit 1; }
  "$GO" vet ./...
  "$GO" test ./... > "$OUT.test.log" 2>&1 || { grep -v '^ok\|no test files' "$OUT.test.log" >&2; exit 1; }
  "$GO" run "./cmd/$DIST" selftest
)
grep -q '__HIGHLIGHTS__' "$OUT/CHANGELOG.md" && { echo "CHANGELOG.md still has the __HIGHLIGHTS__ placeholder — write publish/$DIST/CHANGELOG.md" >&2; exit 1; }
docker run --rm -v "$OUT:/w" -w /w goreleaser/goreleaser:latest check

if [ "$DIST" = agent-one ]; then
  say "vocabulary leak check (every file, and the CLI output)"
  TERMS='isekai|rimuru|veldora|slime|kijin|dark[ -]elf|high[ -]orc|high[ -]elf|\borcs?\b|\belf\b|\belves\b|great[ -]sage|raphael|\bciel\b|tempest|reincarnat'
  leaks=$(grep -rniIE "$TERMS" "$OUT" --exclude-dir=.git --exclude='*.png' || true)
  cli=$(cd "$OUT" && "$GO" run ./cmd/agent-one help 2>&1 | grep -niE "$TERMS" || true)
  if [ -n "$leaks$cli" ]; then echo "$leaks"; echo "$cli"; echo "vocabulary leak — fix before publishing" >&2; exit 1; fi
  echo "no isekai vocabulary in docs, law, bench or CLI help"
fi

if [ "$PUSH" = 1 ]; then
  say "push"
  cd "$OUT"
  git add -A
  git commit -q -m "Publish $TITLE from convention-jura@$(git -C "$SRC" rev-parse --short HEAD)"
  git push origin HEAD:main
  if [ -n "$TAG" ]; then git tag -a "$TAG" -m "$TAG" && git push origin "$TAG"; fi
else
  say "dry run — export ready at $OUT (nothing pushed)"
fi
