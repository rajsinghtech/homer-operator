package homer

import (
	"testing"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

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
		itemNames[item.Name] = true
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
		itemNames[item.Name] = true
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
		itemNames[item.Name] = true
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
	if service.Items[0].Source != "route2" {
		t.Errorf("Expected remaining item to be from route2, got source: %s", service.Items[0].Source)
	}
}

func TestRemoveItemsFromHTTPRouteSource(t *testing.T) {
	// Test the core cleanup function directly
	config := &HomerConfig{
		Services: []Service{
			{
				Name: "test-service",
				Items: []Item{
					{
						Name:      "item1",
						Source:    "route1",
						Namespace: "test-ns",
					},
					{
						Name:      "item2",
						Source:    "route2",
						Namespace: "test-ns",
					},
					{
						Name:      "item3",
						Source:    "route1",
						Namespace: "test-ns",
					},
				},
			},
		},
	}

	// Remove all items from route1
	removeItemsFromHTTPRouteSource(config, "route1", "test-ns")

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

	if config.Services[0].Items[0].Name != "app-route" {
		t.Errorf("Expected item name 'app-route', got '%s'", config.Services[0].Items[0].Name)
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
		itemNames[item.Name] = true
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
