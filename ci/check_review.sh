#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

if [ -s "$ARTIFACT_DIR/cr-issues.md" ]; then
  ISSUES=$(cat "$ARTIFACT_DIR/cr-issues.md")
  echo "Code review found issues:"
  echo "$ISSUES"
  lichyflow set flag cr_issues_found

  CURRENT=$(lichyflow get value review_loop)
  NEW=$((CURRENT + 1))
  lichyflow set value review_loop "$NEW"

  if [ "$NEW" -gt 3 ]; then
    echo "Review loop exceeded 3 iterations, giving up."
    lichyflow set flag loop_exhausted
  fi
else
  echo "Code review clean — no issues found."
  lichyflow unset flag cr_issues_found
fi
