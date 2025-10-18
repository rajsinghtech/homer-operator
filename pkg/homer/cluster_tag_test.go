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
			name:             "Remote cluster ottawa - default blue",
			clusterAnnot:     "ottawa",
			expectTag:        true,
			expectedTagName:  "ottawa",
			expectedTagStyle: "is-info", // Default blue
		},
		{
			name:             "Remote cluster robbinsdale - default blue",
			clusterAnnot:     "robbinsdale",
			expectTag:        true,
			expectedTagName:  "robbinsdale",
			expectedTagStyle: "is-info", // Default blue
		},
		{
			name:             "Remote cluster with custom red color",
			clusterAnnot:     "ottawa",
			tagStyleLabel:    "is-danger",
			expectTag:        true,
			expectedTagName:  "ottawa",
			expectedTagStyle: "is-danger", // Custom red
		},
		{
			name:             "Remote cluster with custom yellow color",
			clusterAnnot:     "staging",
			tagStyleLabel:    "is-warning",
			expectTag:        true,
			expectedTagName:  "staging",
			expectedTagStyle: "is-warning", // Custom yellow
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
			name:             "Remote cluster ottawa - default blue",
			clusterAnnot:     "ottawa",
			expectTag:        true,
			expectedTagName:  "ottawa",
			expectedTagStyle: "is-info", // Default blue
		},
		{
			name:             "Remote cluster robbinsdale - default blue",
			clusterAnnot:     "robbinsdale",
			expectTag:        true,
			expectedTagName:  "robbinsdale",
			expectedTagStyle: "is-info", // Default blue
		},
		{
			name:             "Remote cluster with custom red color",
			clusterAnnot:     "production",
			tagStyleLabel:    "is-danger",
			expectTag:        true,
			expectedTagName:  "production",
			expectedTagStyle: "is-danger", // Custom red
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
