# Homer Operator Helm Chart

A Helm chart for deploying the Homer Operator on Kubernetes. The Homer Operator manages Homer dashboard instances that automatically discover and display your Kubernetes services.

## Prerequisites

- Kubernetes 1.21+ (Kubernetes 1.23+ when enabling the optional HPA)
- Helm 3.8+

## Installation

### Install from OCI Registry (Recommended)

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  -n homer-operator --create-namespace
```

Helm selects the latest published chart when `--version` is omitted. Add
`--version <version-from-GitHub-Releases>` when pinning a release.

### Install from Source

```bash
git clone https://github.com/rajsinghtech/homer-operator.git
cd homer-operator
helm install homer-operator charts/homer-operator -n homer-operator --create-namespace
```

Source and tracked Kustomize installs follow the development `main` image.
Use the OCI chart above, or the version-specific installer asset attached to a
GitHub Release, when you need a pinned operator image.

Versioned chart and operator artifacts are documented in [GitHub
Releases](https://github.com/rajsinghtech/homer-operator/releases). Release
notes are the project's changelog; this repository intentionally does not
maintain a `CHANGELOG.md`.

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
| `operator.leaderElection.enabled` | Enable controller leader election | `true` |
| `homer.image.repository` | Homer dashboard image repository | `b4bz/homer` |
| `homer.image.tag` | Homer dashboard image tag | `latest` |
| `homer.configSyncImage` | Image used to stage Dashboard ConfigMaps into Homer assets | `alpine:3.18` |
| `imagePullSecrets` | Image pull Secret references | `[]` |
| `nameOverride` / `fullnameOverride` | Override generated resource names | `""` / `""` |
| `operator.metrics.enabled` | Enable metrics collection | `true` |
| `operator.metrics.secureMetrics` | Use secure metrics serving | `true` |
| `operator.metrics.bindAddress` | Metrics bind address | `:8443` |
| `services.metrics.enabled` | Create the metrics Service; both this and `operator.metrics.enabled` must be true | `true` |
| `services.metrics.type` | Metrics Service type | `ClusterIP` |
| `services.metrics.port` | Metrics Service port | `8443` |
| `operator.healthProbe.bindAddress` | Health probe bind address | `:8081` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.automount` | Automount the service account token | `true` |
| `serviceAccount.name` | Override the service account name | `""` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `rbac.create` | Create RBAC resources | `true` |
| `rbac.annotations` | RBAC object annotations | `{}` |
| `crd.create` | Create CustomResourceDefinitions | `true` |
| `crd.annotations` | CRD annotations | `{}` |
| `podAnnotations` | Operator Pod annotations | `{}` |
| `podLabels` | Operator Pod labels | `{}` |
| `podSecurityContext.runAsNonRoot` | Require the operator Pod to run as non-root | `true` |
| `podSecurityContext.runAsUser` / `runAsGroup` / `fsGroup` | Operator Pod user, group, and filesystem group IDs | `1000` / `1000` / `1000` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation in the operator container | `false` |
| `securityContext.readOnlyRootFilesystem` | Mount the operator container root filesystem read-only | `true` |
| `securityContext.seccompProfile.type` | Operator container seccomp profile type | `RuntimeDefault` |
| `securityContext.capabilities.drop` | Linux capabilities dropped from the operator container | `[ALL]` |
| `serviceMonitor.enabled` | Create a Prometheus ServiceMonitor | `false` |
| `serviceMonitor.interval` | ServiceMonitor scrape interval | `30s` |
| `serviceMonitor.scrapeTimeout` | ServiceMonitor scrape timeout | `10s` |
| `serviceMonitor.labels` / `serviceMonitor.annotations` | ServiceMonitor metadata | `{}` / `{}` |
| `serviceMonitor.auth.serviceAccountName` | Override the generated secure-metrics scraper ServiceAccount name | `""` |
| `serviceMonitor.auth.tokenSecretName` | Override the generated secure-metrics token Secret name | `""` |
| `services.metrics.annotations` | Metrics Service annotations | `{}` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.limits.cpu` | CPU limit | `200m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `resources.requests.cpu` | CPU request | `50m` |
| `livenessProbe.httpGet.path` / `readinessProbe.httpGet.path` | Liveness/readiness probe endpoint paths | `/healthz` / `/readyz` |
| `livenessProbe.httpGet.port` / `readinessProbe.httpGet.port` | Liveness/readiness probe ports | `8081` / `8081` |
| `livenessProbe.initialDelaySeconds` / `readinessProbe.initialDelaySeconds` | Delay before the first liveness/readiness probe | `15` / `5` |
| `livenessProbe.periodSeconds` / `readinessProbe.periodSeconds` | Seconds between liveness/readiness probes | `20` / `10` |
| `startupProbe.enabled` | Enable the startup probe | `false` |
| `startupProbe.httpGet.path` | Startup probe endpoint path | `/readyz` |
| `startupProbe.httpGet.port` | Startup probe port | `8081` |
| `startupProbe.initialDelaySeconds` | Delay before the first startup probe | `10` |
| `startupProbe.periodSeconds` | Seconds between startup probes | `10` |
| `startupProbe.failureThreshold` | Consecutive startup probe failures before restart | `30` |
| `scheduling.nodeSelector` / `scheduling.tolerations` / `scheduling.affinity` | Pod scheduling constraints | `{}` / `[]` / `{}` |
| `scheduling.priorityClassName` | Pod priority class | `""` |
| `volumes` / `volumeMounts` | Additional operator volumes | `[]` |
| `highAvailability.podDisruptionBudget.enabled` | Create a PodDisruptionBudget | `true` |
| `highAvailability.podDisruptionBudget.minAvailable` | Minimum available Pods; rendered when `maxUnavailable` is null | `1` |
| `highAvailability.podDisruptionBudget.maxUnavailable` | Maximum unavailable Pods or percentage; takes precedence when set | `null` |
| `highAvailability.autoscaling.enabled` | Create an HPA | `false` |
| `highAvailability.autoscaling.minReplicas` / `highAvailability.autoscaling.maxReplicas` | HPA replica bounds | `1` / `3` |
| `highAvailability.autoscaling.targetCPUUtilizationPercentage` | HPA CPU target | `80` |
| `highAvailability.autoscaling.targetMemoryUtilizationPercentage` | HPA memory target | `80` |
| `vpa.enabled` | Create a VerticalPodAutoscaler | `false` |
| `vpa.updateMode` / `vpa.controlledResources` | VPA update and resource policy | `Auto` / `[cpu, memory]` |
| `vpa.maxAllowed.cpu` / `vpa.maxAllowed.memory` | Maximum VPA CPU/memory recommendations | `1` / `512Mi` |
| `vpa.minAllowed.cpu` / `vpa.minAllowed.memory` | Minimum VPA CPU/memory recommendations | `10m` / `32Mi` |
| `vpa.labels` / `vpa.annotations` | VPA metadata | `{}` / `{}` |
| `topologySpreadConstraints` | Pod topology spread constraints | `[]` |
| `prometheusRule.enabled` / `prometheusRule.additionalRules` | Create Prometheus alerting rules | `false` / `[]` |
| `prometheusRule.labels` / `prometheusRule.annotations` | PrometheusRule metadata | `{}` / `{}` |
| `grafanaDashboard.enabled` | Create a Grafana dashboard ConfigMap | `false` |
| `grafanaDashboard.labels` / `grafanaDashboard.annotations` | Grafana dashboard metadata | `{}` / `{}` |
| `deploymentStrategy.type` | Operator Deployment strategy | `RollingUpdate` |
| `deploymentStrategy.rollingUpdate.maxUnavailable` | Maximum unavailable Pods during rolling updates | `1` |
| `deploymentStrategy.rollingUpdate.maxSurge` | Maximum additional Pods during rolling updates | `1` |
| `terminationGracePeriodSeconds` | Pod termination grace period | `10` |
| `env` / `envFrom` | Additional operator environment sources | `[]` / `[]` |

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
    message:
      url: "https://api.chucknorris.io/jokes/random"
      mapping:
        title: "id"
        content: "value"
      refreshInterval: 10000
    services:
      - name: "Web Services"
        icon: "fas fa-globe"
        items:
          - name: "My App"
            logo: "https://example.com/app-logo.png"
            url: "https://myapp.example.com"
            headers:
              X-Example-Header: "example-value"
            subtitle: "Main Application"
            updateIntervalMs: 30000
            quick:
              - name: "App docs"
                icon: "fas fa-book"
                url: "https://example.com/docs"
                target: "_blank"
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
format. Legacy `parameters` blocks remain supported for existing dashboards and
annotation/discovery compatibility. In legacy parameter data, prefer `headers`
or `headers.<Header-Name>`; the legacy `customHeaders` alias remains supported,
including a bare `parameters.customHeaders` string such as
`Authorization: Bearer token`, `customHeaders.<Header-Name>` annotations, and
object/dot/slash forms. For the same key spelling, direct `item.headers` values
take precedence over `parameters.headers`, which takes precedence over
`parameters.customHeaders`. All forms are normalized to Homer's `item.headers`
object.

