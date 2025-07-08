package homer

import (
	"fmt"
	"testing"
	"time"
)

func TestServiceHealthEnhancement(t *testing.T) {
	healthConfig := &ServiceHealthConfig{
		Enabled:      true,
		Interval:     "30s",
		Timeout:      "10s",
		HealthPath:   "/health",
		ExpectedCode: 200,
		Headers: map[string]string{
			"User-Agent": "Homer-Operator-Health-Check",
		},
	}

	item := Item{
		Parameters: map[string]string{
			"name": "test-service",
			"url":  "https://example.com",
		},
	}

	enhanceItemWithHealthCheck(&item, healthConfig)

	// Check Parameters map for type
	actualType := ""
	if item.Parameters != nil {
		actualType = item.Parameters["type"]
	}
	if actualType != GenericType {
		t.Errorf("Expected type to be set to '%s', got '%s'", GenericType, actualType)
	}

	// Check Parameters map for endpoint
	expectedEndpoint := "https://example.com/health"
	actualEndpoint := ""
	if item.Parameters != nil {
		actualEndpoint = item.Parameters["endpoint"]
	}
	if actualEndpoint != expectedEndpoint {
		t.Errorf("Expected endpoint to be '%s', got '%s'", expectedEndpoint, actualEndpoint)
	}

	// Check NestedObjects for headers
	if item.NestedObjects == nil || item.NestedObjects["headers"] == nil {
		t.Fatal("Expected headers to be set")
	}

	if item.NestedObjects["headers"]["User-Agent"] != "Homer-Operator-Health-Check" {
		t.Errorf("Expected User-Agent header to be set")
	}
}

func TestServiceHealthEnhancementDisabled(t *testing.T) {
	healthConfig := &ServiceHealthConfig{
		Enabled: false,
	}

	item := Item{
		Parameters: map[string]string{
			"name": "test-service",
			"url":  "https://example.com",
		},
	}

	// Make a copy of the original parameters for comparison
	originalType := ""
	originalEndpoint := ""
	if item.Parameters != nil {
		originalType = item.Parameters["type"]
		originalEndpoint = item.Parameters["endpoint"]
	}

	enhanceItemWithHealthCheck(&item, healthConfig)

	// Item should remain unchanged
	currentType := ""
	currentEndpoint := ""
	if item.Parameters != nil {
		currentType = item.Parameters["type"]
		currentEndpoint = item.Parameters["endpoint"]
	}

	if currentType != originalType {
		t.Errorf("Expected item type to remain unchanged when health check disabled")
	}
	if currentEndpoint != originalEndpoint {
		t.Errorf("Expected item endpoint to remain unchanged when health check disabled")
	}
}

func TestAggregateServiceMetrics(t *testing.T) {
	service := Service{
		Parameters: map[string]string{
			"name": "test-service",
		},
		Items: []Item{
			{
				Parameters: map[string]string{
					"name":     "api-service",
					"url":      "https://api.example.com",
					"type":     "Generic",
					"endpoint": "https://api.example.com/health",
					"tag":      "api",
				},
				LastUpdate: "2023-01-01T10:00:00Z",
			},
			{
				Parameters: map[string]string{
					"name": "web-service",
					"url":  "https://web.example.com",
					"tag":  "web",
				},
				LastUpdate: "2023-01-01T11:00:00Z",
			},
			{
				Parameters: map[string]string{
					"name":     "static-service",
					"url":      "https://static.example.com",
					"keywords": "static,cdn",
				},
				LastUpdate: "2023-01-01T09:00:00Z",
			},
		},
	}

	metrics := aggregateServiceMetrics(&service)

	if metrics.TotalItems != 3 {
		t.Errorf("Expected 3 total items, got %d", metrics.TotalItems)
	}

	if metrics.HealthyItems != 1 {
		t.Errorf("Expected 1 healthy item (with endpoint), got %d", metrics.HealthyItems)
	}

	if metrics.UnhealthyItems != 2 {
		t.Errorf("Expected 2 unhealthy items (without endpoint), got %d", metrics.UnhealthyItems)
	}

	if metrics.LastUpdated != "2023-01-01T11:00:00Z" {
		t.Errorf("Expected latest update time '2023-01-01T11:00:00Z', got '%s'", metrics.LastUpdated)
	}

	// Check custom metrics
	if itemsWithUrls := metrics.CustomMetrics["itemsWithUrls"]; itemsWithUrls != "3" {
		t.Errorf("Expected '3' items with URLs, got %v", itemsWithUrls)
	}

	if itemsWithTags := metrics.CustomMetrics["itemsWithTags"]; itemsWithTags != "2" {
		t.Errorf("Expected '2' items with tags, got %v", itemsWithTags)
	}

	if smartCards := metrics.CustomMetrics["smartCards"]; smartCards != "1" {
		t.Errorf("Expected '1' smart card, got %v", smartCards)
	}
}

