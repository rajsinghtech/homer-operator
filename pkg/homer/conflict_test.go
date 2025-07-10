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

// Redundant annotation processing tests moved to annotation_processing_test.go

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