To use Homer's upstream external configuration behavior, set
`homerConfig.externalConfig` to a URL or path. Homer fetches that document and
ignores the remaining fields in the generated inline document; the operator
skips discovery and inline Secret injection in this mode. Use `spec.configMap`
when the operator should manage a complete Homer YAML document from a
Kubernetes ConfigMap.

For example:

```yaml
spec:
  homerConfig:
    externalConfig: https://config.example.com/homer.yml
```

Secret-backed API key, token, username, and password references are resolved
for configured smart-card service items (items with a `type`). Header Secrets
configured under `spec.secrets.headers` are resolved for configured items and
applied after discovery to items from Ingress, HTTPRoute, or Service resources,
including generic items without a `type`. For the same configured key, a
Secret-backed header overrides a direct item header and a discovery header.
Header-name matching is case-insensitive and duplicate casing variants are
normalized to one upstream header. Smart-card Secret references must resolve
in the Dashboard's namespace.

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

HTTPRoute `parentRefs` without a `namespace` refer to a Gateway in the
HTTPRoute's namespace. Gateway listeners default to accepting HTTPRoutes from
their own namespace (`allowedRoutes.namespaces.from: Same`). For a
cross-namespace attachment, set `parentRefs.namespace` and configure the
Gateway listener's `allowedRoutes.namespaces` with `All` or an appropriate
`Selector`; `None` disallows attachment. If `allowedRoutes.kinds` is present,
it must allow `HTTPRoute` from the Gateway API group (an omitted group uses
the Gateway API default). Only Gateway API `Gateway` parent references are
considered; omitted parent-reference group and kind use those same defaults.
`sectionName` and `port` restrict the matching listener. Listener hostnames
must overlap the route hostname, and a wildcard such as `*.example.com`
matches any non-apex subdomain, including deeper names, but not the apex.

