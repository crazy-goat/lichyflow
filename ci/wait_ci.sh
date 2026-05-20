#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

BRANCH_NAME=$(lichyflow get value branch_name)

for i in $(seq 1 20); do
  STATUS=$(gh run list --branch "$BRANCH_NAME" --limit 1 --json conclusion 2>&1)
  if echo "$STATUS" | grep -q '"conclusion":"success"'; then
    lichyflow set flag ci_passed
    exit 0
  elif echo "$STATUS" | grep -q '"conclusion":"failure"'; then
    lichyflow unset flag ci_passed
    exit 1
  fi
  sleep 20
done

lichyflow unset flag ci_passed
exit 1
