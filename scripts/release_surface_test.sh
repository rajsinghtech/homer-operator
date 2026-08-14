#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
CHART_DIR="$REPO_ROOT/charts/homer-operator"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

helm lint "$CHART_DIR" -f "$CHART_DIR/ci/values.yaml"

if ! helm lint "$CHART_DIR" \
    --set-string image.repository=localhost:5000/homer-operator \
    --set-string resources.limits.cpu=0.5 \
    --set-string resources.limits.memory=1.5Gi; then
    echo "chart values schema rejects valid image or resource quantities" >&2
    exit 1
fi

RENDERED_DEFAULT="$TEMP_DIR/default.yaml"
helm template release-surface "$CHART_DIR" \
    --set crd.create=true > "$RENDERED_DEFAULT"

if ! awk '
    /^    subresources:[[:space:]]*$/ {
        if (getline != 1 || $0 != "      status: {}") {
            exit 1
        }
        found = 1
    }
    END { exit found ? 0 : 1 }
' "$RENDERED_DEFAULT"; then
    echo "rendered chart is missing the Dashboard status subresource" >&2
    exit 1
fi

if grep -Eq '^[[:space:]]+subresources:[[:space:]]*null[[:space:]]*$' "$RENDERED_DEFAULT"; then
    echo "rendered chart contains a null subresources value" >&2
    exit 1
fi

if ! awk '
    /^---[[:space:]]*$/ { in_service = 0 }
    /^kind: Service$/ { in_service = 1 }
    in_service && /^  name: [^[:space:]]+-metrics$/ { found = 1 }
    END { exit found ? 0 : 1 }
' "$RENDERED_DEFAULT"; then
    echo "metrics are enabled but the metrics Service was not rendered" >&2
    exit 1
fi

RENDERED_METRICS_DISABLED="$TEMP_DIR/metrics-disabled.yaml"
helm template release-surface "$CHART_DIR" \
    --set operator.metrics.enabled=false \
    --set services.metrics.enabled=true > "$RENDERED_METRICS_DISABLED"

if awk '
    /^---[[:space:]]*$/ { in_service = 0 }
    /^kind: Service$/ { in_service = 1 }
    in_service && /^  name: [^[:space:]]+-metrics$/ { found = 1 }
    END { exit found ? 0 : 1 }
' "$RENDERED_METRICS_DISABLED"; then
    echo "metrics Service was rendered while operator metrics are disabled" >&2
    exit 1
fi

if ! awk '
    function finish_rule() {
        if (api == "authentication.k8s.io" && resource == "tokenreviews" && create) {
            tokenreview_rule = 1
        }
        if (api == "authorization.k8s.io" && resource == "subjectaccessreviews" && create) {
            subjectaccessreview_rule = 1
        }
    }
    /^---[[:space:]]*$/ {
        finish_rule()
        if (manager && tokenreview_rule && subjectaccessreview_rule) {
            found = 1
        }
        manager = 0
        in_rule = 0
        api = resource = ""
        create = tokenreview_rule = subjectaccessreview_rule = 0
        next
    }
    /^kind: ClusterRole$/ { cluster_role = 1; next }
    cluster_role && /^  name: release-surface-homer-operator-manager$/ {
        manager = 1
        next
    }
    manager && /^- apiGroups:$/ {
        finish_rule()
        in_rule = 1
        api = resource = ""
        create = 0
        section = "api"
        next
    }
    manager && in_rule && /^  resources:$/ { section = "resources"; next }
    manager && in_rule && /^  verbs:$/ { section = "verbs"; next }
    manager && in_rule && /^  - / {
        item = $0
        sub(/^  - /, "", item)
        if (section == "api") {
            api = item
        } else if (section == "resources") {
            resource = item
        } else if (section == "verbs" && item == "create") {
            create = 1
        }
    }
    END {
        finish_rule()
        if (manager && tokenreview_rule && subjectaccessreview_rule) {
            found = 1
        }
        exit found ? 0 : 1
    }
' "$RENDERED_DEFAULT"; then
    echo "manager ClusterRole is missing secure metrics authentication RBAC" >&2
    exit 1
fi

