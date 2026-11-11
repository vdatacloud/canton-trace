#!/usr/bin/env bash
# next-version.sh -- prints the next semver tag for this module from the
# Conventional Commits since the latest v* tag, or prints nothing if no
# release is warranted. Used by .github/workflows/release.yml; run it locally
# to preview what a merge to main would release. Rules (RELEASING.md):
#
#   type!: / any "BREAKING CHANGE:" footer   -> minor (breaks a consumer: another
#                                               repo, the website); never major
#   feat: / any "Deprecated:" footer         -> minor (substantial feature, deprecation)
#   fix: / perf: / refactor: / revert: /     -> patch (small change or bug fix)
#   build: / chore(deps):
#   only chore/docs/test/ci/style            -> no release
#
#
# Majors are never automatic: they are cut by hand -- see RELEASING.md.
#
# Usage: scripts/next-version.sh [range-end]   (default HEAD)
set -euo pipefail

END="${1:-HEAD}"
LAST_TAG="$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' "$END" 2>/dev/null || true)"

if [ -z "$LAST_TAG" ]; then
  RANGE="$END"
  MAJOR=0 MINOR=0 PATCH=0
else
  RANGE="${LAST_TAG}..${END}"
  IFS=. read -r MAJOR MINOR PATCH <<<"${LAST_TAG#v}"
fi

# Full messages (subject + body) so footers are seen too. -z separates
# commits with NUL only -- a "%B%x00" format leaves git's own newline
# between entries, so every message after the first started with a blank
# line and its subject read as empty (missed the feat: under #8's merge).
bump=none
while IFS= read -r -d '' msg; do
  subject="${msg%%$'\n'*}"
  if [[ "$subject" =~ ^[a-z]+(\([^\)]*\))?!: ]] || grep -q '^BREAKING[ -]CHANGE:' <<<"$msg"; then
    bump=minor; break
  elif [[ "$subject" =~ ^feat(\([^\)]*\))?: ]] || grep -qi '^DEPRECATED:' <<<"$msg"; then
    bump=minor
  elif [[ "$subject" =~ ^((fix|perf|refactor|revert|build)(\([^\)]*\))?|chore\(deps\)): ]] && [ "$bump" = none ]; then
    bump=patch
  fi
done < <(git log -z --format='%B' "$RANGE")

case "$bump" in
  minor) echo "v${MAJOR}.$((MINOR + 1)).0" ;;
  patch) echo "v${MAJOR}.${MINOR}.$((PATCH + 1))" ;;
  none)  : ;;
esac
