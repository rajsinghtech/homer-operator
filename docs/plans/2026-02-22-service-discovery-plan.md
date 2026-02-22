# Service Discovery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Kubernetes Service as a third discoverable resource type so Services with matching labels automatically appear on Homer dashboards.

**Architecture:** Parallel pipeline replicating the existing Ingress/HTTPRoute patterns — CRD selector field, controller watch + filter, ClusterManager discovery, config generation. Services require an explicit `serviceSelector` (nil = no discovery). URL falls back to `<name>.<ns>.svc.cluster.local:<port>`.

**Tech Stack:** Go, controller-runtime, kubebuilder CRDs, Helm charts

---

### Task 1: Add ServiceSelector to CRD types

**Files:**
- Modify: `api/v1alpha1/dashboard_types.go:60-70` (DashboardSpec selectors)
- Modify: `api/v1alpha1/dashboard_types.go:91-134` (RemoteCluster struct)
- Modify: `api/v1alpha1/dashboard_types.go:174-193` (ClusterConnectionStatus struct)

**Step 1: Add ServiceSelector to DashboardSpec**

In `api/v1alpha1/dashboard_types.go`, add after IngressSelector (line 67):

```go
// ServiceSelector optionally filters Services by labels for discovery.
// Unlike Ingress/HTTPRoute, Services are only discovered when a selector is specified.
// If not specified, no Services will be discovered.
ServiceSelector *metav1.LabelSelector `json:"serviceSelector,omitempty"`
```

**Step 2: Add ServiceSelector to RemoteCluster**

After HTTPRouteSelector (line 124):

```go
// ServiceSelector optionally filters Services by labels in this cluster.
// Works the same as the main serviceSelector but only applies to this cluster.
ServiceSelector *metav1.LabelSelector `json:"serviceSelector,omitempty"`
```

**Step 3: Add DiscoveredServices to ClusterConnectionStatus**

After DiscoveredHTTPRoutes (line 192):

```go
// DiscoveredServices is the count of Services discovered from this cluster
DiscoveredServices int `json:"discoveredServices,omitempty"`
```

**Step 4: Regenerate CRDs and deep copy**

Run: `make manifests && make generate`

**Step 5: Verify it compiles**

Run: `go build ./...`
Expected: Clean build, no errors

**Step 6: Commit**

```
feat: add ServiceSelector fields to Dashboard CRD

Adds serviceSelector to DashboardSpec and RemoteCluster for
Service-based discovery, plus DiscoveredServices status counter.
```

---

### Task 2: Add UpdateHomerConfigService in config.go

**Files:**
- Modify: `pkg/homer/config.go` (after UpdateHomerConfigIngressWithGrouping ~line 1083)
- Create: `pkg/homer/service_discovery_test.go`

**Step 1: Write failing tests**

Create `pkg/homer/service_discovery_test.go`:

