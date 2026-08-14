package homer

import (
	"maps"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestRemoteResourceCleanupIsScopedToCluster(t *testing.T) {
	const (
		resourceName = "shared-app"
		namespace    = "shared-ns"
		clusterA     = "cluster-a"
		clusterB     = "cluster-b"
	)

	tests := []struct {
		name       string
		sourceBase string
		add        func(*HomerConfig, string)
		remove     func(*HomerConfig, string)
	}{
		{
			name:       "Ingress",
			sourceBase: "ingress/" + resourceName,
			add: func(config *HomerConfig, cluster string) {
				ingress := networkingv1.Ingress{
					ObjectMeta: remoteObjectMeta(resourceName, namespace, cluster),
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{{Host: cluster + ".example.com"}},
					},
				}
				UpdateHomerConfigIngress(config, ingress, nil)
			},
			remove: func(config *HomerConfig, cluster string) {
				ingress := networkingv1.Ingress{
					ObjectMeta: remoteObjectMeta(resourceName, namespace, cluster),
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{{Host: cluster + ".example.com"}},
					},
				}
				UpdateHomerConfigIngress(config, ingress, []string{"excluded.example.com"})
			},
		},
		{
			name:       "HTTPRoute",
			sourceBase: "httproute/" + resourceName,
			add: func(config *HomerConfig, cluster string) {
				route := &gatewayv1.HTTPRoute{
					ObjectMeta: remoteObjectMeta(resourceName, namespace, cluster),
					Spec: gatewayv1.HTTPRouteSpec{
						Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(cluster + ".example.com")},
					},
				}
				UpdateHomerConfigHTTPRoute(config, route, nil)
			},
			remove: func(config *HomerConfig, cluster string) {
				route := &gatewayv1.HTTPRoute{
					ObjectMeta: remoteObjectMeta(resourceName, namespace, cluster),
				}
				UpdateHomerConfigHTTPRoute(config, route, nil)
			},
		},
		{
			name:       "Service",
			sourceBase: "svc/" + resourceName,
			add: func(config *HomerConfig, cluster string) {
				service := corev1.Service{
					ObjectMeta: remoteObjectMeta(resourceName, namespace, cluster),
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 8080}},
					},
				}
				UpdateHomerConfigService(config, service)
			},
			remove: func(config *HomerConfig, cluster string) {
				service := corev1.Service{
					ObjectMeta: remoteObjectMeta(resourceName, namespace, cluster),
				}
				service.Annotations["item.homer.rajsingh.info/hide"] = "true"
				UpdateHomerConfigService(config, service)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &HomerConfig{}
			tt.add(config, clusterA)
			tt.add(config, clusterB)

			assertSources(t, config, map[string]bool{
				tt.sourceBase + "@" + clusterA: true,
				tt.sourceBase + "@" + clusterB: true,
			})

			tt.remove(config, clusterA)

			assertSources(t, config, map[string]bool{
				tt.sourceBase + "@" + clusterB: true,
			})
		})
	}
}

func TestRemoteSameNamedResourcesRemainDistinctWithoutSuffix(t *testing.T) {
	config := &HomerConfig{}

	for _, cluster := range []string{"cluster-a", "cluster-b"} {
		ingress := networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "shared-app",
				Namespace: "shared-ns",
				Annotations: map[string]string{
					"homer.rajsingh.info/cluster": cluster,
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: cluster + ".example.com"}},
			},
		}
		UpdateHomerConfigIngress(config, ingress, nil)
	}

	if len(config.Services) != 1 || len(config.Services[0].Items) != 2 {
		t.Fatalf("same-named remote resources were merged: %#v", config.Services)
	}

	sources := map[string]bool{}
	for _, item := range config.Services[0].Items {
		sources[item.Source] = true
	}
	for _, want := range []string{"ingress/shared-app@cluster-a", "ingress/shared-app@cluster-b"} {
		if !sources[want] {
			t.Errorf("missing distinct source %q in %v", want, sources)
		}
	}
}

func TestCRDFoundationClaimsOnlyOneSameNamedRemoteResource(t *testing.T) {
	config := &HomerConfig{
		Services: []Service{{
			Name: "shared-ns",
			Items: []Item{{
				Name:      "shared-app",
				URL:       "https://manual.example.com",
				Source:    CRDSource,
				Namespace: "dashboard",
			}},
		}},
	}

	for _, cluster := range []string{"cluster-a", "cluster-b"} {
		ingress := networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "shared-app",
				Namespace: "shared-ns",
				Annotations: map[string]string{
					"homer.rajsingh.info/cluster": cluster,
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: cluster + ".example.com"}},
			},
		}
		UpdateHomerConfigIngress(config, ingress, nil)
	}

	if len(config.Services) != 1 || len(config.Services[0].Items) != 2 {
		t.Fatalf("CRD foundation absorbed both remote resources: %#v", config.Services)
	}

	var remoteCount int
	for _, item := range config.Services[0].Items {
		if item.Source == "ingress/shared-app@cluster-b" {
			remoteCount++
		}
	}
	if remoteCount != 1 {
		t.Fatalf("expected the second remote resource to remain distinct: %#v", config.Services[0].Items)
	}
}

