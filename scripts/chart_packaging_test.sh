#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
CHART_DIR="$REPO_ROOT/charts/homer-operator"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

assert_contains() {
    local file="$1"
    local text="$2"
    if ! grep -Fq -- "$text" "$file"; then
        echo "expected $file to contain: $text" >&2
        exit 1
    fi
}

assert_not_contains() {
    local file="$1"
    local text="$2"
    if grep -Fq -- "$text" "$file"; then
        echo "expected $file not to contain: $text" >&2
        exit 1
    fi
}

assert_probe_has_no_enabled() {
    local file="$1"
    if awk '
        /^        startupProbe:[[:space:]]*$/ { in_probe = 1; next }
        in_probe && /^        [A-Za-z]/ { in_probe = 0 }
        in_probe && /^          enabled:/ { found = 1 }
        END { exit found ? 0 : 1 }
    ' "$file"; then
        echo "startupProbe.enabled leaked into the Kubernetes probe object" >&2
        exit 1
    fi
}

assert_probe_has_only_action() {
    local file="$1"
    local expected="$2"
    if ! awk -v expected="$expected" '
        /^        startupProbe:[[:space:]]*$/ { in_probe = 1; next }
        in_probe && /^        [A-Za-z]/ {
            exit action_count == 1 && action == expected ? 0 : 1
        }
        in_probe && /^          (exec|grpc|httpGet|tcpSocket):[[:space:]]*$/ {
            action = $1
            sub(/:$/, "", action)
            action_count++
        }
        END {
            if (in_probe) {
                exit action_count == 1 && action == expected ? 0 : 1
            }
        }
    ' "$file"; then
        echo "startupProbe did not render exactly one $expected action" >&2
        exit 1
    fi
}

assert_pdb_field() {
    local file="$1"
    local expected="$2"
    if ! awk -v expected="$expected" '
        /^kind: PodDisruptionBudget[[:space:]]*$/ { in_pdb = 1; next }
        in_pdb && /^  minAvailable:/ { field = "minAvailable"; count++ }
        in_pdb && /^  maxUnavailable:/ { field = "maxUnavailable"; count++ }
        in_pdb && /^---[[:space:]]*$/ {
            if (count == 1 && field == expected) {
                found = 1
            }
            exit
        }
        END { exit found ? 0 : 1 }
    ' "$file"; then
        echo "PDB did not render exactly the expected field: $expected" >&2
        exit 1
    fi
}

helm lint "$CHART_DIR" -f "$CHART_DIR/ci/values.yaml"
helm lint "$CHART_DIR" \
    --set startupProbe.enabled=true \
    --set-string image.repository=localhost:5000/homer-operator \
    --set-string resources.limits.cpu=0.5 \
    --set-string resources.limits.memory=1.5Gi

STARTUP_RENDERED="$TEMP_DIR/startup.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set startupProbe.enabled=true > "$STARTUP_RENDERED"
assert_contains "$STARTUP_RENDERED" "        startupProbe:"
assert_probe_has_no_enabled "$STARTUP_RENDERED"
kubeconform -strict -ignore-missing-schemas -summary "$STARTUP_RENDERED"

HTTP_PROBE_RENDERED="$TEMP_DIR/http-probe.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set startupProbe.enabled=true \
    --set-string startupProbe.httpGet.host=localhost \
    --set startupProbe.httpGet.httpHeaders[0].name=X-Health \
    --set-string startupProbe.httpGet.httpHeaders[0].value=ready \
    --set startupProbe.httpGet.scheme=HTTPS > "$HTTP_PROBE_RENDERED"
assert_contains "$HTTP_PROBE_RENDERED" "host: localhost"
assert_contains "$HTTP_PROBE_RENDERED" "name: X-Health"
assert_contains "$HTTP_PROBE_RENDERED" "value: ready"
assert_contains "$HTTP_PROBE_RENDERED" "scheme: HTTPS"
assert_probe_has_no_enabled "$HTTP_PROBE_RENDERED"
assert_probe_has_only_action "$HTTP_PROBE_RENDERED" httpGet

MERGED_ACTION_RENDERED="$TEMP_DIR/merged-action.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set startupProbe.enabled=true \
    --set-string startupProbe.exec.command[0]=/bin/check > "$MERGED_ACTION_RENDERED"
assert_probe_has_no_enabled "$MERGED_ACTION_RENDERED"
assert_probe_has_only_action "$MERGED_ACTION_RENDERED" exec

