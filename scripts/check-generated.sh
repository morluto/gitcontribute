#!/usr/bin/env bash
set -euo pipefail

before_snapshot=$(mktemp)
after_snapshot=$(mktemp)
cleanup() {
  rm -f "$before_snapshot" "$after_snapshot"
}
trap cleanup EXIT

snapshot() {
  git diff --no-ext-diff --binary HEAD
  while IFS= read -r -d '' file; do
    printf '%s  %s\n' "$(git hash-object "$file")" "$file"
  done < <(git ls-files --others --exclude-standard -z)
}

snapshot >"$before_snapshot"
go generate ./...
snapshot >"$after_snapshot"

if ! cmp -s "$before_snapshot" "$after_snapshot"; then
  git status --short
  echo "Generated outputs changed; run 'make generate' and review the result."
  exit 1
fi

echo "Generated outputs are current"
