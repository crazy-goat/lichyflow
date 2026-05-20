#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

gh issue view "$ISSUE_NUMBER" --json title,body,labels > "$ARTIFACT_DIR/issue.json" 2>/dev/null

if [ $? -eq 0 ]; then
  lichyflow set flag issue_fetched

  TITLE=$(gh issue view "$ISSUE_NUMBER" --json title -q '.title' 2>/dev/null || echo "issue-$ISSUE_NUMBER")
  SLUG=$(echo "$TITLE" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$//' | cut -c1-30)
  lichyflow set value issue_slug "$SLUG"
  lichyflow set value issue_title "$TITLE"
else
  lichyflow unset flag issue_fetched
  gh issue view "$ISSUE_NUMBER" 2>"$ARTIFACT_DIR/issue-error.txt" >/dev/null || true
fi
