package homer

import (
	"encoding/json"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestFooterFalseHandling(t *testing.T) {
	t.Run("YAML unmarshal footer: false", func(t *testing.T) {
		yamlData := `
title: "Test Dashboard"
footer: false
`
		var config HomerConfig
		err := yaml.Unmarshal([]byte(yamlData), &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal YAML: %v", err)
		}

		if config.Footer != FooterHidden {
			t.Errorf("Expected Footer to be %q, got %q", FooterHidden, config.Footer)
		}
	})

	t.Run("YAML unmarshal footer: string", func(t *testing.T) {
		yamlData := `
title: "Test Dashboard"
footer: "<p>Custom Footer</p>"
`
		var config HomerConfig
		err := yaml.Unmarshal([]byte(yamlData), &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal YAML: %v", err)
		}

		expected := "<p>Custom Footer</p>"
		if config.Footer != expected {
			t.Errorf("Expected Footer to be %q, got %q", expected, config.Footer)
		}
	})

	t.Run("JSON unmarshal footer: false", func(t *testing.T) {
		jsonData := `{
			"title": "Test Dashboard",
			"footer": false
		}`
		var config HomerConfig
		err := json.Unmarshal([]byte(jsonData), &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		if config.Footer != FooterHidden {
			t.Errorf("Expected Footer to be %q, got %q", FooterHidden, config.Footer)
		}
	})

	t.Run("JSON unmarshal footer: string", func(t *testing.T) {
		jsonData := `{
			"title": "Test Dashboard",
			"footer": "<p>Custom Footer</p>"
		}`
		var config HomerConfig
		err := json.Unmarshal([]byte(jsonData), &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		expected := "<p>Custom Footer</p>"
		if config.Footer != expected {
			t.Errorf("Expected Footer to be %q, got %q", expected, config.Footer)
		}
	})

	t.Run("YAML marshal footer: false", func(t *testing.T) {
		config := &HomerConfig{
			Title:  "Test Dashboard",
			Footer: FooterHidden,
		}

		yamlBytes, err := marshalHomerConfigToYAML(config)
		if err != nil {
			t.Fatalf("Failed to generate Homer YAML: %v", err)
		}

		yamlStr := string(yamlBytes)
		if !strings.Contains(yamlStr, "footer: false") {
			t.Errorf("Expected YAML to contain 'footer: false', got:\n%s", yamlStr)
		}
	})

	t.Run("YAML marshal footer: string", func(t *testing.T) {
		config := &HomerConfig{
			Title:  "Test Dashboard",
			Footer: "<p>Custom Footer</p>",
		}

		yamlBytes, err := marshalHomerConfigToYAML(config)
		if err != nil {
			t.Fatalf("Failed to generate Homer YAML: %v", err)
		}

		yamlStr := string(yamlBytes)
		if !strings.Contains(yamlStr, "footer: <p>Custom Footer</p>") {
			t.Errorf("Expected YAML to contain custom footer string, got:\n%s", yamlStr)
		}
	})

	t.Run("YAML marshal footer: empty (omitted)", func(t *testing.T) {
		config := &HomerConfig{
			Title:  "Test Dashboard",
			Footer: "",
		}

		yamlBytes, err := marshalHomerConfigToYAML(config)
		if err != nil {
			t.Fatalf("Failed to generate Homer YAML: %v", err)
		}

		yamlStr := string(yamlBytes)
		if strings.Contains(yamlStr, "footer:") {
			t.Errorf("Expected YAML to not contain footer field when empty, got:\n%s", yamlStr)
		}
	})
}
