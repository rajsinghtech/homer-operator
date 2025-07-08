package homer

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDetermineServiceGroup(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		labels      map[string]string
		annotations map[string]string
		config      *ServiceGroupingConfig
		expected    string
	}{
		{
			name:        "Default namespace grouping",
			namespace:   "production",
			labels:      map[string]string{},
			annotations: map[string]string{},
			config:      nil,
			expected:    "production",
		},
		{
			name:        "Explicit namespace grouping",
			namespace:   "production",
			labels:      map[string]string{},
			annotations: map[string]string{},
			config:      &ServiceGroupingConfig{Strategy: ServiceGroupingNamespace},
			expected:    "production",
		},
		{
			name:        "Label-based grouping",
			namespace:   "production",
			labels:      map[string]string{"team": "frontend"},
			annotations: map[string]string{},
			config:      &ServiceGroupingConfig{Strategy: ServiceGroupingLabel, LabelKey: "team"},
			expected:    "frontend",
		},
		{
			name:        "Label-based grouping with fallback",
			namespace:   "production",
			labels:      map[string]string{"environment": "prod"},
			annotations: map[string]string{},
			config:      &ServiceGroupingConfig{Strategy: ServiceGroupingLabel, LabelKey: "team"},
			expected:    "production", // fallback to namespace
		},
		{
			name:        "Annotation override",
			namespace:   "production",
			labels:      map[string]string{"team": "frontend"},
			annotations: map[string]string{"service.homer.rajsingh.info/name": "Custom Service Group"},
			config:      &ServiceGroupingConfig{Strategy: ServiceGroupingLabel, LabelKey: "team"},
			expected:    "Custom Service Group", // annotation takes precedence
		},
		{
			name:        "Custom rules grouping",
			namespace:   "production",
			labels:      map[string]string{"app": "web-app", "environment": "prod"},
			annotations: map[string]string{},
			config: &ServiceGroupingConfig{
				Strategy: ServiceGroupingCustom,
				CustomRules: []GroupingRule{
					{
						Name:      "Production Web Services",
						Condition: map[string]string{"app": "web-*", "environment": "prod"},
						Priority:  1,
					},
					{
						Name:      "Development Services",
						Condition: map[string]string{"environment": "dev"},
						Priority:  2,
					},
				},
			},
			expected: "Production Web Services",
		},
		{
			name:        "Custom rules no match fallback",
			namespace:   "production",
			labels:      map[string]string{"app": "database", "environment": "staging"},
			annotations: map[string]string{},
			config: &ServiceGroupingConfig{
				Strategy: ServiceGroupingCustom,
				CustomRules: []GroupingRule{
					{
						Name:      "Production Web Services",
						Condition: map[string]string{"app": "web-*", "environment": "prod"},
						Priority:  1,
					},
				},
			},
			expected: "production", // fallback to namespace
		},
		{
			name:        "Empty namespace defaults to 'default'",
			namespace:   "", // Empty namespace
			labels:      map[string]string{},
			annotations: map[string]string{},
			config:      nil,
			expected:    "default", // should default to "default"
		},
		{
			name:        "Empty namespace with label strategy fallback",
			namespace:   "", // Empty namespace
			labels:      map[string]string{"app": "web"},
			annotations: map[string]string{},
			config:      &ServiceGroupingConfig{Strategy: ServiceGroupingLabel, LabelKey: "team"}, // team label doesn't exist
			expected:    "default",                                                                // fallback to "default"
		},
		{
			name:        "Empty label value with fallback",
			namespace:   "production",
			labels:      map[string]string{"team": ""}, // Empty label value
			annotations: map[string]string{},
			config:      &ServiceGroupingConfig{Strategy: ServiceGroupingLabel, LabelKey: "team"},
			expected:    "production", // should fallback to namespace when label is empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineServiceGroup(tt.namespace, tt.labels, tt.annotations, tt.config)
			if result != tt.expected {
				t.Errorf("Expected service group '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		value    string
		pattern  string
		expected bool
	}{
		{"web-app", "web-*", true},
		{"web-service", "web-*", true},
		{"api-gateway", "web-*", false},
		{"production", "prod*", true},
		{"production", "production", true},
		{"staging", "production", false},
		{"anything", "*", true},
	}

	for _, tt := range tests {
		t.Run(tt.value+"_"+tt.pattern, func(t *testing.T) {
			result := matchesPattern(tt.value, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPattern(%s, %s) = %v, expected %v", tt.value, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestMatchesCondition(t *testing.T) {
	labels := map[string]string{
		"app":         "web-app",
		"environment": "prod",
		"team":        "frontend",
	}
	annotations := map[string]string{
		"deployment.kubernetes.io/revision": "1",
		"custom.annotation":                 "value",
	}

	tests := []struct {
		name      string
		condition map[string]string
		expected  bool
	}{
		{
			name:      "Match single label",
			condition: map[string]string{"environment": "prod"},
			expected:  true,
		},
		{
			name:      "Match multiple labels",
			condition: map[string]string{"environment": "prod", "team": "frontend"},
			expected:  true,
		},
		{
			name:      "Match with wildcard",
			condition: map[string]string{"app": "web-*"},
			expected:  true,
		},
		{
			name:      "No match",
			condition: map[string]string{"environment": "staging"},
			expected:  false,
		},
		{
			name:      "Partial match fails",
			condition: map[string]string{"environment": "prod", "team": "backend"},
			expected:  false,
		},
		{
			name:      "Match annotation",
			condition: map[string]string{"custom.annotation": "value"},
			expected:  true,
		},
		{
			name:      "Missing key",
			condition: map[string]string{"missing": "value"},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesCondition(labels, annotations, tt.condition)
			if result != tt.expected {
				t.Errorf("matchesCondition(%v) = %v, expected %v", tt.condition, result, tt.expected)
			}
		})
	}
}

func TestFlexibleGroupingIntegration(t *testing.T) {
	// Create test ingress with labels
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "production",
			Labels: map[string]string{
				"app":  "web-app",
				"team": "frontend",
			},
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name": "My Web App",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "app.example.com",
				},
			},
		},
	}

	config := &HomerConfig{}

	// Test with label-based grouping
	groupingConfig := &ServiceGroupingConfig{
		Strategy: ServiceGroupingLabel,
		LabelKey: "team",
	}

	UpdateHomerConfigIngressWithGrouping(config, ingress, nil, groupingConfig)

	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	service := config.Services[0]
	expectedName := "frontend"
	actualName := ""
	if service.Parameters != nil {
		actualName = service.Parameters["name"]
	}
	if actualName != expectedName {
		t.Errorf("Expected service name '%s', got '%s'", expectedName, actualName)
	}

	if len(service.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(service.Items))
	}

	item := service.Items[0]
	// In the dynamic system, annotation-provided names are stored in Parameters
	expectedItemName := "My Web App"
	actualItemName := ""
	if item.Parameters != nil && item.Parameters["name"] != "" {
		actualItemName = item.Parameters["name"]
	}
	if actualItemName != expectedItemName {
		t.Errorf("Expected item name '%s', got '%s'", expectedItemName, actualItemName)
	}

	// Test adding another service to the same group
	ingress2 := ingress
	ingress2.ObjectMeta.Name = "another-app"
	ingress2.ObjectMeta.Annotations["item.homer.rajsingh.info/name"] = "Another App"
	ingress2.Spec.Rules[0].Host = "another.example.com"

	UpdateHomerConfigIngressWithGrouping(config, ingress2, nil, groupingConfig)

	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service after adding second app, got %d", len(config.Services))
	}

	if len(config.Services[0].Items) != 2 {
		t.Fatalf("Expected 2 items in service, got %d", len(config.Services[0].Items))
	}
}
