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
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
		expected    map[string]any
	}{
		{
			name: "dot notation headers",
			annotations: map[string]string{
				"item.homer.rajsingh.info/name":                  "Test Service",
				"item.homer.rajsingh.info/headers.authorization": "Bearer token123",
				"item.homer.rajsingh.info/headers.x-api-key":     "key456",
			},
			expected: map[string]any{
				"authorization": "Bearer token123",
				"x-api-key":     "key456",
			},
		},
		{
			name: "comma-separated single header",
			annotations: map[string]string{
				"item.homer.rajsingh.info/headers": "Authorization: Bearer token123",
			},
			expected: map[string]any{
				"Authorization": "Bearer token123",
			},
		},
		{
			name: "comma-separated multiple headers",
			annotations: map[string]string{
				"item.homer.rajsingh.info/headers": "Authorization: Bearer token123, X-API-Key: key456",
			},
			expected: map[string]any{
				"Authorization": "Bearer token123",
				"X-API-Key":     "key456",
			},
		},
		{
			name: "legacy customHeaders slash notation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/customHeaders/Authorization": "Bearer legacy",
			},
			expected: map[string]any{
				"Authorization": "Bearer legacy",
			},
		},
		{
			name: "legacy customHeaders dot notation",
			annotations: map[string]string{
				"item.homer.rajsingh.info/customHeaders.Authorization": "Bearer legacy",
			},
			expected: map[string]any{
				"Authorization": "Bearer legacy",
			},
		},
	}

	for _, tc := range headerTests {
		t.Run(tc.name, func(t *testing.T) {
			item := Item{}
			processItemAnnotations(&item, tc.annotations)

			for key, expected := range tc.expected {
				if item.Headers[key] != expected {
					t.Errorf("Expected header %s %q, got %q", key, expected, item.Headers[key])
				}
			}
			for key := range item.Parameters {
				lowerKey := strings.ToLower(key)
				if lowerKey == headersObjectName || strings.HasPrefix(lowerKey, headersObjectName+".") ||
					strings.HasPrefix(lowerKey, legacyHeadersObjectName) {
					t.Fatalf("headers should not be stored as legacy parameters: %#v", item.Parameters)
				}
			}
		})
	}
}

func TestHeadersRenderAsUpstreamObject(t *testing.T) {
	item := Item{
		Headers: map[string]any{"X-Direct": "direct"},
		Parameters: map[string]string{
			"headers":                       "X-Legacy: legacy",
			"headers.X-Parameter":           "parameter",
			"customHeaders":                 "X-Custom-Parameter: custom-parameter",
			"customHeaders.X-Custom-Dotted": "custom-dotted",
			"name":                          "Service",
		},
		NestedObjects: map[string]map[string]string{
			"customHeaders": {"X-Custom": "custom"},
		},
	}

	got := flattenItemsForYAML([]Item{item})[0]
	headers, ok := got["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers output = %#v, want object", got["headers"])
	}
	for name, want := range map[string]string{
		"X-Direct":           "direct",
		"X-Legacy":           "legacy",
		"X-Parameter":        "parameter",
		"X-Custom":           "custom",
		"X-Custom-Parameter": "custom-parameter",
		"X-Custom-Dotted":    "custom-dotted",
	} {
		if headers[name] != want {
			t.Errorf("headers[%q] = %#v, want %q", name, headers[name], want)
		}
	}
	if _, exists := got["customHeaders"]; exists {
		t.Error("legacy customHeaders must not be emitted")
	}
}

func TestHeaderAnnotationFormsHaveDeterministicPrecedence(t *testing.T) {
	annotations := map[string]string{
		"item.homer.rajsingh.info/customHeaders.Authorization": "legacy-dot",
		"item.homer.rajsingh.info/customHeaders/authorization": "legacy-slash",
		"item.homer.rajsingh.info/headers":                     "Authorization: canonical-object",
		"item.homer.rajsingh.info/headers.authorization":       "canonical-dot",
		"item.homer.rajsingh.info/headers/Authorization":       "canonical-slash",
	}

	for iteration := 0; iteration < 100; iteration++ {
		item := Item{}
		processItemAnnotations(&item, annotations)

		got, ok := getHeaderValue(item.Headers, "AUTHORIZATION")
		if !ok || got != "canonical-slash" {
			t.Fatalf("iteration %d authorization = %#v, want canonical-slash", iteration, got)
		}
		if len(item.Headers) != 1 {
			t.Fatalf("iteration %d produced case-variant header duplicates: %#v", iteration, item.Headers)
		}
	}
}