```go
package homer

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func makeService(name, namespace string, port int32, annotations map[string]string) corev1.Service {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       port,
					TargetPort: intstr.FromInt32(port),
				},
			},
		},
	}
	return svc
}

func TestUpdateHomerConfigService_BasicItem(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("my-app", "default", 8080, nil)

	UpdateHomerConfigService(config, svc)

	if len(config.Services) == 0 {
		t.Fatal("expected at least one service group")
	}

	found := false
	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "my-app" {
				found = true
				expectedURL := "http://my-app.default.svc.cluster.local:8080"
				if item.Parameters["url"] != expectedURL {
					t.Errorf("url = %q, want %q", item.Parameters["url"], expectedURL)
				}
				if item.Parameters["subtitle"] != "default/my-app" {
					t.Errorf("subtitle = %q, want %q", item.Parameters["subtitle"], "default/my-app")
				}
				if item.Parameters["logo"] != ServiceIconURL {
					t.Errorf("logo = %q, want %q", item.Parameters["logo"], ServiceIconURL)
				}
			}
		}
	}
	if !found {
		t.Error("item 'my-app' not found in config")
	}
}

func TestUpdateHomerConfigService_HTTPSPort443(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("secure-app", "prod", 443, nil)

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "secure-app" {
				expected := "https://secure-app.prod.svc.cluster.local:443"
				if item.Parameters["url"] != expected {
					t.Errorf("url = %q, want %q", item.Parameters["url"], expected)
				}
				return
			}
		}
	}
	t.Error("item 'secure-app' not found")
}

func TestUpdateHomerConfigService_AnnotationURLOverride(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("my-app", "default", 8080, map[string]string{
		"item.homer.rajsingh.info/url": "https://myapp.example.com",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "my-app" {
				if item.Parameters["url"] != "https://myapp.example.com" {
					t.Errorf("url = %q, want annotation override", item.Parameters["url"])
				}
				return
			}
		}
	}
	t.Error("item 'my-app' not found")
}

func TestUpdateHomerConfigService_CustomAnnotations(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("my-app", "default", 8080, map[string]string{
		"item.homer.rajsingh.info/name":     "Custom Name",
		"item.homer.rajsingh.info/subtitle": "Custom Subtitle",
		"service.homer.rajsingh.info/name":  "My Group",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		sgName := getServiceName(&sg)
		if sgName == "My Group" {
			for _, item := range sg.Items {
				if item.Parameters["name"] != "Custom Name" {
					t.Errorf("name = %q, want 'Custom Name'", item.Parameters["name"])
				}
				if item.Parameters["subtitle"] != "Custom Subtitle" {
					t.Errorf("subtitle = %q, want 'Custom Subtitle'", item.Parameters["subtitle"])
				}
				return
			}
		}
	}
	t.Error("service group 'My Group' not found")
}

func TestUpdateHomerConfigService_NoPorts(t *testing.T) {
	config := &HomerConfig{}
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-ports",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{},
	}

	UpdateHomerConfigService(config, svc)

	// Should still create an item with no port in URL
	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "no-ports" {
				expected := "http://no-ports.default.svc.cluster.local"
				if item.Parameters["url"] != expected {
					t.Errorf("url = %q, want %q", item.Parameters["url"], expected)
				}
				return
			}
		}
	}
	t.Error("item 'no-ports' not found")
}

func TestUpdateHomerConfigService_HiddenItem(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("hidden-app", "default", 8080, map[string]string{
		"item.homer.rajsingh.info/hidden": "true",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "hidden-app" {
				t.Error("hidden item should not appear in config")
			}
		}
	}
}

func TestUpdateHomerConfigService_MultipleServices(t *testing.T) {
	config := &HomerConfig{}

	for i := 0; i < 3; i++ {
		svc := makeService(fmt.Sprintf("app-%d", i), "default", int32(8080+i), nil)
		UpdateHomerConfigService(config, svc)
	}

	totalItems := 0
	for _, sg := range config.Services {
		totalItems += len(sg.Items)
	}
	if totalItems != 3 {
		t.Errorf("expected 3 items, got %d", totalItems)
	}
}

func TestUpdateHomerConfigService_ClusterAnnotation(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("remote-app", "default", 8080, map[string]string{
		"homer.rajsingh.info/cluster": "prod-cluster",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "remote-app" {
				if item.Source != "remote-app@prod-cluster" {
					t.Errorf("source = %q, want 'remote-app@prod-cluster'", item.Source)
				}
				return
			}
		}
	}
	t.Error("item 'remote-app' not found")
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/rajsingh/Documents/GitHub/homer-operator && go test ./pkg/homer/ -run TestUpdateHomerConfigService -v`
Expected: FAIL — `UpdateHomerConfigService` not defined

**Step 3: Implement UpdateHomerConfigService**

In `pkg/homer/config.go`, add after line 1083 (after `UpdateHomerConfigIngressWithGrouping`):

