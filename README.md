# Homer Operator

<div align="center">
  <img width="200" alt="Homer Operator Logo" src="https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/homer/Homer-Operator.png">
  
  [![Go Report Card](https://goreportcard.com/badge/github.com/rajsinghtech/homer-operator)](https://goreportcard.com/report/github.com/rajsinghtech/homer-operator)
  [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
  [![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.20+-blue.svg)](https://kubernetes.io/)

  **🚀 Kubernetes operator for automated Homer dashboard deployment and management**
  
  Automatically generate beautiful, dynamic dashboards from your Kubernetes Ingress and Gateway API resources.
</div>

---

## ✨ Features

### 🎯 **Core Capabilities**
- **Automatic Dashboard Generation** - Discover and display services from Ingress & HTTPRoute resources
- **Ingress & Gateway API Compatible** - Full support for both traditional Ingress and modern Gateway API HTTPRoute
- **Multi-tenancy Support** - Deploy multiple dashboards in the same namespace
- **High Availability** - Configurable replicas, HPA, and Pod Disruption Budgets
- **Production Ready** - Comprehensive Helm chart with RBAC and security contexts

### 🎨 **Customization & Theming**
- **Custom Themes** - Built-in support for `default`, `neon`, and `walkxcode` themes
- **Custom Assets** - Upload logos, icons, and CSS via ConfigMaps
- **Color Schemes** - Extensive light/dark theme customization
- **PWA Support** - Progressive Web App manifests for mobile installation

### 🔒 **Security & Integration**
- **Secret Management** - Kubernetes Secret integration for API keys
- **RBAC Ready** - Minimal required permissions with owner references
- **Dual Networking Support** - Works with both Ingress (v1) and Gateway API HTTPRoute (v1)
- **Annotation-driven** - Fine-grained control via Kubernetes annotations

### 📊 **Operations & Monitoring**
- **Prometheus Metrics** - Built-in monitoring and observability
- **Health Checks** - Liveness and readiness probes
- **OCI Registry** - Helm charts published to GitHub Container Registry

---

## 🚀 Quick Start

### Prerequisites
- Kubernetes cluster (v1.20+)
- `kubectl` configured
- Helm 3.x (recommended)

### 1. Install with Helm (Recommended)

```bash
# Install latest stable release
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator

# Install with Gateway API support
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --set operator.enableGatewayAPI=true
```

### 2. Create Your First Dashboard

```yaml
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  namespace: default
spec:
  replicas: 2
  homerConfig:
    title: "🏠 My Dashboard"
    subtitle: "Welcome to my services"
    theme: "default"
    header: true
    footer: '<p>Powered by Homer Operator</p>'
    logo: "https://raw.githubusercontent.com/rajsinghtech/homer-operator/main/homer/Homer-Operator.png"
```

```bash
kubectl apply -f dashboard.yaml
```

### 3. Access Your Dashboard

```bash
# Port-forward to access locally
kubectl port-forward svc/my-dashboard-homer 8080:80

# Open http://localhost:8080
```

---

## 📖 Configuration Examples

### 🎨 Themed Dashboard with PWA

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
    title: "🌟 Neon Dashboard"
    subtitle: "Cyberpunk vibes"
    theme: "neon"
    header: true
    defaults:
      layout: "columns"
      colorTheme: "dark"
```

### 🔐 Dashboard with Secret Integration

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: api-keys
type: Opaque
data:
  plex-token: <base64-encoded-token>
---
apiVersion: homer.rajsingh.info/v1alpha1
kind: Dashboard
metadata:
  name: media-dashboard
spec:
  secrets:
    apiKey:
      name: api-keys
      key: plex-token
  homerConfig:
    title: "📺 Media Center"
    services:
      - name: "Media Services"
        items:
          - name: "Plex Server"
            type: "Emby"  # Smart card type
            url: "https://plex.example.com"
            # Note: Configure API key reference in smart card configuration
```

### 🎨 Custom Assets & Styling

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
    title: "🎨 Custom Dashboard"
    stylesheet:
      - "custom.css"
```

---

## 🛠️ Advanced Configuration

### Gateway API Support

Enable HTTPRoute processing for modern Kubernetes networking:

```bash
# Install Gateway API CRDs
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

# Install operator with Gateway API support
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
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
    title: "🏭 Production Services"
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

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 80

serviceMonitor:
  create: true
  interval: 30s

podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

```bash
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --version 0.1.0 -f values.yaml
```

---

## 📊 Monitoring & Observability

### Prometheus Metrics

The operator exposes comprehensive metrics:

```bash
# Enable metrics and ServiceMonitor
helm install homer-operator oci://ghcr.io/rajsinghtech/homer-operator/charts/homer-operator \
  --set operator.metrics.enabled=true \
  --set serviceMonitor.create=true
```

**Available Metrics:**
- `controller_runtime_reconcile_total` - Reconciliation counter
- `controller_runtime_reconcile_time_seconds` - Reconciliation duration
- Standard controller-runtime metrics for monitoring operator health

### Health Endpoints

- **Liveness**: `GET /healthz`
- **Readiness**: `GET /readyz`  
- **Metrics**: `GET /metrics`

---

## 🔄 Migration & Compatibility

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

The operator supports both simultaneously - migrate gradually:

1. **Phase 1**: Enable Gateway API alongside existing Ingress
2. **Phase 2**: Migrate services to HTTPRoute resources  
3. **Phase 3**: Deprecate Ingress resources

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

# Run end-to-end tests
make test-e2e

# Generate manifests
make manifests
```

---

## 📄 License

This project is licensed under the [Apache License 2.0](LICENSE).

---

<div align="center">
  
**⭐ Star this project if you find it useful!**

*Built with ❤️ using [Kubebuilder](https://kubebuilder.io/) and [Homer](https://github.com/bastienwirtz/homer)*

</div>