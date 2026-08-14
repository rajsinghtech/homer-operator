package utils

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type updateCountingClient struct {
	client.Client
	updateCalls int
	updateErr   error
}

func (c *updateCountingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updateCalls++
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}

func TestUpdateConfigMapWithRetryUpdatesObservedVersion(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard-homer", Namespace: "default"},
		Data:       map[string]string{"config.yml": "old"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	desired := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: existing.Name, Namespace: existing.Namespace}
	if err := baseClient.Get(context.Background(), key, desired); err != nil {
		t.Fatalf("get ConfigMap: %v", err)
	}
	desired.Data["config.yml"] = "new"
	k8sClient := &updateCountingClient{Client: baseClient}

	if err := UpdateConfigMapWithRetry(context.Background(), k8sClient, desired, "dashboard"); err != nil {
		t.Fatalf("update ConfigMap: %v", err)
	}
	if k8sClient.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", k8sClient.updateCalls)
	}
	actual := &corev1.ConfigMap{}
	if err := baseClient.Get(context.Background(), key, actual); err != nil {
		t.Fatalf("get updated ConfigMap: %v", err)
	}
	if got := actual.Data["config.yml"]; got != "new" {
		t.Fatalf("ConfigMap data = %q, want new", got)
	}
}

func TestUpdateConfigMapWithRetryReturnsConflictWithoutReplayingStaleMaps(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}
	latest := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard-homer", Namespace: "default"},
		Data:       map[string]string{"config.yml": "newer output"},
		BinaryData: map[string][]byte{"asset": []byte("newer asset")},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(latest).Build()
	conflict := apierrors.NewConflict(
		schema.GroupResource{Resource: "configmaps"}, latest.Name, errors.New("changed concurrently"),
	)
	k8sClient := &updateCountingClient{Client: baseClient, updateErr: conflict}
	stale := latest.DeepCopy()
	stale.Data["config.yml"] = "stale output"
	stale.BinaryData["asset"] = []byte("stale asset")

	err := UpdateConfigMapWithRetry(context.Background(), k8sClient, stale, "dashboard")
	if !apierrors.IsConflict(err) {
		t.Fatalf("error = %v, want conflict", err)
	}
	if k8sClient.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", k8sClient.updateCalls)
	}

	actual := &corev1.ConfigMap{}
	if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(latest), actual); err != nil {
		t.Fatalf("get latest ConfigMap: %v", err)
	}
	if got := actual.Data["config.yml"]; got != "newer output" {
		t.Fatalf("ConfigMap data = %q, stale payload was replayed", got)
	}
	if got := string(actual.BinaryData["asset"]); got != "newer asset" {
		t.Fatalf("ConfigMap binary data = %q, stale payload was replayed", got)
	}
}