```go
// UpdateHomerConfigService updates Homer config from a Kubernetes Service resource
func UpdateHomerConfigService(homerConfig *HomerConfig, svc corev1.Service) {
	UpdateHomerConfigServiceWithGrouping(homerConfig, svc, nil)
}

// UpdateHomerConfigServiceWithGrouping updates Homer config from a Service with custom grouping
func UpdateHomerConfigServiceWithGrouping(
	homerConfig *HomerConfig,
	svc corev1.Service,
	groupingConfig *ServiceGroupingConfig,
) {
	// Setup service group
	service := setupK8sServiceGroup(homerConfig, svc, groupingConfig)

	// Remove existing items from this source
	removeItemsFromIngressSource(homerConfig, svc.ObjectMeta.Name, svc.ObjectMeta.Namespace)
	processServiceAnnotations(&service, svc.ObjectMeta.Annotations)

	// Create item
	item := createK8sServiceItem(svc)
	processItemAnnotations(&item, svc.ObjectMeta.Annotations)

	// Append cluster name suffix after annotation processing
	if clusterName, ok := svc.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
		if suffix, hasSuffix := svc.ObjectMeta.Labels["cluster-name-suffix"]; hasSuffix && suffix != "" {
			if currentName, hasName := item.Parameters["name"]; hasName && currentName != "" {
				setItemParameter(&item, "name", currentName+suffix)
			}
		}
	}

	// Skip hidden items
	if isItemHidden(&item) {
		return
	}

	updateOrAddServiceItems(homerConfig, service, []Item{item})
}

// setupK8sServiceGroup creates a Homer service group for a Kubernetes Service
func setupK8sServiceGroup(
	homerConfig *HomerConfig,
	svc corev1.Service,
	groupingConfig *ServiceGroupingConfig,
) Service {
	service := Service{}
	serviceName := determineServiceGroupWithCRDRespect(
		homerConfig,
		svc.ObjectMeta.Namespace,
		svc.ObjectMeta.Labels,
		svc.ObjectMeta.Annotations,
		groupingConfig,
	)
	setServiceParameter(&service, "name", serviceName)
	setServiceParameter(&service, "logo", NamespaceIconURL)
	return service
}

// createK8sServiceItem creates a Homer item from a Kubernetes Service
func createK8sServiceItem(svc corev1.Service) Item {
	item := Item{}

	setItemParameter(&item, "name", svc.ObjectMeta.Name)
	setItemParameter(&item, "logo", ServiceIconURL)
	setItemParameter(&item, "subtitle", svc.ObjectMeta.Namespace+"/"+svc.ObjectMeta.Name)

	// Build URL: protocol://name.namespace.svc.cluster.local[:port]
	protocol := "http"
	portSuffix := ""
	if len(svc.Spec.Ports) > 0 {
		port := svc.Spec.Ports[0].Port
		if port == 443 {
			protocol = "https"
		}
		portSuffix = fmt.Sprintf(":%d", port)
	}
	url := fmt.Sprintf("%s://%s.%s.svc.cluster.local%s",
		protocol, svc.ObjectMeta.Name, svc.ObjectMeta.Namespace, portSuffix)
	setItemParameter(&item, "url", url)

	// Set source for conflict detection
	item.Source = svc.ObjectMeta.Name
	if clusterName, ok := svc.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
		item.Source = svc.ObjectMeta.Name + "@" + clusterName
	}
	item.Namespace = svc.ObjectMeta.Namespace
	item.LastUpdate = svc.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

	// Auto-tag with cluster name if cluster-tagstyle label is set
	if clusterName, ok := svc.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
		if tagStyle, hasStyle := svc.ObjectMeta.Labels["cluster-tagstyle"]; hasStyle && tagStyle != "" {
			setItemParameter(&item, "tag", clusterName)
			setItemParameter(&item, "tagstyle", tagStyle)
		}
	}

	return item
}
```

Add `"fmt"` to imports if not already present (it should be).

**Step 4: Run tests to verify they pass**

Run: `cd /Users/rajsingh/Documents/GitHub/homer-operator && go test ./pkg/homer/ -run TestUpdateHomerConfigService -v`
Expected: All PASS

**Step 5: Run full test suite**

Run: `make test`
Expected: All existing tests still pass

**Step 6: Commit**

```
feat: add UpdateHomerConfigService for Service-to-Homer-item conversion

Generates Homer dashboard items from Kubernetes Services with DNS-based
URL fallback and full annotation support.
```

---

### Task 3: Wire Service discovery into ConfigMap creation

**Files:**
- Modify: `pkg/homer/config.go:325-376` (CreateConfigMap and CreateConfigMapWithHTTPRoutes)

**Step 1: Update CreateConfigMap to accept Services**

Change the `CreateConfigMap` signature (line 325) to accept services:

```go
func CreateConfigMap(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	services []corev1.Service,
	owner client.Object,
) (corev1.ConfigMap, error) {
```

Add Service processing after the Ingress loop (after line 336):

```go
	for _, svc := range services {
		UpdateHomerConfigService(config, svc)
	}
```

**Step 2: Update CreateConfigMapWithHTTPRoutes signature**