func TestIngressAndHTTPRouteSourcesAreKindQualified(t *testing.T) {
	config := &HomerConfig{}
	clusterAnnotations := map[string]string{"homer.rajsingh.info/cluster": "cluster-a"}

	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-app", Namespace: "shared-ns", Annotations: maps.Clone(clusterAnnotations)},
		Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "ingress.example.com"}}},
	}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-app", Namespace: "shared-ns", Annotations: maps.Clone(clusterAnnotations)},
		Spec:       gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{"route.example.com"}},
	}

	UpdateHomerConfigIngress(config, ingress, nil)
	UpdateHomerConfigHTTPRoute(config, route, nil)

	if len(config.Services) != 1 || len(config.Services[0].Items) != 2 {
		t.Fatalf("Ingress and HTTPRoute were merged: %#v", config.Services)
	}
}

//nolint:unparam // Keeping the resource-name argument makes the fixtures read like the objects they create.
func remoteObjectMeta(name, namespace, cluster string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Annotations: map[string]string{
			"homer.rajsingh.info/cluster": cluster,
		},
		Labels: map[string]string{
			"cluster-name-suffix": "@" + cluster,
		},
	}
}

func assertSources(t *testing.T, config *HomerConfig, want map[string]bool) {
	t.Helper()

	got := make(map[string]bool)
	for _, service := range config.Services {
		for _, item := range service.Items {
			got[item.Source] = true
		}
	}

	if len(got) != len(want) {
		t.Fatalf("sources = %v, want %v", got, want)
	}
	for source := range want {
		if !got[source] {
			t.Errorf("source %q was removed or not created; got %v", source, got)
		}
	}
}

func TestHTTPRouteHostnameRemovalCleanup(t *testing.T) {
	config := &HomerConfig{}

	// Create HTTPRoute with multiple hostnames
	httproute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "immich-server",
			Namespace:         "immich",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{
				"immich.rajsingh.info",
				"immich.lukehouge.com",
				"immich.k8s.rajsingh.info", // This will be removed later
			},
		},
	}

	// First update: add all hostnames
	UpdateHomerConfigHTTPRoute(config, httproute, nil)

	// Verify all 3 items were created
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]
	if len(service.Items) != 3 {
		t.Fatalf("Expected 3 items initially, got %d", len(service.Items))
	}

	// Verify the specific items exist
	itemNames := make(map[string]bool)
	for _, item := range service.Items {
		if item.Parameters != nil {
			itemNames[item.Parameters["name"]] = true
		}
	}

	expectedInitialItems := []string{
		"immich-server-immich.rajsingh.info",
		"immich-server-immich.lukehouge.com",
		"immich-server-immich.k8s.rajsingh.info",
	}

	for _, expectedItem := range expectedInitialItems {
		if !itemNames[expectedItem] {
			t.Errorf("Expected item '%s' to exist initially", expectedItem)
		}
	}

	// Now simulate hostname removal by updating the HTTPRoute
	httproute.Spec.Hostnames = []gatewayv1.Hostname{
		"immich.rajsingh.info",
		"immich.lukehouge.com",
		// "immich.k8s.rajsingh.info" is removed
	}

	// Second update: remove one hostname
	UpdateHomerConfigHTTPRoute(config, httproute, nil)

	// Verify only 2 items remain
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service after update, got %d", len(config.Services))
	}

	service = config.Services[0]
	if len(service.Items) != 2 {
		t.Fatalf("Expected 2 items after hostname removal, got %d", len(service.Items))
	}

	// Verify the correct items remain
	itemNames = make(map[string]bool)
	for _, item := range service.Items {
		if item.Parameters != nil {
			itemNames[item.Parameters["name"]] = true
		}
	}

	expectedRemainingItems := []string{
		"immich-server-immich.rajsingh.info",
		"immich-server-immich.lukehouge.com",
	}

	for _, expectedItem := range expectedRemainingItems {
		if !itemNames[expectedItem] {
			t.Errorf("Expected item '%s' to remain after update", expectedItem)
		}
	}

	// Verify the removed item is gone
	if itemNames["immich-server-immich.k8s.rajsingh.info"] {
		t.Error("Expected item 'immich-server-immich.k8s.rajsingh.info' to be removed")
	}
}

