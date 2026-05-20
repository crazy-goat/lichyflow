#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

SESSION_FILE="$ARTIFACT_DIR/pi-session.jsonl"
MODEL="${LLM_MODEL:-go-extra/deepseek-v4-flash}"
THINKING="${LLM_THINKING:-high}"

# ═══════════════════════════════════════════════════════════════════════
# Phase 1 — Fix: analyse issue, edit code, run tests, stage, commit
# ═══════════════════════════════════════════════════════════════════════
echo "=== Phase 1: Fix ==="
pi-stream \
  --session "$SESSION_FILE" \
  --model "$MODEL" \
  --thinking "$THINKING" \
  -t read,edit,write,bash,grep,find,ls \
  "You are fixing issue #$ISSUE_NUMBER in lichyflow (Go project).
   Read the issue from $ARTIFACT_DIR/issue.json.
   Analyze the code, make necessary changes, and run tests to verify.
   When tests pass, stage the changed files (git add) and commit them
   with a descriptive message. Use bash for go test, git add, git commit."

EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "pi-stream fix phase exited with code $EXIT_CODE"
  exit $EXIT_CODE
fi

# ═══════════════════════════════════════════════════════════════════════
# Phase 2 — Double-check: are you sure you didn't forget anything?
# ═══════════════════════════════════════════════════════════════════════
echo "=== Phase 2: Double-check ==="
pi-stream \
  --session "$SESSION_FILE" \
  --model "$MODEL" \
  --thinking "$THINKING" \
  -t read,bash,grep,find,ls \
  "Now double-check your work from Phase 1 — be your own code reviewer:

   1. Run 'git status' and 'git log -1 --stat' to see what was changed
      and committed.
   2. Did you forget anything? Are there leftover TODO comments,
      debug prints, or temporary files that should be cleaned up?
   3. Run 'go build ./...' and 'go vet ./...' — does it still compile
      cleanly with no warnings?
   4. Is there anything in git status (untracked or unstaged) that
      should have been part of the fix? If so, amend the commit.
   5. If you find issues: fix them and amend the commit.
      If everything looks good: just confirm with a short summary.

   This is your last chance to catch mistakes before code review."

EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "pi-stream double-check exited with code $EXIT_CODE"
  exit $EXIT_CODE
fi

# ═══════════════════════════════════════════════════════════════════════
# Final: did anything change in the repo?
# ═══════════════════════════════════════════════════════════════════════
if git diff --quiet && git diff --cached --quiet; then
  lichyflow unset flag changes_made
  echo "No changes were made."
else
  lichyflow set flag changes_made
  echo "Changes are ready."
fi

# Check whether there's at least one commit on the branch vs default
DEFAULT_BRANCH=$(lichyflow get value default_branch)
if git log "origin/$DEFAULT_BRANCH..HEAD" --oneline | grep -q .; then
  lichyflow set flag has_commits
  echo "Commits found on branch."
else
  lichyflow unset flag has_commits
  echo "No new commits on branch."
fi
