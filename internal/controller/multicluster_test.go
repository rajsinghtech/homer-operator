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

package controller

import (
	"context"
	"testing"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestClusterManager_Creation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	cm := NewClusterManager(client, scheme)
	if cm == nil {
		t.Fatal("Expected ClusterManager to be created")
	}

	if cm.localClient == nil {
		t.Error("Expected localClient to be set")
	}

	if cm.scheme == nil {
		t.Error("Expected scheme to be set")
	}

	if cm.clients == nil {
		t.Error("Expected clients map to be initialized")
	}
}

func TestClusterManager_UpdateClusters_NoRemoteClusters(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	cm := NewClusterManager(client, scheme)

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "default",
		},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{}, // No remote clusters
		},
	}

	ctx := context.Background()
	err := cm.UpdateClusters(ctx, dashboard)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should have local cluster
	if _, ok := cm.clients["local"]; !ok {
		t.Error("Expected local cluster to be present")
	}
}

func TestClusterManager_UpdateClusters_WithDisabledCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	cm := NewClusterManager(client, scheme)

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "default",
		},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{
				{
					Name:    "disabled-cluster",
					Enabled: false, // Disabled
					SecretRef: homerv1alpha1.KubeconfigSecretRef{
						Name: "test-secret",
					},
				},
			},
		},
	}

	ctx := context.Background()
	err := cm.UpdateClusters(ctx, dashboard)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should not have disabled cluster
	if _, ok := cm.clients["disabled-cluster"]; ok {
		t.Error("Expected disabled cluster to not be present")
	}

	// Should still have local cluster
	if _, ok := cm.clients["local"]; !ok {
		t.Error("Expected local cluster to be present")
	}
}

func TestClusterManager_GetClusterStatuses(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	cm := NewClusterManager(client, scheme)

	// Add test clusters
	cm.clients["test-cluster"] = &ClusterClient{
		Name:      "test-cluster",
		Connected: true,
		LastError: nil,
	}

	statuses := cm.GetClusterStatuses()

	if len(statuses) != 1 {
		t.Errorf("Expected 1 status, got %d", len(statuses))
	}

	if statuses[0].Name != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got %s", statuses[0].Name)
	}

	if !statuses[0].Connected {
		t.Error("Expected cluster to be connected")
	}
}

func TestClusterManager_DiscoverIngresses_LocalOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	// Create test ingress
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "test.example.com",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress).
		Build()

	cm := NewClusterManager(client, scheme)
	cm.clients["local"] = &ClusterClient{
		Name:      "local",
		Client:    client,
		Connected: true,
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	results, err := cm.DiscoverIngresses(ctx, dashboard)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 cluster result, got %d", len(results))
	}

	localIngresses, ok := results["local"]
	if !ok {
		t.Error("Expected local cluster results")
	}

	if len(localIngresses) != 1 {
		t.Errorf("Expected 1 ingress, got %d", len(localIngresses))
	}

	if localIngresses[0].Name != "test-ingress" {
		t.Errorf("Expected ingress name 'test-ingress', got %s", localIngresses[0].Name)
	}

	// Check that cluster annotation was added
	if localIngresses[0].Annotations["homer.rajsingh.info/cluster"] != "local" {
		t.Error("Expected cluster annotation to be added")
	}
}

func TestClusterManager_DiscoverHTTPRoutes_LocalOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	// Create test HTTPRoute
	hostname := gatewayv1.Hostname("test.example.com")
	httproute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-httproute",
			Namespace: "default",
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{hostname},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(httproute).
		Build()

	cm := NewClusterManager(client, scheme)
	cm.clients["local"] = &ClusterClient{
		Name:      "local",
		Client:    client,
		Connected: true,
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	results, err := cm.DiscoverHTTPRoutes(ctx, dashboard)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 cluster result, got %d", len(results))
	}

	localHTTPRoutes, ok := results["local"]
	if !ok {
		t.Error("Expected local cluster results")
	}

	if len(localHTTPRoutes) != 1 {
		t.Errorf("Expected 1 HTTPRoute, got %d", len(localHTTPRoutes))
	}

	if localHTTPRoutes[0].Name != "test-httproute" {
		t.Errorf("Expected HTTPRoute name 'test-httproute', got %s", localHTTPRoutes[0].Name)
	}

	// Check that cluster annotation was added
	if localHTTPRoutes[0].Annotations["homer.rajsingh.info/cluster"] != "local" {
		t.Error("Expected cluster annotation to be added")
	}
}

