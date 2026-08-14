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

crd_count="$(grep -c '^kind: CustomResourceDefinition$' "$RENDERED_MANIFEST" || true)"
if [ "$crd_count" -ne 1 ]; then
    echo "default Kustomize output contains $crd_count CustomResourceDefinitions; expected exactly one" >&2
    exit 1
fi

if ! awk '
    /^    subresources:[[:space:]]*$/ {
        if (getline != 1 || $0 != "      status: {}") {
            exit 1
        }
        found = 1
    }
    END { exit found ? 0 : 1 }
' "$RENDERED_MANIFEST"; then
    echo "default Kustomize output is missing the Dashboard status subresource" >&2
    exit 1
fi

"$SCRIPT_DIR/kustomize_metrics_surface_test.sh" "$KUSTOMIZE_BIN"

echo "manifest generation regression check passed"