Change the signature (line 365) to accept services:

```go
func CreateConfigMapWithHTTPRoutes(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	httproutes []gatewayv1.HTTPRoute,
	services []corev1.Service,
	owner client.Object,
	domainFilters []string,
) (corev1.ConfigMap, error) {
	return createConfigMapWithHTTPRoutesAndHealth(
		config, name, namespace, ingresses, httproutes, services, owner, domainFilters, nil)
}
```

**Step 3: Update createConfigMapWithHTTPRoutesAndHealth**

Change signature (line 378) and add Service processing after HTTPRoute loop (after line 397):

```go
func createConfigMapWithHTTPRoutesAndHealth(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	httproutes []gatewayv1.HTTPRoute,
	services []corev1.Service,
	owner client.Object,
	domainFilters []string,
	healthConfig *ServiceHealthConfig,
) (corev1.ConfigMap, error) {
```

Add after the HTTPRoute loop:

```go
	for _, svc := range services {
		UpdateHomerConfigService(config, svc)
	}
```

**Step 4: Fix all callers**

Search for all callers of `CreateConfigMap` and `CreateConfigMapWithHTTPRoutes` and add the `services` parameter. The callers are in `dashboard_controller.go` (lines 999, 1002) and potentially in test files. Pass `nil` or `[]corev1.Service{}` for callers that don't have services yet — these will be updated in Task 5.

Run: `grep -rn "CreateConfigMap\b\|CreateConfigMapWithHTTPRoutes" --include="*.go" .` to find all callers.

**Step 5: Verify compilation**

Run: `go build ./...`
Expected: Clean build

**Step 6: Run tests**

Run: `make test`
Expected: All pass (callers updated with nil/empty services)

**Step 7: Commit**

```
feat: wire Service slice through ConfigMap creation pipeline

Both CreateConfigMap and CreateConfigMapWithHTTPRoutes now accept
a services parameter for processing alongside Ingress/HTTPRoute.
```

---

### Task 4: Add ClusterManager Service discovery

**Files:**
- Modify: `internal/controller/cluster_manager.go` (after DiscoverHTTPRoutes ~line 469)
- Create: `internal/controller/service_discovery_test.go`

**Step 1: Write failing test**

Create `internal/controller/service_discovery_test.go`:

```go
package controller

import (
	"context"
	"testing"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClusterManager_DiscoverServices_LocalOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
			Labels:    map[string]string{"homer": "true"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 8080, TargetPort: intstr.FromInt32(8080)},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc).
		Build()

	cm := NewClusterManager(fakeClient, scheme)

	dashboard := &homerv1alpha1.Dashboard{
		Spec: homerv1alpha1.DashboardSpec{
			ServiceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"homer": "true"},
			},
		},
	}

	results, err := cm.DiscoverServices(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("DiscoverServices error: %v", err)
	}

	localServices, ok := results[localClusterName]
	if !ok {
		t.Fatal("expected local cluster results")
	}

	if len(localServices) != 1 {
		t.Fatalf("expected 1 service, got %d", len(localServices))
	}

	if localServices[0].Name != "my-app" {
		t.Errorf("service name = %q, want 'my-app'", localServices[0].Name)
	}
}

func TestClusterManager_DiscoverServices_NoSelectorReturnsEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 8080, TargetPort: intstr.FromInt32(8080)},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc).
		Build()

	cm := NewClusterManager(fakeClient, scheme)

	dashboard := &homerv1alpha1.Dashboard{
		Spec: homerv1alpha1.DashboardSpec{
			// No ServiceSelector — should discover nothing
		},
	}

	results, err := cm.DiscoverServices(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("DiscoverServices error: %v", err)
	}

	localServices := results[localClusterName]
	if len(localServices) != 0 {
		t.Errorf("expected 0 services without selector, got %d", len(localServices))
	}
}

func TestClusterManager_DiscoverServices_SelectorFiltering(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)

	matchingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "matching-app",
			Namespace: "default",
			Labels:    map[string]string{"homer": "true"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}
	nonMatchingSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-app",
			Namespace: "default",
			Labels:    map[string]string{"homer": "false"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(matchingSvc, nonMatchingSvc).
		Build()

	cm := NewClusterManager(fakeClient, scheme)

	dashboard := &homerv1alpha1.Dashboard{
		Spec: homerv1alpha1.DashboardSpec{
			ServiceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"homer": "true"},
			},
		},
	}

	results, err := cm.DiscoverServices(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("DiscoverServices error: %v", err)
	}

	localServices := results[localClusterName]
	if len(localServices) != 1 {
		t.Fatalf("expected 1 filtered service, got %d", len(localServices))
	}
	if localServices[0].Name != "matching-app" {
		t.Errorf("got %q, want 'matching-app'", localServices[0].Name)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/rajsingh/Documents/GitHub/homer-operator && go test ./internal/controller/ -run TestClusterManager_DiscoverServices -v`