func TestClusterManager_NamespaceFilter(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	// Create test ingresses in different namespaces
	ingress1 := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress1",
			Namespace: "namespace1",
		},
	}
	ingress2 := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress2",
			Namespace: "namespace2",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress1, ingress2).
		Build()

	cm := NewClusterManager(client, scheme)

	// Test cluster with namespace filter
	testCluster := &ClusterClient{
		Name:      "test",
		Client:    client,
		Connected: true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{
			Name:            "test",
			NamespaceFilter: []string{"namespace1"}, // Only namespace1
		},
	}

	dashboard := &homerv1alpha1.Dashboard{}

	ctx := context.Background()
	ingresses, err := cm.discoverClusterIngresses(ctx, testCluster, dashboard)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(ingresses) != 1 {
		t.Errorf("Expected 1 ingress, got %d", len(ingresses))
	}

	if ingresses[0].Namespace != "namespace1" {
		t.Errorf("Expected ingress from namespace1, got %s", ingresses[0].Namespace)
	}
}

func TestClusterManager_ClusterLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ingress).
		Build()

	cm := NewClusterManager(client, scheme)

	testCluster := &ClusterClient{
		Name:      "test",
		Client:    client,
		Connected: true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{
			Name: "test",
			ClusterLabels: map[string]string{
				"cluster":     "test",
				"environment": "staging",
			},
		},
	}

	dashboard := &homerv1alpha1.Dashboard{}

	ctx := context.Background()
	ingresses, err := cm.discoverClusterIngresses(ctx, testCluster, dashboard)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(ingresses) != 1 {
		t.Errorf("Expected 1 ingress, got %d", len(ingresses))
	}

	// Check cluster labels were added
	if ingresses[0].Labels["cluster"] != "test" {
		t.Error("Expected cluster label to be added")
	}
	if ingresses[0].Labels["environment"] != "staging" {
		t.Error("Expected environment label to be added")
	}
}