func TestFindServiceDependencies(t *testing.T) {
	services := []Service{
		{
			Parameters: map[string]string{
				"name": "frontend",
			},
			Items: []Item{
				{
					Parameters: map[string]string{
						"name":     "web-app",
						"keywords": "web,api,backend",
						"subtitle": "Frontend application using backend API",
					},
				},
			},
		},
		{
			Parameters: map[string]string{
				"name": "backend",
			},
			Items: []Item{
				{
					Parameters: map[string]string{
						"name":     "api-server",
						"keywords": "api,database",
						"subtitle": "Backend API server",
					},
				},
			},
		},
		{
			Parameters: map[string]string{
				"name": "database",
			},
			Items: []Item{
				{
					Parameters: map[string]string{
						"name":     "postgres",
						"keywords": "database,storage",
						"subtitle": "PostgreSQL database",
					},
				},
			},
		},
	}

	dependencies := findServiceDependencies(services)

	// Should find dependencies based on keywords and subtitles
	if len(dependencies) == 0 {
		t.Error("Expected to find some dependencies")
	}

	// Check for specific dependencies
	foundFrontendToBackend := false
	foundBackendToDatabase := false

	for _, dep := range dependencies {
		if dep.ServiceName == "backend" && dep.Type == "soft" {
			foundFrontendToBackend = true
		}
		if dep.ServiceName == "database" && dep.Type == "soft" {
			foundBackendToDatabase = true
		}
	}

	if !foundFrontendToBackend {
		t.Error("Expected to find dependency from frontend to backend")
	}

	if !foundBackendToDatabase {
		t.Error("Expected to find dependency from backend to database")
	}
}

func TestOptimizeServiceLayout(t *testing.T) {
	services := []Service{
		{
			Parameters: map[string]string{"name": "small-service"},
			Items: []Item{
				{Parameters: map[string]string{"name": "item1"}},
			},
		},
		{
			Parameters: map[string]string{"name": "large-service"},
			Items: []Item{
				{Parameters: map[string]string{"name": "item1"}},
				{Parameters: map[string]string{"name": "item2"}},
				{Parameters: map[string]string{"name": "item3"}},
			},
		},
		{
			Parameters: map[string]string{"name": "medium-service"},
			Items: []Item{
				{Parameters: map[string]string{"name": "item1"}},
				{Parameters: map[string]string{"name": "item2"}},
			},
		},
		{
			Parameters: map[string]string{"name": "another-small-service"},
			Items: []Item{
				{Parameters: map[string]string{"name": "item1"}},
			},
		},
	}

	dependencies := []ServiceDependency{} // Empty for this test

	optimized := optimizeServiceLayout(services, dependencies)

	// Should be sorted by item count (descending), then by name (ascending)
	expectedOrder := []string{"large-service", "medium-service", "another-small-service", "small-service"}

	if len(optimized) != len(expectedOrder) {
		t.Fatalf("Expected %d services, got %d", len(expectedOrder), len(optimized))
	}

	for i, expectedName := range expectedOrder {
		actualName := ""
		if optimized[i].Parameters != nil {
			actualName = optimized[i].Parameters["name"]
		}
		if actualName != expectedName {
			t.Errorf("Expected service at position %d to be '%s', got '%s'", i, expectedName, actualName)
		}
	}
}

