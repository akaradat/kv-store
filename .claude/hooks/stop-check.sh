#!/usr/bin/env bash
# Stop hook: runs `make check` when Go-related files changed, and blocks the
# stop (exit 2) with the failure output so Claude fixes it before finishing.
set -uo pipefail

input="$(cat)"
if [ "$(jq -r '.stop_hook_active // false' <<<"$input")" = "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}" || exit 0

changed="$(git status --porcelain --untracked-files=all -- '*.go' go.mod go.sum Makefile scripts/check-arch.sh)"
if [ -z "$changed" ]; then
  exit 0
fi

if ! out="$(make check 2>&1)"; then
  {
    echo "make check failed. Fix it before finishing (AGENTS.md: Definition of done)."
    echo "$out" | tail -n 60
  } >&2
  exit 2
fi
exit 0