func TestIngressHostnameRemovalCleanup(t *testing.T) {
	config := &HomerConfig{}

	// Create Ingress with multiple rules
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ingress",
			Namespace:         "test-namespace",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "app1.example.com"},
				{Host: "app2.example.com"},
				{Host: "app3.example.com"}, // This will be removed later
			},
		},
	}

	// First update: add all hosts
	UpdateHomerConfigIngress(config, ingress, nil)

	// Verify all 3 items were created
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]
	if len(service.Items) != 3 {
		t.Fatalf("Expected 3 items initially, got %d", len(service.Items))
	}

	// Now simulate host removal by updating the Ingress
	ingress.Spec.Rules = []networkingv1.IngressRule{
		{Host: "app1.example.com"},
		{Host: "app2.example.com"},
		// app3.example.com is removed
	}

	// Second update: remove one host
	UpdateHomerConfigIngress(config, ingress, nil)

	// Verify only 2 items remain
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service after update, got %d", len(config.Services))
	}

	service = config.Services[0]
	if len(service.Items) != 2 {
		t.Fatalf("Expected 2 items after host removal, got %d", len(service.Items))
	}

	// Verify the correct items remain
	itemNames := make(map[string]bool)
	for _, item := range service.Items {
		if item.Parameters != nil {
			itemNames[item.Parameters["name"]] = true
		}
	}

	expectedRemainingItems := []string{
		"test-ingress-app1.example.com",
		"test-ingress-app2.example.com",
	}

	for _, expectedItem := range expectedRemainingItems {
		if !itemNames[expectedItem] {
			t.Errorf("Expected item '%s' to remain after update", expectedItem)
		}
	}

	// Verify the removed item is gone
	if itemNames["test-ingress-app3.example.com"] {
		t.Error("Expected item 'test-ingress-app3.example.com' to be removed")
	}
}

func TestIngressWithNoRulesRemovesItsExistingItems(t *testing.T) {
	config := &HomerConfig{}
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-on-update", Namespace: "apps"},
		Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app.example.com"}}},
	}
	UpdateHomerConfigIngress(config, ingress, nil)
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("initial Ingress item was not created: %#v", config.Services)
	}

	ingress.Spec.Rules = nil
	UpdateHomerConfigIngress(config, ingress, nil)
	if len(config.Services) != 0 {
		t.Fatalf("empty Ingress left stale services: %#v", config.Services)
	}
}

func TestEnhancedCRDItemStillSelectsItsCRDServiceGroup(t *testing.T) {
	config := &HomerConfig{Services: []Service{{
		Parameters: map[string]string{"name": "apps"},
		Items:      []Item{{Parameters: map[string]string{"name": "api"}, Source: CRDSource}},
	}}}
	apiService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}
	UpdateHomerConfigService(config, apiService)
	if len(config.Services) != 1 || config.Services[0].Items[0].Source != "svc/api" || !config.Services[0].Items[0].crdFoundation {
		t.Fatalf("CRD foundation was not retained after enhancement: %#v", config.Services)
	}

	webService := apiService
	webService.Name = "web"
	UpdateHomerConfigService(config, webService)
	if len(config.Services) != 1 || getServiceName(&config.Services[0]) != "apps" || len(config.Services[0].Items) != 2 {
		t.Fatalf("enhanced CRD service stopped selecting its group: %#v", config.Services)
	}
}

func TestCompleteResourceRemoval(t *testing.T) {
	config := &HomerConfig{}

	// Create HTTPRoute
	httproute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-route",
			Namespace:         "test-ns",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{
				"test.example.com",
			},
		},
	}

	// Add the HTTPRoute
	UpdateHomerConfigHTTPRoute(config, httproute, nil)

	// Verify item was created
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatal("Expected 1 service with 1 item")
	}

	// Now simulate complete removal by removing all hostnames
	httproute.Spec.Hostnames = []gatewayv1.Hostname{}

	// Update with empty hostnames
	UpdateHomerConfigHTTPRoute(config, httproute, nil)

	// Verify the service was removed completely (since it has no items)
	if len(config.Services) != 0 {
		t.Errorf("Expected no services after complete removal, got %d", len(config.Services))
	}
}