func TestCountingFunctions(t *testing.T) {
	items := []Item{
		{
			Parameters: map[string]string{
				"name": "item1",
				"url":  "https://example1.com",
				"tag":  "web",
				"type": "Generic",
			},
		},
		{
			Parameters: map[string]string{
				"name": "item2",
				"url":  "https://example2.com",
				// No tag
				// No type
			},
		},
		{
			Parameters: map[string]string{
				"name": "item3",
				// No URL
				"tag":  "api",
				"type": "Prometheus",
			},
		},
		{
			Parameters: map[string]string{
				"name": "item4",
				// No URL, no tag, no type
			},
		},
	}

	urlCount := countItemsWithUrls(items)
	if urlCount != 2 {
		t.Errorf("Expected 2 items with URLs, got %d", urlCount)
	}

	tagCount := countItemsWithTags(items)
	if tagCount != 2 {
		t.Errorf("Expected 2 items with tags, got %d", tagCount)
	}

	smartCardCount := countSmartCards(items)
	if smartCardCount != 2 {
		t.Errorf("Expected 2 smart cards, got %d", smartCardCount)
	}
}

func TestEnhanceHomerConfigWithAggregation(t *testing.T) {
	config := &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "test-service",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "api-service",
							"url":  "https://api.example.com",
						},
					},
					{
						Parameters: map[string]string{
							"name": "web-service",
							"url":  "https://web.example.com",
						},
					},
				},
			},
		},
	}

	healthConfig := &ServiceHealthConfig{
		Enabled:    true,
		HealthPath: "/health",
		Headers: map[string]string{
			"User-Agent": "Homer-Health-Check",
		},
	}

	enhanceHomerConfigWithAggregation(config, healthConfig)

	// Verify that items were enhanced with health check capabilities
	for _, service := range config.Services {
		for _, item := range service.Items {
			itemType := ""
			endpoint := ""
			if item.Parameters != nil {
				itemType = item.Parameters["type"]
				endpoint = item.Parameters["endpoint"]
			}

			if itemType != GenericType {
				t.Errorf("Expected item type to be '%s', got '%s'", GenericType, itemType)
			}
			if endpoint == "" {
				t.Error("Expected item endpoint to be set")
			}

			userAgent := ""
			if item.NestedObjects != nil && item.NestedObjects["headers"] != nil {
				userAgent = item.NestedObjects["headers"]["User-Agent"]
			}
			if userAgent != "Homer-Health-Check" {
				t.Error("Expected health check headers to be set")
			}
		}
	}
}

func TestServiceHealthConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   *ServiceHealthConfig
		expected bool
	}{
		{
			name:     "Nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "Disabled config",
			config: &ServiceHealthConfig{
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "Enabled config",
			config: &ServiceHealthConfig{
				Enabled: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Item{
				Parameters: map[string]string{
					"name": "test-item",
					"url":  "https://example.com",
				},
			}

			originalType := ""
			if item.Parameters != nil {
				originalType = item.Parameters["type"]
			}
			enhanceItemWithHealthCheck(&item, tt.config)

			currentType := ""
			if item.Parameters != nil {
				currentType = item.Parameters["type"]
			}
			enhanced := currentType != originalType
			if enhanced != tt.expected {
				t.Errorf("Expected enhancement=%v, got enhancement=%v", tt.expected, enhanced)
			}
		})
	}
}

func TestPerformanceWithLargeConfig(t *testing.T) {
	// Create a large configuration
	config := &HomerConfig{}

	// Add 50 services with 20 items each
	for i := 0; i < 50; i++ {
		service := Service{
			Parameters: map[string]string{
				"name": fmt.Sprintf("service-%d", i),
			},
		}
		for j := 0; j < 20; j++ {
			item := Item{
				Parameters: map[string]string{
					"name":     fmt.Sprintf("item-%d-%d", i, j),
					"url":      fmt.Sprintf("https://example-%d-%d.com", i, j),
					"keywords": fmt.Sprintf("keyword%d,tag%d", i, j),
				},
			}
			service.Items = append(service.Items, item)
		}
		config.Services = append(config.Services, service)
	}

	healthConfig := &ServiceHealthConfig{
		Enabled:    true,
		HealthPath: "/health",
	}

	start := time.Now()
	enhanceHomerConfigWithAggregation(config, healthConfig)
	duration := time.Since(start)

	// Enhancement should complete in reasonable time for large configs
	if duration > time.Second {
		t.Errorf("Enhancement took too long: %v", duration)
	}

	// Verify all items were enhanced
	for _, service := range config.Services {
		for _, item := range service.Items {
			itemType := ""
			if item.Parameters != nil {
				itemType = item.Parameters["type"]
			}
			if itemType != GenericType {
				t.Errorf("Expected all items to be enhanced with type '%s'", GenericType)
			}
		}
	}
}
