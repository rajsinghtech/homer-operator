#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
KUSTOMIZE_BIN="${1:-${KUSTOMIZE:-kustomize}}"

if [ ! -x "$KUSTOMIZE_BIN" ] && ! command -v "$KUSTOMIZE_BIN" >/dev/null 2>&1; then
    echo "kustomize executable not found: $KUSTOMIZE_BIN" >&2
    exit 1
fi

RENDERED_MANIFEST="$(mktemp)"
trap 'rm -f "$RENDERED_MANIFEST"' EXIT

"$KUSTOMIZE_BIN" build "$REPO_ROOT/config/default" > "$RENDERED_MANIFEST"

expected_hash="$(sha256sum "$RENDERED_MANIFEST" | awk '{print $1}')"
for checked_in in "$REPO_ROOT/deploy/operator.yaml" "$REPO_ROOT/dist/install.yaml"; do
    actual_hash="$(sha256sum "$checked_in" | awk '{print $1}')"
    if [ "$actual_hash" != "$expected_hash" ]; then
        echo "$checked_in is out of sync with kustomize build config/default" >&2
        echo "expected: $expected_hash" >&2
        echo "actual:   $actual_hash" >&2
        exit 1
    fi
done

deploy_hash="$(sha256sum "$REPO_ROOT/deploy/operator.yaml" | awk '{print $1}')"
dist_hash="$(sha256sum "$REPO_ROOT/dist/install.yaml" | awk '{print $1}')"
if [ "$deploy_hash" != "$dist_hash" ]; then
    echo "deploy/operator.yaml and dist/install.yaml differ" >&2
    exit 1
fi

echo "installer manifests are synchronized"
