package homer

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestSortServicesAndItems(t *testing.T) {
	// Create test config with unsorted services and items
	config := &HomerConfig{
		Title: "Test Dashboard",
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "Zebra Services",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "Zoo App",
							"url":  "https://zoo.example.com",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Alpha App",
							"url":  "https://alpha.example.com",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Beta App",
							"url":  "https://beta.example.com",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "Alpha Services",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "Zebra API",
							"url":  "https://zebra-api.example.com",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Alpha API",
							"url":  "https://alpha-api.example.com",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "Beta Services",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "Charlie App",
							"url":  "https://charlie.example.com",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Alpha Web",
							"url":  "https://alpha-web.example.com",
						},
					},
				},
			},
		},
	}

	// Apply sorting
	sortServicesAndItems(config)

	// Verify services are sorted alphabetically
	expectedServiceOrder := []string{"Alpha Services", "Beta Services", "Zebra Services"}
	if len(config.Services) != len(expectedServiceOrder) {
		t.Fatalf("Expected %d services, got %d", len(expectedServiceOrder), len(config.Services))
	}

	for i, expectedName := range expectedServiceOrder {
		actualName := getServiceName(&config.Services[i])
		if actualName != expectedName {
			t.Errorf("Service %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}

	// Verify items within first service (Alpha Services) are sorted
	alphaServiceItems := config.Services[0].Items
	expectedAlphaItems := []string{"Alpha API", "Zebra API"}
	if len(alphaServiceItems) != len(expectedAlphaItems) {
		t.Fatalf("Alpha Services: expected %d items, got %d", len(expectedAlphaItems), len(alphaServiceItems))
	}

	for i, expectedName := range expectedAlphaItems {
		actualName := getItemName(&alphaServiceItems[i])
		if actualName != expectedName {
			t.Errorf("Alpha Services item %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}

	// Verify items within second service (Beta Services) are sorted
	betaServiceItems := config.Services[1].Items
	expectedBetaItems := []string{"Alpha Web", "Charlie App"}
	if len(betaServiceItems) != len(expectedBetaItems) {
		t.Fatalf("Beta Services: expected %d items, got %d", len(expectedBetaItems), len(betaServiceItems))
	}

	for i, expectedName := range expectedBetaItems {
		actualName := getItemName(&betaServiceItems[i])
		if actualName != expectedName {
			t.Errorf("Beta Services item %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}

	// Verify items within third service (Zebra Services) are sorted
	zebraServiceItems := config.Services[2].Items
	expectedZebraItems := []string{"Alpha App", "Beta App", "Zoo App"}
	if len(zebraServiceItems) != len(expectedZebraItems) {
		t.Fatalf("Zebra Services: expected %d items, got %d", len(expectedZebraItems), len(zebraServiceItems))
	}

	for i, expectedName := range expectedZebraItems {
		actualName := getItemName(&zebraServiceItems[i])
		if actualName != expectedName {
			t.Errorf("Zebra Services item %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}
}

func TestSortingWithCaseInsensitive(t *testing.T) {
	config := createTestConfigWithMixedCase()
	sortServicesAndItems(config)

	// Verify case-insensitive service sorting
	expectedServiceOrder := []string{"Alpha Services", "zebra services"}
	verifyServiceOrder(t, config, expectedServiceOrder)

	// Verify case-insensitive item sorting
	zebraServiceItems := config.Services[1].Items
	expectedItemOrder := []string{"alpha App", "Beta App", "ZEBRA App"}
	verifyItemOrder(t, zebraServiceItems, expectedItemOrder)
}

func TestSortingWithEmptyNamesAndMissingParameters(t *testing.T) {
	config := &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "Service B",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "Item 2",
						},
					},
					{
						Parameters: nil, // No parameters map
					},
					{
						Parameters: map[string]string{
							"name": "Item 1",
						},
					},
					{
						Parameters: map[string]string{
							"name": "", // Empty name
						},
					},
				},
			},
			{
				Parameters: nil, // No parameters map
				Items:      []Item{},
			},
			{
				Parameters: map[string]string{
					"name": "Service A",
				},
				Items: []Item{},
			},
			{
				Parameters: map[string]string{
					"name": "", // Empty name
				},
				Items: []Item{},
			},
		},
	}

	// Should not panic and should handle edge cases gracefully
	sortServicesAndItems(config)

	// Verify it didn't crash and maintains structure
	if len(config.Services) != 4 {
		t.Errorf("Expected 4 services after sorting, got %d", len(config.Services))
	}

	// Services with empty/missing names should come first (empty string sorts before others)
	firstServiceName := getServiceName(&config.Services[0])
	if firstServiceName != "" {
		t.Errorf("Expected first service to have empty name, got '%s'", firstServiceName)
	}
}

func TestNormalizeHomerConfigIncludesSorting(t *testing.T) {
	config := &HomerConfig{
		Title:  "Test Dashboard",
		Header: false, // Will be set to true by normalize
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "Zebra Service",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "Zebra Item",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Alpha Item",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "Alpha Service",
				},
				Items: []Item{},
			},
		},
	}

	// Call normalize which should include sorting
	normalizeHomerConfig(config)

	// Verify header was set to true
	if !config.Header {
		t.Error("Expected Header to be set to true by normalizeHomerConfig")
	}

	// Verify services were sorted
	if len(config.Services) >= 2 {
		firstServiceName := getServiceName(&config.Services[0])
		secondServiceName := getServiceName(&config.Services[1])
		if firstServiceName != "Alpha Service" || secondServiceName != "Zebra Service" {
			t.Errorf("Services not sorted properly: got '%s', '%s'", firstServiceName, secondServiceName)
		}
	}

	// Verify items within first service were sorted
	if len(config.Services) > 0 && len(config.Services[1].Items) >= 2 {
		firstItemName := getItemName(&config.Services[1].Items[0])
		secondItemName := getItemName(&config.Services[1].Items[1])
		if firstItemName != "Alpha Item" || secondItemName != "Zebra Item" {
			t.Errorf("Items not sorted properly: got '%s', '%s'", firstItemName, secondItemName)
		}
	}
}

func TestSortingWithSpecialCharacters(t *testing.T) {
	config := createTestConfigWithSpecialChars()
	sortServicesAndItems(config)

	// Services should be sorted: "Service A" comes before "Service-B"
	expectedServiceOrder := []string{"Service A", "Service-B"}
	verifyServiceOrder(t, config, expectedServiceOrder)

	// Items should be sorted: "Item 3" (space comes before symbols), "Item-1", "Item_2"
	serviceBItems := config.Services[1].Items
	expectedItemOrder := []string{"Item 3", "Item-1", "Item_2"}
	verifyItemOrder(t, serviceBItems, expectedItemOrder)
}

func TestSortingIntegrationWithIngressDiscovery(t *testing.T) {
	// Test end-to-end sorting with real Ingress processing
	config := &HomerConfig{Title: "Test Dashboard"}

	// Create ingresses with names that should be sorted alphabetically
	ingressZebra := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zebra-app",
			Namespace: "test",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":    "Zebra Application",
				"service.homer.rajsingh.info/name": "Applications",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "zebra.example.com"},
			},
		},
	}

	ingressAlpha := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpha-app",
			Namespace: "test",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":    "Alpha Application",
				"service.homer.rajsingh.info/name": "Applications",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "alpha.example.com"},
			},
		},
	}

	ingressBeta := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "beta-app",
			Namespace: "test",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":    "Beta Application",
				"service.homer.rajsingh.info/name": "Applications",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "beta.example.com"},
			},
		},
	}

	// Process ingresses in non-alphabetical order
	UpdateHomerConfigIngress(config, ingressZebra, nil)
	UpdateHomerConfigIngress(config, ingressAlpha, nil)
	UpdateHomerConfigIngress(config, ingressBeta, nil)

	// Normalize config (which includes sorting)
	normalizeHomerConfig(config)

	// Verify items are sorted alphabetically by name
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	items := config.Services[0].Items
	if len(items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(items))
	}

	expectedOrder := []string{"Alpha Application", "Beta Application", "Zebra Application"}
	for i, expectedName := range expectedOrder {
		actualName := getItemName(&items[i])
		if actualName != expectedName {
			t.Errorf("Item %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}
}

func TestSortingIntegrationWithHTTPRouteDiscovery(t *testing.T) {
	// Test end-to-end sorting with real HTTPRoute processing
	config := &HomerConfig{Title: "Test Dashboard"}

	// Create HTTPRoutes with names that should be sorted alphabetically
	routeCharlie := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "charlie-route",
			Namespace: "test",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":    "Charlie Service",
				"service.homer.rajsingh.info/name": "Services",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"charlie.example.com"},
		},
	}

	routeAlpha := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpha-route",
			Namespace: "test",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":    "Alpha Service",
				"service.homer.rajsingh.info/name": "Services",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"alpha.example.com"},
		},
	}

	// Process HTTPRoutes in non-alphabetical order
	UpdateHomerConfigHTTPRoute(config, routeCharlie, nil)
	UpdateHomerConfigHTTPRoute(config, routeAlpha, nil)

	// Normalize config (which includes sorting)
	normalizeHomerConfig(config)

	// Verify items are sorted alphabetically by name
	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	items := config.Services[0].Items
	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}

	expectedOrder := []string{"Alpha Service", "Charlie Service"}
	for i, expectedName := range expectedOrder {
		actualName := getItemName(&items[i])
		if actualName != expectedName {
			t.Errorf("Item %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}
}

// Helper functions to reduce code duplication

func createTestConfigWithMixedCase() *HomerConfig {
	return &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "zebra services",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "ZEBRA App",
						},
					},
					{
						Parameters: map[string]string{
							"name": "alpha App",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Beta App",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "Alpha Services",
				},
				Items: []Item{},
			},
		},
	}
}

func createTestConfigWithSpecialChars() *HomerConfig {
	return &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "Service-B",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "Item_2",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Item-1",
						},
					},
					{
						Parameters: map[string]string{
							"name": "Item 3",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "Service A",
				},
				Items: []Item{},
			},
		},
	}
}

func verifyServiceOrder(t *testing.T, config *HomerConfig, expectedOrder []string) {
	for i, expectedName := range expectedOrder {
		actualName := getServiceName(&config.Services[i])
		if actualName != expectedName {
			t.Errorf("Service %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}
}

func verifyItemOrder(t *testing.T, items []Item, expectedOrder []string) {
	for i, expectedName := range expectedOrder {
		actualName := getItemName(&items[i])
		if actualName != expectedName {
			t.Errorf("Item %d: expected '%s', got '%s'", i, expectedName, actualName)
		}
	}
}