Expected: FAIL — `DiscoverServices` not defined

**Step 3: Implement DiscoverServices and discoverClusterServices**

In `internal/controller/cluster_manager.go`, add after `DiscoverHTTPRoutes` (around line 469):

```go
// DiscoverServices discovers Service resources from all connected clusters
func (m *ClusterManager) DiscoverServices(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (map[string][]corev1.Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string][]corev1.Service)
	log := log.FromContext(ctx)

	// If no ServiceSelector is set at all (dashboard + remote clusters), skip entirely
	if dashboard.Spec.ServiceSelector == nil {
		hasRemoteSelector := false
		for _, rc := range dashboard.Spec.RemoteClusters {
			if rc.ServiceSelector != nil {
				hasRemoteSelector = true
				break
			}
		}
		if !hasRemoteSelector {
			return results, nil
		}
	}

	for name, cluster := range m.clients {
		if !cluster.Connected && name != localClusterName {
			log.V(1).Info("Skipping disconnected cluster for Service discovery", "cluster", name)
			continue
		}

		services, err := m.discoverClusterServices(ctx, cluster, dashboard)
		if err != nil {
			log.Error(err, "Failed to discover Services", "cluster", name)
			if name != localClusterName {
				cluster.Connected = false
				cluster.LastError = err
				cluster.LastCheck = time.Now()
			}
			continue
		}

		if name != localClusterName {
			cluster.Connected = true
			cluster.LastError = nil
			cluster.LastCheck = time.Now()
		}

		results[name] = services
		log.V(1).Info("Discovered Services", "cluster", name, "count", len(services))
	}

	return results, nil
}

// discoverClusterServices discovers Services from a specific cluster
func (m *ClusterManager) discoverClusterServices(ctx context.Context, cluster *ClusterClient, dashboard *homerv1alpha1.Dashboard) ([]corev1.Service, error) {
	// Determine which selector to use
	var selector *metav1.LabelSelector
	if cluster.ClusterCfg != nil && cluster.ClusterCfg.ServiceSelector != nil {
		selector = cluster.ClusterCfg.ServiceSelector
	} else if cluster.Name == localClusterName && dashboard.Spec.ServiceSelector != nil {
		selector = dashboard.Spec.ServiceSelector
	}

	// No selector means no Service discovery for this cluster
	if selector == nil {
		return nil, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}

	clusterServices := &corev1.ServiceList{}

	// Apply namespace filter if specified for remote clusters
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.NamespaceFilter) > 0 {
		filteredServices := []corev1.Service{}
		for _, ns := range cluster.ClusterCfg.NamespaceFilter {
			nsServices := &corev1.ServiceList{}
			if err := cluster.Client.List(ctx, nsServices, client.InNamespace(ns)); err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, err
				}
				continue
			}
			filteredServices = append(filteredServices, nsServices.Items...)
		}
		clusterServices.Items = filteredServices
	} else {
		if err := cluster.Client.List(ctx, clusterServices); err != nil {
			return nil, err
		}
	}

	// Filter by label selector and add metadata
	filtered := []corev1.Service{}
	for i := range clusterServices.Items {
		svc := &clusterServices.Items[i]

		if !labelSelector.Matches(labels.Set(svc.Labels)) {
			continue
		}

		// Add cluster labels if specified
		if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
			if svc.Labels == nil {
				svc.Labels = make(map[string]string)
			}
			for k, v := range cluster.ClusterCfg.ClusterLabels {
				svc.Labels[k] = v
			}
		}

		// Add cluster annotation for identification
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations["homer.rajsingh.info/cluster"] = cluster.Name

		// Merge namespace annotations
		m.mergeNamespaceAnnotationsForService(ctx, cluster.Client, svc)

		filtered = append(filtered, *svc)
	}

	return filtered, nil
}

// mergeNamespaceAnnotationsForService merges namespace annotations into Service annotations
func (m *ClusterManager) mergeNamespaceAnnotationsForService(ctx context.Context, clusterClient client.Client, svc *corev1.Service) {
	ns := &corev1.Namespace{}
	if err := clusterClient.Get(ctx, client.ObjectKey{Name: svc.Namespace}, ns); err != nil {
		return
	}

	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}

	for k, v := range ns.Annotations {
		if strings.HasPrefix(k, serviceAnnotationPrefix) || strings.HasPrefix(k, itemAnnotationPrefix) {
			if _, exists := svc.Annotations[k]; !exists {
				svc.Annotations[k] = v
			}
		}
	}
}
```