DIGEST="sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
RENDERED_DIGEST="$TEMP_DIR/digest.yaml"
helm template release-surface "$CHART_DIR" \
    --set image.repository=example.invalid/homer-operator \
    --set image.tag=ignored \
    --set image.digest="$DIGEST" > "$RENDERED_DIGEST"

if ! grep -Fq "image: example.invalid/homer-operator@$DIGEST" "$RENDERED_DIGEST"; then
    echo "chart did not render the configured image digest" >&2
    exit 1
fi

if grep -Fq "image: example.invalid/homer-operator:ignored" "$RENDERED_DIGEST"; then
    echo "chart rendered the mutable image tag despite a configured digest" >&2
    exit 1
fi

HELM_RELEASE="$REPO_ROOT/.github/workflows/helm-release.yml"
RELEASE_WORKFLOW="$REPO_ROOT/.github/workflows/release.yml"

if grep -Eq '^    tags:' "$HELM_RELEASE"; then
    echo "helm-release workflow still publishes on tags" >&2
    exit 1
fi

if ! awk '
    /^  release-helm:/ { in_job = 1; next }
    in_job && /if:.*needs\.verify-release\.result == .success./ { found = 1 }
    in_job && /^  [A-Za-z0-9_-]+:/ { job_done = 1; exit found ? 0 : 1 }
    END { exit found && (job_done || in_job) ? 0 : 1 }
' "$RELEASE_WORKFLOW"; then
    echo "release workflow Helm publication is not restricted to tags" >&2
    exit 1
fi

if ! grep -Fq 'sed -i "s|^  digest: .*|  digest:' "$RELEASE_WORKFLOW" ||
    ! grep -Fq 'DIGEST' "$RELEASE_WORKFLOW"; then
    echo "release workflow does not persist the built image digest into the chart" >&2
    exit 1
fi

if ! grep -Fq 'generate_release_notes: true' "$RELEASE_WORKFLOW"; then
    echo "release workflow does not generate GitHub release notes" >&2
    exit 1
fi

for verification_marker in \
    '  verify-release:' \
    '  verify-release-e2e:' \
    'needs: verify-release' \
    'make manifests' \
    'make generate' \
    'make lint' \
    'make envtest' \
    "--bin-dir \"\$PWD/bin\"" \
    'KUBEBUILDER_ASSETS=' \
    'go vet ./...' \
    'go test' \
    'go build ./...' \
    'make build-installer' \
    'check-installer-sync.sh' \
    'chart_packaging_test.sh' \
    'shellcheck scripts/*.sh' \
    'actionlint'; do
    if ! grep -Fq -- "$verification_marker" "$RELEASE_WORKFLOW"; then
        echo "release workflow is missing verification gate: $verification_marker" >&2
        exit 1
    fi
done

for release_safety_marker in \
    'concurrency:' \
    'cancel-in-progress: true' \
    'Verify release tag target and semantic version' \
    'git ls-remote --exit-code --refs origin' \
    'Upload version-specific installer' \
    "installer-\${{ github.ref_name }}" \
    'Prepare digest-pinned installer' \
    "homer-operator-\${VERSION}-install.yaml" \
    "release_image=\"\${REGISTRY}/\${IMAGE_NAME}@\${DIGEST}\"" \
    'provenance: mode=max' \
    'sbom: true' \
    'attestations: write' \
    'actions/attest-build-provenance@v2' \
    'Verify container provenance' \
    'gh attestation verify' \
    '--bundle-from-oci' \
    'Verify container signatures' \
    'helm pull'; do
    if ! grep -Fq -- "$release_safety_marker" "$RELEASE_WORKFLOW"; then
        echo "release workflow is missing safety or installer marker: $release_safety_marker" >&2
        exit 1
    fi
done

for workflow in "$RELEASE_WORKFLOW" "$HELM_RELEASE"; do
    if ! grep -Fq "actionlint_\${ACTIONLINT_VERSION}_linux_amd64.tar.gz" "$workflow"; then
        echo "$workflow downloads the wrong actionlint Linux asset" >&2
        exit 1
    fi
    if grep -Fq "actionlint_\${ACTIONLINT_VERSION}_linux_x86_64.tar.gz" "$workflow"; then
        echo "$workflow still references the nonexistent x86_64 actionlint asset" >&2
        exit 1
    fi
