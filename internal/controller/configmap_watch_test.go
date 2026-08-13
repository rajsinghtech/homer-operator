package controller

import (
	"context"
	"testing"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindDashboardsForConfigMapMatchesExternalAndAssetReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := homerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Homer scheme: %v", err)
	}

	const dashboardNamespace = "dashboards"
	const assetsNamespace = "shared-assets"
	const sharedConfigMap = "shared-config"

	dashboards := []client.Object{
		&homerv1alpha1.Dashboard{
			ObjectMeta: metav1.ObjectMeta{Name: "external-only", Namespace: dashboardNamespace},
			Spec:       homerv1alpha1.DashboardSpec{ConfigMap: homerv1alpha1.ConfigMap{Name: sharedConfigMap}},
		},
		&homerv1alpha1.Dashboard{
			ObjectMeta: metav1.ObjectMeta{Name: "asset-only", Namespace: dashboardNamespace},
			Spec:       homerv1alpha1.DashboardSpec{Assets: &homerv1alpha1.AssetsConfig{ConfigMapRef: &homerv1alpha1.AssetConfigMapRef{Name: sharedConfigMap}}},
		},
		&homerv1alpha1.Dashboard{
			ObjectMeta: metav1.ObjectMeta{Name: "both-references", Namespace: dashboardNamespace},
			Spec: homerv1alpha1.DashboardSpec{
				ConfigMap: homerv1alpha1.ConfigMap{Name: sharedConfigMap},
				Assets:    &homerv1alpha1.AssetsConfig{ConfigMapRef: &homerv1alpha1.AssetConfigMapRef{Name: sharedConfigMap}},
			},
		},
		&homerv1alpha1.Dashboard{
			ObjectMeta: metav1.ObjectMeta{Name: "wrong-namespace", Namespace: "other-dashboard-namespace"},
			Spec:       homerv1alpha1.DashboardSpec{ConfigMap: homerv1alpha1.ConfigMap{Name: sharedConfigMap}},
		},
		&homerv1alpha1.Dashboard{
			ObjectMeta: metav1.ObjectMeta{Name: "cross-namespace-asset", Namespace: dashboardNamespace},
			Spec:       homerv1alpha1.DashboardSpec{Assets: &homerv1alpha1.AssetsConfig{ConfigMapRef: &homerv1alpha1.AssetConfigMapRef{Name: "cross-assets", Namespace: assetsNamespace}}},
		},
	}

	reconciler := &DashboardReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(dashboards...).Build(),
		Scheme: scheme,
	}

	requests := reconciler.findDashboardsForAssetConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: sharedConfigMap, Namespace: dashboardNamespace},
	})
	assertRequests(t, requests, map[client.ObjectKey]bool{
		{Name: "external-only", Namespace: dashboardNamespace}:   true,
		{Name: "asset-only", Namespace: dashboardNamespace}:      true,
		{Name: "both-references", Namespace: dashboardNamespace}: true,
	})

	requests = reconciler.findDashboardsForAssetConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cross-assets", Namespace: assetsNamespace},
	})
	assertRequests(t, requests, map[client.ObjectKey]bool{
		{Name: "cross-namespace-asset", Namespace: dashboardNamespace}: true,
	})

	requests = reconciler.findDashboardsForAssetConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: sharedConfigMap, Namespace: "unrelated-namespace"},
	})
	if len(requests) != 0 {
		t.Fatalf("requests for same-named ConfigMap in another namespace = %#v, want none", requests)
	}
}

func assertRequests(t *testing.T, requests []ctrl.Request, expected map[client.ObjectKey]bool) {
	t.Helper()
	if len(requests) != len(expected) {
		t.Fatalf("got %d requests (%#v), want %d (%#v)", len(requests), requests, len(expected), expected)
	}
	seen := make(map[client.ObjectKey]int, len(requests))
	for _, request := range requests {
		seen[request.NamespacedName]++
	}
	for key := range expected {
		if seen[key] != 1 {
			t.Fatalf("request count for %s/%s = %d, want 1", key.Namespace, key.Name, seen[key])
		}
	}
}
