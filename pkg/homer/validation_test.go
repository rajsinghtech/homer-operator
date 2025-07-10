package homer

import (
	"testing"
)

// TestHomerConfigValidation tests overall configuration validation
func TestHomerConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      HomerConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: HomerConfig{
				Title:    "Test Dashboard",
				Subtitle: "Test Subtitle",
			},
			expectError: false,
		},
		{
			name:        "empty title",
			config:      HomerConfig{},
			expectError: true,
			errorMsg:    "title: required",
		},
		{
			name: "valid config with services",
			config: HomerConfig{
				Title: "Test Dashboard",
				Services: []Service{
					{
						Parameters: map[string]string{
							"name": "Test Service",
						},
						Items: []Item{
							{
								Parameters: map[string]string{
									"name": "Test Item",
									"url":  "https://example.com",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHomerConfig(&tt.config)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for config, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for config, but got: %v", err)
			}
			if tt.expectError && err != nil && tt.errorMsg != "" {
				if err.Error() != tt.errorMsg {
					t.Errorf("Expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
			}
		})
	}
}

// TestThemeValidation tests theme validation
func TestThemeValidation(t *testing.T) {
	tests := []struct {
		name        string
		theme       string
		expectError bool
	}{
		{
			name:        "valid default theme",
			theme:       "default",
			expectError: false,
		},
		{
			name:        "valid neon theme",
			theme:       "neon",
			expectError: false,
		},
		{
			name:        "empty theme",
			theme:       "",
			expectError: false, // Empty theme defaults to "default"
		},
		{
			name:        "invalid theme",
			theme:       "invalid-theme",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTheme(tt.theme)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for theme '%s', but got none", tt.theme)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for theme '%s', but got: %v", tt.theme, err)
			}
		})
	}
}