func TestLegacyParameterHeaderFormsUseCanonicalPhasePrecedence(t *testing.T) {
	item := Item{Parameters: map[string]string{
		"customHeaders":               "Authorization: legacy-object",
		"customHeaders.Authorization": "legacy-dotted",
		"headers.authorization":       "canonical-dotted",
	}}

	got, ok := getHeaderValue(flattenItemHeaders(item), "AUTHORIZATION")
	if !ok || got != "canonical-dotted" {
		t.Fatalf("flattened authorization = %#v, want canonical-dotted", got)
	}
}

func TestSecretHeaderResolutionAllowsGenericItems(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "default"},
		Data:       map[string][]byte{"header": []byte("secret-value")},
	}).Build()

	item := &Item{}
	if err := ResolveHeaderFromSecret(context.Background(), k8sClient, item, "X-Secret", &SecretKeyRef{
		Name: "credentials", Key: "header",
	}, "default"); err != nil {
		t.Fatalf("resolve generic header secret: %v", err)
	}
	if got := item.Headers["X-Secret"]; got != "secret-value" {
		t.Fatalf("generic item header = %#v, want secret-value", got)
	}
}

func TestNestedObjectDotNotation(t *testing.T) {
	item := Item{}
	processItemAnnotations(&item, map[string]string{
		"item.homer.rajsingh.info/mapping.status":  "health.status",
		"item.homer.rajsingh.info/mapping.version": "info.version",
	})

	want := map[string]string{"status": "health.status", "version": "info.version"}
	if got := item.NestedObjects["mapping"]; len(got) != len(want) {
		t.Fatalf("mapping = %#v, want %#v", got, want)
	}
	for key, value := range want {
		if got := item.NestedObjects["mapping"][key]; got != value {
			t.Errorf("mapping[%q] = %q, want %q", key, got, value)
		}
	}
}

func TestSecretHeadersRenderAsUpstreamObject(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "default"},
		Data: map[string][]byte{
			"header": []byte("secret-value"),
			"token":  []byte("secret-token"),
		},
	}).Build()
	item := &Item{Type: "Grafana"}

	if err := ResolveHeaderFromSecret(context.Background(), k8sClient, item, "X-Secret", &SecretKeyRef{Name: "credentials", Key: "header"}, "default"); err != nil {
		t.Fatalf("resolve header secret: %v", err)
	}
	if err := ResolveTokenFromSecret(context.Background(), k8sClient, item, &SecretKeyRef{Name: "credentials", Key: "token"}, "default"); err != nil {
		t.Fatalf("resolve token secret: %v", err)
	}

	want := map[string]any{"X-Secret": "secret-value", "Authorization": "Bearer secret-token"}
	if len(item.Headers) != len(want) {
		t.Fatalf("resolved headers = %#v, want %#v", item.Headers, want)
	}
	for name, value := range want {
		if item.Headers[name] != value {
			t.Errorf("resolved header %q = %#v, want %#v", name, item.Headers[name], value)
		}
	}
	if len(item.NestedObjects) != 0 {
		t.Fatalf("secret headers unexpectedly used nested objects: %#v", item.NestedObjects)
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
				if item.Headers["authorization"] != "Bearer token123" {
					t.Errorf("Expected authorization header 'Bearer token123', got '%s'", item.Headers["authorization"])
				}
				if item.Headers["x-api-key"] != "key456" {
					t.Errorf("Expected x-api-key header 'key456', got '%s'", item.Headers["x-api-key"])
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
			"name":           "Complete Test Service",
			"subtitle":       "A comprehensive test",
			"url":            "https://example.com/api",
			"target":         "_blank",
			"tag":            "test",
			"tagstyle":       "is-primary",
			"keywords":       "api,test,service",
			"type":           "Generic",
			"warning_value":  "80",
			"danger_value":   "90",
			"usecredentials": "true",
		}

		for key, expectedValue := range expectedParams {
			if actualValue := item.Parameters[key]; actualValue != expectedValue {
				t.Errorf("Expected parameter %s='%s', got '%s'", key, expectedValue, actualValue)
			}
		}

		if _, exists := item.Parameters["unrelated.annotation"]; exists {
			t.Error("Unrelated annotation should not be processed")
		}
		for key, expected := range map[string]string{
			"authorization": "Bearer test-token",
			"content-type":  "application/json",
			"X-Custom":      "custom-value",
			"X-Test":        "test-value",
		} {
			if actual := item.Headers[key]; actual != expected {
				t.Errorf("Expected header %s=%q, got %q", key, expected, actual)
			}
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