for probe_json in \
    '{"enabled":true,"exec":{"command":["/bin/check"]}}' \
    '{"enabled":true,"tcpSocket":{"host":"127.0.0.1","port":8081}}' \
    '{"enabled":true,"grpc":{"port":9090,"service":"health"}}'; do
    ACTION_RENDERED="$TEMP_DIR/action-${RANDOM}.yaml"
    helm template packaging-surface "$CHART_DIR" \
        --set-json "startupProbe=${probe_json}" > "$ACTION_RENDERED"
    assert_probe_has_no_enabled "$ACTION_RENDERED"
    action_name="unknown"
    case "$probe_json" in
        *'"exec"'*) action_name='exec' ;;
        *'"tcpSocket"'*) action_name='tcpSocket' ;;
        *'"grpc"'*) action_name='grpc' ;;
    esac
    assert_probe_has_only_action "$ACTION_RENDERED" "$action_name"
    kubeconform -strict -ignore-missing-schemas -summary "$ACTION_RENDERED"
done

PDB_DEFAULT="$TEMP_DIR/pdb-default.yaml"
helm template packaging-surface "$CHART_DIR" > "$PDB_DEFAULT"
assert_pdb_field "$PDB_DEFAULT" minAvailable

PDB_MAX="$TEMP_DIR/pdb-max.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set highAvailability.podDisruptionBudget.maxUnavailable=0 > "$PDB_MAX"
assert_pdb_field "$PDB_MAX" maxUnavailable

PDB_PERCENT="$TEMP_DIR/pdb-percent.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set highAvailability.podDisruptionBudget.minAvailable=null \
    --set-string highAvailability.podDisruptionBudget.maxUnavailable=50% > "$PDB_PERCENT"
assert_pdb_field "$PDB_PERCENT" maxUnavailable

if helm template packaging-surface "$CHART_DIR" \
    --set highAvailability.podDisruptionBudget.minAvailable=null \
    --set highAvailability.podDisruptionBudget.maxUnavailable=null >/dev/null 2>&1; then
    echo "PDB rendered successfully without minAvailable or maxUnavailable" >&2
    exit 1
fi

for invalid_repository in '-bad' 'bad/' 'bad//repo' 'bad:tag' 'bad repo' 'bad@sha256:abc'; do
    if helm lint "$CHART_DIR" --set-string image.repository="$invalid_repository" >/dev/null 2>&1; then
        echo "image.repository schema accepted an invalid value: $invalid_repository" >&2
        exit 1
    fi
done

for invalid_repository in 'registry.example.com:0/homer' 'registry.example.com:65536/homer' 'registry.example.com:99999/homer'; do
    if helm lint "$CHART_DIR" --set-string image.repository="$invalid_repository" >/dev/null 2>&1; then
        echo "image.repository schema accepted an invalid registry port: $invalid_repository" >&2
        exit 1
    fi
done

for invalid_homer_repository in '-bad' 'bad/' 'bad//repo' 'bad repo' 'bad@sha256:abc'; do
    if helm lint "$CHART_DIR" --set-string homer.image.repository="$invalid_homer_repository" >/dev/null 2>&1; then
        echo "homer.image.repository schema accepted an invalid value: $invalid_homer_repository" >&2
        exit 1
    fi
done

for invalid_config_sync_image in 'bad repo' 'bad@sha256:abc' 'registry.example.com:99999/alpine'; do
    if helm lint "$CHART_DIR" --set-string homer.configSyncImage="$invalid_config_sync_image" >/dev/null 2>&1; then
        echo "homer.configSyncImage schema accepted an invalid value: $invalid_config_sync_image" >&2
        exit 1
    fi
done

for invalid_bind_address in ':0' ':65536' ':99999'; do
    if helm lint "$CHART_DIR" --set-string operator.metrics.bindAddress="$invalid_bind_address" >/dev/null 2>&1; then
        echo "operator metrics schema accepted an invalid bind port: $invalid_bind_address" >&2
        exit 1
    fi
done

for invalid_quantity in '1..2' '1foo' '-1' '1MiB' '1mi' '1K'; do
    if helm lint "$CHART_DIR" --set-string resources.limits.cpu="$invalid_quantity" >/dev/null 2>&1; then
        echo "resource quantity schema accepted an invalid value: $invalid_quantity" >&2
        exit 1
    fi
done

for valid_quantity in '1E' '1Ei'; do
    if ! helm lint "$CHART_DIR" --set-string "resources.limits.cpu=$valid_quantity" >/dev/null 2>&1; then
        echo "resource quantity schema rejected a valid value: $valid_quantity" >&2
        exit 1
    fi
done

for invalid_tag in 'bad tag' 'bad/tag' 'bad:tag' "$(printf 'a%.0s' {1..129})"; do
    if helm lint "$CHART_DIR" --set-string "image.tag=$invalid_tag" >/dev/null 2>&1; then
        echo "image.tag schema accepted an invalid value: $invalid_tag" >&2
        exit 1
    fi
done

if ! helm lint "$CHART_DIR" --set-string homer.image.repository=localhost:5000/homer >/dev/null 2>&1; then
    echo "homer.image.repository schema rejected a valid registry port" >&2
    exit 1
