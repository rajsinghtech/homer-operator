package homer

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// TestIngressClusterAutoTag verifies that Ingresses from remote clusters get auto-tagged
func TestIngressClusterAutoTag(t *testing.T) {
	tests := []struct {
		name             string
		clusterAnnot     string
		tagStyleLabel    string
		expectTag        bool
		expectedTagName  string
		expectedTagStyle string
	}{
		{
			name:         "Remote cluster without tagstyle - no tag",
			clusterAnnot: "ottawa",
			expectTag:    false,
		},
		{
			name:             "Remote cluster with blue tag",
			clusterAnnot:     "ottawa",
			tagStyleLabel:    "is-info",
			expectTag:        true,
			expectedTagName:  "ottawa",
			expectedTagStyle: "is-info",
		},
		{
			name:             "Remote cluster with red tag",
			clusterAnnot:     "production",
			tagStyleLabel:    "is-danger",
			expectTag:        true,
			expectedTagName:  "production",
			expectedTagStyle: "is-danger",
		},
		{
			name:             "Remote cluster with yellow tag",
			clusterAnnot:     "staging",
			tagStyleLabel:    "is-warning",
			expectTag:        true,
			expectedTagName:  "staging",
			expectedTagStyle: "is-warning",
		},
		{
			name:         "Local cluster",
			clusterAnnot: "local",
			expectTag:    false,
		},
		{
			name:      "No cluster annotation",
			expectTag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			}

			if tt.clusterAnnot != "" {
				ingress.ObjectMeta.Annotations["homer.rajsingh.info/cluster"] = tt.clusterAnnot
			}

			if tt.tagStyleLabel != "" {
				ingress.ObjectMeta.Labels["cluster-tagstyle"] = tt.tagStyleLabel
			}

			config := &HomerConfig{
				Title:  "Test Dashboard",
				Header: true,
			}

			UpdateHomerConfigIngress(config, *ingress, nil)

			if len(config.Services) == 0 {
				t.Fatal("Expected at least one service")
			}

			service := config.Services[0]
			if len(service.Items) == 0 {
				t.Fatal("Expected at least one item")
			}

			item := service.Items[0]

			if tt.expectTag {
				tag, hasTag := item.Parameters["tag"]
				if !hasTag {
					t.Errorf("Expected tag to be set for cluster %s", tt.clusterAnnot)
				}
				if tag != tt.expectedTagName {
					t.Errorf("Expected tag %q, got %q", tt.expectedTagName, tag)
				}

				tagstyle, hasTagStyle := item.Parameters["tagstyle"]
				if !hasTagStyle {
					t.Error("Expected tagstyle to be set")
				}
				if tagstyle != tt.expectedTagStyle {
					t.Errorf("Expected tagstyle %q, got %q", tt.expectedTagStyle, tagstyle)
				}
			} else {
				if tag, hasTag := item.Parameters["tag"]; hasTag {
					t.Errorf("Expected no tag for %s, but got %q", tt.name, tag)
				}
			}
		})
	}
}

// TestHTTPRouteClusterAutoTag verifies that HTTPRoutes from remote clusters get auto-tagged
func TestHTTPRouteClusterAutoTag(t *testing.T) {
	tests := []struct {
		name             string
		clusterAnnot     string
		tagStyleLabel    string
		expectTag        bool
		expectedTagName  string
		expectedTagStyle string
	}{
		{
			name:         "Remote cluster without tagstyle - no tag",
			clusterAnnot: "ottawa",
			expectTag:    false,
		},
		{
			name:             "Remote cluster with blue tag",
			clusterAnnot:     "ottawa",
			tagStyleLabel:    "is-info",
			expectTag:        true,
			expectedTagName:  "ottawa",
			expectedTagStyle: "is-info",
		},
		{
			name:             "Remote cluster with red tag",
			clusterAnnot:     "production",
			tagStyleLabel:    "is-danger",
			expectTag:        true,
			expectedTagName:  "production",
			expectedTagStyle: "is-danger",
		},
		{
			name:         "Local cluster",
			clusterAnnot: "local",
			expectTag:    false,
		},
		{
			name:      "No cluster annotation",
			expectTag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostname := gatewayv1.Hostname("test.example.com")
			httproute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-httproute",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{hostname},
				},
			}

			if tt.clusterAnnot != "" {
				httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"] = tt.clusterAnnot
			}

			if tt.tagStyleLabel != "" {
				httproute.ObjectMeta.Labels["cluster-tagstyle"] = tt.tagStyleLabel
			}

			config := &HomerConfig{
				Title:  "Test Dashboard",
				Header: true,
			}

			UpdateHomerConfigHTTPRoute(config, httproute, nil)

			if len(config.Services) == 0 {
				t.Fatal("Expected at least one service")
			}

			service := config.Services[0]
			if len(service.Items) == 0 {
				t.Fatal("Expected at least one item")
			}

			item := service.Items[0]

			if tt.expectTag {
				tag, hasTag := item.Parameters["tag"]
				if !hasTag {
					t.Errorf("Expected tag to be set for cluster %s", tt.clusterAnnot)
				}
				if tag != tt.expectedTagName {
					t.Errorf("Expected tag %q, got %q", tt.expectedTagName, tag)
				}

				tagstyle, hasTagStyle := item.Parameters["tagstyle"]
				if !hasTagStyle {
					t.Error("Expected tagstyle to be set")
				}
				if tagstyle != tt.expectedTagStyle {
					t.Errorf("Expected tagstyle %q, got %q", tt.expectedTagStyle, tagstyle)
				}
			} else {
				if tag, hasTag := item.Parameters["tag"]; hasTag {
					t.Errorf("Expected no tag for %s, but got %q", tt.name, tag)
				}
			}
		})
	}
}

