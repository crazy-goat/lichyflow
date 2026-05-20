#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

PR_URL=$(cat "$ARTIFACT_DIR/pr-url")
PR_NUMBER=$(echo "$PR_URL" | grep -o '[0-9]*$')

gh pr merge "$PR_NUMBER" --squash

if [ $? -eq 0 ]; then
  lichyflow set flag pr_merged
else
  lichyflow unset flag pr_merged
  exit 1
fi