Ensure imports include `corev1 "k8s.io/api/core/v1"` and `"strings"` (they should already be there).

**Step 4: Run tests to verify they pass**

Run: `cd /Users/rajsingh/Documents/GitHub/homer-operator && go test ./internal/controller/ -run TestClusterManager_DiscoverServices -v`
Expected: All PASS

**Step 5: Run full test suite**

Run: `make test`
Expected: All pass

**Step 6: Commit**

```
feat: add DiscoverServices to ClusterManager

Multi-cluster Service discovery with label selector filtering,
namespace filtering, and namespace annotation merging.
```

---

### Task 5: Wire Service discovery into the controller reconciliation loop

**Files:**
- Modify: `internal/controller/dashboard_controller.go` (Reconcile, createConfigMap, SetupWithManager, new filter/mapper functions)

**Step 1: Add shouldIncludeService**

Add after `shouldIncludeIngress` (~line 853):

```go
func (r *DashboardReconciler) shouldIncludeService(ctx context.Context, svc *corev1.Service, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	log := log.FromContext(ctx)

	// No selector means no Service discovery
	if dashboard.Spec.ServiceSelector == nil {
		return false, nil
	}

	if match, err := validateLabelSelector(dashboard.Spec.ServiceSelector, svc.Labels, svc.Name, "service", log); err != nil {
		return false, err
	} else if !match {
		return false, nil
	}

	return true, nil
}
```

**Step 2: Add findDashboardsForService**

Add after `findDashboardsForIngress` (~line 617):

```go
// findDashboardsForService finds all dashboards that should be reconciled when a Service changes
func (r *DashboardReconciler) findDashboardsForService(ctx context.Context, obj client.Object) []ctrl.Request {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil
	}

	dashboards := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboards); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, dashboard := range dashboards.Items {
		if shouldInclude, err := r.shouldIncludeService(ctx, svc, &dashboard); err == nil && shouldInclude {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: dashboard.Namespace,
					Name:      dashboard.Name,
				},
			})
		}
	}

	return requests
}
```

**Step 3: Add getFilteredServices and getMultiClusterFilteredServices**

Add after `getMultiClusterFilteredIngresses` (~line 382):

```go
func (r *DashboardReconciler) getFilteredServices(ctx context.Context, dashboard *homerv1alpha1.Dashboard) ([]corev1.Service, error) {
	if dashboard.Spec.ServiceSelector == nil {
		return nil, nil
	}

	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList); err != nil {
		return nil, err
	}

	var filtered []corev1.Service
	for i := range serviceList.Items {
		shouldInclude, err := r.shouldIncludeService(ctx, &serviceList.Items[i], dashboard)
		if err != nil {
			return nil, err
		}
		if shouldInclude {
			filtered = append(filtered, serviceList.Items[i])
		}
	}

	return filtered, nil
}

func (r *DashboardReconciler) getMultiClusterFilteredServices(ctx context.Context, dashboard *homerv1alpha1.Dashboard) ([]corev1.Service, error) {
	if r.ClusterManager != nil && len(dashboard.Spec.RemoteClusters) > 0 {
		clusterServices, err := r.ClusterManager.DiscoverServices(ctx, dashboard)
		if err != nil {
			log := log.FromContext(ctx)
			log.Error(err, "Failed to discover Services from clusters")
		}

		var allServices []corev1.Service
		for _, services := range clusterServices {
			allServices = append(allServices, services...)
		}
		return allServices, nil
	}

	return r.getFilteredServices(ctx, dashboard)
}
```