// TestClusterTagNotOverriddenByAnnotations verifies that manual annotations take precedence
func TestClusterTagNotOverriddenByAnnotations(t *testing.T) {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			Annotations: map[string]string{
				"homer.rajsingh.info/cluster":       "ottawa",
				"item.homer.rajsingh.info/tag":      "production",
				"item.homer.rajsingh.info/tagstyle": "is-danger",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "test.example.com"},
			},
		},
	}

	config := &HomerConfig{
		Title:  "Test Dashboard",
		Header: true,
	}

	UpdateHomerConfigIngress(config, *ingress, nil)

	if len(config.Services) == 0 {
		t.Fatal("Expected at least one service")
	}

	service := config.Services[0]
	if len(service.Items) == 0 {
		t.Fatal("Expected at least one item")
	}

	item := service.Items[0]

	// Manual annotation should take precedence over auto-tag
	tag := item.Parameters["tag"]
	if tag != "production" {
		t.Errorf("Expected manual tag 'production', got %q", tag)
	}

	tagstyle := item.Parameters["tagstyle"]
	if tagstyle != "is-danger" {
		t.Errorf("Expected manual tagstyle 'is-danger', got %q", tagstyle)
	}
}

// TestClusterNameSuffix verifies that cluster name suffix is appended to item names from label
func TestClusterNameSuffix(t *testing.T) {
	tests := []struct {
		name               string
		clusterAnnot       string
		clusterNameSuffix  string // stored in label
		expectedNameSuffix string
	}{
		{
			name:               "Remote cluster with suffix in parentheses",
			clusterAnnot:       "ottawa",
			clusterNameSuffix:  " (ottawa)",
			expectedNameSuffix: " (ottawa)",
		},
		{
			name:               "Remote cluster with dash suffix",
			clusterAnnot:       "production",
			clusterNameSuffix:  " - production",
			expectedNameSuffix: " - production",
		},
		{
			name:               "Remote cluster with bracket suffix",
			clusterAnnot:       "staging",
			clusterNameSuffix:  " [staging]",
			expectedNameSuffix: " [staging]",
		},
		{
			name:              "Remote cluster without suffix label",
			clusterAnnot:      "ottawa",
			clusterNameSuffix: "",
		},
		{
			name:              "Local cluster with suffix label (should not apply)",
			clusterAnnot:      "local",
			clusterNameSuffix: " (local)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with Ingress
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "test.example.com"},
					},
				},
			}

			if tt.clusterAnnot != "" {
				ingress.ObjectMeta.Annotations["homer.rajsingh.info/cluster"] = tt.clusterAnnot
			}

			if tt.clusterNameSuffix != "" {
				ingress.ObjectMeta.Labels["cluster-name-suffix"] = tt.clusterNameSuffix
			}

			config := &HomerConfig{
				Title:  "Test Dashboard",
				Header: true,
			}

			UpdateHomerConfigIngress(config, *ingress, nil)

			if len(config.Services) == 0 {
				t.Fatal("Expected at least one service")
			}

			service := config.Services[0]
			if len(service.Items) == 0 {
				t.Fatal("Expected at least one item")
			}

			item := service.Items[0]
			itemName := item.Parameters["name"]
			baseName := "test-ingress"

			if tt.expectedNameSuffix != "" {
				expectedName := baseName + tt.expectedNameSuffix
				if itemName != expectedName {
					t.Errorf("Expected name %q, got %q", expectedName, itemName)
				}
			} else {
				// Should not have suffix
				if itemName != baseName {
					t.Errorf("Expected name %q without suffix, got %q", baseName, itemName)
				}
			}

			// Test with HTTPRoute
			hostname := gatewayv1.Hostname("test.example.com")
			httproute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-httproute",
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{hostname},
				},
			}

			if tt.clusterAnnot != "" {
				httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"] = tt.clusterAnnot
			}

			if tt.clusterNameSuffix != "" {
				httproute.ObjectMeta.Labels["cluster-name-suffix"] = tt.clusterNameSuffix
			}

			config2 := &HomerConfig{
				Title:  "Test Dashboard",
				Header: true,
			}

			UpdateHomerConfigHTTPRoute(config2, httproute, nil)

			if len(config2.Services) == 0 {
				t.Fatal("Expected at least one service")
			}

			service2 := config2.Services[0]
			if len(service2.Items) == 0 {
				t.Fatal("Expected at least one item")
			}

			item2 := service2.Items[0]
			itemName2 := item2.Parameters["name"]
			baseName2 := "test-httproute"

			if tt.expectedNameSuffix != "" {
				expectedName2 := baseName2 + tt.expectedNameSuffix
				if itemName2 != expectedName2 {
					t.Errorf("Expected HTTPRoute name %q, got %q", expectedName2, itemName2)
				}
			} else {
				// Should not have suffix
				if itemName2 != baseName2 {
					t.Errorf("Expected HTTPRoute name %q without suffix, got %q", baseName2, itemName2)
				}
			}
		})
	}
}
