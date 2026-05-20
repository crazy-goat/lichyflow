#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

PR_URL=$(cat "$ARTIFACT_DIR/pr-url")
PR_NUMBER=$(echo "$PR_URL" | grep -o '[0-9]*$')

gh issue close "$ISSUE_NUMBER" --comment "Fixed in #$PR_NUMBER"
