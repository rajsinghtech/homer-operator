package homer

import "testing"

func TestHomerConfigDeepCopyDeepCopiesMessageRefreshInterval(t *testing.T) {
	config := &HomerConfig{
		Message: MessageConfig{
			RefreshInterval: map[string]any{
				"nested": []any{
					map[string]any{"enabled": true},
				},
			},
		},
	}

	copy := config.DeepCopy()
	refreshInterval, ok := copy.Message.RefreshInterval.(map[string]any)
	if !ok {
		t.Fatalf("expected copied refresh interval to be map[string]any, got %T", copy.Message.RefreshInterval)
	}
	nested, ok := refreshInterval["nested"].([]any)
	if !ok || len(nested) != 1 {
		t.Fatalf("expected copied nested refresh interval slice, got %#v", refreshInterval["nested"])
	}
	nestedMap, ok := nested[0].(map[string]any)
	if !ok {
		t.Fatalf("expected copied nested refresh interval map, got %T", nested[0])
	}
	nestedMap["enabled"] = false

	originalRefreshInterval := config.Message.RefreshInterval.(map[string]any)
	originalNested := originalRefreshInterval["nested"].([]any)[0].(map[string]any)
	if originalNested["enabled"] != true {
		t.Fatalf("mutating the copy changed the original refresh interval: %#v", originalNested)
	}
}

func TestItemDeepCopyPreservesNilArrayObjectsEntry(t *testing.T) {
	item := &Item{
		ArrayObjects: map[string][]map[string]string{
			"quick": nil,
		},
	}

	copy := item.DeepCopy()
	values, ok := copy.ArrayObjects["quick"]
	if !ok {
		t.Fatal("expected nil ArrayObjects entry key to be preserved")
	}
	if values != nil {
		t.Fatalf("expected preserved ArrayObjects entry to remain nil, got %#v", values)
	}
}
