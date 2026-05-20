#!/usr/bin/env bash
set -euo pipefail

WORK_DIR="${WORK_DIR:-.}"
mkdir -p "$WORK_DIR"

if [ ! -d "$WORK_DIR/.git" ]; then
  git clone "$REPO_URL" "$WORK_DIR" 2>&1
fi

cd "$WORK_DIR"
git fetch origin
git checkout "$REPO_DEFAULT_BRANCH"
git reset --hard "origin/$REPO_DEFAULT_BRANCH"
git pull origin "$REPO_DEFAULT_BRANCH"

DEFAULT_BRANCH=$(git branch --show-current)
lichyflow set value default_branch "$DEFAULT_BRANCH"
lichyflow set value test_loop 0
lichyflow set value review_loop 0
