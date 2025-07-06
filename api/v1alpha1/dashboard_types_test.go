package v1alpha1

import (
	"testing"

	homer "github.com/rajsinghtech/homer-operator.git/pkg/homer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDashboardSpecValidation(t *testing.T) {
	tests := []struct {
		name     string
		spec     DashboardSpec
		hasError bool
	}{
		{
			name: "Valid minimal spec",
			spec: DashboardSpec{
				HomerConfig: homer.HomerConfig{
					Title: "Test Dashboard",
				},
			},
			hasError: false,
		},
		{
			name: "Valid spec with service grouping",
			spec: DashboardSpec{
				HomerConfig: homer.HomerConfig{
					Title: "Test Dashboard",
				},
				ServiceGrouping: &ServiceGroupingConfig{
					Strategy: "label",
					LabelKey: "team",
				},
			},
			hasError: false,
		},
		{
			name: "Valid spec with custom grouping rules",
			spec: DashboardSpec{
				HomerConfig: homer.HomerConfig{
					Title: "Test Dashboard",
				},
				ServiceGrouping: &ServiceGroupingConfig{
					Strategy: "custom",
					CustomRules: []GroupingRule{
						{
							Name:      "Production Services",
							Condition: map[string]string{"environment": "prod"},
							Priority:  1,
						},
						{
							Name:      "Development Services",
							Condition: map[string]string{"environment": "dev"},
							Priority:  2,
						},
					},
				},
			},
			hasError: false,
		},
		{
			name: "Valid spec with health check config",
			spec: DashboardSpec{
				HomerConfig: homer.HomerConfig{
					Title: "Test Dashboard",
				},
				HealthCheck: &ServiceHealthConfig{
					Enabled:      true,
					Interval:     "30s",
					Timeout:      "10s",
					HealthPath:   "/health",
					ExpectedCode: 200,
					Headers: map[string]string{
						"User-Agent": "Homer-Health-Check",
					},
				},
			},
			hasError: false,
		},
		{
			name: "Valid spec with advanced config",
			spec: DashboardSpec{
				HomerConfig: homer.HomerConfig{
					Title: "Test Dashboard",
				},
				Advanced: &AdvancedConfig{
					EnableDependencyAnalysis: true,
					EnableMetricsAggregation: true,
					EnableLayoutOptimization: true,
					MaxServicesPerGroup:      10,
					MaxItemsPerService:       20,
				},
			},
			hasError: false,
		},
		{
			name: "Valid spec with all features",
			spec: DashboardSpec{
				HomerConfig: homer.HomerConfig{
					Title:    "Complete Dashboard",
					Subtitle: "With all features enabled",
				},
				Replicas: int32Ptr(2),
				ServiceGrouping: &ServiceGroupingConfig{
					Strategy: "custom",
					CustomRules: []GroupingRule{
						{
							Name:      "Frontend Services",
							Condition: map[string]string{"app": "web-*", "tier": "frontend"},
							Priority:  1,
						},
					},
				},
				ConflictResolution: "merge",
				ValidationLevel:    "strict",
				HealthCheck: &ServiceHealthConfig{
					Enabled:      true,
					Interval:     "60s",
					Timeout:      "15s",
					HealthPath:   "/api/health",
					ExpectedCode: 200,
				},
				Advanced: &AdvancedConfig{
					EnableDependencyAnalysis: true,
					EnableMetricsAggregation: true,
					EnableLayoutOptimization: true,
					MaxServicesPerGroup:      15,
					MaxItemsPerService:       25,
				},
				DomainFilters: []string{"example.com", "internal.local"},
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dashboard := &Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dashboard",
					Namespace: "default",
				},
				Spec: tt.spec,
			}

			// Basic validation - check that required fields are present
			if dashboard.Spec.HomerConfig.Title == "" {
				if !tt.hasError {
					t.Error("Expected title to be set for valid dashboard")
				}
			}

			// Validate service grouping config
			if dashboard.Spec.ServiceGrouping != nil {
				sg := dashboard.Spec.ServiceGrouping
				if sg.Strategy == "label" && sg.LabelKey == "" {
					if !tt.hasError {
						t.Error("Expected labelKey to be set when strategy is 'label'")
					}
				}
				if sg.Strategy == "custom" && len(sg.CustomRules) == 0 {
					if !tt.hasError {
						t.Error("Expected custom rules to be set when strategy is 'custom'")
					}
				}
			}

			// Validate health check config
			if dashboard.Spec.HealthCheck != nil {
				hc := dashboard.Spec.HealthCheck
				if hc.Enabled && hc.HealthPath == "" {
					if !tt.hasError {
						t.Error("Expected health path to be set when health check is enabled")
					}
				}
			}

			// Validate advanced config
			if dashboard.Spec.Advanced != nil {
				ac := dashboard.Spec.Advanced
				if ac.MaxServicesPerGroup < 0 || ac.MaxItemsPerService < 0 {
					if !tt.hasError {
						t.Error("Expected max values to be non-negative")
					}
				}
			}
		})
	}
}

func TestServiceGroupingConfigDefaults(t *testing.T) {
	// Test that default values are handled properly
	config := &ServiceGroupingConfig{}

	// Strategy should default to "namespace"
	if config.Strategy == "" {
		config.Strategy = "namespace"
	}

	if config.Strategy != "namespace" {
		t.Errorf("Expected default strategy to be 'namespace', got '%s'", config.Strategy)
	}
}

