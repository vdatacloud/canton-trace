---
name: github-ci
description: >
  Manages the full GitHub workflow for the canton-trace project: branching, signed commits, pull request creation via `gh`, CI pipeline monitoring, and local pre-push verification. Activate whenever the user wants to create a branch, stage changes, commit, open a PR, check CI status, fix a failing CI job, or review the merge checklist for this project.
---

# GitHub & CI Workflow Skill

This skill governs every interaction with source control and the GitHub Actions CI pipeline for the `canton-trace` project.
Always follow these rules in full, in the order given.

**Policy authority:** `.gemini/repo_rules.md` is the canonical source for
branching/signing/PR/review policy. This skill covers the operational
commands (git/gh invocations, CI failure triage) that policy doesn't spell
out — if the two ever disagree, `repo_rules.md` wins and this file should be
updated to match.

---

## 1. Pre-Commit Local Verification (MANDATORY — do this before every commit)

```bash
make verify
# equivalent to: go build ./... && go vet ./... && go test ./...
```

This is also what the `pre-push` git hook (installed via `make install-hooks`)
runs automatically before allowing a push. **Do NOT commit or push until it
passes.**

Use `go test -v -run TestName ./...` to isolate a single failing test.

---

## 2. Branching Rules

`main` is the only long-lived branch. See `.gemini/repo_rules.md` (canonical).

| Work type | Branch prefix | Example |
|-----------|---------------|---------|
| New feature | `feature/` | `feature/add-idempotency-key-package` |
| Bug fix | `fix/` | `fix/hmacsig-timing-comparison` |
| Docs-only | `docs/` | `docs/update-readme` |
| Tooling / CI config | `chore/` | `chore/bump-golangci-lint` |
| Security patch | `sec/` | `sec/pin-gojsonschema-cve` |

- **NEVER push directly to `main`.**
- Bug-fix direct pushes to `main` require **explicit user confirmation** before executing.

```bash
git checkout main
git pull origin main
git checkout -b feature/<short-description>
```

---

## 3. Commit Standards

### Mandatory Flags

| Flag | Purpose |
|------|---------|
| `-s` / `--signoff` | Developer Certificate of Origin (DCO) compliance |
| `-S` | GPG signature (only "Verified" commits merge to `main`) — redundant if `git config commit.gpgsign true` is already set globally, but pass it explicitly for portability |

```bash
git commit -S -s -m "feat: add idempotency-key package"
```

### Conventional Commit Prefixes

```
feat:     new feature or capability
fix:      bug fix
refactor: code change with no behaviour delta
chore:    build tooling, dependencies, CI config
docs:     documentation only
test:     adding or updating tests
sec:      security vulnerability remediation
```

- Use **imperative mood**: "add X" not "added X".
- Keep the subject line <= 72 characters.
- Add a blank line + body for anything non-trivial.

---

## 4. Staging & Pushing Changes

```bash
git status
git diff --stat

# Stage selectively — never use `git add .` blindly
git add <specific-files>
git diff --cached --stat

git commit -S -s -m "<type>: <description>"
git push origin feature/<short-description>
```

---

## 5. Pull Request Creation

```bash
gh pr create \
  --base main \
  --title "<type>: <short description>" \
  --body "$(cat <<'EOF'
## Summary
<!-- One paragraph: what changed and why -->

## What's Affected
<!-- Which package(s)? Does this belong here at all -- see .claude/skills/commons-contribution/SKILL.md's promotion checklist -->

## Changes Made
<!-- Bullet list of specific files and what changed -->

## Test Evidence
- [ ] make verify — all passing (build, vet, unit tests)

## Security Considerations
<!-- This module is a dependency of every consumer repo's build -- a bad change here breaks all of them at once -->
EOF
)"
```

---

## 6. CI Pipeline Overview

Defined in `.github/workflows/ci.yml`.

| Job | Runner | What it does |
|-----|--------|-------------|
| `lint` | ubuntu-latest | `golangci-lint` on Go source |
| `unit-tests` | ubuntu-latest | `go build ./...` then `go test -v ./...` |

No integration-test job — this module has zero network/Docker dependencies
by design.

---

## 7. Monitoring CI After PR Creation

```bash
gh pr checks <pr-number>
gh run view --log-failed
gh pr checks <pr-number> --watch
```

**Failure handling loop:**
1. Pull failing logs: `gh run view <run-id> --log-failed`
2. Diagnose root cause (lint error / test failure / build error).
3. Fix locally -> `make verify` -> commit fix -> push.
4. Repeat until `gh pr checks <pr-number>` exits 0 with all jobs green.

---

## 8. Common CI Failure Patterns & Fixes

### Go lint failure
```bash
golangci-lint run ./...
golangci-lint run --fix ./...
```

### Go test failure
```bash
go test -v -count=1 ./...
go test -v -run TestFunctionName ./schema/...
```

### go.mod Go-version drift
This module pins `go 1.25.0` in `go.mod` to match CI's `setup-go` version
(and every consumer repo's own pin). If `go mod tidy` bumps it to your local
toolchain version, revert to `1.25.0` before committing — a known recurring
failure class in this family of repos (see `RELEASING.md`).

---

## 9. Merge Checklist

- [ ] `make verify` passes locally
- [ ] All CI jobs green (`gh pr checks <pr-number>`)
- [ ] PR description has all four required sections (Section 5)
- [ ] No secrets or credentials in any committed file
- [ ] At least 1 reviewer assigned
- [ ] Commits are GPG-signed (`git log --show-signature -1`)
- [ ] DCO sign-off present on every commit (`git log --oneline` shows `Signed-off-by:`)
- [ ] If this is a new package or breaking change: does `RELEASING.md`'s versioning guidance apply?

---

## 10. Useful Reference Commands

```bash
git status && git log --oneline -5
git log --show-signature -1
gh pr list
gh pr view <pr-number>
git add <files> && git commit -S -s -m "fix: ..."
git push origin $(git branch --show-current)
git fetch origin && git rebase origin/main
```

---

## 11. References

- `.github/workflows/ci.yml` — CI pipeline definition
- `.gemini/repo_rules.md` — Repository guardrails (canonical policy, incl. branch protection strategy)
- `.claude/skills/commons-contribution/SKILL.md` — what belongs in this repo vs. a consumer repo
- `RELEASING.md` — manual semantic-release workflow for tagging real versions
