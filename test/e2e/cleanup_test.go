/*
Copyright 2026 RajSingh.

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

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
)

func TestCleanupE2ETestDeletesDashboardsBeforeNamespace(t *testing.T) {
	scheme := cleanupTestScheme(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-cleanup-success"}}
	dashboard := &homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: namespace.Name}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, dashboard).Build()

	withCleanupTiming(t, time.Second, time.Millisecond)
	if err := cleanupE2ETest(client, context.Background(), namespace.Name); err != nil {
		t.Fatalf("cleanupE2ETest() error = %v", err)
	}

	if err := client.Get(context.Background(), types.NamespacedName{Name: dashboard.Name, Namespace: namespace.Name}, &homerv1alpha1.Dashboard{}); err == nil {
		t.Fatal("Dashboard still exists after cleanup")
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: namespace.Name}, &corev1.Namespace{}); err == nil {
		t.Fatal("Namespace was deleted before cleanup completed")
	}
}

func TestCleanupE2ETestReportsStuckDashboardFinalizer(t *testing.T) {
	scheme := cleanupTestScheme(t)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-cleanup-stuck"}}
	dashboard := &homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{
		Name:       "dashboard",
		Namespace:  namespace.Name,
		Finalizers: []string{"homer.rajsingh.info/finalizer"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, dashboard).Build()

	withCleanupTiming(t, 20*time.Millisecond, time.Millisecond)
	err := cleanupE2ETest(client, context.Background(), namespace.Name)
	if err == nil {
		t.Fatal("cleanupE2ETest() error = nil, want failure for a stuck finalizer")
	}
	if !strings.Contains(err.Error(), "Dashboard e2e-cleanup-stuck/dashboard finalizers=[homer.rajsingh.info/finalizer]") {
		t.Fatalf("cleanup error does not report the stuck Dashboard finalizer: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: namespace.Name}, &corev1.Namespace{}); err != nil {
		t.Fatalf("Namespace should not be deleted while Dashboard finalizer is stuck: %v", err)
	}
}

func cleanupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := homerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Homer scheme: %v", err)
	}
	return scheme
}

func withCleanupTiming(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	previousTimeout, previousInterval := e2eCleanupTimeout, e2eCleanupPollInterval
	e2eCleanupTimeout, e2eCleanupPollInterval = timeout, interval
	t.Cleanup(func() {
		e2eCleanupTimeout, e2eCleanupPollInterval = previousTimeout, previousInterval
	})
}
