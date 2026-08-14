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
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestUpdateStatusUsesDiscoveryResultsWithoutRediscovery(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := homerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatal(err)
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{{Name: "remote"}},
		},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dashboard-homer", Namespace: "default"},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(dashboard).
		WithObjects(dashboard, deployment).
		Build()

	clusterManager := NewClusterManager(k8sClient, scheme)
	// A second discovery pass would dereference this nil client. The status
	// update must use the successful results from the reconcile discovery pass.
	clusterManager.clients["remote"] = &ClusterClient{Name: "remote", Connected: true}

	controller := &DashboardReconciler{Client: k8sClient, Scheme: scheme, ClusterManager: clusterManager}
	firstPass := &discoveredClusterResources{
		ingresses:  map[string][]networkingv1.Ingress{"remote": make([]networkingv1.Ingress, 2)},
		httpRoutes: map[string][]gatewayv1.HTTPRoute{"remote": make([]gatewayv1.HTTPRoute, 3)},
		services:   map[string][]corev1.Service{"remote": make([]corev1.Service, 1)},
	}

	if err := controller.updateStatus(context.Background(), dashboard, firstPass); err != nil {
		t.Fatalf("updateStatus() returned an error: %v", err)
	}

	updated := &homerv1alpha1.Dashboard{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(dashboard), updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Status.ClusterStatuses; len(got) != 1 {
		t.Fatalf("expected one cluster status, got %d", len(got))
	} else {
		if got[0].DiscoveredIngresses != 2 || got[0].DiscoveredHTTPRoutes != 3 || got[0].DiscoveredServices != 1 {
			t.Fatalf("unexpected discovery counts: %+v", got[0])
		}
	}
}

func TestReconcileFailureClearsStaleReadyCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := homerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default", Generation: 4},
		Status: homerv1alpha1.DashboardStatus{
			Ready: true,
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue,
				ObservedGeneration: 3, Reason: "DeploymentAvailable",
			}},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(dashboard).
		WithObjects(dashboard).
		Build()

	reconciler := &DashboardReconciler{Client: k8sClient}
	if err := reconciler.markReconcileFailure(context.Background(), dashboard, fmt.Errorf("invalid Homer configuration")); err != nil {
		t.Fatalf("markReconcileFailure() error = %v", err)
	}

	updated := &homerv1alpha1.Dashboard{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(dashboard), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Ready {
		t.Fatal("failed reconciliation left status.ready=true")
	}
	condition := apiMeta.FindStatusCondition(updated.Status.Conditions, "Ready")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ReconcileError" || condition.ObservedGeneration != 4 {
		t.Fatalf("unexpected Ready condition: %#v", condition)
	}
}
