# Homer Operator Helm Chart

A Helm chart for deploying the Homer Operator on Kubernetes. The Homer Operator manages Homer dashboard instances that automatically discover and display your Kubernetes services.

## Prerequisites

- Kubernetes 1.21+ (Kubernetes 1.23+ when enabling the optional HPA)
- Helm 3.8+

## Installation

### Install from OCI Registry (Recommended)

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --version 1.2.3 -n homer-operator --create-namespace
```

### Install from Source

```bash
git clone https://github.com/rajsinghtech/homer-operator.git
cd homer-operator
helm install homer-operator charts/homer-operator -n homer-operator --create-namespace
```

## Configuration

The following table lists the configurable parameters of the Homer Operator chart and their default values.

| Parameter | Description | Default |
| --- | --- | --- |
| `replicaCount` | Number of operator replicas | `1` |
| `image.repository` | Operator image repository | `ghcr.io/rajsinghtech/homer-operator` |
| `image.tag` | Operator image tag | `Chart.appVersion` |
| `image.digest` | Immutable operator image digest; takes precedence over `image.tag` | `""` |
| `image.pullPolicy` | Image pull policy | `Always` |
| `operator.enableGatewayAPI` | Enable Gateway API support | `false` |
| `homer.image.repository` | Homer dashboard image repository | `b4bz/homer` |
| `homer.image.tag` | Homer dashboard image tag | `latest` |
| `homer.configSyncImage` | Image used to stage Dashboard ConfigMaps into Homer assets | `alpine:3.18` |
| `operator.metrics.enabled` | Enable metrics collection | `true` |
| `operator.metrics.secureMetrics` | Use secure metrics serving | `true` |
| `operator.metrics.bindAddress` | Metrics bind address | `:8443` |
| `services.metrics.enabled` | Create the metrics Service when operator metrics are enabled | `true` |
| `services.metrics.type` | Metrics Service type | `ClusterIP` |
| `services.metrics.port` | Metrics Service port | `8443` |
| `operator.healthProbe.bindAddress` | Health probe bind address | `:8081` |
| `serviceAccount.create` | Create service account | `true` |
| `rbac.create` | Create RBAC resources | `true` |
| `crd.create` | Create CustomResourceDefinitions | `true` |
| `serviceMonitor.enabled` | Create a Prometheus ServiceMonitor | `false` |
| `serviceMonitor.auth.serviceAccountName` | Override the generated secure-metrics scraper ServiceAccount name | `""` |
| `serviceMonitor.auth.tokenSecretName` | Override the generated secure-metrics token Secret name | `""` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.limits.cpu` | CPU limit | `200m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `resources.requests.cpu` | CPU request | `50m` |
| `highAvailability.podDisruptionBudget.enabled` | Create a PodDisruptionBudget | `true` |
| `highAvailability.podDisruptionBudget.minAvailable` | Minimum available Pods; rendered when `maxUnavailable` is null | `1` |
| `highAvailability.podDisruptionBudget.maxUnavailable` | Maximum unavailable Pods or percentage; takes precedence when set | `null` |
| `highAvailability.autoscaling.enabled` | Create an HPA | `false` |

Legacy values for operator log formatting, reconcile tuning, leader-election
timers, and `homer.image.pullPolicy` are intentionally unsupported because the
operator binary has no corresponding runtime settings; Helm rejects them.

## Examples

### Basic Installation

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator -n homer-operator --create-namespace
```

### With Custom Values

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  -n homer-operator --create-namespace \
  --set operator.enableGatewayAPI=true \
  --set operator.metrics.enabled=false \
  --set resources.limits.memory=256Mi
```

### With Values File

```yaml
# values.yaml
operator:
  enableGatewayAPI: true
  metrics:
    enabled: true
    secureMetrics: false

resources:
  limits:
    memory: 256Mi
    cpu: 1000m
  requests:
    memory: 128Mi
    cpu: 100m

serviceMonitor:
  enabled: true
  interval: 60s
```

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator -n homer-operator --create-namespace -f values.yaml
```

## Features

- **Automatic Service Discovery**: Discovers Kubernetes Ingress resources and creates Homer dashboard entries
- **Gateway API Support**: Optional support for Gateway API HTTPRoute resources
- **Metrics Collection**: Prometheus metrics for monitoring operator performance
- **Security**: Runs with non-root user and restrictive security contexts
- **High Availability**: Configurable replica count and pod disruption budgets
- **Monitoring**: ServiceMonitor support for Prometheus Operator

