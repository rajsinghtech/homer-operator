package homer

import (
	"fmt"
	"testing"
	"time"
)

func TestBasicServiceUpdate(t *testing.T) {
	config := &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "test-namespace",
				},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "existing-service",
							"url":  "http://old.example.com",
							"tag":  "old-tag",
						},
					},
				},
			},
		},
	}

	// Test adding a new item to existing service
	service := Service{
		Parameters: map[string]string{
			"name": "test-namespace",
		},
	}
	newItem := Item{
		Parameters: map[string]string{
			"name": "new-service",
			"url":  "https://new.example.com",
			"tag":  "new-tag",
		},
	}
	updateOrAddServiceItems(config, service, []Item{newItem})

	if len(config.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(config.Services))
	}

	if len(config.Services[0].Items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(config.Services[0].Items))
	}

	// Test replacing existing item
	replacementItem := Item{
		Parameters: map[string]string{
			"name": "existing-service",
			"url":  "https://updated.example.com",
			"tag":  "updated-tag",
		},
	}
	updateOrAddServiceItems(config, service, []Item{replacementItem})

	if len(config.Services[0].Items) != 2 {
		t.Fatalf("Expected 2 items after replacement, got %d", len(config.Services[0].Items))
	}

	// Find the updated item
	var updatedItem *Item
	for i, item := range config.Services[0].Items {
		if item.Parameters != nil && item.Parameters["name"] == "existing-service" {
			updatedItem = &config.Services[0].Items[i]
			break
		}
	}

	if updatedItem == nil {
		t.Fatal("Could not find updated item")
	}

	expectedURL := "https://updated.example.com"
	actualURL := ""
	if updatedItem.Parameters != nil {
		actualURL = updatedItem.Parameters["url"]
	}
	if actualURL != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, actualURL)
	}
}

func TestHeadersAnnotation(t *testing.T) {
	item := Item{}
	annotations := map[string]string{
		"item.homer.rajsingh.info/headers.authorization": "Bearer token123",
		"item.homer.rajsingh.info/headers.x-api-key":     "key456",
		"item.homer.rajsingh.info/name":                  "test-service",
	}

	processItemAnnotations(&item, annotations)

	if item.Parameters["name"] != "test-service" {
		t.Errorf("Expected name 'test-service', got '%s'", item.Parameters["name"])
	}

	// Headers are now stored in Parameters with headers. prefix
	if item.Parameters["headers.authorization"] != "Bearer token123" {
		t.Errorf("Expected authorization header 'Bearer token123', got '%s'", item.Parameters["headers.authorization"])
	}

	if item.Parameters["headers.x-api-key"] != "key456" {
		t.Errorf("Expected x-api-key header 'key456', got '%s'", item.Parameters["headers.x-api-key"])
	}
}

func TestParseBooleanValue(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"on", true},
		{"ON", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"off", false},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseBooleanValue(tt.input)
			if result != tt.expected {
				t.Errorf("parseBooleanValue(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseHeadersAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "single header",
			input: "Authorization: Bearer token123",
			expected: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
		{
			name:  "multiple headers",
			input: "Authorization: Bearer token123, X-API-Key: key456, Content-Type: application/json",
			expected: map[string]string{
				"Authorization": "Bearer token123",
				"X-API-Key":     "key456",
				"Content-Type":  "application/json",
			},
		},
		{
			name:  "headers with extra spaces",
			input: "  Authorization:Bearer token123  ,  X-API-Key:key456  ",
			expected: map[string]string{
				"Authorization": "Bearer token123",
				"X-API-Key":     "key456",
			},
		},
		{
			name:     "invalid format",
			input:    "invalid-header-format",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(map[string]string)
			parseHeadersAnnotation(headers, tt.input)

			if len(headers) != len(tt.expected) {
				t.Errorf("Expected %d headers, got %d", len(tt.expected), len(headers))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := headers[key]; !exists {
					t.Errorf("Expected header %s to exist", key)
				} else if actualValue != expectedValue {
					t.Errorf("Expected header %s to be '%s', got '%s'", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestValidateAnnotationValue(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		value     string
		level     ValidationLevel
		expectErr bool
	}{
		{
			name:      "valid URL",
			fieldName: "url",
			value:     "https://example.com",
			level:     ValidationLevelStrict,
			expectErr: false,
		},
		{
			name:      "invalid URL with strict validation",
			fieldName: "url",
			value:     "not-a-url",
			level:     ValidationLevelStrict,
			expectErr: true,
		},
		{
			name:      "invalid URL with warn validation",
			fieldName: "url",
			value:     "not-a-url",
			level:     ValidationLevelWarn,
			expectErr: false,
		},
		{
			name:      "valid target",
			fieldName: "target",
			value:     "_blank",
			level:     ValidationLevelStrict,
			expectErr: false,
		},
		{
			name:      "invalid target",
			fieldName: "target",
			value:     "_invalid",
			level:     ValidationLevelStrict,
			expectErr: true,
		},
		{
			name:      "valid numeric value",
			fieldName: "warning_value",
			value:     "85.5",
			level:     ValidationLevelStrict,
			expectErr: false,
		},
		{
			name:      "invalid numeric value",
			fieldName: "danger_value",
			value:     "not-a-number",
			level:     ValidationLevelStrict,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnnotationValue(tt.fieldName, tt.value, tt.level)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for %s='%s', but got none", tt.fieldName, tt.value)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for %s='%s', but got: %v", tt.fieldName, tt.value, err)
			}
		})
	}
}

func TestServiceGroupingPerformance(t *testing.T) {
	// Create a large config with many services and items
	config := &HomerConfig{}

	// Add 100 services with 10 items each
	for i := 0; i < 100; i++ {
		service := Service{
			Parameters: map[string]string{
				"name": fmt.Sprintf("service-%d", i),
			},
		}
		for j := 0; j < 10; j++ {
			item := Item{
				Parameters: map[string]string{
					"name": fmt.Sprintf("item-%d-%d", i, j),
					"url":  fmt.Sprintf("https://example-%d-%d.com", i, j),
				},
			}
			service.Items = append(service.Items, item)
		}
		config.Services = append(config.Services, service)
	}

	// Test adding new items to existing services
	start := time.Now()
	for i := 0; i < 100; i++ {
		service := Service{
			Parameters: map[string]string{
				"name": fmt.Sprintf("service-%d", i),
			},
		}
		newItem := Item{
			Parameters: map[string]string{
				"name": fmt.Sprintf("new-item-%d", i),
				"url":  fmt.Sprintf("https://new-example-%d.com", i),
			},
		}
		updateOrAddServiceItems(config, service, []Item{newItem})
	}
	duration := time.Since(start)

	// Performance should be reasonable for 100 services with 10 items each
	if duration > time.Millisecond*100 {
		t.Errorf("Service grouping took too long: %v", duration)
	}

	// Verify that new items were added
	for i := 0; i < 100; i++ {
		service := config.Services[i]
		if len(service.Items) != 11 { // 10 original + 1 new
			t.Errorf("Expected 11 items in service %d, got %d", i, len(service.Items))
		}
	}
}
