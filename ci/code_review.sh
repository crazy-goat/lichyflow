#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

MODEL="${LLM_MODEL:-go-extra/deepseek-v4-flash}"
THINKING="${LLM_THINKING:-high}"

pi-stream \
  --session "$ARTIFACT_DIR/pi-review-session.jsonl" \
  --model "$MODEL" \
  --thinking "$THINKING" \
  -t read,bash,grep,find,ls \
  "Review the code changes in this Go project (lichyflow).
   If you find bugs, style issues, or security problems,
   write them to $ARTIFACT_DIR/cr-issues.md.
   If everything looks good, do not create that file."

EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "pi-stream review exited with code $EXIT_CODE"
  exit $EXIT_CODE
fi
