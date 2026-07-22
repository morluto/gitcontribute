#!/usr/bin/env bash
# Backticks in the sed expression below are literal Markdown delimiters.
# shellcheck disable=SC2016
set -euo pipefail

errors=0

report_ok() { echo "  OK: $1"; }
report_err() {
  echo "AGENTS.md: $1" >&2
  errors=$((errors + 1))
}

if [[ ! -s AGENTS.md ]]; then
  echo "AGENTS.md is missing or empty" >&2
  exit 1
fi

while IFS= read -r link; do
  [[ -z "$link" ]] && continue
  [[ "$link" =~ ^(https?://|mailto:|#) ]] && continue
  if [[ ! -e "$link" ]]; then
    report_err "Markdown link target missing: $link"
  else
    report_ok "$link"
  fi
done < <(sed -n 's/.*\[[^]]*\](\([^)]*\)).*/\1/p' AGENTS.md | sort -u)

while IFS= read -r path; do
  [[ -z "$path" ]] && continue
  [[ "$path" =~ ^https?:// ]] && continue
  [[ "$path" =~ ^(gofmt|go\ test|go\ vet|git\ ) ]] && continue
  if [[ ! -e "$path" ]]; then
    report_err "Backtick-quoted path missing: $path"
  else
    report_ok "$path"
  fi
done < <(sed -n 's/.*`\([^`]*\)`.*/\1/p' AGENTS.md | grep -E '^(docs/|LICENSES|internal/|cmd/)' | sort -u)

while IFS= read -r dir; do
  [[ -z "$dir" ]] && continue
  if [[ ! -d "$dir" ]]; then
    report_err "Referenced directory missing: $dir"
  else
    report_ok "$dir"
  fi
done < <(grep -oE '\b(internal/[a-zA-Z0-9_/]+|cmd/[a-zA-Z0-9_/]+)\b' AGENTS.md | sort -u)

if ! grep -q '^#' AGENTS.md; then
  report_err "No section headers found"
fi

if ((errors > 0)); then
  echo "AGENTS.md validation failed with $errors error(s)" >&2
  exit 1
fi

echo "AGENTS.md validation passed"
