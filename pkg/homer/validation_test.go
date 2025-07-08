package homer

import (
	"testing"
)

func TestEnhancedAnnotationValidation(t *testing.T) {
	tests := []struct {
		name              string
		annotations       map[string]string
		validationLevel   ValidationLevel
		expectedName      string
		expectedURL       string
		expectedTarget    string
		shouldHaveHeaders bool
	}{
		{
			name: "Valid annotations",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name":   "Test Service",
				"item.homer.rajsingh.info/url":    "https://example.com",
				"item.homer.rajsingh.info/target": "_blank",
			},
			validationLevel: ValidationLevelStrict,
			expectedName:    "Test Service",
			expectedURL:     "https://example.com",
			expectedTarget:  "_blank",
		},
		{
			name: "Invalid URL with strict validation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name": "Test Service",
				"item.homer.rajsingh.info/url":  "not-a-valid-url",
			},
			validationLevel: ValidationLevelStrict,
			expectedName:    "Test Service",
			expectedURL:     "", // Should be empty due to validation failure
		},
		{
			name: "Invalid URL with warn validation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name": "Test Service",
				"item.homer.rajsingh.info/url":  "not-a-valid-url",
			},
			validationLevel: ValidationLevelWarn,
			expectedName:    "Test Service",
			expectedURL:     "not-a-valid-url", // Should be set despite warning
		},
		{
			name: "Invalid target with strict validation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name":   "Test Service",
				"item.homer.rajsingh.info/target": "_invalid",
			},
			validationLevel: ValidationLevelStrict,
			expectedName:    "Test Service",
			expectedTarget:  "", // Should be empty due to validation failure
		},
		{
			name: "Headers with dot notation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name":                  "Test Service",
				"item.homer.rajsingh.info/headers.authorization": "Bearer token123",
				"item.homer.rajsingh.info/headers.x-api-key":     "key456",
			},
			validationLevel:   ValidationLevelNone,
			expectedName:      "Test Service",
			shouldHaveHeaders: true,
		},
		{
			name: "Keywords cleaning",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name":     "Test Service",
				"item.homer.rajsingh.info/keywords": "  web  ,  api , service,  ",
			},
			validationLevel: ValidationLevelNone,
			expectedName:    "Test Service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotationsWithValidation(&item, tt.annotations, tt.validationLevel)

			// Check parameters since we're using dynamic system now
			if item.Parameters["name"] != tt.expectedName {
				t.Errorf("Expected name '%s', got '%s'", tt.expectedName, item.Parameters["name"])
			}

			if item.Parameters["url"] != tt.expectedURL {
				t.Errorf("Expected URL '%s', got '%s'", tt.expectedURL, item.Parameters["url"])
			}

			if item.Parameters["target"] != tt.expectedTarget {
				t.Errorf("Expected target '%s', got '%s'", tt.expectedTarget, item.Parameters["target"])
			}

			if tt.shouldHaveHeaders {
				// For the new dynamic system, headers. prefix is stored in Parameters
				if item.Parameters["headers.authorization"] != "Bearer token123" {
					t.Errorf("Expected authorization header 'Bearer token123', got '%s'", item.Parameters["headers.authorization"])
				}
				if item.Parameters["headers.x-api-key"] != "key456" {
					t.Errorf("Expected x-api-key header 'key456', got '%s'", item.Parameters["headers.x-api-key"])
				}
			}

			// Test keywords cleaning for the specific test case
			if tt.name == "Keywords cleaning" {
				expectedKeywords := "web,api,service"
				if item.Parameters["keywords"] != expectedKeywords {
					t.Errorf("Expected keywords '%s', got '%s'", expectedKeywords, item.Parameters["keywords"])
				}
			}
		})
	}
}

func TestNumericValidation(t *testing.T) {
	tests := []struct {
		name            string
		fieldName       string
		value           string
		validationLevel ValidationLevel
		expectError     bool
	}{
		{
			name:            "Valid integer",
			fieldName:       "warning_value",
			value:           "85",
			validationLevel: ValidationLevelStrict,
			expectError:     false,
		},
		{
			name:            "Valid float",
			fieldName:       "danger_value",
			value:           "95.5",
			validationLevel: ValidationLevelStrict,
			expectError:     false,
		},
		{
			name:            "Invalid numeric with strict validation",
			fieldName:       "warning_value",
			value:           "not-a-number",
			validationLevel: ValidationLevelStrict,
			expectError:     true,
		},
		{
			name:            "Invalid numeric with warn validation",
			fieldName:       "warning_value",
			value:           "not-a-number",
			validationLevel: ValidationLevelWarn,
			expectError:     false,
		},
		{
			name:            "Empty value",
			fieldName:       "danger_value",
			value:           "",
			validationLevel: ValidationLevelStrict,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnnotationValue(tt.fieldName, tt.value, tt.validationLevel)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for %s='%s', but got none", tt.fieldName, tt.value)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for %s='%s', but got: %v", tt.fieldName, tt.value, err)
			}
		})
	}
}