done

if ! awk '
    /^  build-image:/ { in_job = 1; next }
    in_job && /^    needs: \[verify-release, verify-release-e2e\]$/ { found = 1 }
    in_job && /^  [A-Za-z0-9_-]+:/ { exit found ? 0 : 1 }
    END { exit found ? 0 : 1 }
' "$RELEASE_WORKFLOW"; then
    echo "build-image is not gated by the versioned-release verification job" >&2
    exit 1
fi

if ! grep -Fq 'needs: [verify-release, verify-release-e2e]' "$RELEASE_WORKFLOW" ||
   ! grep -Fq 'needs.verify-release-e2e.result == '\''success'\''' "$RELEASE_WORKFLOW"; then
    echo "build-image is not gated by the isolated Helm E2E verification job" >&2
    exit 1
fi

if ! grep -Fq 'GITHUB_REF_NAME' "$RELEASE_WORKFLOW" ||
   ! grep -Fq 'v[0-9]+\.[0-9]+\.[0-9]+$' "$RELEASE_WORKFLOW"; then
    echo "release workflow does not validate vMAJOR.MINOR.PATCH tags" >&2
    exit 1
fi

if ! grep -Fq 'set -euo pipefail' "$RELEASE_WORKFLOW" ||
   ! grep -Fq 'cosign sign --yes' "$RELEASE_WORKFLOW" ||
   grep -Fq 'Failed to sign' "$RELEASE_WORKFLOW"; then
    echo "release workflow does not fail on cosign signing errors" >&2
    exit 1
fi

for pull_request in '/pull/104' '/pull/105'; do
    if ! grep -Fq "$pull_request" "$RELEASE_WORKFLOW"; then
        echo "release workflow is missing the detailed change summary for $pull_request" >&2
        exit 1
    fi
done

digest_variable=\$DIGEST
if ! grep -Fq "$digest_variable" "$RELEASE_WORKFLOW"; then
    echo "release workflow does not use the built image digest" >&2
    exit 1
fi

if [ -e "$REPO_ROOT/CHANGELOG.md" ]; then
    echo "release notes must remain GitHub Releases only; remove CHANGELOG.md" >&2
    exit 1
fi

if grep -Eq '^  workflow_dispatch:|^      - main$' "$RELEASE_WORKFLOW"; then
    echo "versioned release workflow must not publish development images" >&2
    exit 1
fi

DEV_IMAGE_WORKFLOW="$REPO_ROOT/.github/workflows/dev-image.yml"
for development_marker in \
    'name: Development Image' \
    "github.ref == 'refs/heads/main'" \
    'type=raw,value=main' \
    'type=raw,value=latest' \
    'packages: write' \
    'attestations: write' \
    '.github/workflows/dev-image.yml@refs/heads/main$'; do
    if ! grep -Fq -- "$development_marker" "$DEV_IMAGE_WORKFLOW"; then
        echo "development image workflow is missing safety marker: $development_marker" >&2
        exit 1
    fi
done

if ! sed -n '/^  verify-release-e2e:/,/^  build-image:/p' "$RELEASE_WORKFLOW" |
    grep -Fq '      contents: read'; then
    echo "versioned release E2E job does not have read-only contents permissions" >&2
    exit 1
fi

for publication_marker in \
    "cmp --silent \"\$local_chart\" \"\$pulled_chart\"" \
    'chart_digest=' \
    'OCI chart digest' \
    "charts/homer-operator@\${{ needs.release-helm.outputs.chart_digest }}" \
    'jq -e --arg digest' \
    'upload-artifact: false' \
    'documentNamespace' \
    "kubeconform -strict -ignore-missing-schemas -summary \"\$installer_path\""; do
    if ! grep -Fq -- "$publication_marker" "$RELEASE_WORKFLOW"; then
        echo "release workflow is missing publication verification: $publication_marker" >&2
        exit 1
    fi
done

if [[ "$(grep -c 'Final tag consistency check before' "$RELEASE_WORKFLOW")" -lt 7 ]]; then
    echo "release workflow is missing final tag checks before publication steps" >&2
    exit 1
fi

echo "release surface regression check passed"
