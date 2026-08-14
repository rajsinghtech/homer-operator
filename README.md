# Homer Operator

<div align="center">
  <img width="200" alt="Homer Operator Logo" src="https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/homer/Homer-Operator.png">
  
  [![Go Report Card](https://goreportcard.com/badge/github.com/rajsinghtech/homer-operator)](https://goreportcard.com/report/github.com/rajsinghtech/homer-operator)
  [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
  [![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.21+-blue.svg)](https://kubernetes.io/)

  **Kubernetes operator for automated Homer dashboard deployment and management**

  Automatically generate beautiful, dynamic dashboards from your Kubernetes Ingress, Gateway API, and Service resources.
</div>

---

## Quick Start

### Prerequisites
- Kubernetes cluster (v1.21+; v1.23+ when enabling the optional HPA)
- `kubectl` configured
- Helm 3.x (recommended)

### Install with Helm (Recommended)

```bash
# Create namespace and install latest stable release
kubectl create namespace homer-operator
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --namespace homer-operator

# Install with Gateway API support
kubectl create namespace homer-operator
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --namespace homer-operator \
  --set operator.enableGatewayAPI=true
```

### Install with static Kustomize

The default Kustomize installation does not include a Prometheus Operator
`ServiceMonitor`, so it can be applied to a plain Kubernetes cluster:

```bash
kubectl apply -k config/default
```

The tracked Kustomize overlay follows the development `main` image. For a
version-pinned installation, use the Helm chart or the version-specific
`homer-operator-<version>-install.yaml` asset attached to the corresponding
GitHub Release.

The operator metrics `Service` is included in this installation. To add the
optional Prometheus integration, first install the Prometheus Operator and
then apply the optional operator-plus-monitor overlay documented in
[`config/prometheus/README.md`](config/prometheus/README.md).

### Create Your First Dashboard

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  namespace: default
spec:
  replicas: 2
  homerConfig:
    title: "My Dashboard"
    subtitle: "Welcome to my services"
    theme: "default"
    header: true
    footer: '<p>Powered by Homer Operator</p>'
    logo: "https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/homer/Homer-Operator.png"
    message:
      url: "https://api.chucknorris.io/jokes/random"
      mapping:
        title: "id"
        content: "value"
      refreshInterval: 10000
    services:
      - name: "Applications"
        icon: "fas fa-cloud"
        items:
          - name: "Homer"
            url: "https://github.com/bastienwirtz/homer"
            target: "_blank"
            subtitle: "Upstream Homer"
            updateIntervalMs: 30000
            quick:
              - name: "Homer docs"
                icon: "fas fa-book"
                url: "https://github.com/bastienwirtz/homer"
                target: "_blank"
  pages:
    status:
      subtitle: "A second Homer page"
      services:
        - name: "Status"
          items:
            - name: "Example"
              url: "https://example.com"
```

```bash
kubectl apply -f dashboard.yaml
```

`homerConfig.services` and `pages` use the same direct field names as
upstream Homer. The older `parameters`/annotation-oriented form remains
supported for existing dashboards and discovery workflows.

---

## Configuration Examples

### Themed Dashboard with PWA

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: neon-dashboard
spec:
  replicas: 1
  assets:
    pwa:
      enabled: true
      name: "My Dashboard PWA"
      shortName: "Dashboard"
      description: "Personal dashboard with PWA support"
      themeColor: "#00d4aa"
      backgroundColor: "#1b1b1b"
      display: "standalone"
  homerConfig:
    title: "Neon Dashboard"
    subtitle: "Cyberpunk vibes"
    theme: "neon"
    header: true
    defaults:
      layout: "columns"
      colorTheme: "dark"
```

### Dashboard with Secret Integration

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: api-keys
type: Opaque
data:
  emby-api-key: <base64-encoded-api-key>
---
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: media-dashboard
spec:
  secrets:
    apiKey:
      name: api-keys
      key: emby-api-key
  homerConfig:
    title: "Media Center"
    services:
      - parameters:
          name: "Media Services"
        items:
          - parameters:
              name: "Emby Server"
              type: "Emby"  # Smart card type
              url: "https://emby.example.com"
              libraryType: "series" # music, series, or movies
```

Smart-card Secrets must be in the Dashboard's namespace. Cross-namespace
references are rejected; remote-cluster kubeconfig references under
`spec.remoteClusters[].secretRef` are separate and may still use an explicit
namespace.

### Custom Assets & Styling

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: custom-assets
binaryData:
  logo.png: <base64-encoded-image>
  custom.css: <base64-encoded-css>
---
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: custom-dashboard
spec:
  assets:
    configMapRef:
      name: custom-assets
    icons:
      favicon: "logo.png"
      appleTouchIcon: "logo.png"
  homerConfig:
    title: "Custom Dashboard"
    stylesheet:
      - "assets/custom.css"
```

### External Homer Configuration and Shared Assets

Use `spec.configMap` when the complete Homer configuration should live in a
ConfigMap rather than inline in the Dashboard resource. The ConfigMap must be
in the Dashboard's namespace; `key` defaults to `config.yml`. Changes to the
referenced ConfigMap are watched and trigger reconciliation.

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: external-config-dashboard
  namespace: default
spec:
  configMap:
    name: homer-config
    key: config.yml
```

Custom asset ConfigMaps can be shared from another namespace by setting
`assets.configMapRef.namespace`. The operator watches the source and mirrors
it into the Dashboard's namespace so the Dashboard pod can mount it. Omit the
namespace for a same-namespace reference.

```yaml
spec:
  assets:
    configMapRef:
      name: shared-homer-assets
      namespace: platform-assets
```

Asset files are staged below Homer’s `/www/assets` directory. Use flat
filenames in the referenced ConfigMap: the operator mounts ConfigMap keys at
the asset root and does not create `items[].path` projections, so nested names
such as `assets/tools/sample.png` are not a portable `ConfigMap` representation.
Configured icon sources can still be copied to Homer’s canonical `icons/`
paths (`favicon.ico`, `apple-touch-icon.png`, and the PWA icon paths). When PWA
support is enabled, the operator also writes `manifest.json`; icon paths in
that manifest match the staged paths. Changing or replacing a cross-namespace
source updates its owned mirror, and stale mirrors are removed when the
Dashboard reference changes.

---

## Advanced Configuration

### Gateway API Support

Enable HTTPRoute processing for modern Kubernetes networking:

```bash
# Install Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml

# Install operator with Gateway API support
kubectl create namespace homer-operator
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --namespace homer-operator \
  --set operator.enableGatewayAPI=true
```

#### Advanced Filtering Options

Control exactly which resources are included in your dashboard with comprehensive filtering options:

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: production-dashboard
spec:
  # Filter HTTPRoutes by Gateway labels
  gatewaySelector:
    matchLabels:
      environment: "production"
      gateway: "public"
    matchExpressions:
    - key: "app.kubernetes.io/name" 
      operator: In
      values: ["istio-gateway", "envoy-gateway", "nginx-gateway"]
  
  # Filter HTTPRoutes by their own labels
  httpRouteSelector:
    matchLabels:
      team: "platform"
      tier: "frontend"
    matchExpressions:
    - key: "app.kubernetes.io/component"
      operator: In
      values: ["api", "service", "web"]
  
  # Filter Ingresses by labels
  ingressSelector:
    matchLabels:
      environment: "production"
      public: "true"
    matchExpressions:
    - key: "kubernetes.io/ingress.class"
      operator: In
      values: ["nginx", "traefik"]
  
  # Filter by hostname/domain (works for both HTTPRoutes and Ingresses)
  domainFilters:
    - "mycompany.com"      # Exact match: mycompany.com
    - "internal.local"     # Subdomain match: *.internal.local
    - "rajsingh.info"      # Both exact and subdomain matching
  
  homerConfig:
    title: "Production Services"
    subtitle: "Filtered production endpoints"
    # ... rest of config
```

**Filtering Capabilities:**

| Filter Type | Description | Applies To | Default Behavior |
|-------------|-------------|------------|------------------|
| `gatewaySelector` | Filter HTTPRoutes by parent Gateway labels | HTTPRoutes only | Include all HTTPRoutes |
| `httpRouteSelector` | Filter HTTPRoutes by their own labels | HTTPRoutes only | Include all HTTPRoutes |
| `ingressSelector` | Filter Ingresses by their labels | Ingresses only | Include all Ingresses |
| `domainFilters` | Filter by hostname/domain names | Both HTTPRoutes & Ingresses | Include all domains |
| `serviceSelector` | Discover Kubernetes Services by labels | Services only | No Services discovered |

**Domain Filtering Examples:**
- `example.com` - Matches exactly `example.com`
- `internal.local` - Matches `api.internal.local`, `web.internal.local`, etc.
- Multiple filters are OR'd together (any match includes the resource)

**Real-world Use Cases:**
- **Environment Separation**: Production vs staging dashboards
- **Team Dashboards**: Platform team vs application team services  
- **Security Zones**: Public vs internal service separation
- **Domain Organization**: Company domains vs personal projects
- **Gateway Migration**: Gradual migration between gateway implementations

### Kubernetes Service Discovery

Discover internal Kubernetes Services and add them to your dashboard — useful for cluster-internal services that don't have Ingress or HTTPRoute resources:

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: internal-dashboard
spec:
  # Discover Services matching these labels
  serviceSelector:
    matchLabels:
      homer.rajsingh.info/enabled: "true"

  homerConfig:
    title: "Internal Services"
```

Annotate your Services to customize their dashboard appearance:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api
  namespace: backend
  labels:
    homer.rajsingh.info/enabled: "true"
  annotations:
    item.homer.rajsingh.info/name: "Backend API"
    item.homer.rajsingh.info/subtitle: "Core API service"
    item.homer.rajsingh.info/logo: "https://example.com/api-logo.png"
    item.homer.rajsingh.info/type: "Ping"
    service.homer.rajsingh.info/name: "Backend"
    service.homer.rajsingh.info/icon: "fas fa-cogs"
spec:
  ports:
    - port: 8080
```

Service URLs are automatically generated as `http://<name>.<namespace>.svc.cluster.local:<port>` for local-cluster Services. Unlike Ingress/HTTPRoute discovery, Services are **opt-in** — they are only discovered when `serviceSelector` is specified. For remote-cluster Services, the operator keeps the item visible but omits that local-cluster DNS URL unless `item.homer.rajsingh.info/url` supplies an explicit reachable URL; this prevents a link that points at the wrong cluster.

### Annotation-driven Service Discovery

Control how your services appear on dashboards using annotations on both Ingress and HTTPRoute resources:

```yaml
# Traditional Ingress
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app-ingress
  annotations:
    # Item configuration
    item.homer.rajsingh.info/name: "My Application"
    item.homer.rajsingh.info/subtitle: "Production instance"
    item.homer.rajsingh.info/logo: "https://example.com/logo.png"
    item.homer.rajsingh.info/tag: "production"
    item.homer.rajsingh.info/keywords: "app, api, service"
    
    # Service group configuration
    service.homer.rajsingh.info/name: "Production Services"
    service.homer.rajsingh.info/icon: "fas fa-server"
spec:
  # ... ingress configuration
---
# Gateway API HTTPRoute
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-app-route
  annotations:
    # Same annotations work for HTTPRoute!
    item.homer.rajsingh.info/name: "My Application (Gateway API)"
    item.homer.rajsingh.info/subtitle: "Modern routing"
    item.homer.rajsingh.info/logo: "https://example.com/logo.png"
    item.homer.rajsingh.info/tag: "gateway-api"
    
    service.homer.rajsingh.info/name: "Gateway Services"
    service.homer.rajsingh.info/icon: "fas fa-route"
spec:
  # ... httproute configuration
```

### Dynamic Annotation System

The Homer Operator features an intelligent, convention-based annotation system that supports **any Homer configuration parameter** automatically, without requiring code changes:

#### Smart Type Detection

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: smart-app
  annotations:
    # Basic parameters (auto-detected as strings)
    item.homer.rajsingh.info/name: "Smart Application"
    item.homer.rajsingh.info/subtitle: "AI-powered service"
    
    # Boolean parameters (case-insensitive, multiple formats)
    item.homer.rajsingh.info/useCredentials: "TRUE"  # or "true", "yes", "1"
    item.homer.rajsingh.info/legacyapi: "false"      # or "FALSE", "no", "0"
    
    # Integer parameters (auto-detected by naming patterns)
    item.homer.rajsingh.info/timeout: "30"           # *_value, *Interval patterns
    item.homer.rajsingh.info/updateInterval: "5000"  # Auto-detected as integer
    item.homer.rajsingh.info/warning_value: "80"     # Threshold parameters
    item.homer.rajsingh.info/danger_value: "95"      # Auto-validated
    
    # Array parameters (comma-separated, auto-cleaned)
    item.homer.rajsingh.info/keywords: " api , service , smart "  # Spaces trimmed
    
    # Headers are emitted as Homer's item.headers object.
    item.homer.rajsingh.info/headers: "Authorization: Bearer token, Content-Type: application/json"

    # A single header can also use dot notation.
    item.homer.rajsingh.info/headers.Authorization: "Bearer token"
```

#### Nested Object Support

Supports complex nested configurations using dot notation, which is valid in
Kubernetes annotation keys:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: advanced-app
  annotations:
    # Legacy customHeaders dot notation is also converted to item.headers.
    item.homer.rajsingh.info/customHeaders.Authorization: "Bearer secret-token"
    item.homer.rajsingh.info/customHeaders.X-API-Key: "api-key-123"
    item.homer.rajsingh.info/customHeaders.Content-Type: "application/json"
    
    # Nested object: mapping (for smart cards)
    item.homer.rajsingh.info/mapping.status: "health.status"
    item.homer.rajsingh.info/mapping.version: "info.version"
    
    # Any Homer parameter works automatically!
    item.homer.rajsingh.info/checkInterval: "30000"      # Smart card refresh
    item.homer.rajsingh.info/location: "US-East"         # Custom parameters
    item.homer.rajsingh.info/environment: "production"   # User-defined fields
```

#### Hide Items from Dashboard

Control item visibility using the `hide` annotation:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: maintenance-app
  annotations:
    item.homer.rajsingh.info/name: "Maintenance Service"
    item.homer.rajsingh.info/subtitle: "Currently under maintenance"
    
    # Hide this item from the dashboard
    # Supports flexible boolean values (case-insensitive)
    item.homer.rajsingh.info/hide: "true"     # true, yes, 1, on
    # item.homer.rajsingh.info/hide: "false"  # false, no, 0, off
    # item.homer.rajsingh.info/hide: "maintenance-mode"  # Any non-empty string = true
spec:
  rules:
  - host: maintenance.example.com
    # ... rest of spec

---
# HTTPRoute example
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: beta-api
  annotations:
    item.homer.rajsingh.info/name: "Beta API"
    item.homer.rajsingh.info/hide: "yes"  # Hide during beta testing
spec:
  hostnames:
  - beta-api.example.com
  # ... rest of spec
```

**Hide Annotation Values:**
- **Hidden**: `true`, `yes`, `1`, `on`, or any non-empty string
- **Visible**: `false`, `no`, `0`, `off`, or empty string (default)
- Case-insensitive and works with both Ingress and HTTPRoute resources

#### Convention-Based Intelligence

The system automatically detects parameter types using intelligent patterns:

- **Booleans**: Parameters ending in `_enabled`, `_flag` or named `usecredentials`, `legacyapi`, `hide`
- **Integers**: Parameters ending in `Interval`, `_value`, `Value` or named `timeout`, `limit`
- **Objects**: Parameters named `headers` and `mapping`; `customHeaders` is a legacy alias for `headers`
- **Arrays**: Comma-separated values (automatically cleaned and trimmed)
- **Validation**: Built-in validation for `url`, `target`, numeric values

#### Common Annotation Parameters

| Annotation | Type | Description | Example Values |
|------------|------|-------------|----------------|
| `item.homer.rajsingh.info/name` | String | Display name for the item | `"My Application"` |
| `item.homer.rajsingh.info/subtitle` | String | Subtitle/description | `"Production API"` |
| `item.homer.rajsingh.info/logo` | String | URL to logo/icon | `"https://example.com/logo.png"` |
| `item.homer.rajsingh.info/tag` | String | Tag label | `"production"`, `"api"` |
| `item.homer.rajsingh.info/tagstyle` | String | Tag color style | `"is-primary"`, `"is-info"` |
| `item.homer.rajsingh.info/keywords` | String | Comma-separated search keywords | `"api, service, web"` |
| `item.homer.rajsingh.info/hide` | Boolean | Hide item from dashboard | `"true"`, `"false"`, `"yes"`, `"no"` |
| `item.homer.rajsingh.info/target` | String | Link target | `"_blank"`, `"_self"` |
| `item.homer.rajsingh.info/type` | String | Smart card type | `"Ping"`, `"Emby"`, `"AdGuardHome"` |
| `service.homer.rajsingh.info/name` | String | Service group name | `"Production Services"` |
| `service.homer.rajsingh.info/icon` | String | Service group icon | `"fas fa-server"` |

### Production Deployment

```yaml
# values.yaml
replicaCount: 3

operator:
  enableGatewayAPI: true
  metrics:
    enabled: true
    secureMetrics: true

resources:
  limits:
    memory: 512Mi
    cpu: 1000m
  requests:
    memory: 256Mi
    cpu: 100m

highAvailability:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 5
    targetCPUUtilizationPercentage: 80
  podDisruptionBudget:
    enabled: true
    minAvailable: 1

serviceMonitor:
  enabled: true
  interval: 30s
```

```bash
kubectl create namespace homer-operator
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --namespace homer-operator \
  --version 1.2.3 -f values.yaml
```

---

## Monitoring & Observability

### Prometheus Metrics

The operator exposes comprehensive metrics:

```bash
# Enable metrics and ServiceMonitor
kubectl create namespace homer-operator
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --namespace homer-operator \
  --set operator.metrics.enabled=true \
  --set serviceMonitor.enabled=true
```

For the static Kustomize installation, `config/default` includes the
authenticated metrics Service but intentionally omits `ServiceMonitor`.
`kubectl apply -k config/prometheus` adds the monitor only after the
Prometheus Operator CRD is installed. The overlay cannot infer the Prometheus
service account, so see its README for the intentional manual binding to
`homer-operator-metrics-reader`.

**Available Metrics:**
- `controller_runtime_reconcile_total` - Reconciliation counter
- `controller_runtime_reconcile_time_seconds` - Reconciliation duration
- Standard controller-runtime metrics for monitoring operator health

The metrics Service is created only when both `operator.metrics.enabled` and
`services.metrics.enabled` are true. The ServiceMonitor additionally requires
`serviceMonitor.enabled=true`.

Secure metrics use controller-runtime TokenReview/SubjectAccessReview
authorization. When `serviceMonitor.enabled=true` and secure metrics remain
enabled, the Helm chart creates a dedicated scraper ServiceAccount and token
Secret, grants it `/metrics` access, and references that Secret from the
ServiceMonitor. Disabling secure metrics switches the ServiceMonitor to HTTP
and removes the auth resources.

### Health Endpoints

- **Liveness**: `GET /healthz`
- **Readiness**: `GET /readyz`  
- **Metrics**: `GET https://<operator-service>:8443/metrics` (authenticated)

---

## Migration & Compatibility

### From Static Homer Configurations

The operator maintains 95%+ compatibility with existing Homer configurations:

```yaml
# Your existing config.yml works directly in homerConfig
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: migrated-dashboard
spec:
  homerConfig:
    # Paste your existing Homer config here
    title: "My Existing Dashboard"
    subtitle: "Migrated from static config"
    # ... rest of your config
```

### Ingress to Gateway API Migration

The operator supports both simultaneously for zero-downtime migration:

1. **Phase 1**: Enable Gateway API
   ```bash
   helm upgrade homer-operator charts/homer-operator --set operator.enableGatewayAPI=true
   ```

2. **Phase 2**: Install Gateway API CRDs and create Gateway resources
   ```bash
   kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml
   ```

3. **Phase 3**: Migrate services to HTTPRoute resources
   - Convert Ingress rules to HTTPRoute specs
   - Use same label selectors and domain filters
   - Test HTTPRoute discovery with existing dashboards

4. **Phase 4**: Update Dashboard selectors and deprecate Ingress
   ```yaml
   spec:
     ingressSelector: null      # Disable Ingress discovery
     httpRouteSelector:         # Enable HTTPRoute discovery
       matchLabels:
         homer.rajsingh.info/enabled: "true"
   ```

---

## Multi-Cluster Support

Discover and aggregate services from multiple Kubernetes clusters into a single unified dashboard.

### Features

- **Multiple Cluster Connections**: Connect to any number of remote clusters using kubeconfig secrets
- **Automatic Secret Rotation**: Detects kubeconfig changes and automatically reconnects without pod restarts
- **Per-Cluster Filtering**: Apply namespace, label, domain, and service filters independently per cluster
- **Cluster Metadata**: Automatically enriches discovered services with cluster information
- **Status Tracking**: Monitor connection status and resource counts per cluster
- **Secure Authentication**: Token-based authentication with RBAC support

### Quick Start

1. **Create a read-only service account in the remote cluster:**

```bash
# On remote cluster
kubectl create namespace kube-system
kubectl create serviceaccount homer-reader -n kube-system

# Create ClusterRole with read permissions
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: homer-reader
rules:
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["httproutes", "gateways"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["namespaces", "services"]
  verbs: ["get", "list", "watch"]
EOF

# Bind the role
kubectl create clusterrolebinding homer-reader \
  --clusterrole=homer-reader \
  --serviceaccount=kube-system:homer-reader

# Create long-lived token
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: homer-reader-token
  namespace: kube-system
  annotations:
    kubernetes.io/service-account.name: homer-reader
type: kubernetes.io/service-account-token
EOF
```

2. **Generate kubeconfig for the remote cluster:**

```bash
# Get token and CA cert
TOKEN=$(kubectl get secret homer-reader-token -n kube-system -o jsonpath='{.data.token}' | base64 -d)
CA_CERT=$(kubectl get secret homer-reader-token -n kube-system -o jsonpath='{.data.ca\.crt}')
SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')

# Create kubeconfig
cat > remote-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- name: production
  cluster:
    certificate-authority-data: ${CA_CERT}
    server: ${SERVER}
contexts:
- name: production
  context:
    cluster: production
    user: homer-reader
current-context: production
users:
- name: homer-reader
  user:
    token: ${TOKEN}
EOF
```

3. **Create secret in the main cluster:**

```bash
# On main cluster where operator is running
kubectl create secret generic remote-cluster-kubeconfig \
  --from-file=kubeconfig=remote-kubeconfig.yaml \
  -n default
```

4. **Create multi-cluster dashboard:**

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: multi-cluster-dashboard
  namespace: default
spec:
  replicas: 1

  # Configure remote clusters
  remoteClusters:
    - name: production
      enabled: true
      secretRef:
        name: remote-cluster-kubeconfig
        namespace: default
        key: kubeconfig

      # Optional: Filter resources in remote cluster
      namespaceFilter:
        - default
        - production

      # Optional: Add cluster labels to discovered resources
      clusterLabels:
        cluster: production
        region: us-east-1

      # Optional: Apply label selectors to this remote cluster
      ingressSelector:
        matchLabels:
          environment: production

      domainFilters:
        - "mycompany.com"

  homerConfig:
    title: "Multi-Cluster Dashboard"
    subtitle: "Services from multiple clusters"
```

### Configuration Options

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: advanced-multicluster
spec:
  remoteClusters:
    - name: cluster-name
      enabled: true  # Set to false to temporarily disable

      # Kubeconfig secret reference (required)
      secretRef:
        name: kubeconfig-secret-name
        namespace: secret-namespace
        key: kubeconfig  # Key in secret containing kubeconfig

      # Namespace filtering (optional)
      namespaceFilter:
        - namespace-1
        - namespace-2

      # Cluster labels (optional) - added to all discovered resources
      clusterLabels:
        cluster: production
        region: us-west-2
        team: platform
        cluster-tagstyle: "is-danger"  # Optional: Customize badge color (red/yellow/blue/green/gray)

      # Resource filtering (optional) - same as main cluster
      ingressSelector:
        matchLabels:
          app: web

      httpRouteSelector:
        matchLabels:
          gateway: public

      gatewaySelector:
        matchLabels:
          type: ingress

      # Service discovery (optional) - discover K8s Services by labels
      serviceSelector:
        matchLabels:
          homer.rajsingh.info/enabled: "true"

      # Per-cluster domain filtering (optional); remote clusters do not inherit
      # the dashboard-level domainFilters.
      domainFilters:
        - "prod.example.com"     # Only production domains from this cluster
        - "api.example.com"      # API endpoints
```

**Cluster Name Suffix:**
Append custom suffixes to service display names to distinguish items from different clusters. Configure using the `cluster-name-suffix` label in `clusterLabels`:

```yaml
remoteClusters:
  - name: ottawa
    clusterLabels:
      cluster-name-suffix: " (ottawa)"      # -> "Service Name (ottawa)"
      # Other formats:
      # " - ottawa"     -> "Service Name - ottawa"
      # " [ottawa]"     -> "Service Name [ottawa]"
```

This suffix is only applied to items from **remote clusters**, not the local cluster. If the label is not set, no suffix is appended.

**Automatic Cluster Tagging (Optional):**
Services from remote clusters can also get badge tags with the cluster name when `cluster-tagstyle` is set in `clusterLabels`. Available colors:
- `is-danger` (red) - production
- `is-warning` (yellow) - staging
- `is-info` (blue) - development
- `is-success` (green) - QA/testing
- `is-light` (gray) - deprecated

**Important:** Tags are only added when `cluster-tagstyle` is explicitly configured. You can use both suffix and tags, or just one approach for cluster identification.

**Multi-cluster selector and domain semantics:**

- Dashboard-level `ingressSelector`, `httpRouteSelector`, and `gatewaySelector` apply only to the local cluster. Remote clusters use their corresponding selector, when configured; omission includes all resources of that type.
- A remote `serviceSelector` overrides the dashboard-level `serviceSelector`. If omitted, the dashboard-level service selector is inherited. Services remain opt-in: without either selector, no Services are discovered.
- Dashboard-level `domainFilters` apply only to the local cluster. Remote clusters use their own `domainFilters`; omission includes all remote Ingress and HTTPRoute hostnames.
- `namespaceFilter` limits discovery in that remote cluster. An empty or omitted filter means all namespaces allowed by the remote RBAC rules.

### Status Monitoring

Check multi-cluster connection status:

```bash
kubectl get dashboard multi-cluster-dashboard -o jsonpath='{.status.clusterStatuses}' | jq
```

Example output:
```json
[
  {
    "name": "production",
    "connected": true,
    "lastConnectionTime": "2025-10-02T10:39:15Z",
    "discoveredIngresses": 5,
    "discoveredHTTPRoutes": 3
  },
  {
    "name": "staging",
    "connected": false,
    "lastError": "failed to connect: unauthorized",
    "lastConnectionTime": "2025-10-02T10:38:00Z"
  }
]
```

### Automatic Secret Rotation

The operator automatically detects kubeconfig changes and reconnects:

```bash
# Update the secret with new credentials
kubectl create secret generic remote-cluster-kubeconfig \
  --from-file=kubeconfig=new-kubeconfig.yaml \
  -n default \
  --dry-run=client -o yaml | kubectl apply -f -

# Operator automatically detects the change and reconnects
# Check logs to verify:
kubectl logs -n <operator-namespace> deployment/<operator-deployment>
```

### Cluster Metadata

Resources discovered through the multi-cluster path carry the source cluster
in the `homer.rajsingh.info/cluster` annotation. Remote resources also receive
the user-defined `clusterLabels` as Kubernetes labels; the operator does not
create a separate `homer.rajsingh.info/source-cluster` annotation. The source
annotation is used internally for deterministic item cleanup and for optional
cluster suffixes/tags in the generated Homer configuration.

### Security Considerations

- Use read-only service accounts with minimal RBAC permissions
- Store kubeconfigs in Kubernetes secrets
- Use network policies to restrict operator egress
- Rotate tokens regularly using automatic secret rotation
- Consider using certificate-based authentication for production

### Troubleshooting

**Connection failures:**
```bash
# Check Dashboard status
kubectl get dashboard <name> -o yaml

# Check operator logs
kubectl logs -n <operator-namespace> deployment/<operator-deployment>

# Verify secret exists and is readable
kubectl get secret <secret-name> -n <namespace> -o yaml

# Test kubeconfig manually
kubectl --kubeconfig=<path> get namespaces
```

**Common issues:**
- **Connection refused**: Ensure cluster API server is accessible from operator pod
- **Unauthorized**: Verify service account has correct RBAC permissions
- **Certificate errors**: Check certificate-authority-data in kubeconfig
- **No resources discovered**: Verify namespace filters and label selectors

---

### Local Development

```bash
# Clone repository
git clone https://github.com/rajsinghtech/homer-operator.git
cd homer-operator

# Install dependencies
make install

# Run locally against cluster
make run

# Build and deploy
make docker-build IMG=your-registry/homer-operator:dev
make deploy IMG=your-registry/homer-operator:dev
```

### Testing

```bash
# Run unit tests
make test

# Run the black-box suite against a dedicated, isolated cluster
E2E_KUBECONFIG=$PWD/.kube/homer-operator-e2e \
make test-e2e

# For the default Kustomize installation, override the operator location
E2E_KUBECONFIG=$PWD/.kube/homer-operator-e2e \
E2E_OPERATOR_NAMESPACE=homer-operator-system \
E2E_OPERATOR_DEPLOYMENT=homer-operator-controller-manager \
make test-e2e

# CI installs the Helm chart, applies config/samples/
# homer_v1alpha1_dashboard.yaml, verifies its generated page asset, and runs
# the same black-box suite.

# Generate manifests
make manifests
```

---

## License

This project is licensed under the [Apache License 2.0](LICENSE).

---

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=rajsinghtech/homer-operator&type=Date)](https://star-history.com/#rajsinghtech/homer-operator&Date)

Release notes are published in [GitHub Releases](https://github.com/rajsinghtech/homer-operator/releases).

---

<div align="center">

*Built with love using [Kubebuilder](https://kubebuilder.io/) and [Homer](https://github.com/bastienwirtz/homer)*

</div>
