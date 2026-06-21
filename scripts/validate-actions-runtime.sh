#!/usr/bin/env bash
# Keep GitHub Actions workflows off deprecated JavaScript runtimes.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

failures=()

while IFS= read -r match; do
  failures+=("${match}")
done < <(rg -n 'uses: (actions/checkout@v[1-6]|actions/setup-node@v[1-5]|actions/setup-go@v[1-5]|actions/setup-java@v[1-4]|pnpm/action-setup@v[1-5])\b' .github/workflows || true)

while IFS= read -r match; do
  failures+=("${match}")
done < <(rg -n 'uses: actions/(checkout|setup-node|setup-go)@[0-9a-f]{40} # v[1-5]\b|uses: actions/setup-java@[0-9a-f]{40} # v[1-4]\b|uses: pnpm/action-setup@[0-9a-f]{40} # v[1-5]\b' .github/workflows || true)

if (( ${#failures[@]} > 0 )); then
  printf 'GitHub Actions runtime validation failed; update these actions to Node 24-backed versions:\n' >&2
  printf '  %s\n' "${failures[@]}" >&2
  exit 1
fi

echo "actions runtime metadata ok"