// TestMultiClusterIngressAnnotationsPreserved verifies that Homer annotations on Ingresses
// from remote clusters are preserved and applied to the generated Homer config
func TestMultiClusterIngressAnnotationsPreserved(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "annotated-ingress",
			Namespace: "remote-namespace",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":     "My Application",
				"item.homer.rajsingh.info/subtitle": "Production instance",
				"item.homer.rajsingh.info/logo":     "https://example.com/logo.png",
				"item.homer.rajsingh.info/tag":      "production",
				"item.homer.rajsingh.info/keywords": "app, api, service",
				"service.homer.rajsingh.info/name":  "Remote Services",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "app.remote.example.com"}},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ingress).Build()
	cm := NewClusterManager(client, scheme)

	testCluster := &ClusterClient{
		Name:       "remote-cluster",
		Client:     client,
		Connected:  true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote-cluster"},
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			HomerConfig: homer.HomerConfig{Title: "Test Dashboard", Header: true},
		},
	}

	ctx := context.Background()
	ingresses, err := cm.discoverClusterIngresses(ctx, testCluster, dashboard)
	if err != nil {
		t.Fatalf("Failed to discover ingresses: %v", err)
	}

	if len(ingresses) != 1 {
		t.Fatalf("Expected 1 ingress, got %d", len(ingresses))
	}

	// Verify annotations preserved after discovery
	expectedAnnotations := map[string]string{
		"item.homer.rajsingh.info/name":     "My Application",
		"item.homer.rajsingh.info/subtitle": "Production instance",
		"item.homer.rajsingh.info/logo":     "https://example.com/logo.png",
		"item.homer.rajsingh.info/tag":      "production",
		"item.homer.rajsingh.info/keywords": "app, api, service",
		"service.homer.rajsingh.info/name":  "Remote Services",
		"homer.rajsingh.info/cluster":       "remote-cluster",
	}

	for key, expectedValue := range expectedAnnotations {
		if actualValue, ok := ingresses[0].Annotations[key]; !ok {
			t.Errorf("Annotation %s is missing", key)
		} else if actualValue != expectedValue {
			t.Errorf("Annotation %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	// Test annotations processed into Homer config
	homerConfig := &homer.HomerConfig{Title: "Test Dashboard", Header: true}
	for _, ing := range ingresses {
		homer.UpdateHomerConfigIngress(homerConfig, ing, nil)
	}

	if len(homerConfig.Services) == 0 {
		t.Fatal("No services created in Homer config")
	}

	var targetService *homer.Service
	for i := range homerConfig.Services {
		if homerConfig.Services[i].Parameters["name"] == "Remote Services" {
			targetService = &homerConfig.Services[i]
			break
		}
	}

	if targetService == nil || len(targetService.Items) == 0 {
		t.Fatal("Service 'Remote Services' not found or has no items")
	}

	item := targetService.Items[0]
	expectedParams := map[string]string{
		"name":     "My Application",
		"subtitle": "Production instance",
		"logo":     "https://example.com/logo.png",
		"tag":      "production",
		"keywords": "app,api,service",
	}

	for key, expectedValue := range expectedParams {
		if actualValue, ok := item.Parameters[key]; !ok || actualValue != expectedValue {
			t.Errorf("Parameter %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	// Verify YAML output
	ingressList := networkingv1.IngressList{Items: ingresses}
	configMap, err := homer.CreateConfigMap(homerConfig, "test", "default", ingressList, dashboard)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	yamlStr := configMap.Data["config.yml"]
	expectedStrings := []string{"My Application", "Production instance", "https://example.com/logo.png", "production", "app,api,service", "Remote Services"}
	for _, expected := range expectedStrings {
		if !contains(yamlStr, expected) {
			t.Errorf("Generated YAML does not contain %q", expected)
		}
	}
}

// TestMultiClusterHTTPRouteAnnotationsPreserved verifies that Homer annotations on HTTPRoutes
// from remote clusters are preserved and applied to the generated Homer config
func TestMultiClusterHTTPRouteAnnotationsPreserved(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	hostname := gatewayv1.Hostname("api.remote.example.com")
	httproute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "annotated-httproute",
			Namespace: "remote-namespace",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":     "API Gateway",
				"item.homer.rajsingh.info/subtitle": "GraphQL API",
				"item.homer.rajsingh.info/logo":     "https://example.com/api-logo.png",
				"item.homer.rajsingh.info/tag":      "api",
				"item.homer.rajsingh.info/keywords": "graphql, rest, api",
				"service.homer.rajsingh.info/name":  "API Services",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{hostname}},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(httproute).Build()
	cm := NewClusterManager(client, scheme)

	testCluster := &ClusterClient{
		Name:       "remote-cluster",
		Client:     client,
		Connected:  true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote-cluster"},
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			HomerConfig: homer.HomerConfig{Title: "Test Dashboard", Header: true},
		},
	}

	ctx := context.Background()
	httproutes, err := cm.discoverClusterHTTPRoutes(ctx, testCluster, dashboard)
	if err != nil {
		t.Fatalf("Failed to discover HTTPRoutes: %v", err)
	}

	if len(httproutes) != 1 {
		t.Fatalf("Expected 1 HTTPRoute, got %d", len(httproutes))
	}

	// Verify annotations preserved after discovery
	expectedAnnotations := map[string]string{
		"item.homer.rajsingh.info/name":     "API Gateway",
		"item.homer.rajsingh.info/subtitle": "GraphQL API",
		"item.homer.rajsingh.info/logo":     "https://example.com/api-logo.png",
		"item.homer.rajsingh.info/tag":      "api",
		"item.homer.rajsingh.info/keywords": "graphql, rest, api",
		"service.homer.rajsingh.info/name":  "API Services",
		"homer.rajsingh.info/cluster":       "remote-cluster",
	}

	for key, expectedValue := range expectedAnnotations {
		if actualValue, ok := httproutes[0].Annotations[key]; !ok {
			t.Errorf("Annotation %s is missing", key)
		} else if actualValue != expectedValue {
			t.Errorf("Annotation %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	// Test annotations processed into Homer config
	homerConfig := &homer.HomerConfig{Title: "Test Dashboard", Header: true}
	for i := range httproutes {
		homer.UpdateHomerConfigHTTPRoute(homerConfig, &httproutes[i], nil)
	}

	if len(homerConfig.Services) == 0 {
		t.Fatal("No services created in Homer config")
	}

	var targetService *homer.Service
	for i := range homerConfig.Services {
		if homerConfig.Services[i].Parameters["name"] == "API Services" {
			targetService = &homerConfig.Services[i]
			break
		}
	}

	if targetService == nil || len(targetService.Items) == 0 {
		t.Fatal("Service 'API Services' not found or has no items")
	}

	item := targetService.Items[0]
	expectedParams := map[string]string{
		"name":     "API Gateway",
		"subtitle": "GraphQL API",
		"logo":     "https://example.com/api-logo.png",
		"tag":      "api",
		"keywords": "graphql,rest,api",
	}

	for key, expectedValue := range expectedParams {
		if actualValue, ok := item.Parameters[key]; !ok || actualValue != expectedValue {
			t.Errorf("Parameter %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}

	// Verify YAML output
	configMap, err := homer.CreateConfigMapWithHTTPRoutes(homerConfig, "test", "default", networkingv1.IngressList{}, httproutes, dashboard, nil)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	yamlStr := configMap.Data["config.yml"]
	expectedStrings := []string{"API Gateway", "GraphQL API", "https://example.com/api-logo.png", "api", "graphql,rest,api", "API Services"}
	for _, expected := range expectedStrings {
		if !contains(yamlStr, expected) {
			t.Errorf("Generated YAML does not contain %q", expected)
		}
	}
}

// TestMultiClusterMultipleResourcesWithAnnotations tests multiple ingresses and HTTPRoutes
// from different clusters with different annotations
func TestMultiClusterMultipleResourcesWithAnnotations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = homerv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)

	// Cluster 1: Ingress
	cluster1Ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1",
			Namespace: "default",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name": "Cluster 1 App",
				"item.homer.rajsingh.info/tag":  "cluster1",
			},
		},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app1.cluster1.com"}}},
	}

	// Cluster 2: HTTPRoute
	hostname := gatewayv1.Hostname("app2.cluster2.com")
	cluster2HTTPRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app2",
			Namespace: "default",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name": "Cluster 2 App",
				"item.homer.rajsingh.info/tag":  "cluster2",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{hostname}},
	}

	client1 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster1Ingress).Build()
	client2 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster2HTTPRoute).Build()

	cm := NewClusterManager(client1, scheme)

	cluster1 := &ClusterClient{
		Name:       "cluster1",
		Client:     client1,
		Connected:  true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "cluster1"},
	}

	cluster2 := &ClusterClient{
		Name:       "cluster2",
		Client:     client2,
		Connected:  true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "cluster2"},
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-cluster-dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			HomerConfig: homer.HomerConfig{Title: "Multi-Cluster Dashboard", Header: true},
		},
	}

	ctx := context.Background()

	// Discover from both clusters
	ingresses, err := cm.discoverClusterIngresses(ctx, cluster1, dashboard)
	if err != nil {
		t.Fatalf("Failed to discover from cluster1: %v", err)
	}

	httproutes, err := cm.discoverClusterHTTPRoutes(ctx, cluster2, dashboard)
	if err != nil {
		t.Fatalf("Failed to discover from cluster2: %v", err)
	}

	if len(ingresses) != 1 || len(httproutes) != 1 {
		t.Fatalf("Expected 1 ingress and 1 HTTPRoute, got %d and %d", len(ingresses), len(httproutes))
	}

	// Create Homer config with all resources
	homerConfig := &homer.HomerConfig{Title: "Multi-Cluster Dashboard", Header: true}
	for _, ing := range ingresses {
		homer.UpdateHomerConfigIngress(homerConfig, ing, nil)
	}
	for i := range httproutes {
		homer.UpdateHomerConfigHTTPRoute(homerConfig, &httproutes[i], nil)
	}

	// Generate YAML and verify both apps are present
	ingressList := networkingv1.IngressList{Items: ingresses}
	configMap, err := homer.CreateConfigMapWithHTTPRoutes(homerConfig, "multi-cluster-test", "default", ingressList, httproutes, dashboard, nil)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	yamlStr := configMap.Data["config.yml"]

	// Verify both clusters' apps are in the config
	expectedStrings := []string{"Cluster 1 App", "cluster1", "Cluster 2 App", "cluster2"}
	for _, expected := range expectedStrings {
		if !contains(yamlStr, expected) {
			t.Errorf("Generated YAML does not contain %q", expected)
		}
	}
}

// Helper function for string search
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}