For URL scheme resolution, an explicitly rejected GatewayClass
(`Accepted=False`), Gateway (`Accepted=False`, `Programmed=False`, or
`Ready=False`), or listener (`Accepted=False`, `ResolvedRefs=False`,
`Programmed=False`, or `Conflicted=True`) is not used. A listener status that
does not support HTTPRoute is also ignored. When a matching HTTPRoute parent
status is reported for the GatewayClass controller, `Accepted=True` is
required; statuses from other controllers are ignored. If a resource reports
no status at all, the resolver keeps the compatibility fallback; when a
Gateway reports listener statuses, the selected listener must have an eligible
status. Missing matching-parent status does not by itself exclude the route.
These checks affect protocol resolution, not selector-based discovery. The
operator must be able to read `gatewayclasses` in addition to `gateways` and
`httproutes` to verify GatewayClass ownership; without that permission it does
not trust a listener to select an HTTPS URL. HTTPS is
preferred when multiple eligible listeners match, and the generated URL falls
back to HTTP when no protocol is resolved.

## Monitoring

The operator exposes authenticated HTTPS Prometheus metrics on port 8443. Its
health and readiness probes listen on port 8081. The metrics Service is created
only when both `operator.metrics.enabled` and `services.metrics.enabled` are
true; disabling either gate also removes the Service and ServiceMonitor.
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
