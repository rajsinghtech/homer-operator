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
		Name: "test-service",
		Url:  "https://example.com",
	}

	enhanceItemWithHealthCheck(&item, healthConfig)

	if item.Type != GenericType {
		t.Errorf("Expected type to be set to '%s', got '%s'", GenericType, item.Type)
	}

	if item.Endpoint != "https://example.com/health" {
		t.Errorf("Expected endpoint to be 'https://example.com/health', got '%s'", item.Endpoint)
	}

	if item.Headers == nil {
		t.Fatal("Expected headers to be set")
	}

	if item.Headers["User-Agent"] != "Homer-Operator-Health-Check" {
		t.Errorf("Expected User-Agent header to be set")
	}
}

func TestServiceHealthEnhancementDisabled(t *testing.T) {
	healthConfig := &ServiceHealthConfig{
		Enabled: false,
	}

	item := Item{
		Name: "test-service",
		Url:  "https://example.com",
	}

	original := item
	enhanceItemWithHealthCheck(&item, healthConfig)

	// Item should remain unchanged
	if item.Type != original.Type {
		t.Errorf("Expected item to remain unchanged when health check disabled")
	}
	if item.Endpoint != original.Endpoint {
		t.Errorf("Expected item to remain unchanged when health check disabled")
	}
}

func TestAggregateServiceMetrics(t *testing.T) {
	service := Service{
		Name: "test-service",
		Items: []Item{
			{
				Name:       "api-service",
				Url:        "https://api.example.com",
				Type:       "Generic",
				Endpoint:   "https://api.example.com/health",
				Tag:        "api",
				LastUpdate: "2023-01-01T10:00:00Z",
			},
			{
				Name:       "web-service",
				Url:        "https://web.example.com",
				Tag:        "web",
				LastUpdate: "2023-01-01T11:00:00Z",
			},
			{
				Name:       "static-service",
				Url:        "https://static.example.com",
				Keywords:   "static,cdn",
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
			Name: "frontend",
			Items: []Item{
				{
					Name:     "web-app",
					Keywords: "web,api,backend",
					Subtitle: "Frontend application using backend API",
				},
			},
		},
		{
			Name: "backend",
			Items: []Item{
				{
					Name:     "api-server",
					Keywords: "api,database",
					Subtitle: "Backend API server",
				},
			},
		},
		{
			Name: "database",
			Items: []Item{
				{
					Name:     "postgres",
					Keywords: "database,storage",
					Subtitle: "PostgreSQL database",
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
			Name: "small-service",
			Items: []Item{
				{Name: "item1"},
			},
		},
		{
			Name: "large-service",
			Items: []Item{
				{Name: "item1"},
				{Name: "item2"},
				{Name: "item3"},
			},
		},
		{
			Name: "medium-service",
			Items: []Item{
				{Name: "item1"},
				{Name: "item2"},
			},
		},
		{
			Name: "another-small-service",
			Items: []Item{
				{Name: "item1"},
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
		if optimized[i].Name != expectedName {
			t.Errorf("Expected service at position %d to be '%s', got '%s'", i, expectedName, optimized[i].Name)
		}
	}
}

func TestCountingFunctions(t *testing.T) {
	items := []Item{
		{
			Name: "item1",
			Url:  "https://example1.com",
			Tag:  "web",
			Type: "Generic",
		},
		{
			Name: "item2",
			Url:  "https://example2.com",
			// No tag
			// No type
		},
		{
			Name: "item3",
			// No URL
			Tag:  "api",
			Type: "Prometheus",
		},
		{
			Name: "item4",
			// No URL, no tag, no type
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
				Name: "test-service",
				Items: []Item{
					{
						Name: "api-service",
						Url:  "https://api.example.com",
					},
					{
						Name: "web-service",
						Url:  "https://web.example.com",
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
			if item.Type != GenericType {
				t.Errorf("Expected item type to be '%s', got '%s'", GenericType, item.Type)
			}
			if item.Endpoint == "" {
				t.Error("Expected item endpoint to be set")
			}
			if item.Headers == nil || item.Headers["User-Agent"] != "Homer-Health-Check" {
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
				Name: "test-item",
				Url:  "https://example.com",
			}

			originalType := item.Type
			enhanceItemWithHealthCheck(&item, tt.config)

			enhanced := item.Type != originalType
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
			Name: fmt.Sprintf("service-%d", i),
		}
		for j := 0; j < 20; j++ {
			item := Item{
				Name:     fmt.Sprintf("item-%d-%d", i, j),
				Url:      fmt.Sprintf("https://example-%d-%d.com", i, j),
				Keywords: fmt.Sprintf("keyword%d,tag%d", i, j),
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
			if item.Type != GenericType {
				t.Errorf("Expected all items to be enhanced with type '%s'", GenericType)
			}
		}
	}
}
