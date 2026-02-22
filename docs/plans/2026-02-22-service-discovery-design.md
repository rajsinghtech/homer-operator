# Service Resource Discovery

## Problem

Homer operator only discovers Ingress and HTTPRoute resources for dashboard items. Kubernetes Services — which are the most common way to expose applications — cannot automatically appear on dashboards.

## Design

Add Service as a third discoverable resource type using the same parallel pipeline pattern as Ingress and HTTPRoute.

### Opt-In Model

Services require a `serviceSelector` label selector in the Dashboard CRD. Nil selector = no Service discovery. The label selector IS the opt-in mechanism — no additional annotations needed.

### URL Construction

1. `item.homer.rajsingh.info/url` annotation (if set) — used as-is
2. Fallback: `http://<name>.<namespace>.svc.cluster.local:<first-port>`
3. Port 443 uses `https://` automatically

First port in the Service spec is used. Override with the url annotation for specific ports.

### CRD Changes (dashboard_types.go)

New fields:
- `DashboardSpec.ServiceSelector` — `*metav1.LabelSelector`, nil means no Service discovery
- `RemoteCluster.ServiceSelector` — per-cluster Service filtering
- `ClusterConnectionStatus.DiscoveredServices` — count field for status

### Controller Changes (dashboard_controller.go)

- `shouldIncludeService()` — label selector validation (no domain filtering)
- `findDashboardsForService()` — watch mapper, only triggers if dashboard has ServiceSelector
- Watch registration in `SetupWithManager()` for `corev1.Service`
- `Reconcile()` passes filtered Services to config generation
- `createConfigMap()` processes Services alongside Ingresses/HTTPRoutes

### Config Generation (pkg/homer/config.go)

- `UpdateHomerConfigService()` — converts K8s Service to Homer items
- Same annotation pipeline: `processServiceAnnotations()` + `processItemAnnotations()`
- Item defaults: name = Service name, subtitle = `<namespace>/<name>`
- Distinct default icon for Services

### ClusterManager (cluster_manager.go)

- `DiscoverServices()` — multi-cluster discovery parallel to Ingresses/HTTPRoutes
- `discoverClusterServices()` — per-cluster namespace filtering, label selection, metadata
- Namespace annotation merging

### RBAC

No changes needed. Operator already has full Service RBAC for managing the Homer Service deployment.

### Out of Scope

- No domain filtering (Services don't have hostnames)
- No Gateway selector interaction
- One Homer item per Service (not per-port)

## Example Usage

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
spec:
  serviceSelector:
    matchLabels:
      homer-dashboard: "true"
  remoteClusters:
  - name: prod
    secretRef:
      name: prod-kubeconfig
    serviceSelector:
      matchLabels:
        homer-dashboard: "true"
```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app
  namespace: default
  labels:
    homer-dashboard: "true"
  annotations:
    item.homer.rajsingh.info/name: "My Application"
    item.homer.rajsingh.info/url: "https://myapp.example.com"
    item.homer.rajsingh.info/subtitle: "Production App"
    service.homer.rajsingh.info/name: "Applications"
spec:
  ports:
  - port: 8080
    targetPort: 8080
```
