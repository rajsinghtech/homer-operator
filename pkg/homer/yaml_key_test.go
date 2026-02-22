package homer

import "testing"

func TestGetYAMLKeyMappings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"legacyapi", "legacyApi"},
		{"librarytype", "libraryType"},
		{"usecredentials", "useCredentials"},
		{"apiversion", "apiVersion"},
		{"checkinterval", "checkInterval"},
		{"updateinterval", "updateInterval"},
		{"refreshinterval", "refreshInterval"},
		{"successcodes", "successCodes"},
		{"rateinterval", "rateInterval"},
		{"torrentinterval", "torrentInterval"},
		{"downloadinterval", "downloadInterval"},
		{"hideaverages", "hideaverages"},
		{"locationid", "locationId"},
		{"api_token", "api_token"},
		{"warning_value", "warning_value"},
		{"danger_value", "danger_value"},
		{"hide_decimals", "hide_decimals"},
		{"small_font_on_small_screens", "small_font_on_small_screens"},
		{"small_font_on_desktop", "small_font_on_desktop"},
		{"documenttitle", "documentTitle"},
		{"colortheme", "colorTheme"},
		{"connectivitycheck", "connectivityCheck"},
		{"externalconfig", "externalConfig"},
		{"tagstyle", "tagstyle"},
		// Already correct casing passes through
		{"apikey", "apikey"},
		{"name", "name"},
		{"url", "url"},
		{"subtitle", "subtitle"},
		// Preserve user-provided casing for unknown keys
		{"customField", "customField"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := getYAMLKey(tt.input)
			if result != tt.expected {
				t.Errorf("getYAMLKey(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