## Usage

After installing the operator, create a Dashboard resource:

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  namespace: default
spec:
  replicas: 2
  homerConfig:
    title: "My Services"
    subtitle: "Application Dashboard"
    logo: "https://example.com/logo.png"
    services:
      - name: "Web Services"
        icon: "fas fa-globe"
        items:
          - name: "My App"
            logo: "https://example.com/app-logo.png"
            url: "https://myapp.example.com"
            subtitle: "Main Application"
  pages:
    status:
      subtitle: "Status page"
      services:
        - name: "Status"
          items:
            - name: "Example"
              url: "https://example.com"
```

The `services` and `pages` blocks follow upstream Homer's direct configuration
format. Legacy `parameters` blocks remain supported for existing dashboards.

The checked-in sample at `config/samples/homer_v1alpha1_dashboard.yaml` is
used by the Helm kind smoke test. It exercises direct Homer fields, a second
page, and the generated `<page>.yml` asset.

### External Configuration and Shared Assets

Set `spec.configMap.name` to load the complete Homer configuration from a
ConfigMap in the Dashboard's namespace. The default key is `config.yml`.
ConfigMap updates are watched and reconcile the Dashboard automatically.

For custom assets, `spec.assets.configMapRef.namespace` may point to a
ConfigMap in another namespace. The operator watches that source and creates a
namespace-local mirror for the Dashboard pod. Leaving `namespace` empty uses
the Dashboard's namespace.

Asset files are staged below Homer’s `/www/assets` directory. Use flat
filenames in the referenced ConfigMap: the operator mounts ConfigMap keys at
the asset root and does not create `items[].path` projections, so nested names
such as `assets/tools/sample.png` are not a portable `ConfigMap` representation.
Icon mappings can populate Homer’s canonical icon paths, and enabled PWA
configuration generates `manifest.json` with matching icon URLs.
Cross-namespace mirrors are updated when their source changes and removed when
the Dashboard switches references.

For remote-cluster Service discovery, the operator omits the generated
cluster-internal DNS URL because it would resolve in the operator’s cluster.
Add `item.homer.rajsingh.info/url` when the remote Service has a reachable URL;
otherwise the item remains visible without a link.

## Troubleshooting

### Namespace Creation Issues

If you encounter `namespaces 'homer-operator' not found`, add the `--create-namespace` flag:

```bash
helm upgrade --install homer-operator charts/homer-operator -n homer-operator --create-namespace
```

## Gateway API Support

To enable Gateway API support, set `operator.enableGatewayAPI=true`. This requires Gateway API CRDs to be installed in your cluster.

## Monitoring

The operator exposes authenticated HTTPS Prometheus metrics on port 8443. Its
health and readiness probes listen on port 8081. The metrics Service is created
only when both `operator.metrics.enabled` and `services.metrics.enabled` are
true; disabling operator metrics also removes the Service and ServiceMonitor.
Secure metrics use controller-runtime's TokenReview/SubjectAccessReview filter,
so the operator ServiceAccount needs `create` access to
`authentication.k8s.io/tokenreviews` and
`authorization.k8s.io/subjectaccessreviews`. With secure metrics and
`serviceMonitor.enabled`, the chart also creates a dedicated scraper
ServiceAccount, service-account-token Secret, metrics-reader ClusterRole, and
ClusterRoleBinding; the ServiceMonitor references that Secret directly. Keep
`rbac.create=true` for this self-contained path. To enable monitoring:

```yaml
operator:
  metrics:
    enabled: true

serviceMonitor:
  enabled: true
  interval: 30s
```

When `operator.metrics.secureMetrics` is false, the ServiceMonitor uses HTTP and
does not create or send a secure-metrics bearer token or TLS settings. The
Prometheus Operator must support the ServiceMonitor endpoint `authorization`
field for secure scraping.

## Security

The operator follows security best practices:

- Runs as non-root user (UID 1000 in the Helm deployment)
- Uses read-only root filesystem
- Drops all capabilities
- Implements least-privilege RBAC

## Uninstalling

```bash
helm uninstall homer-operator -n homer-operator
```

The Dashboard CRD is rendered as a normal Helm template, so uninstalling the
chart removes the CRD and Kubernetes consequently removes all Dashboard
resources. The namespace is retained. Back up any Dashboard resources first if
you need to preserve them. To remove the namespace separately:

```bash
kubectl delete namespace homer-operator
```

## Support

For issues and questions, please visit the [GitHub repository](https://github.com/rajsinghtech/homer-operator).
