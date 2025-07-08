package homer

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestHTTPRouteWithEmptyServiceName(t *testing.T) {
	// Test case 1: HTTPRoute with empty namespace
	t.Run("empty namespace", func(t *testing.T) {
		httproute := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "", // Empty namespace
			},
			Spec: gatewayv1.HTTPRouteSpec{
				Hostnames: []gatewayv1.Hostname{"test.rajsingh.info"},
			},
		}

		config := &HomerConfig{Title: "Test Dashboard"}
		UpdateHomerConfigHTTPRoute(config, httproute, []string{"rajsingh.info"})

		if err := ValidateHomerConfig(config); err != nil {
			t.Errorf("Validation should pass with empty namespace, but got error: %v", err)
		}

		// Check that a service was created with a proper name
		if len(config.Services) == 0 {
			t.Error("Expected at least one service to be created")
		} else {
			serviceName := getServiceName(&config.Services[0])
			if serviceName == "" {
				t.Error("Service name should not be empty")
			}
			if serviceName != DefaultNamespace {
				t.Errorf("Expected service name to be '%s', got '%s'", DefaultNamespace, serviceName)
			}
		}
	})

	// Test case 2: HTTPRoute with empty service name annotation
	t.Run("empty service name annotation", func(t *testing.T) {
		httproute := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					"service.homer.rajsingh.info/name": "", // Empty service name annotation
				},
			},
			Spec: gatewayv1.HTTPRouteSpec{
				Hostnames: []gatewayv1.Hostname{"test.rajsingh.info"},
			},
		}

		config := &HomerConfig{Title: "Test Dashboard"}
		UpdateHomerConfigHTTPRoute(config, httproute, []string{"rajsingh.info"})

		if err := ValidateHomerConfig(config); err != nil {
			t.Errorf("Validation should pass with empty service name annotation, but got error: %v", err)
		}

		// Check that a service was created with a proper name (should fall back to namespace)
		if len(config.Services) == 0 {
			t.Error("Expected at least one service to be created")
		} else {
			serviceName := getServiceName(&config.Services[0])
			if serviceName == "" {
				t.Error("Service name should not be empty")
			}
			if serviceName != "test-namespace" {
				t.Errorf("Expected service name to be 'test-namespace', got '%s'", serviceName)
			}
		}
	})

	// Test case 3: HTTPRoute with no matching hostnames (should not create empty service)
	t.Run("no matching hostnames", func(t *testing.T) {
		httproute := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "test-namespace",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				Hostnames: []gatewayv1.Hostname{"test.other-domain.com"}, // Doesn't match filter
			},
		}

		config := &HomerConfig{Title: "Test Dashboard"}
		UpdateHomerConfigHTTPRoute(config, httproute, []string{"rajsingh.info"})

		if err := ValidateHomerConfig(config); err != nil {
			t.Errorf("Validation should pass even with no matching hostnames, but got error: %v", err)
		}

		// Should not create any services since no hostnames match
		if len(config.Services) != 0 {
			t.Errorf("Expected no services to be created when no hostnames match, but got %d services", len(config.Services))
		}
	})

	// Test case 4: HomerConfig with pre-existing invalid services (simulates Dashboard CRD with bad data)
	t.Run("invalid pre-existing services", func(t *testing.T) {
		config := &HomerConfig{
			Title: "Test Dashboard",
			Services: []Service{
				{
					// Service with empty Parameters (like what we saw in the error)
					Items: []Item{
						{
							// Item with empty Parameters too
						},
					},
				},
			},
		}

		// This should clean up the invalid service
		cleanupHomerConfig(config)

		if err := ValidateHomerConfig(config); err != nil {
			t.Errorf("Validation should pass after cleanup, but got error: %v", err)
		}

		// Should have no services since the invalid one was removed
		if len(config.Services) != 0 {
			t.Errorf("Expected no services after cleanup, but got %d services", len(config.Services))
		}
	})

	// Test case 5: HomerConfig with pre-existing services that can be fixed
	t.Run("fixable pre-existing services", func(t *testing.T) {
		config := &HomerConfig{
			Title: "Test Dashboard",
			Services: []Service{
				{
					// Service with no Parameters but valid namespace context
					Parameters: map[string]string{"name": "valid-service"},
					Items: []Item{
						{
							Parameters: map[string]string{"name": "valid-item"},
						},
						{
							// Item with no name - should be removed
						},
					},
				},
			},
		}

		// This should clean up the invalid items but keep the valid service
		cleanupHomerConfig(config)

		if err := ValidateHomerConfig(config); err != nil {
			t.Errorf("Validation should pass after cleanup, but got error: %v", err)
		}

		// Should have 1 service with 1 item
		if len(config.Services) != 1 {
			t.Errorf("Expected 1 service after cleanup, but got %d services", len(config.Services))
		} else if len(config.Services[0].Items) != 1 {
			t.Errorf("Expected 1 item after cleanup, but got %d items", len(config.Services[0].Items))
		}
	})
}
