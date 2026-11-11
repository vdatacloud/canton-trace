#!/usr/bin/env bash
# Regression tests for scripts/next-version.sh, run by CI. Builds throwaway
# repos shaped like real main history -- the deciding commit buried under
# PR merge commits and [skip ci] chores, which is what the first
# version of the script got wrong (#8 merged but released nothing).
set -euo pipefail
SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/next-version.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"
git init -q
git config user.email ci@example.com
git config user.name ci
git config commit.gpgsign false
git config tag.gpgsign false

c() { git commit -q --allow-empty -m "$1"; }
tg() { git tag -a "$1" -m "$1"; }
fails=0
expect() {
  local got; got="$(bash "$SCRIPT")"
  if [ "$got" = "$2" ]; then echo "ok   $1 -> '${got}'"; else echo "FAIL $1: got '${got}', want '$2'"; fails=$((fails + 1)); fi
}

c "chore: init";                          expect "no tags, chore only" ""
c "feat: first";                           expect "no tags, feat" "v0.1.0"
tg v0.2.0
c "feat(metering): x"; c "Merge pull request #8 from a/b"; c "chore: graph [skip ci]"
expect "feat under merge + chore" "v0.3.0"
c "fix: y"; c "Merge pull request #9"; expect "feat + later fix" "v0.3.0"
tg v0.3.0
c "fix: z"; c "Merge pull request #10";  expect "fix under merge" "v0.3.1"
c "perf: p";                               expect "perf" "v0.3.1"
c "$(printf 'chore: a\n\nBREAKING CHANGE: b')"; c "Merge pull request #11"
expect "BREAKING footer under merge is minor" "v0.4.0"
tg v0.4.0
c "docs: d"; c "ci: e"; c "Merge pull request #12"; c "chore: graph [skip ci]"
expect "docs/ci/chore/test/style only" ""
c "test: t"; c "style: s"; expect "test/style only" ""
c "refactor: r"; expect "refactor is a small change" "v0.4.1"
tg v0.4.1
c "chore(deps): bump x"; expect "dependency bump" "v0.4.2"
tg v0.4.2
c "build: b"; expect "build" "v0.4.3"
tg v0.4.3
c "$(printf 'fix: old flag\n\nDeprecated: --legacy, use --mode')"; c "Merge pull request #14"
expect "Deprecated footer is minor" "v0.5.0"
tg v0.5.0
c "chore(deps)!: drop node 20"; expect "breaking deps bump is minor" "v0.6.0"
tg v2.3.4
c "fix(api)!: drop field"; c "Merge pull request #13"; expect "breaking after a 2.x product release is still minor" "v2.4.0"

[ "$fails" -eq 0 ] || { echo "$fails next-version case(s) failed"; exit 1; }
echo "all next-version cases passed"
