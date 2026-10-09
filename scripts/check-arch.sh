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

# Production binaries use the stdlib plus chi only; ginkgo/gomega may appear in tests only.
# A stdlib import path has no dot in its first element.
if has_packages ./cmd/...; then
  for dep in $(go list -deps ./cmd/...); do
    first="${dep%%/*}"
    case "$dep" in
      "$module" | "$module"/*) ;;
      github.com/go-chi/chi/v5 | github.com/go-chi/chi/v5/*) ;;
      *)
        if [[ "$first" == *.* ]]; then
          violation "binaries depend on third-party package $dep"
        fi
        ;;
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
