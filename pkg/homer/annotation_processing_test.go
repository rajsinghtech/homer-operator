/*
Copyright 2024 RajSingh.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package homer

import (
	"testing"
)

// TestBooleanValueParsing tests comprehensive boolean value parsing
// Note: "1" and "0" are intentionally NOT boolean - they are parsed as integers
// This is correct because Homer uses these values for fields like apiVersion, timeout, etc.
// JavaScript's truthiness will handle integer values correctly when accessing boolean fields.
func TestBooleanValueParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// True values (explicit boolean strings)
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"yes", true},
		{"YES", true},
		{"on", true},
		{"ON", true},
		{" true ", true}, // test trimming

		// False values (explicit boolean strings)
		{"false", false},
		{"FALSE", false},
		{"no", false},
		{"off", false},
		{"invalid", false}, // non-boolean strings are not parsed as bool
		{"", false},
		{" false ", false}, // test trimming
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Test direct function using smartInferType logic
			result := smartInferType(tt.input)
			boolResult, isBool := result.(bool)
			if !isBool {
				boolResult = false // Non-boolean values are treated as false
			}
			if boolResult != tt.expected {
				t.Errorf("boolean parsing(%s) = %v, expected %v", tt.input, boolResult, tt.expected)
			}

			// Test annotation processing integration
			item := Item{}
			annotations := map[string]string{
				"item.homer.rajsingh.info/usecredentials": tt.input,
			}

			processItemAnnotations(&item, annotations)

			// Check that the parameter was stored correctly
			storedValue, exists := item.Parameters["usecredentials"]
			if !exists {
				t.Errorf("Expected usecredentials parameter to exist for input '%s'", tt.input)
				return
			}

			// Test the actual stored value using smartInferType
			actualResult := smartInferType(storedValue)
			actualBool, isActualBool := actualResult.(bool)
			if !isActualBool {
				actualBool = false
			}

			if actualBool != tt.expected {
				t.Errorf("Expected UseCredentials %v for input '%s', got %v (param value: %s)",
					tt.expected, tt.input, actualBool, storedValue)
			}
		})
	}
}

// TestAnnotationValidation tests validation at different levels
func TestAnnotationValidation(t *testing.T) {
	testCases := []struct {
		name            string
		fieldName       string
		value           string
		validationLevel ValidationLevel
		expectError     bool
	}{
		// URL validation
		{"valid URL", "url", "https://example.com", ValidationLevelStrict, false},
		{"invalid URL strict", "url", "not-a-url", ValidationLevelStrict, true},
		{"invalid URL warn", "url", "not-a-url", ValidationLevelWarn, false},
		// Target validation
		{"valid target", "target", "_blank", ValidationLevelStrict, false},
		{"invalid target strict", "target", "_invalid", ValidationLevelStrict, true},
		// Numeric validation
		{"valid integer", "warning_value", "85", ValidationLevelStrict, false},
		{"valid float", "danger_value", "95.5", ValidationLevelStrict, false},
		{"invalid numeric strict", "warning_value", "not-a-number", ValidationLevelStrict, true},
		{"invalid numeric warn", "warning_value", "not-a-number", ValidationLevelWarn, false},
		{"empty numeric", "danger_value", "", ValidationLevelStrict, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAnnotationValue(tc.fieldName, tc.value, tc.validationLevel)
			if tc.expectError && err == nil {
				t.Errorf("Expected error for %s='%s', but got none", tc.fieldName, tc.value)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error for %s='%s', but got: %v", tc.fieldName, tc.value, err)
			}
		})
	}
}

// TestHeadersAnnotationProcessing tests various header annotation formats
func TestHeadersAnnotationProcessing(t *testing.T) {
	headerTests := []struct {
		name        string
		annotations map[string]string
		expected    map[string]string
	}{
		{
			name: "dot notation headers",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name":                  "Test Service",
				"item.homer.rajsingh.info/headers.authorization": "Bearer token123",
				"item.homer.rajsingh.info/headers.x-api-key":     "key456",
			},
			expected: map[string]string{
				"name":                  "Test Service",
				"headers.authorization": "Bearer token123",
				"headers.x-api-key":     "key456",
			},
		},
		{
			name: "comma-separated single header",
			annotations: map[string]string{
				"item.homer.rajsingh.info/headers": "Authorization: Bearer token123",
			},
			expected: map[string]string{
				"headers": "Authorization: Bearer token123",
			},
		},
		{
			name: "comma-separated multiple headers",
			annotations: map[string]string{
				"item.homer.rajsingh.info/headers": "Authorization: Bearer token123, X-API-Key: key456",
			},
			expected: map[string]string{
				"headers": "Authorization: Bearer token123, X-API-Key: key456",
			},
		},
	}

	for _, tc := range headerTests {
		t.Run(tc.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotations(&item, tc.annotations)

			for key, expected := range tc.expected {
				if item.Parameters[key] != expected {
					t.Errorf("Expected %s '%s', got '%s'", key, expected, item.Parameters[key])
				}
			}
		})
	}
}

// TestComprehensiveAnnotationProcessing tests end-to-end annotation processing
func TestComprehensiveAnnotationProcessing(t *testing.T) {
	comprehensiveTests := []struct {
		name              string
		annotations       map[string]string
		validationLevel   ValidationLevel
		expectedName      string
		expectedURL       string
		expectedTarget    string
		shouldHaveHeaders bool
	}{
		{"valid annotations", map[string]string{
			"item.homer.rajsingh.info/name":   "Test Service",
			"item.homer.rajsingh.info/url":    "https://example.com",
			"item.homer.rajsingh.info/target": "_blank",
		}, ValidationLevelStrict, "Test Service", "https://example.com", "_blank", false},
		{"invalid URL strict", map[string]string{
			"item.homer.rajsingh.info/name": "Test Service",
			"item.homer.rajsingh.info/url":  "not-a-valid-url",
		}, ValidationLevelStrict, "Test Service", "", "", false},
		{"invalid URL warn", map[string]string{
			"item.homer.rajsingh.info/name": "Test Service",
			"item.homer.rajsingh.info/url":  "not-a-valid-url",
		}, ValidationLevelWarn, "Test Service", "not-a-valid-url", "", false},
		{"invalid target strict", map[string]string{
			"item.homer.rajsingh.info/name":   "Test Service",
			"item.homer.rajsingh.info/target": "_invalid",
		}, ValidationLevelStrict, "Test Service", "", "", false},
		{"headers with dot notation", map[string]string{
			"item.homer.rajsingh.info/name":                  "Test Service",
			"item.homer.rajsingh.info/headers.authorization": "Bearer token123",
			"item.homer.rajsingh.info/headers.x-api-key":     "key456",
		}, ValidationLevelNone, "Test Service", "", "", true},
		{"keywords cleaning", map[string]string{
			"item.homer.rajsingh.info/name":     "Test Service",
			"item.homer.rajsingh.info/keywords": "  web  ,  api , service,  ",
		}, ValidationLevelNone, "Test Service", "", "", false},
	}

	for _, tc := range comprehensiveTests {
		t.Run(tc.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotationsWithValidation(&item, tc.annotations, tc.validationLevel)

			if item.Parameters["name"] != tc.expectedName {
				t.Errorf("Expected name '%s', got '%s'", tc.expectedName, item.Parameters["name"])
			}
			if item.Parameters["url"] != tc.expectedURL {
				t.Errorf("Expected URL '%s', got '%s'", tc.expectedURL, item.Parameters["url"])
			}
			if item.Parameters["target"] != tc.expectedTarget {
				t.Errorf("Expected target '%s', got '%s'", tc.expectedTarget, item.Parameters["target"])
			}

			if tc.shouldHaveHeaders {
				if item.Parameters["headers.authorization"] != "Bearer token123" {
					t.Errorf("Expected authorization header 'Bearer token123', got '%s'", item.Parameters["headers.authorization"])
				}
				if item.Parameters["headers.x-api-key"] != "key456" {
					t.Errorf("Expected x-api-key header 'key456', got '%s'", item.Parameters["headers.x-api-key"])
				}
			}

			if tc.name == "keywords cleaning" {
				if item.Parameters["keywords"] != "web,api,service" {
					t.Errorf("Expected keywords 'web,api,service', got '%s'", item.Parameters["keywords"])
				}
			}
		})
	}

	t.Run("full annotation processing integration", func(t *testing.T) {
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

		expectedParams := map[string]string{
			"name":                  "Complete Test Service",
			"subtitle":              "A comprehensive test",
			"url":                   "https://example.com/api",
			"target":                "_blank",
			"tag":                   "test",
			"tagstyle":              "is-primary",
			"keywords":              "api,test,service",
			"type":                  "Generic",
			"warning_value":         "80",
			"danger_value":          "90",
			"usecredentials":        "true",
			"headers.authorization": "Bearer test-token",
			"headers.content-type":  "application/json",
			"headers":               "X-Custom: custom-value, X-Test: test-value",
		}

		for key, expectedValue := range expectedParams {
			if actualValue := item.Parameters[key]; actualValue != expectedValue {
				t.Errorf("Expected parameter %s='%s', got '%s'", key, expectedValue, actualValue)
			}
		}

		if _, exists := item.Parameters["unrelated.annotation"]; exists {
			t.Error("Unrelated annotation should not be processed")
		}
	})
}

// TestNumericAnnotationProcessing tests numeric value processing with different validation levels
func TestNumericAnnotationProcessing(t *testing.T) {
	numericTests := []struct {
		name            string
		annotations     map[string]string
		validationLevel ValidationLevel
		expectedWarning string
		expectedDanger  string
	}{
		{"valid numeric values", map[string]string{
			"item.homer.rajsingh.info/warning_value": "85",
			"item.homer.rajsingh.info/danger_value":  "95.5",
		}, ValidationLevelStrict, "85", "95.5"},
		{"invalid numeric strict", map[string]string{
			"item.homer.rajsingh.info/warning_value": "not-a-number",
			"item.homer.rajsingh.info/danger_value":  "also-invalid",
		}, ValidationLevelStrict, "", ""},
		{"invalid numeric warn", map[string]string{
			"item.homer.rajsingh.info/warning_value": "not-a-number",
			"item.homer.rajsingh.info/danger_value":  "also-invalid",
		}, ValidationLevelWarn, "not-a-number", "also-invalid"},
	}

	for _, tc := range numericTests {
		t.Run(tc.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotationsWithValidation(&item, tc.annotations, tc.validationLevel)

			if item.Parameters["warning_value"] != tc.expectedWarning {
				t.Errorf("Expected warning value '%s', got '%s'", tc.expectedWarning, item.Parameters["warning_value"])
			}

			if item.Parameters["danger_value"] != tc.expectedDanger {
				t.Errorf("Expected danger value '%s', got '%s'", tc.expectedDanger, item.Parameters["danger_value"])
			}
		})
	}
}

func TestIsItemHidden(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		expected bool
	}{
		{
			name: "item with hide=true",
			item: Item{
				Parameters: map[string]string{
					"hide": "true",
				},
			},
			expected: true,
		},
		{
			name: "item with hide=false",
			item: Item{
				Parameters: map[string]string{
					"hide": "false",
				},
			},
			expected: false,
		},
		{
			name: "item with hide=1",
			item: Item{
				Parameters: map[string]string{
					"hide": "1",
				},
			},
			expected: true,
		},
		{
			name: "item with hide=0",
			item: Item{
				Parameters: map[string]string{
					"hide": "0",
				},
			},
			expected: false,
		},
		{
			name: "item with hide=yes",
			item: Item{
				Parameters: map[string]string{
					"hide": "yes",
				},
			},
			expected: true,
		},
		{
			name: "item with hide=no",
			item: Item{
				Parameters: map[string]string{
					"hide": "no",
				},
			},
			expected: false,
		},
		{
			name: "item with hide=non-empty string",
			item: Item{
				Parameters: map[string]string{
					"hide": "anything",
				},
			},
			expected: true,
		},
		{
			name: "item with hide=empty string",
			item: Item{
				Parameters: map[string]string{
					"hide": "",
				},
			},
			expected: false,
		},
		{
			name:     "item without hide parameter",
			item:     Item{},
			expected: false,
		},
		{
			name: "item with no parameters",
			item: Item{
				Parameters: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isItemHidden(&tt.item)
			if result != tt.expected {
				t.Errorf("isItemHidden() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestHideAnnotationIntegration(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name: "hide annotation with true",
			annotations: map[string]string{
				"item.homer.rajsingh.info/hide": "true",
			},
			expected: true,
		},
		{
			name: "hide annotation with false",
			annotations: map[string]string{
				"item.homer.rajsingh.info/hide": "false",
			},
			expected: false,
		},
		{
			name: "hide annotation with 1",
			annotations: map[string]string{
				"item.homer.rajsingh.info/hide": "1",
			},
			expected: true,
		},
		{
			name: "hide annotation case insensitive",
			annotations: map[string]string{
				"item.homer.rajsingh.info/hide": "TRUE",
			},
			expected: true,
		},
		{
			name:        "no hide annotation",
			annotations: map[string]string{},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotations(&item, tt.annotations)
			result := isItemHidden(&item)
			if result != tt.expected {
				t.Errorf("isItemHidden() after processItemAnnotations() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
