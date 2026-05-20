#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

git add -A

if git diff --cached --quiet; then
  echo "No changes to commit."
  git commit -m "fix: issue #$ISSUE_NUMBER" --allow-empty
else
  git commit -m "fix: issue #$ISSUE_NUMBER"
fi

BRANCH_NAME=$(lichyflow get value branch_name)
git push origin "$BRANCH_NAME"

if [ $? -eq 0 ]; then
  lichyflow set flag pushed
else
  lichyflow unset flag pushed
  exit 1
fi
