#!/usr/bin/env bash
# Enforces the layer rules in docs/standard/structure.md.
set -euo pipefail

module=github.com/akaradat/kv-store
fail=0

violation() {
  echo "arch: $1" >&2
  fail=1
}

has_packages() {
  [ -n "$(go list "$1" 2>/dev/null)" ]
}

if has_packages ./internal/core/...; then
  while read -r pkg imports; do
    for imp in $imports; do
      case "$imp" in
        net/http | net/http/*) violation "$pkg imports $imp (core must not speak HTTP)" ;;
        "$module"/internal/transport/*) violation "$pkg imports $imp (core must not depend on transport)" ;;
      esac
    done
  done < <(go list -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/core/...)
fi

if has_packages ./internal/core/kv; then
  for imp in $(go list -f '{{join .Imports " "}}' ./internal/core/kv); do
    case "$imp" in
      "$module"/*) violation "core/kv imports $imp (kv must import nothing internal)" ;;
    esac
  done
fi

for dir in pkg util utils common mocks models; do
  if [ -e "internal/$dir" ] || [ -e "$dir" ]; then
    violation "forbidden folder: $dir"
  fi
done

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "arch: ok"
