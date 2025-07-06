# Homer Operator Helm Chart

A Helm chart for deploying the Homer Operator on Kubernetes. The Homer Operator manages Homer dashboard instances that automatically discover and display your Kubernetes services.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.8+

## Installation

### Install from OCI Registry (Recommended)

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator --version 0.0.0-latest -n homer-operator --create-namespace
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
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `operator.enableGatewayAPI` | Enable Gateway API support | `false` |
| `operator.metrics.enabled` | Enable metrics collection | `true` |
| `operator.metrics.secureMetrics` | Use secure metrics serving | `true` |
| `serviceAccount.create` | Create service account | `true` |
| `rbac.create` | Create RBAC resources | `true` |
| `crd.create` | Create CustomResourceDefinitions | `true` |
| `namespace.create` | Create namespace | `true` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `resources.requests.cpu` | CPU request | `10m` |

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
  create: true
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
```

## Troubleshooting

### Namespace Creation Issues

If you encounter `namespaces 'homer-operator' not found`, add the `--create-namespace` flag:

```bash
helm upgrade --install homer-operator charts/homer-operator -n homer-operator --create-namespace
```

## Gateway API Support

To enable Gateway API support, set `operator.enableGatewayAPI=true`. This requires Gateway API CRDs to be installed in your cluster.

## Monitoring

The operator exposes Prometheus metrics on port 8080. To enable monitoring:

```yaml
operator:
  metrics:
    enabled: true

serviceMonitor:
  create: true
  interval: 30s
```

## Security

The operator follows security best practices:

- Runs as non-root user (UID 65532)
- Uses read-only root filesystem
- Drops all capabilities
- Implements least-privilege RBAC

## Uninstalling

```bash
helm uninstall homer-operator -n homer-operator
```

Note: This will not remove the CustomResourceDefinitions or the namespace. To remove them:

```bash
kubectl delete crd dashboards.homer.rajsingh.info
kubectl delete namespace homer-operator
```

## Support

For issues and questions, please visit the [GitHub repository](https://github.com/rajsinghtech/homer-operator).