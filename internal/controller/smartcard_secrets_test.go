package controller

import (
	"context"
	"strings"
	"testing"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildHomerConfigRejectsCrossNamespaceSmartCardSecret(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := smartCardSecretDashboard("dashboards", "shared-secrets")
	reconciler := &DashboardReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "shared-secrets"},
			Data:       map[string][]byte{"api-key": []byte("must-not-leak")},
		}).Build(),
		Scheme: scheme,
	}

	_, err := reconciler.buildHomerConfig(context.Background(), dashboard)
	if err == nil {
		t.Fatal("buildHomerConfig() error = nil, want cross-namespace smart-card Secret rejection")
	}
	for _, want := range []string{"smart-card Secret reference", "shared-secrets", "dashboards"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("buildHomerConfig() error = %q, want %q", err, want)
		}
	}
}

func TestBuildHomerConfigAllowsDashboardNamespaceSmartCardSecret(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := smartCardSecretDashboard("dashboards", "")
	reconciler := &DashboardReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "dashboards"},
			Data:       map[string][]byte{"api-key": []byte("local-secret")},
		}).Build(),
		Scheme: scheme,
	}

	config, err := reconciler.buildHomerConfig(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("buildHomerConfig() error = %v", err)
	}
	if got := config.Services[0].Items[0].Parameters["apikey"]; got != "local-secret" {
		t.Fatalf("resolved API key = %q, want local Secret value", got)
	}
}

func TestSmartCardPolicyDoesNotRestrictRemoteKubeconfigSecretNamespace(t *testing.T) {
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "dashboards"},
		Spec: homerv1alpha1.DashboardSpec{RemoteClusters: []homerv1alpha1.RemoteCluster{{
			Name: "remote",
			SecretRef: homerv1alpha1.KubeconfigSecretRef{
				Name: "remote-kubeconfig", Namespace: "shared-secrets",
			},
		}}},
	}
	if err := validateSmartCardSecretReferences(dashboard); err != nil {
		t.Fatalf("validateSmartCardSecretReferences() restricted kubeconfig SecretRef: %v", err)
	}
}

func TestFindDashboardsForSecretIgnoresCrossNamespaceSmartCardReference(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := smartCardSecretDashboard("dashboards", "shared-secrets")
	dashboard.Spec.RemoteClusters = []homerv1alpha1.RemoteCluster{{
		Name:      "remote",
		SecretRef: homerv1alpha1.KubeconfigSecretRef{Name: "remote-kubeconfig", Namespace: "shared-secrets"},
	}}
	reconciler := &DashboardReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(dashboard).Build(),
		Scheme: scheme,
	}

	requests := reconciler.findDashboardsForSecret(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "shared-secrets"},
	})
	if len(requests) != 0 {
		t.Fatalf("cross-namespace smart-card Secret requests = %#v, want none", requests)
	}

	requests = reconciler.findDashboardsForSecret(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-kubeconfig", Namespace: "shared-secrets"},
	})
	if want := (client.ObjectKey{Name: dashboard.Name, Namespace: dashboard.Namespace}); len(requests) != 1 || requests[0].NamespacedName != want {
		t.Fatalf("remote kubeconfig Secret requests = %#v, want one request for %s/%s", requests, want.Namespace, want.Name)
	}
}

func smartCardSecretDashboard(namespace, secretNamespace string) *homerv1alpha1.Dashboard {
	return &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: namespace},
		Spec: homerv1alpha1.DashboardSpec{
			HomerConfig: homer.HomerConfig{Services: []homer.Service{{Items: []homer.Item{{
				Parameters: map[string]string{"name": "Grafana", "type": "Grafana"},
			}}}}},
			Secrets: &homerv1alpha1.SmartCardSecrets{APIKey: &homerv1alpha1.SecretKeyRef{
				Name: "credentials", Key: "api-key", Namespace: secretNamespace,
			}},
		},
	}
}
