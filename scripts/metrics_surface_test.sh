#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
CHART_DIR="$REPO_ROOT/charts/homer-operator"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

render() {
    local output="$1"
    shift
    helm template metrics-surface "$CHART_DIR" "$@" > "$output"
}

manager_args() {
    awk '
        /^kind: Deployment$/ { in_deployment = 1; next }
        in_deployment && /^        args:$/ { in_args = 1; next }
        in_args && /^        [A-Za-z]/ { exit }
        in_args && /^        - / { sub(/^        - /, ""); print }
        /^---$/ {
            if (in_args) exit
            in_deployment = in_manager = 0
        }
    ' "$1"
}

assert_arg() {
    local rendered="$1"
    local expected="$2"
    if ! manager_args "$rendered" | grep -Fxq -- "$expected"; then
        echo "expected manager args to contain: $expected" >&2
        manager_args "$rendered" >&2
        exit 1
    fi
}

assert_no_arg() {
    local rendered="$1"
    local unexpected="$2"
    if manager_args "$rendered" | grep -Fxq -- "$unexpected"; then
        echo "expected manager args not to contain: $unexpected" >&2
        exit 1
    fi
}

DISABLED="$TEMP_DIR/disabled.yaml"
render "$DISABLED" --set operator.metrics.enabled=false
assert_arg "$DISABLED" "--metrics-bind-address=0"
assert_no_arg "$DISABLED" "--metrics-secure"

SECURE="$TEMP_DIR/secure.yaml"
render "$SECURE" --set operator.metrics.enabled=true --set operator.metrics.secureMetrics=true
assert_arg "$SECURE" "--metrics-bind-address=:8443"
assert_arg "$SECURE" "--metrics-secure"
assert_no_arg "$SECURE" "--metrics-bind-address=0"

INSECURE="$TEMP_DIR/insecure.yaml"
render "$INSECURE" --set operator.metrics.enabled=true --set operator.metrics.secureMetrics=false --set serviceMonitor.enabled=true
assert_arg "$INSECURE" "--metrics-bind-address=:8443"
assert_no_arg "$INSECURE" "--metrics-secure"
assert_no_arg "$INSECURE" "--metrics-bind-address=0"

if grep -Fq 'authorization:' "$INSECURE" ||
   grep -Fq 'kubernetes.io/service-account-token' "$INSECURE" ||
   grep -Fq 'metrics-reader' "$INSECURE"; then
    echo "insecure metrics rendered secure scrape authorization resources" >&2
    exit 1
fi

SECURE_MONITOR="$TEMP_DIR/secure-monitor.yaml"
render "$SECURE_MONITOR" --set operator.metrics.enabled=true --set operator.metrics.secureMetrics=true --set serviceMonitor.enabled=true
if ! grep -Fq 'authorization:' "$SECURE_MONITOR" ||
   ! grep -Fq 'key: token' "$SECURE_MONITOR" ||
   ! grep -Fq 'kind: ClusterRoleBinding' "$SECURE_MONITOR" ||
   ! grep -Fq 'metrics-reader' "$SECURE_MONITOR"; then
    echo "secure ServiceMonitor is missing its self-contained scrape authorization path" >&2
    exit 1
fi
if grep -Fq 'bearerTokenFile:' "$SECURE_MONITOR"; then
    echo "secure ServiceMonitor still uses the Prometheus pod token implicitly" >&2
    exit 1
fi

echo "metrics surface regression check passed"
