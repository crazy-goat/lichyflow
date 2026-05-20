#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

DEFAULT_BRANCH=$(lichyflow get value default_branch)
SLUG=$(lichyflow get value issue_slug)

BRANCH_NAME="fix/${ISSUE_NUMBER}-${SLUG}"
git checkout -b "$BRANCH_NAME"
lichyflow set value branch_name "$BRANCH_NAME"
