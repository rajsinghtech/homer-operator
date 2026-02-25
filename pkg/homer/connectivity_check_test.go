package homer

import (
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestConnectivityCheckFalseInYAML(t *testing.T) {
	config := &HomerConfig{
		Title:             "Test",
		ConnectivityCheck: boolPtr(false),
	}

	yamlBytes, err := marshalHomerConfigToYAML(config)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "connectivityCheck: false") {
		t.Errorf("expected 'connectivityCheck: false' in YAML, got:\n%s", yamlStr)
	}
}

func TestConnectivityCheckTrueInYAML(t *testing.T) {
	config := &HomerConfig{
		Title:             "Test",
		ConnectivityCheck: boolPtr(true),
	}

	yamlBytes, err := marshalHomerConfigToYAML(config)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "connectivityCheck: true") {
		t.Errorf("expected 'connectivityCheck: true' in YAML, got:\n%s", yamlStr)
	}
}

func TestConnectivityCheckNilOmitted(t *testing.T) {
	config := &HomerConfig{
		Title: "Test",
		// ConnectivityCheck is nil - should not appear in YAML
	}

	yamlBytes, err := marshalHomerConfigToYAML(config)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	yamlStr := string(yamlBytes)
	if strings.Contains(yamlStr, "connectivityCheck") {
		t.Errorf("expected connectivityCheck to be omitted when nil, got:\n%s", yamlStr)
	}
}

func TestConnectivityCheckJSONRoundTrip(t *testing.T) {
	config := HomerConfig{
		Title:             "Test",
		ConnectivityCheck: boolPtr(false),
	}

	yamlBytes, err := marshalHomerConfigToYAML(&config)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(yamlBytes, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	val, ok := parsed["connectivityCheck"]
	if !ok {
		t.Fatal("expected connectivityCheck in parsed YAML")
	}
	if val != false {
		t.Errorf("expected connectivityCheck=false, got %v", val)
	}
}
