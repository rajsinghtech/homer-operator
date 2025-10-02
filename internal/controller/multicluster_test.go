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
