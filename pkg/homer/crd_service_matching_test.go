package homer

import (
	"testing"
)

func TestScoreCRDServiceGroupMatch(t *testing.T) {
	tests := []struct {
		name                  string
		crdServiceName        string
		discoveredNamespace   string
		discoveredAnnotations map[string]string
		expectedScore         int
		description           string
	}{
		{
			name:                  "exact namespace match",
			crdServiceName:        "home",
			discoveredNamespace:   "home",
			discoveredAnnotations: map[string]string{},
			expectedScore:         100,
			description:           "Direct namespace match should score 100",
		},
		{
			name:                  "partial namespace match",
			crdServiceName:        "kube-system",
			discoveredNamespace:   "system",
			discoveredAnnotations: map[string]string{},
			expectedScore:         50,
			description:           "Partial namespace match should score 50",
		},
		{
			name:                  "no namespace match",
			crdServiceName:        "infrastructure",
			discoveredNamespace:   "speedtest",
			discoveredAnnotations: map[string]string{},
			expectedScore:         0,
			description:           "No namespace match should score 0 (below threshold)",
		},
		{
			name:                "explicit service annotation match",
			crdServiceName:      "Production Services",
			discoveredNamespace: "random-namespace",
			discoveredAnnotations: map[string]string{
				"service.homer.rajsingh.info/name": "Production Services",
			},
			expectedScore: 200,
			description:   "Explicit service annotation should score 200",
		},
		{
			name:                "explicit service annotation no match",
			crdServiceName:      "Development Services",
			discoveredNamespace: "dev",
			discoveredAnnotations: map[string]string{
				"service.homer.rajsingh.info/name": "Production Services",
			},
			expectedScore: 0,
			description:   "Wrong service annotation should score 0, ignore namespace",
		},
		{
			name:                "case insensitive service annotation match",
			crdServiceName:      "Media Services",
			discoveredNamespace: "media",
			discoveredAnnotations: map[string]string{
				"service.homer.rajsingh.info/name": "MEDIA SERVICES",
			},
			expectedScore: 200,
			description:   "Case insensitive service annotation should work",
		},
		{
			name:                  "case insensitive namespace match",
			crdServiceName:        "HOME",
			discoveredNamespace:   "home",
			discoveredAnnotations: map[string]string{},
			expectedScore:         100,
			description:           "Case insensitive namespace match should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreCRDServiceGroupMatch(
				tt.crdServiceName,
				tt.discoveredNamespace,
				tt.discoveredAnnotations,
			)

			if score != tt.expectedScore {
				t.Errorf("%s: expected score %d, got %d", tt.description, tt.expectedScore, score)
			}
		})
	}
}

func TestFindBestMatchingCRDServiceGroupWithThreshold(t *testing.T) {
	// Create a test config with existing CRD services
	config := &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "Infrastructure",
				},
				Items: []Item{
					{
						Source: CRDSource,
						Parameters: map[string]string{
							"name": "Existing CRD Item",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "home",
				},
				Items: []Item{
					{
						Source: CRDSource,
						Parameters: map[string]string{
							"name": "Home Assistant",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		namespace     string
		annotations   map[string]string
		expectedMatch string
		description   string
	}{
		{
			name:          "exact namespace match above threshold",
			namespace:     "home",
			annotations:   map[string]string{},
			expectedMatch: "home",
			description:   "Should match home service with score 100",
		},
		{
			name:          "weak match below threshold",
			namespace:     "speedtest",
			annotations:   map[string]string{},
			expectedMatch: "",
			description:   "Should not match any service (all scores below 30 threshold)",
		},
		{
			name:      "explicit annotation above threshold",
			namespace: "random",
			annotations: map[string]string{
				"service.homer.rajsingh.info/name": "Infrastructure",
			},
			expectedMatch: "Infrastructure",
			description:   "Should match Infrastructure service via annotation with score 200",
		},
		{
			name:          "partial match above threshold",
			namespace:     "infra",
			annotations:   map[string]string{},
			expectedMatch: "Infrastructure",
			description:   "Should match Infrastructure service with partial match score 50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := findBestMatchingCRDServiceGroup(
				config,
				tt.namespace,
				tt.annotations,
			)

			if match != tt.expectedMatch {
				t.Errorf("%s: expected match '%s', got '%s'", tt.description, tt.expectedMatch, match)
			}
		})
	}
}

func TestMinimumScoreThresholdPreventsWeakMatches(t *testing.T) {
	// This test specifically addresses the bug where speedtest was incorrectly
	// matched to Infrastructure service group

	config := &HomerConfig{
		Services: []Service{
			{
				Parameters: map[string]string{
					"name": "Infrastructure",
				},
				Items: []Item{
					{
						Source: CRDSource,
						Parameters: map[string]string{
							"name": "Prometheus",
						},
					},
				},
			},
			{
				Parameters: map[string]string{
					"name": "Network Tools",
				},
				Items: []Item{
					{
						Source: CRDSource,
						Parameters: map[string]string{
							"name": "Network Scanner",
						},
					},
				},
			},
		},
	}

	// Test the original bug scenario: speedtest with no annotations
	match := findBestMatchingCRDServiceGroup(
		config,
		"speedtest",         // namespace that doesn't match any existing service names
		map[string]string{}, // no annotations
	)

	// Should return empty string because no match scores above threshold (30)
	if match != "" {
		t.Errorf("Expected no match for speedtest namespace, got match: '%s'", match)
	}

	// Verify that with proper annotation, it would work
	matchWithAnnotation := findBestMatchingCRDServiceGroup(
		config,
		"speedtest",
		map[string]string{
			"service.homer.rajsingh.info/name": "Network Tools",
		},
	)

	if matchWithAnnotation != "Network Tools" {
		t.Errorf("Expected 'Network Tools' match with annotation, got: '%s'", matchWithAnnotation)
	}
}