fi

for invalid_homer_repository in 'bad repo' 'bad/' 'bad:tag'; do
    if helm lint "$CHART_DIR" --set-string "homer.image.repository=$invalid_homer_repository" >/dev/null 2>&1; then
        echo "homer.image.repository schema accepted an invalid value: $invalid_homer_repository" >&2
        exit 1
    fi
done

for invalid_homer_tag in 'bad tag' 'bad/tag' 'bad:tag'; do
    if helm lint "$CHART_DIR" --set-string "homer.image.tag=$invalid_homer_tag" >/dev/null 2>&1; then
        echo "homer.image.tag schema accepted an invalid value: $invalid_homer_tag" >&2
        exit 1
    fi
done

LONG_RELEASE="$(printf 'r%.0s' {1..53})"
AUTH_NAME="$(printf 'a%.0s' {1..63})"
TOKEN_NAME="$(printf 'b%.0s' {1..63})"
LONG_NAMES_RENDERED="$TEMP_DIR/long-names.yaml"
helm template "$LONG_RELEASE" "$CHART_DIR" \
    --set serviceMonitor.enabled=true \
    --set grafanaDashboard.enabled=true \
    --set prometheusRule.enabled=true \
    --set vpa.enabled=true \
    --set highAvailability.autoscaling.enabled=true \
    --set-string serviceMonitor.auth.serviceAccountName="$AUTH_NAME" \
    --set-string serviceMonitor.auth.tokenSecretName="$TOKEN_NAME" > "$LONG_NAMES_RENDERED"

if awk '
    /^metadata:[[:space:]]*$/ { in_metadata = 1; next }
    in_metadata && /^  name:[[:space:]]*/ {
        name = $0
        sub(/^  name:[[:space:]]*/, "", name)
        if (length(name) > 63) {
            print name
            found = 1
        }
        in_metadata = 0
    }
    in_metadata && /^[^[:space:]]/ { in_metadata = 0 }
    END { exit found ? 0 : 1 }
' "$LONG_NAMES_RENDERED"; then
    echo "a rendered metadata.name exceeded 63 characters" >&2
    exit 1
fi
assert_contains "$LONG_NAMES_RENDERED" "name: $AUTH_NAME"
assert_contains "$LONG_NAMES_RENDERED" "name: $TOKEN_NAME"

SECURE_RENDERED="$TEMP_DIR/secure.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set serviceMonitor.enabled=true > "$SECURE_RENDERED"
SECURE_NAME="packaging-surface-homer-operator"
assert_contains "$SECURE_RENDERED" "name: ${SECURE_NAME}-metrics"
assert_contains "$SECURE_RENDERED" "name: ${SECURE_NAME}-metrics-token"
assert_contains "$SECURE_RENDERED" "type: kubernetes.io/service-account-token"
assert_contains "$SECURE_RENDERED" "name: ${SECURE_NAME}-metrics-reader"
assert_contains "$SECURE_RENDERED" "authorization:"
assert_contains "$SECURE_RENDERED" "key: token"
assert_not_contains "$SECURE_RENDERED" "bearerTokenFile:"

INSECURE_RENDERED="$TEMP_DIR/insecure.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set serviceMonitor.enabled=true \
    --set operator.metrics.secureMetrics=false > "$INSECURE_RENDERED"
assert_contains "$INSECURE_RENDERED" "scheme: http"
assert_not_contains "$INSECURE_RENDERED" "authorization:"
assert_not_contains "$INSECURE_RENDERED" "type: kubernetes.io/service-account-token"
assert_not_contains "$INSECURE_RENDERED" "metrics-reader"
assert_not_contains "$INSECURE_RENDERED" "tokenreviews"
assert_not_contains "$INSECURE_RENDERED" "subjectaccessreviews"

METRICS_DISABLED_RENDERED="$TEMP_DIR/metrics-disabled.yaml"
helm template packaging-surface "$CHART_DIR" \
    --set operator.metrics.enabled=false \
    --set serviceMonitor.enabled=true > "$METRICS_DISABLED_RENDERED"
assert_not_contains "$METRICS_DISABLED_RENDERED" "tokenreviews"
assert_not_contains "$METRICS_DISABLED_RENDERED" "subjectaccessreviews"
assert_not_contains "$METRICS_DISABLED_RENDERED" "metrics-reader"
assert_not_contains "$METRICS_DISABLED_RENDERED" "kind: ServiceMonitor"
assert_not_contains "$METRICS_DISABLED_RENDERED" "name: packaging-surface-homer-operator-metrics"
assert_not_contains "$METRICS_DISABLED_RENDERED" "kubernetes.io/service-account-token"

echo "chart packaging regression check passed"
