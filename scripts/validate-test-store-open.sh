#!/usr/bin/env bash
# Tests open a library database through storetest.Open, not store.Open.
#
# store.Open on a new file runs the whole migration chain, and under -race that
# costs about 0.7 s (modernc.org/sqlite is SQLite compiled to Go, so the race
# detector instruments the SQL engine). One fresh store per test turned CI's
# `go test -race` into a 25-minute run. storetest.Open copies a database the
# chain already ran on and is otherwise the same call.
#
# The store package's own tests are exempt: they exercise migrations directly.
set -euo pipefail
cd "$(dirname "$0")/../engine"

# grep, not rg: the Linux CI runner ships no ripgrep. A capture, not `if grep`,
# so a grep error aborts instead of reading as "no match".
status=0
hits=$(grep -rnE --include='*_test.go' '(^|[^.[:alnum:]_])store\.Open\(' . | grep -v '^\./internal/store/') || status=$?
if [ "$status" -gt 1 ]; then
  echo "error: grep failed" >&2
  exit "$status"
fi
if [ -n "$hits" ]; then
  echo "$hits"
  echo "error: tests must open the library with storetest.Open (internal/store/storetest), not store.Open" >&2
  exit 1
fi
