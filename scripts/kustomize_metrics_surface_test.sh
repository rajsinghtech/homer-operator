#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
KUSTOMIZE_BIN="${1:-${KUSTOMIZE:-kustomize}}"

if [ ! -x "$KUSTOMIZE_BIN" ] && ! command -v "$KUSTOMIZE_BIN" >/dev/null 2>&1; then
    echo "kustomize executable not found: $KUSTOMIZE_BIN" >&2
    exit 1
fi

DEFAULT_MANIFEST="$(mktemp)"
PROMETHEUS_MANIFEST="$(mktemp)"
trap 'rm -f "$DEFAULT_MANIFEST" "$PROMETHEUS_MANIFEST"' EXIT

"$KUSTOMIZE_BIN" build "$REPO_ROOT/config/default" > "$DEFAULT_MANIFEST"
"$KUSTOMIZE_BIN" build "$REPO_ROOT/config/prometheus" > "$PROMETHEUS_MANIFEST"

count_kind() {
    local kind="$1"
    local manifest="$2"
    awk -v expected_kind="$kind" '$0 == "kind: " expected_kind { count++ } END { print count + 0 }' "$manifest"
}

assert_count() {
    local expected="$1"
    local kind="$2"
    local manifest="$3"
    local actual
    actual="$(count_kind "$kind" "$manifest")"
    if [ "$actual" -ne "$expected" ]; then
        echo "expected $expected $kind resources in $manifest, found $actual" >&2
        exit 1
    fi
}

assert_contains() {
    local manifest="$1"
    local text="$2"
    if ! grep -Fq -- "$text" "$manifest"; then
        echo "expected $manifest to contain: $text" >&2
        exit 1
    fi
}

assert_resource_namespace() {
    local kind="$1"
    local name="$2"
    local namespace="$3"
    local manifest="$4"
    if ! awk -v expected_kind="$kind" -v expected_name="$name" -v expected_namespace="$namespace" '
        function reset() {
            in_resource = 0
            matched_name = 0
        }
        BEGIN { reset() }
        $0 == "---" { reset(); next }
        $0 == "kind: " expected_kind { in_resource = 1 }
        in_resource && $0 == "  name: " expected_name { matched_name = 1 }
        in_resource && matched_name && $0 == "  namespace: " expected_namespace { found = 1 }
        END { exit found ? 0 : 1 }
    ' "$manifest"; then
        echo "expected $kind/$name in $manifest to use namespace $namespace" >&2
        exit 1
    fi
}

assert_resource_name() {
    local kind="$1"
    local name="$2"
    local manifest="$3"
    if ! awk -v expected_kind="$kind" -v expected_name="$name" '
        function reset() {
            in_resource = 0
        }
        BEGIN { reset() }
        $0 == "---" { reset(); next }
        $0 == "kind: " expected_kind { in_resource = 1 }
        in_resource && $0 == "  name: " expected_name { found = 1 }
        END { exit found ? 0 : 1 }
    ' "$manifest"; then
        echo "expected $kind/$name in $manifest" >&2
        exit 1
    fi
}

assert_count 0 ServiceMonitor "$DEFAULT_MANIFEST"
assert_count 1 Service "$DEFAULT_MANIFEST"
assert_contains "$DEFAULT_MANIFEST" "name: homer-operator-controller-manager-metrics"
assert_contains "$DEFAULT_MANIFEST" "targetPort: https"
assert_contains "$DEFAULT_MANIFEST" "--metrics-secure"
assert_contains "$DEFAULT_MANIFEST" "name: homer-operator-metrics-reader"
assert_resource_namespace Service homer-operator-controller-manager-metrics homer-operator-system "$DEFAULT_MANIFEST"

assert_count 1 ServiceMonitor "$PROMETHEUS_MANIFEST"
assert_count 1 Service "$PROMETHEUS_MANIFEST"
assert_count 1 CustomResourceDefinition "$PROMETHEUS_MANIFEST"
assert_resource_namespace Service homer-operator-controller-manager-metrics homer-operator-system "$PROMETHEUS_MANIFEST"
assert_resource_namespace ServiceMonitor homer-operator-controller-manager-metrics-monitor homer-operator-system "$PROMETHEUS_MANIFEST"
assert_resource_name ClusterRole homer-operator-metrics-reader "$PROMETHEUS_MANIFEST"
assert_contains "$PROMETHEUS_MANIFEST" "name: homer-operator-controller-manager-metrics-monitor"
assert_contains "$PROMETHEUS_MANIFEST" "scheme: https"
assert_contains "$PROMETHEUS_MANIFEST" "bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token"
assert_contains "$PROMETHEUS_MANIFEST" "port: https"
assert_contains "$PROMETHEUS_MANIFEST" "name: homer-operator-metrics-reader"

echo "static Kustomize metrics surface regression check passed"
