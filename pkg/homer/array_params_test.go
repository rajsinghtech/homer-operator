package homer

import (
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestSmartInferTypeJSONArray(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{"json int array", `[200,301,404]`, []interface{}{float64(200), float64(301), float64(404)}},
		{"json string array", `["vms","lxcs","disk"]`, []interface{}{"vms", "lxcs", "disk"}},
		{"json mixed array", `["load",42,true]`, []interface{}{"load", float64(42), true}},
		{"not an array", `[invalid`, "[invalid"},
		{"plain string", "hello", "hello"},
		{"integer still works", "42", 42},
		{"boolean still works", "true", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := smartInferType(tt.input)
			switch expected := tt.expected.(type) {
			case []interface{}:
				arr, ok := result.([]interface{})
				if !ok {
					t.Fatalf("expected array, got %T: %v", result, result)
				}
				if len(arr) != len(expected) {
					t.Fatalf("expected len %d, got %d", len(expected), len(arr))
				}
				for i, v := range expected {
					if arr[i] != v {
						t.Errorf("element [%d]: expected %v (%T), got %v (%T)", i, v, v, arr[i], arr[i])
					}
				}
			default:
				if result != tt.expected {
					t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
				}
			}
		})
	}
}

func TestSmartInferTypeForParamKnownArrays(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		expectArray bool
		expectLen   int
	}{
		{"successCodes comma", "successCodes", "200,301,404", true, 3},
		{"hide comma", "hide", "vms,lxcs,disk", true, 3},
		{"groups comma", "groups", "api,web", true, 2},
		{"environments comma", "environments", "Production,Staging", true, 2},
		{"stats comma spaces", "stats", "load, cpu, mem, swap", true, 4},
		{"items comma", "items", "name,version,entities", true, 3},
		{"subtitle stays string", "subtitle", "Hello, world", false, 0},
		{"name stays string", "name", "My, Service", false, 0},
		{"json array override", "hide", `["vms","lxcs"]`, true, 2},
		{"single value not array", "hide", "vms", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := smartInferTypeForParam(tt.key, tt.value)
			arr, isArray := result.([]interface{})
			if tt.expectArray && !isArray {
				t.Fatalf("expected array for key=%s value=%s, got %T: %v", tt.key, tt.value, result, result)
			}
			if !tt.expectArray && isArray {
				t.Fatalf("did not expect array for key=%s value=%s, got array: %v", tt.key, tt.value, arr)
			}
			if tt.expectArray && len(arr) != tt.expectLen {
				t.Errorf("expected len %d, got %d: %v", tt.expectLen, len(arr), arr)
			}
		})
	}
}

func TestSmartInferTypeForParamIntArray(t *testing.T) {
	result := smartInferTypeForParam("successCodes", "200,301,404")
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", result)
	}
	for _, v := range arr {
		if _, ok := v.(int); !ok {
			t.Errorf("expected int element, got %T: %v", v, v)
		}
	}
}

func TestArrayParamsInYAMLOutput(t *testing.T) {
	config := &HomerConfig{
		Title: "Test",
		Services: []Service{
			{
				Parameters: map[string]string{"name": "Proxmox"},
				Items: []Item{
					{
						Parameters: map[string]string{
							"name":         "pve",
							"url":          "https://pve.local",
							"type":         "Proxmox",
							"hide":         "vms,lxcs,disk",
							"successCodes": `[200,301]`,
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

	var parsed map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	services, ok := parsed["services"].([]interface{})
	if !ok || len(services) == 0 {
		t.Fatal("expected services in output")
	}

	svc := services[0].(map[interface{}]interface{})
	items := svc["items"].([]interface{})
	item := items[0].(map[interface{}]interface{})

	// hide should be an array
	hideVal, ok := item["hide"]
	if !ok {
		t.Fatal("expected hide field")
	}
	hideArr, ok := hideVal.([]interface{})
	if !ok {
		t.Fatalf("expected hide to be array, got %T: %v", hideVal, hideVal)
	}
	if len(hideArr) != 3 {
		t.Errorf("expected hide len 3, got %d", len(hideArr))
	}

	// successCodes should be an array
	scVal, ok := item["successCodes"]
	if !ok {
		t.Fatal("expected successCodes field")
	}
	scArr, ok := scVal.([]interface{})
	if !ok {
		t.Fatalf("expected successCodes to be array, got %T: %v", scVal, scVal)
	}
	if len(scArr) != 2 {
		t.Errorf("expected successCodes len 2, got %d", len(scArr))
	}
}
