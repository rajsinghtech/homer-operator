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

			if item.Name != tt.expectedName {
				t.Errorf("Expected name '%s', got '%s'", tt.expectedName, item.Name)
			}

			if item.Url != tt.expectedURL {
				t.Errorf("Expected URL '%s', got '%s'", tt.expectedURL, item.Url)
			}

			if item.Target != tt.expectedTarget {
				t.Errorf("Expected target '%s', got '%s'", tt.expectedTarget, item.Target)
			}

			if tt.shouldHaveHeaders {
				if item.Headers == nil {
					t.Error("Expected headers to be set")
				} else {
					if len(item.Headers) == 0 {
						t.Error("Expected non-empty headers")
					}
					if item.Headers["authorization"] != "Bearer token123" {
						t.Errorf("Expected authorization header 'Bearer token123', got '%s'", item.Headers["authorization"])
					}
					if item.Headers["x-api-key"] != "key456" {
						t.Errorf("Expected x-api-key header 'key456', got '%s'", item.Headers["x-api-key"])
					}
				}
			}

			// Test keywords cleaning for the specific test case
			if tt.name == "Keywords cleaning" {
				expectedKeywords := "web,api,service"
				if item.Keywords != expectedKeywords {
					t.Errorf("Expected keywords '%s', got '%s'", expectedKeywords, item.Keywords)
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

			if item.Warningvalue != tt.expectedWarning {
				t.Errorf("Expected warning value '%s', got '%s'", tt.expectedWarning, item.Warningvalue)
			}

			if item.Dangervalue != tt.expectedDanger {
				t.Errorf("Expected danger value '%s', got '%s'", tt.expectedDanger, item.Dangervalue)
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

			if item.UseCredentials != tt.expected {
				t.Errorf("Expected UseCredentials %v for input '%s', got %v", tt.expected, tt.input, item.UseCredentials)
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

	// Verify all fields are set correctly
	if item.Name != "Complete Test Service" {
		t.Errorf("Expected name 'Complete Test Service', got '%s'", item.Name)
	}
	if item.Subtitle != "A comprehensive test" {
		t.Errorf("Expected subtitle 'A comprehensive test', got '%s'", item.Subtitle)
	}
	if item.Url != "https://example.com/api" {
		t.Errorf("Expected URL 'https://example.com/api', got '%s'", item.Url)
	}
	if item.Target != "_blank" {
		t.Errorf("Expected target '_blank', got '%s'", item.Target)
	}
	if item.Tag != "test" {
		t.Errorf("Expected tag 'test', got '%s'", item.Tag)
	}
	if item.Tagstyle != "is-primary" {
		t.Errorf("Expected tagstyle 'is-primary', got '%s'", item.Tagstyle)
	}
	if item.Keywords != "api,test,service" {
		t.Errorf("Expected keywords 'api,test,service', got '%s'", item.Keywords)
	}
	if item.Type != "Generic" {
		t.Errorf("Expected type 'Generic', got '%s'", item.Type)
	}
	if item.Warningvalue != "80" {
		t.Errorf("Expected warning value '80', got '%s'", item.Warningvalue)
	}
	if item.Dangervalue != "90" {
		t.Errorf("Expected danger value '90', got '%s'", item.Dangervalue)
	}
	if !item.UseCredentials {
		t.Error("Expected UseCredentials to be true")
	}

	// Verify headers are merged correctly
	if item.Headers == nil {
		t.Fatal("Expected headers to be set")
	}

	expectedHeaders := map[string]string{
		"authorization": "Bearer test-token",
		"content-type":  "application/json",
		"X-Custom":      "custom-value",
		"X-Test":        "test-value",
	}

	for key, expectedValue := range expectedHeaders {
		if actualValue := item.Headers[key]; actualValue != expectedValue {
			t.Errorf("Expected header %s='%s', got '%s'", key, expectedValue, actualValue)
		}
	}
}
