package controller

import (
	"context"
	"testing"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildHomerConfigInjectsSecretsIntoExternalBase(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "default"},
			Data: map[string]string{"config.yml": `title: External
services:
- name: External Services
  items:
  - name: External Card
    type: Prometheus
    url: https://external.example.com
`},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cards", Namespace: "default"},
			Data:       map[string][]byte{"api-key": []byte("external-secret")},
		},
	).Build()
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			ConfigMap: homerv1alpha1.ConfigMap{Name: "external"},
			HomerConfig: homer.HomerConfig{Title: "Inline", Services: []homer.Service{{
				Name: "Inline Services", Items: []homer.Item{{Name: "Inline Card", Type: "Prometheus"}},
			}}},
			Secrets: &homerv1alpha1.SmartCardSecrets{APIKey: &homerv1alpha1.SecretKeyRef{Name: "cards", Key: "api-key"}},
		},
	}

	config, err := (&DashboardReconciler{Client: client}).buildHomerConfig(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("buildHomerConfig() error = %v", err)
	}
	if config.Title != "External" || len(config.Services) != 1 || config.Services[0].Items[0].Name != "External Card" {
		t.Fatalf("effective config did not use external base: %#v", config)
	}
	if got := config.Services[0].Items[0].Parameters["apikey"]; got != "external-secret" {
		t.Fatalf("external smart card API key = %q, want external-secret", got)
	}
}

func TestDiscoveryConfigForDashboardCopiesFeatures(t *testing.T) {
	dashboard := &homerv1alpha1.Dashboard{Spec: homerv1alpha1.DashboardSpec{
		ValidationLevel: "strict",
		ServiceGrouping: &homerv1alpha1.ServiceGroupingConfig{
			Strategy: "custom", LabelKey: "team",
			CustomRules: []homerv1alpha1.GroupingRule{{Name: "Apps", Condition: map[string]string{"tier": "app"}, Priority: 3}},
		},
		HealthCheck: &homerv1alpha1.ServiceHealthConfig{
			Enabled: true, Interval: "1m", Timeout: "5s", HealthPath: "/ready",
			ExpectedCode: 204, Headers: map[string]string{"X-Probe": "yes"},
		},
	}}

	got := discoveryConfigForDashboard(dashboard)
	if got.ValidationLevel != homer.ValidationLevelStrict || got.ServiceGrouping == nil || got.ServiceGrouping.Strategy != homer.ServiceGroupingCustom {
		t.Fatalf("discovery config conversion = %#v", got)
	}
	if got.HealthCheck == nil || got.HealthCheck.HealthPath != "/ready" || got.HealthCheck.ExpectedCode != 204 {
		t.Fatalf("health config conversion = %#v", got.HealthCheck)
	}

	// Ensure conversion did not alias mutable API maps.
	dashboard.Spec.ServiceGrouping.CustomRules[0].Condition["tier"] = "changed"
	dashboard.Spec.HealthCheck.Headers["X-Probe"] = "changed"
	if got.ServiceGrouping.CustomRules[0].Condition["tier"] != "app" || got.HealthCheck.Headers["X-Probe"] != "yes" {
		t.Fatal("discovery config aliases Dashboard maps")
	}
}