**Step 4: Update Reconcile() to discover Services**

In `Reconcile()`, after the ingress discovery block (after line 97), add:

```go
	filteredServices, err := r.getMultiClusterFilteredServices(ctx, &dashboard)
	if err != nil {
		return ctrl.Result{}, err
	}
```

Update `prepareResources` call (line 103) to pass services:

```go
	resources, _, err := r.prepareResources(ctx, &dashboard, filteredIngressList, filteredServices)
```

**Step 5: Update prepareResources signature**

Change `prepareResources` (line 391):

```go
func (r *DashboardReconciler) prepareResources(ctx context.Context, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList, filteredServices []corev1.Service) ([]client.Object, *homer.HomerConfig, error) {
```

Pass services through to `createConfigMap`:

```go
	configMap, err := r.createConfigMap(ctx, homerConfig, dashboard, filteredIngressList, filteredServices)
```

**Step 6: Update createConfigMap signature and implementation**

Change `createConfigMap` (line 938):

```go
func (r *DashboardReconciler) createConfigMap(ctx context.Context, homerConfig *homer.HomerConfig, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList, filteredServices []corev1.Service) (corev1.ConfigMap, error) {
```

Merge namespace annotations for services (add after the HTTPRoute annotation merging block, before the return):

```go
	// Merge namespace annotations into Service annotations
	mergedServices := make([]corev1.Service, len(filteredServices))
	for i, svc := range filteredServices {
		svcCopy := svc.DeepCopy()
		svcCopy.Annotations = r.mergeNamespaceAnnotations(ctx, svc.Namespace, svc.Annotations)
		mergedServices[i] = *svcCopy
	}
```

Update the calls to `CreateConfigMapWithHTTPRoutes` (line 999) and `CreateConfigMap` (line 1002) to pass the merged services:

```go
		return homer.CreateConfigMapWithHTTPRoutes(homerConfig, dashboard.Name, dashboard.Namespace, mergedIngressList, mergedHTTPRoutes, mergedServices, dashboard, dashboard.Spec.DomainFilters)
	}

	return homer.CreateConfigMap(homerConfig, dashboard.Name, dashboard.Namespace, mergedIngressList, mergedServices, dashboard)
```

**Step 7: Register Service watch in SetupWithManager**

In `SetupWithManager()`, add after the Namespace watch (after line 587):

```go
	// Watch Services for discovery (only triggers reconciliation for dashboards with ServiceSelector)
	builder = builder.Watches(&corev1.Service{},
		handler.EnqueueRequestsFromMapFunc(r.findDashboardsForService))
```

**Step 8: Verify compilation**

Run: `go build ./...`
Expected: Clean build

**Step 9: Run tests**

Run: `make test`
Expected: All pass

**Step 10: Commit**

```
feat: wire Service discovery into controller reconciliation

Services matching serviceSelector are now discovered, filtered,
and rendered as Homer dashboard items alongside Ingress/HTTPRoute.
```

---

### Task 6: Update Helm chart CRD and add config sample

**Files:**
- Regenerate: `charts/homer-operator/templates/crd.yaml`
- Regenerate: `config/crd/bases/homer.rajsingh.info_dashboards.yaml`
- Modify: `config/samples/` (add or update sample with serviceSelector)

**Step 1: Regenerate all manifests**

Run: `make manifests`
Expected: CRD YAML files updated with serviceSelector fields

**Step 2: Verify CRD contains serviceSelector**

Run: `grep -A2 serviceSelector config/crd/bases/homer.rajsingh.info_dashboards.yaml | head -10`
Expected: serviceSelector field present in CRD spec

**Step 3: Run linting**

Run: `make lint`
Expected: Clean

**Step 4: Run full test suite**

Run: `make test`
Expected: All pass

**Step 5: Commit**

```
feat: regenerate CRD manifests with serviceSelector support
```

---

### Task 7: Final verification and cleanup

**Step 1: Run full build + lint + test**

Run: `make lint && make test`
Expected: All clean, all pass

**Step 2: Verify end-to-end flow manually**

Check that:
- `go build ./...` succeeds
- CRD YAML has `serviceSelector` in both `spec` and `remoteClusters`
- No unintended file changes: `git diff --stat`

**Step 3: Commit any remaining changes**

Only if there are fixups needed from the verification step.