func TestNumericAnnotationProcessing(t *testing.T) {
	tests := []struct {
		name            string
		annotations     map[string]string
		validationLevel ValidationLevel
		expectedWarning string
		expectedDanger  string
	}{
		{
			name: "Valid numeric values",
			annotations: map[string]string{
				"item.homer.rajsingh.info/warning_value": "85",
				"item.homer.rajsingh.info/danger_value":  "95.5",
			},
			validationLevel: ValidationLevelStrict,
			expectedWarning: "85",
			expectedDanger:  "95.5",
		},
		{
			name: "Invalid numeric with strict validation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/warning_value": "not-a-number",
				"item.homer.rajsingh.info/danger_value":  "also-invalid",
			},
			validationLevel: ValidationLevelStrict,
			expectedWarning: "", // Should be empty due to validation failure
			expectedDanger:  "", // Should be empty due to validation failure
		},
		{
			name: "Invalid numeric with warn validation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/warning_value": "not-a-number",
				"item.homer.rajsingh.info/danger_value":  "also-invalid",
			},
			validationLevel: ValidationLevelWarn,
			expectedWarning: "not-a-number", // Should be set despite warning
			expectedDanger:  "also-invalid", // Should be set despite warning
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotationsWithValidation(&item, tt.annotations, tt.validationLevel)

			if item.Parameters["warning_value"] != tt.expectedWarning {
				t.Errorf("Expected warning value '%s', got '%s'", tt.expectedWarning, item.Parameters["warning_value"])
			}

			if item.Parameters["danger_value"] != tt.expectedDanger {
				t.Errorf("Expected danger value '%s', got '%s'", tt.expectedDanger, item.Parameters["danger_value"])
			}
		})
	}
}

func TestBooleanEnhancements(t *testing.T) {
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
		{" true ", true}, // test trimming
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"off", false},
		{"invalid", false},
		{"", false},
		{" false ", false}, // test trimming
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			item := Item{}
			annotations := map[string]string{
				"item.homer.rajsingh.info/usecredentials": tt.input,
			}

			processItemAnnotations(&item, annotations)

			// In the dynamic system, booleans are stored as strings and converted during YAML marshaling
			// Use parseBooleanValue to handle case-insensitive boolean parsing
			actualBool := parseBooleanValue(item.Parameters["usecredentials"])

			if actualBool != tt.expected {
				t.Errorf("Expected UseCredentials %v for input '%s', got %v (param value: %s)",
					tt.expected, tt.input, actualBool, item.Parameters["usecredentials"])
			}
		})
	}
}

func TestCompleteAnnotationProcessing(t *testing.T) {
	annotations := map[string]string{
		"item.homer.rajsingh.info/name":                  "Complete Test Service",
		"item.homer.rajsingh.info/subtitle":              "A comprehensive test",
		"item.homer.rajsingh.info/url":                   "https://example.com/api",
		"item.homer.rajsingh.info/target":                "_blank",
		"item.homer.rajsingh.info/tag":                   "test",
		"item.homer.rajsingh.info/tagstyle":              "is-primary",
		"item.homer.rajsingh.info/keywords":              "api, test, service",
		"item.homer.rajsingh.info/type":                  "Generic",
		"item.homer.rajsingh.info/warning_value":         "80",
		"item.homer.rajsingh.info/danger_value":          "90",
		"item.homer.rajsingh.info/usecredentials":        "true",
		"item.homer.rajsingh.info/headers.authorization": "Bearer test-token",
		"item.homer.rajsingh.info/headers.content-type":  "application/json",
		"item.homer.rajsingh.info/headers":               "X-Custom: custom-value, X-Test: test-value",
		"unrelated.annotation":                           "should-be-ignored",
	}

	item := Item{}
	processItemAnnotationsWithValidation(&item, annotations, ValidationLevelWarn)

	// Verify all fields are set correctly in Parameters map
	if item.Parameters["name"] != "Complete Test Service" {
		t.Errorf("Expected name 'Complete Test Service', got '%s'", item.Parameters["name"])
	}
	if item.Parameters["subtitle"] != "A comprehensive test" {
		t.Errorf("Expected subtitle 'A comprehensive test', got '%s'", item.Parameters["subtitle"])
	}
	if item.Parameters["url"] != "https://example.com/api" {
		t.Errorf("Expected URL 'https://example.com/api', got '%s'", item.Parameters["url"])
	}
	if item.Parameters["target"] != "_blank" {
		t.Errorf("Expected target '_blank', got '%s'", item.Parameters["target"])
	}
	if item.Parameters["tag"] != "test" {
		t.Errorf("Expected tag 'test', got '%s'", item.Parameters["tag"])
	}
	if item.Parameters["tagstyle"] != "is-primary" {
		t.Errorf("Expected tagstyle 'is-primary', got '%s'", item.Parameters["tagstyle"])
	}
	if item.Parameters["keywords"] != "api,test,service" {
		t.Errorf("Expected keywords 'api,test,service', got '%s'", item.Parameters["keywords"])
	}
	if item.Parameters["type"] != "Generic" {
		t.Errorf("Expected type 'Generic', got '%s'", item.Parameters["type"])
	}
	if item.Parameters["warning_value"] != "80" {
		t.Errorf("Expected warning value '80', got '%s'", item.Parameters["warning_value"])
	}
	if item.Parameters["danger_value"] != "90" {
		t.Errorf("Expected danger value '90', got '%s'", item.Parameters["danger_value"])
	}
	if item.Parameters["usecredentials"] != "true" {
		t.Error("Expected usecredentials to be 'true'")
	}

	// Verify headers are stored in parameters correctly
	expectedHeaders := map[string]string{
		"headers.authorization": "Bearer test-token",
		"headers.content-type":  "application/json",
		"headers":               "X-Custom: custom-value, X-Test: test-value",
	}

	for key, expectedValue := range expectedHeaders {
		if actualValue := item.Parameters[key]; actualValue != expectedValue {
			t.Errorf("Expected parameter %s='%s', got '%s'", key, expectedValue, actualValue)
		}
	}
}