func TestMultipleResourcesCleanup(t *testing.T) {
	config := &HomerConfig{}

	// Create multiple HTTPRoutes in the same namespace
	httproute1 := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "route1",
			Namespace:         "shared-ns",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"app1.example.com"},
		},
	}

	httproute2 := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "route2",
			Namespace:         "shared-ns",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"app2.example.com"},
		},
	}

	// Add both routes
	UpdateHomerConfigHTTPRoute(config, httproute1, nil)
	UpdateHomerConfigHTTPRoute(config, httproute2, nil)

	// Verify both items exist in the same service
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]
	if len(service.Items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(service.Items))
	}

	// Remove all hostnames from route1
	httproute1.Spec.Hostnames = []gatewayv1.Hostname{}
	UpdateHomerConfigHTTPRoute(config, httproute1, nil)

	// Verify only route2's item remains
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service after route1 removal, got %d", len(config.Services))
	}

	service = config.Services[0]
	if len(service.Items) != 1 {
		t.Fatalf("Expected 1 item after route1 removal, got %d", len(service.Items))
	}

	// Verify it's route2's item
	if service.Items[0].Source != "httproute/route2" {
		t.Errorf("Expected remaining item to be from route2, got source: %s", service.Items[0].Source)
	}
}

func TestRemoveItemsFromHTTPRouteSource(t *testing.T) {
	// Test the core cleanup function directly
	config := &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "test-service",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "item1",
						},
						Source:    "route1",
						Namespace: "test-ns",
					},
					{
						Parameters: map[string]string{
							"name": "item2",
						},
						Source:    "route2",
						Namespace: "test-ns",
					},
					{
						Parameters: map[string]string{
							"name": "item3",
						},
						Source:    "route1",
						Namespace: "test-ns",
					},
				},
			},
		},
	}

	// Remove all items from route1
	removeItemsFromSource(config, "route1", "test-ns")

	// Verify only route2's item remains
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]
	if len(service.Items) != 1 {
		t.Fatalf("Expected 1 item remaining, got %d", len(service.Items))
	}

	if service.Items[0].Source != "route2" {
		t.Errorf("Expected remaining item to be from route2, got: %s", service.Items[0].Source)
	}
}

func TestSingleHostnameToMultipleHostnameTransition(t *testing.T) {
	config := &HomerConfig{}

	// Start with single hostname (no suffix)
	httproute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "app-route",
			Namespace:         "apps",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{
				"app.example.com",
			},
		},
	}

	// First update: single hostname
	UpdateHomerConfigHTTPRoute(config, httproute, nil)

	// Verify single item with base name
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatal("Expected 1 service with 1 item")
	}

	actualName := ""
	if config.Services[0].Items[0].Parameters != nil {
		actualName = config.Services[0].Items[0].Parameters["name"]
	}
	if actualName != "app-route" {
		t.Errorf("Expected item name 'app-route', got '%s'", actualName)
	}

	// Now add a second hostname
	httproute.Spec.Hostnames = []gatewayv1.Hostname{
		"app.example.com",
		"app.internal.com",
	}

	// Second update: multiple hostnames
	UpdateHomerConfigHTTPRoute(config, httproute, nil)

	// Verify 2 items with hostname suffixes
	service := config.Services[0]
	if len(service.Items) != 2 {
		t.Fatalf("Expected 2 items after adding hostname, got %d", len(service.Items))
	}

	itemNames := make(map[string]bool)
	for _, item := range service.Items {
		if item.Parameters != nil {
			itemNames[item.Parameters["name"]] = true
		}
	}

	expectedItems := []string{
		"app-route-app.example.com",
		"app-route-app.internal.com",
	}

	for _, expectedItem := range expectedItems {
		if !itemNames[expectedItem] {
			t.Errorf("Expected item '%s' to exist", expectedItem)
		}
	}

	// The old item without suffix should be gone
	if itemNames["app-route"] {
		t.Error("Expected old item 'app-route' to be removed")
	}
}

func TestHTTPRouteWithEmptyNamespace(t *testing.T) {
	config := &HomerConfig{
		Title: "Test Dashboard",
	}

	// HTTPRoute with empty namespace - this was causing validation failures
	httproute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "empty-namespace-route",
			Namespace:         "", // Empty namespace!
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{
				"app.internal.local",
			},
		},
	}

	domainFilters := []string{"internal.local"}
	UpdateHomerConfigHTTPRoute(config, httproute, domainFilters)

	// Verify service was created
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]

	// Verify service has proper parameters
	if service.Parameters == nil {
		t.Fatal("Service parameters should not be nil")
	}

	// Verify service name defaults to "default" when namespace is empty
	serviceName := service.Parameters["name"]
	if serviceName != "default" {
		t.Errorf("Expected service name 'default' for empty namespace, got '%s'", serviceName)
	}

	// Verify the service has items
	if len(service.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(service.Items))
	}

	// Most importantly: verify validation passes
	if err := ValidateHomerConfig(config); err != nil {
		t.Errorf("Configuration validation failed: %v", err)
	}
}
