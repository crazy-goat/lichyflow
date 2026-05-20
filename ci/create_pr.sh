#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

BRANCH_NAME=$(lichyflow get value branch_name)
DEFAULT_BRANCH=$(lichyflow get value default_branch)

PR_URL=$(gh pr create \
  --base "$DEFAULT_BRANCH" \
  --head "$BRANCH_NAME" \
  --title "fix: issue #$ISSUE_NUMBER" \
  --body "Fixes #$ISSUE_NUMBER" \
  2>&1)

if [ $? -eq 0 ]; then
  echo "$PR_URL" > "$ARTIFACT_DIR/pr-url"
  lichyflow set flag pr_created
else
  lichyflow unset flag pr_created
  exit 1
fi
