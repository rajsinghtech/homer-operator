#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
CRD_FILE="$REPO_ROOT/config/crd/bases/homer.rajsingh.info_dashboards.yaml"
CHART_DIR="$REPO_ROOT/charts/homer-operator"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

if ! command -v helm >/dev/null 2>&1; then
    echo "helm is required to run this test" >&2
    exit 1
fi

GENERATED_TEMPLATE="$TEMP_DIR/crd.yaml"
RENDERED_CHART="$TEMP_DIR/rendered.yaml"

"$SCRIPT_DIR/sync-crd-to-helm.sh" "$CRD_FILE" > "$GENERATED_TEMPLATE"

if ! awk '
    FNR == NR {
        expected[FNR] = $0
        expected_lines = FNR
        next
    }
    {
        if (FNR > expected_lines || $0 != expected[FNR]) {
            mismatch = 1
        }
    }
    END {
        if (FNR != expected_lines) {
            mismatch = 1
        }
        exit mismatch
    }
' "$GENERATED_TEMPLATE" "$CHART_DIR/templates/crd.yaml"; then
    echo "generated CRD template is out of sync with the Helm chart" >&2
    exit 1
fi

helm template test "$CHART_DIR" --set crd.create=true > "$RENDERED_CHART"

if ! awk '
    /^    subresources:[[:space:]]*$/ {
        found=1
        if (getline != 1 || $0 != "      status: {}") {
            exit 1
        }
        valid=1
    }
    END {
        if (!found || !valid) {
            exit 1
        }
    }
' "$RENDERED_CHART"; then
    echo "rendered CRD is missing the status subresource" >&2
    exit 1
fi

if grep -Eq '^[[:space:]]+subresources:[[:space:]]*null[[:space:]]*$' "$RENDERED_CHART"; then
    echo "rendered CRD contains a null subresources value" >&2
    exit 1
fi

echo "CRD-to-Helm sync regression test passed"
