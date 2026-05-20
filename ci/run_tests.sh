#!/usr/bin/env bash
set -euo pipefail
cd "${WORK_DIR:-.}"

CURRENT=$(lichyflow get value test_loop)
NEW=$((CURRENT + 1))
lichyflow set value test_loop "$NEW"

if [ "$NEW" -gt 5 ]; then
  echo "Test loop exceeded 5 iterations, giving up."
  lichyflow unset flag tests_passed
  lichyflow set flag loop_exhausted
  exit 1
fi

echo "=== Test loop iteration $NEW ==="

# ── Build check ──────────────────────────────────────────────────────
echo "--- Checking build ---"
if ! go build ./... > "$ARTIFACT_DIR/build-output.txt" 2>&1; then
  echo "BUILD FAILED — configuration / compilation error. Aborting pipeline."
  cat "$ARTIFACT_DIR/build-output.txt"
  lichyflow unset flag tests_passed
  lichyflow set flag build_failed
  exit 1
fi
echo "Build OK."

# ── Run tests ────────────────────────────────────────────────────────
echo "--- Running tests ---"
go test ./... -v -count=1 > "$ARTIFACT_DIR/test-results.txt" 2>&1
EXIT_CODE=$?
cat "$ARTIFACT_DIR/test-results.txt"

if [ $EXIT_CODE -eq 0 ]; then
  lichyflow set flag tests_passed
  echo "All tests passed."
else
  lichyflow unset flag tests_passed
  grep -A 5 "FAIL\|ERROR\|panic" "$ARTIFACT_DIR/test-results.txt" > "$ARTIFACT_DIR/test-failures.txt" 2>/dev/null || true
  echo "Tests failed — failures saved to test-failures.txt (loop $NEW/5)."
fi
