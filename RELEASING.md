# Releasing

Merging to `main` releases automatically (`.github/workflows/release.yml`).
`scripts/next-version.sh` reads the Conventional Commits since the last `v*`
tag and picks the bump:

| Commits since the last tag | Release |
| --- | --- |
| `type!:` or a `BREAKING CHANGE:` footer | minor (pre-1.0, a break is a minor) |
| `feat:` or a `Deprecated:` footer | minor |
| `fix:`, `perf:`, `refactor:`, `revert:`, `build:`, `chore(deps):` | patch |
| only `chore`, `docs`, `test`, `ci`, `style` | none |

The workflow rebuilds and tests, then creates the tag and a GitHub release
with generated notes. Preview the next version locally with
`bash scripts/next-version.sh`.

The module stays `v0.x` until its API is settled. A `v1.0.0` is tagged by
hand.