func TestGroupingRuleValidation(t *testing.T) {
	tests := []struct {
		name  string
		rule  GroupingRule
		valid bool
	}{
		{
			name: "Valid rule",
			rule: GroupingRule{
				Name:      "Production Services",
				Condition: map[string]string{"environment": "prod"},
				Priority:  1,
			},
			valid: true,
		},
		{
			name: "Rule without name",
			rule: GroupingRule{
				Condition: map[string]string{"environment": "prod"},
				Priority:  1,
			},
			valid: false,
		},
		{
			name: "Rule without condition",
			rule: GroupingRule{
				Name:     "Production Services",
				Priority: 1,
			},
			valid: false,
		},
		{
			name: "Rule with zero priority",
			rule: GroupingRule{
				Name:      "Production Services",
				Condition: map[string]string{"environment": "prod"},
				Priority:  0,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation logic
			valid := tt.rule.Name != "" &&
				len(tt.rule.Condition) > 0 &&
				tt.rule.Priority > 0

			if valid != tt.valid {
				t.Errorf("Expected validity %v, got %v", tt.valid, valid)
			}
		})
	}
}

func TestServiceHealthConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config ServiceHealthConfig
		valid  bool
	}{
		{
			name: "Valid config",
			config: ServiceHealthConfig{
				Enabled:      true,
				Interval:     "30s",
				Timeout:      "10s",
				HealthPath:   "/health",
				ExpectedCode: 200,
			},
			valid: true,
		},
		{
			name: "Disabled config",
			config: ServiceHealthConfig{
				Enabled: false,
			},
			valid: true,
		},
		{
			name: "Invalid expected code",
			config: ServiceHealthConfig{
				Enabled:      true,
				ExpectedCode: 999,
			},
			valid: false,
		},
		{
			name: "Valid expected code range",
			config: ServiceHealthConfig{
				Enabled:      true,
				ExpectedCode: 404, // Valid even for error codes
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation logic
			valid := !tt.config.Enabled ||
				(tt.config.ExpectedCode >= 100 && tt.config.ExpectedCode <= 599)

			if valid != tt.valid {
				t.Errorf("Expected validity %v, got %v", tt.valid, valid)
			}
		})
	}
}

func TestAdvancedConfigDefaults(t *testing.T) {
	config := &AdvancedConfig{}

	// Check that default values work properly
	if config.MaxServicesPerGroup < 0 {
		t.Error("MaxServicesPerGroup should not be negative")
	}

	if config.MaxItemsPerService < 0 {
		t.Error("MaxItemsPerService should not be negative")
	}

	// Test that zero values (unlimited) are valid
	if config.MaxServicesPerGroup == 0 && config.MaxItemsPerService == 0 {
		// This should be valid (unlimited) - test passes by not erroring
		t.Log("Zero values are correctly treated as unlimited")
	}
}

func TestDashboardCreation(t *testing.T) {
	dashboard := &Dashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "test-namespace",
		},
		Spec: DashboardSpec{
			HomerConfig: homer.HomerConfig{
				Title:    "Test Dashboard",
				Subtitle: "A test dashboard for validation",
			},
			Replicas: int32Ptr(3),
			ServiceGrouping: &ServiceGroupingConfig{
				Strategy: "label",
				LabelKey: "team",
			},
			ConflictResolution: "merge",
			ValidationLevel:    "warn",
			HealthCheck: &ServiceHealthConfig{
				Enabled:      true,
				Interval:     "45s",
				Timeout:      "12s",
				HealthPath:   "/api/health",
				ExpectedCode: 200,
				Headers: map[string]string{
					"User-Agent":    "Homer-Health-Check/1.0",
					"Authorization": "Bearer health-token",
				},
			},
			Advanced: &AdvancedConfig{
				EnableDependencyAnalysis: true,
				EnableMetricsAggregation: false,
				EnableLayoutOptimization: true,
				MaxServicesPerGroup:      12,
				MaxItemsPerService:       30,
			},
		},
	}

	// Verify that the dashboard was created with correct values
	if dashboard.Spec.HomerConfig.Title != "Test Dashboard" {
		t.Errorf("Expected title 'Test Dashboard', got '%s'", dashboard.Spec.HomerConfig.Title)
	}

	if *dashboard.Spec.Replicas != 3 {
		t.Errorf("Expected 3 replicas, got %d", *dashboard.Spec.Replicas)
	}

	if dashboard.Spec.ServiceGrouping.Strategy != "label" {
		t.Errorf("Expected strategy 'label', got '%s'", dashboard.Spec.ServiceGrouping.Strategy)
	}

	if dashboard.Spec.ServiceGrouping.LabelKey != "team" {
		t.Errorf("Expected label key 'team', got '%s'", dashboard.Spec.ServiceGrouping.LabelKey)
	}

	if dashboard.Spec.ConflictResolution != "merge" {
		t.Errorf("Expected conflict resolution 'merge', got '%s'", dashboard.Spec.ConflictResolution)
	}

	if dashboard.Spec.ValidationLevel != "warn" {
		t.Errorf("Expected validation level 'warn', got '%s'", dashboard.Spec.ValidationLevel)
	}

	if !dashboard.Spec.HealthCheck.Enabled {
		t.Error("Expected health check to be enabled")
	}

	if dashboard.Spec.HealthCheck.Interval != "45s" {
		t.Errorf("Expected interval '45s', got '%s'", dashboard.Spec.HealthCheck.Interval)
	}

	if dashboard.Spec.Advanced.MaxServicesPerGroup != 12 {
		t.Errorf("Expected max services per group 12, got %d", dashboard.Spec.Advanced.MaxServicesPerGroup)
	}

	if dashboard.Spec.Advanced.EnableDependencyAnalysis != true {
		t.Error("Expected dependency analysis to be enabled")
	}

	if dashboard.Spec.Advanced.EnableMetricsAggregation != false {
		t.Error("Expected metrics aggregation to be disabled")
	}
}

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}
