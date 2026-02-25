package homer

import (
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestQuickLinksAnnotationProcessing(t *testing.T) {
	annotations := map[string]string{
		"item.homer.rajsingh.info/name":           "My Service",
		"item.homer.rajsingh.info/quick.0.name":   "Admin",
		"item.homer.rajsingh.info/quick.0.url":    "https://example.com/admin",
		"item.homer.rajsingh.info/quick.0.target": "_blank",
		"item.homer.rajsingh.info/quick.0.icon":   "fas fa-cog",
		"item.homer.rajsingh.info/quick.0.color":  "#333",
		"item.homer.rajsingh.info/quick.1.name":   "API",
		"item.homer.rajsingh.info/quick.1.url":    "https://example.com/api",
	}

	item := Item{}
	processItemAnnotations(&item, annotations)

	if item.Parameters["name"] != "My Service" {
		t.Errorf("expected name 'My Service', got '%s'", item.Parameters["name"])
	}

	if item.ArrayObjects == nil {
		t.Fatal("expected ArrayObjects to be initialized")
	}

	quickLinks := item.ArrayObjects["quick"]
	if len(quickLinks) != 2 {
		t.Fatalf("expected 2 quick links, got %d", len(quickLinks))
	}

	if quickLinks[0]["name"] != "Admin" {
		t.Errorf("expected quick[0].name='Admin', got '%s'", quickLinks[0]["name"])
	}
	if quickLinks[0]["url"] != "https://example.com/admin" {
		t.Errorf("expected quick[0].url, got '%s'", quickLinks[0]["url"])
	}
	if quickLinks[0]["target"] != "_blank" {
		t.Errorf("expected quick[0].target='_blank', got '%s'", quickLinks[0]["target"])
	}
	if quickLinks[0]["icon"] != "fas fa-cog" {
		t.Errorf("expected quick[0].icon, got '%s'", quickLinks[0]["icon"])
	}
	if quickLinks[0]["color"] != "#333" {
		t.Errorf("expected quick[0].color='#333', got '%s'", quickLinks[0]["color"])
	}

	if quickLinks[1]["name"] != "API" {
		t.Errorf("expected quick[1].name='API', got '%s'", quickLinks[1]["name"])
	}
	if quickLinks[1]["url"] != "https://example.com/api" {
		t.Errorf("expected quick[1].url, got '%s'", quickLinks[1]["url"])
	}
}

func TestQuickLinksYAMLOutput(t *testing.T) {
	config := &HomerConfig{
		Title: "Test",
		Services: []Service{
			{
				Parameters: map[string]string{"name": "Test"},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name": "My Service",
							"url":  "https://example.com",
						},
						ArrayObjects: map[string][]map[string]string{
							"quick": {
								{"name": "Admin", "url": "https://example.com/admin", "icon": "fas fa-cog"},
								{"name": "API", "url": "https://example.com/api"},
							},
						},
					},
				},
			},
		},
	}

	yamlBytes, err := marshalHomerConfigToYAML(config)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(yamlBytes, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	services := parsed["services"].([]any)
	svc := services[0].(map[any]any)
	items := svc["items"].([]any)
	item := items[0].(map[any]any)

	quickVal, ok := item["quick"]
	if !ok {
		t.Fatal("expected quick field in YAML output")
	}

	quickArr, ok := quickVal.([]any)
	if !ok {
		t.Fatalf("expected quick to be array, got %T", quickVal)
	}

	if len(quickArr) != 2 {
		t.Fatalf("expected 2 quick links, got %d", len(quickArr))
	}

	first := quickArr[0].(map[any]any)
	if first["name"] != "Admin" {
		t.Errorf("expected quick[0].name='Admin', got '%v'", first["name"])
	}
	if first["url"] != "https://example.com/admin" {
		t.Errorf("expected quick[0].url, got '%v'", first["url"])
	}
}

func TestQuickLinksSparseIndices(t *testing.T) {
	annotations := map[string]string{
		"item.homer.rajsingh.info/quick.0.name": "First",
		"item.homer.rajsingh.info/quick.2.name": "Third",
	}

	item := Item{}
	processItemAnnotations(&item, annotations)

	quickLinks := item.ArrayObjects["quick"]
	if len(quickLinks) != 3 {
		t.Fatalf("expected 3 entries (with gap), got %d", len(quickLinks))
	}
	if quickLinks[0]["name"] != "First" {
		t.Errorf("expected quick[0].name='First', got '%s'", quickLinks[0]["name"])
	}
	if len(quickLinks[1]) != 0 {
		t.Errorf("expected quick[1] to be empty, got %v", quickLinks[1])
	}
	if quickLinks[2]["name"] != "Third" {
		t.Errorf("expected quick[2].name='Third', got '%s'", quickLinks[2]["name"])
	}
}
