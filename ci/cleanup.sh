#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

DEFAULT_BRANCH=$(lichyflow get value default_branch)
BRANCH_NAME=$(lichyflow get value branch_name)

git checkout "$DEFAULT_BRANCH"
git branch -D "$BRANCH_NAME"
git push origin --delete "$BRANCH_NAME" 2>/dev/null || true
git pull origin "$DEFAULT_BRANCH"
