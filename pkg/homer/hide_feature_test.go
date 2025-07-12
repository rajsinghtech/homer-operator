package homer

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestIngressHideFeature(t *testing.T) {
	tests := []struct {
		name              string
		ingress           networkingv1.Ingress
		expectedItemCount int
		description       string
	}{
		{
			name: "ingress without hide annotation",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test App",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			},
			expectedItemCount: 1,
			description:       "should create item when no hide annotation",
		},
		{
			name: "ingress with hide=false",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test App",
						"item.homer.rajsingh.info/hide": "false",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			},
			expectedItemCount: 1,
			description:       "should create item when hide=false",
		},
		{
			name: "ingress with hide=true",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test App",
						"item.homer.rajsingh.info/hide": "true",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			},
			expectedItemCount: 0,
			description:       "should not create item when hide=true",
		},
		{
			name: "ingress with hide=1",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test App",
						"item.homer.rajsingh.info/hide": "1",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			},
			expectedItemCount: 0,
			description:       "should not create item when hide=1",
		},
		{
			name: "ingress with hide=yes",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test App",
						"item.homer.rajsingh.info/hide": "yes",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			},
			expectedItemCount: 0,
			description:       "should not create item when hide=yes",
		},
		{
			name: "ingress with multiple hosts, one hidden",
			ingress: networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test App",
						"item.homer.rajsingh.info/hide": "true",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test1.example.com"},
						{Host: "test2.example.com"},
					},
				},
			},
			expectedItemCount: 0,
			description:       "should not create any items when hide=true applies to all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &HomerConfig{Title: "Test Dashboard"}
			UpdateHomerConfigIngress(config, tt.ingress, nil)

			totalItems := 0
			for _, service := range config.Services {
				totalItems += len(service.Items)
			}

			if totalItems != tt.expectedItemCount {
				t.Errorf("%s: expected %d items, got %d items", tt.description, tt.expectedItemCount, totalItems)
			}
		})
	}
}

func TestHTTPRouteHideFeature(t *testing.T) {
	tests := []struct {
		name              string
		httproute         gatewayv1.HTTPRoute
		expectedItemCount int
		description       string
	}{
		{
			name: "httproute without hide annotation",
			httproute: gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test Route",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"test.example.com"},
				},
			},
			expectedItemCount: 1,
			description:       "should create item when no hide annotation",
		},
		{
			name: "httproute with hide=false",
			httproute: gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test Route",
						"item.homer.rajsingh.info/hide": "false",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"test.example.com"},
				},
			},
			expectedItemCount: 1,
			description:       "should create item when hide=false",
		},
		{
			name: "httproute with hide=true",
			httproute: gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test Route",
						"item.homer.rajsingh.info/hide": "true",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"test.example.com"},
				},
			},
			expectedItemCount: 0,
			description:       "should not create item when hide=true",
		},
		{
			name: "httproute with hide=0",
			httproute: gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test Route",
						"item.homer.rajsingh.info/hide": "0",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"test.example.com"},
				},
			},
			expectedItemCount: 1,
			description:       "should create item when hide=0",
		},
		{
			name: "httproute with multiple hostnames, all hidden",
			httproute: gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test Route",
						"item.homer.rajsingh.info/hide": "true",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{
						"test1.example.com",
						"test2.example.com",
					},
				},
			},
			expectedItemCount: 0,
			description:       "should not create any items when hide=true applies to all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &HomerConfig{Title: "Test Dashboard"}
			UpdateHomerConfigHTTPRoute(config, &tt.httproute, nil)

			totalItems := 0
			for _, service := range config.Services {
				totalItems += len(service.Items)
			}

			if totalItems != tt.expectedItemCount {
				t.Errorf("%s: expected %d items, got %d items", tt.description, tt.expectedItemCount, totalItems)
			}
		})
	}
}

func TestHideFeatureWithDomainFilters(t *testing.T) {
	t.Run("hidden ingress with domain filters", func(t *testing.T) {
		ingress := networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "default",
				Annotations: map[string]string{
					"item.homer.rajsingh.info/hide": "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{Host: "test.example.com"},
				},
			},
		}

		config := &HomerConfig{Title: "Test Dashboard"}
		UpdateHomerConfigIngress(config, ingress, []string{"example.com"})

		totalItems := 0
		for _, service := range config.Services {
			totalItems += len(service.Items)
		}

		if totalItems != 0 {
			t.Errorf("Expected 0 items when hide=true even with matching domain filters, got %d", totalItems)
		}
	})

	t.Run("hidden httproute with domain filters", func(t *testing.T) {
		httproute := gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "default",
				Annotations: map[string]string{
					"item.homer.rajsingh.info/hide": "true",
				},
			},
			Spec: gatewayv1.HTTPRouteSpec{
				Hostnames: []gatewayv1.Hostname{"test.example.com"},
			},
		}

		config := &HomerConfig{Title: "Test Dashboard"}
		UpdateHomerConfigHTTPRoute(config, &httproute, []string{"example.com"})

		totalItems := 0
		for _, service := range config.Services {
			totalItems += len(service.Items)
		}

		if totalItems != 0 {
			t.Errorf("Expected 0 items when hide=true even with matching domain filters, got %d", totalItems)
		}
	})
}
