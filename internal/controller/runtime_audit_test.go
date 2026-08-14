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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestEnvVarsEqualComparesValueFrom(t *testing.T) {
	reconciler := &DashboardReconciler{}
	desired := []corev1.EnvVar{{Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "one"}, Key: "token"},
	}}}
	existing := []corev1.EnvVar{{Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "two"}, Key: "token"},
	}}}
	if reconciler.envVarsEqual(desired, existing) {
		t.Fatal("envVarsEqual considered different ValueFrom references equal")
	}
}

func TestDeploymentSpecsDifferCoversManagedFieldsAndDefaults(t *testing.T) {
	desired := homer.CreateDeployment("dashboard", "default", nil, nil, nil)
	existing := desired.DeepCopy()
	for i := range existing.Spec.Template.Spec.Containers {
		existing.Spec.Template.Spec.Containers[i].ImagePullPolicy = defaultImagePullPolicy(existing.Spec.Template.Spec.Containers[i].Image)
		existing.Spec.Template.Spec.Containers[i].TerminationMessagePath = corev1.TerminationMessagePathDefault
		existing.Spec.Template.Spec.Containers[i].TerminationMessagePolicy = corev1.TerminationMessageReadFile
		for j := range existing.Spec.Template.Spec.Containers[i].Ports {
			existing.Spec.Template.Spec.Containers[i].Ports[j].Protocol = corev1.ProtocolTCP
		}
	}
	for i := range existing.Spec.Template.Spec.Volumes {
		if source := existing.Spec.Template.Spec.Volumes[i].ConfigMap; source != nil {
			mode := corev1.ConfigMapVolumeSourceDefaultMode
			source.DefaultMode = &mode
		}
	}
	reconciler := &DashboardReconciler{}
	if reconciler.deploymentSpecsDiffer(context.Background(), &desired, existing) {
		t.Fatal("deploymentSpecsDiffer did not tolerate API-server defaults")
	}

	mutations := map[string]func(*appsv1.Deployment){
		"ports": func(d *appsv1.Deployment) { d.Spec.Template.Spec.Containers[1].Ports[0].ContainerPort++ },
		"container security": func(d *appsv1.Deployment) {
			value := true
			d.Spec.Template.Spec.Containers[1].SecurityContext.Privileged = &value
		},
		"probe": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers[1].ReadinessProbe = &corev1.Probe{InitialDelaySeconds: 5}
		},
		"lifecycle": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.Containers[1].Lifecycle = &corev1.Lifecycle{PostStart: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}}}
		},
		"init container": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.InitContainers = []corev1.Container{{Name: "init", Image: "busybox:1"}}
		},
		"pod field": func(d *appsv1.Deployment) {
			d.Spec.Template.Spec.DNSConfig = &corev1.PodDNSConfig{Searches: []string{"example.test"}}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := existing.DeepCopy()
			mutate(changed)
			if !reconciler.deploymentSpecsDiffer(context.Background(), &desired, changed) {
				t.Fatalf("deploymentSpecsDiffer missed %s change", name)
			}
		})
	}
}

func TestServicePortsMatchUnnamedDesiredToNamedExisting(t *testing.T) {
	desired := corev1.ServicePort{Port: 80, TargetPort: intstr.FromInt(8080)}
	existing := corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP, NodePort: 30080}
	if !servicePortsMatch(desired, existing) {
		t.Fatal("unnamed desired Service port did not semantically match named existing port")
	}
	if serviceSpecsDiffer(&corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{desired}}}, &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{existing}}}) {
		t.Fatal("serviceSpecsDiffer reported an API-assigned port name/NodePort difference")
	}
}

func TestSecretWatcherDeduplicatesDashboardRequests(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{{Name: "remote", SecretRef: homerv1alpha1.KubeconfigSecretRef{Name: "shared"}}},
			Secrets: &homerv1alpha1.SmartCardSecrets{
				APIKey:  &homerv1alpha1.SecretKeyRef{Name: "shared", Key: "api-key"},
				Headers: map[string]*homerv1alpha1.SecretKeyRef{"Authorization": {Name: "shared", Key: "header"}},
			},
		},
	}
	reconciler := &DashboardReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(dashboard).Build()}
	requests := reconciler.findDashboardsForSecret(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"}})
	if len(requests) != 1 {
		t.Fatalf("Secret watcher requests = %#v, want one", requests)
	}
}

func TestNamespaceWatcherEnqueuesAfterAnnotationsRemoved(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := &homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"}}
	reconciler := &DashboardReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(dashboard).Build()}
	requests := reconciler.findDashboardsForNamespace(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps"}})
	if len(requests) != 1 || requests[0].Name != "dashboard" {
		t.Fatalf("Namespace watcher requests = %#v, want dashboard", requests)
	}
}

func TestDashboardClusterManagersAreIsolatedByDashboard(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	reconciler := &DashboardReconciler{Scheme: scheme}
	if reconciler.ClusterManager != nil {
		t.Fatal("production-style reconciler unexpectedly has an injected shared ClusterManager")
	}
	first := &homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "apps"}}
	second := &homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: "apps"}}

	firstManager := reconciler.clusterManagerForDashboard(first)
	secondManager := reconciler.clusterManagerForDashboard(second)
	if firstManager == secondManager {
		t.Fatal("same-named remote clusters on different Dashboards share a ClusterManager")
	}

	firstClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	secondClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	firstManager.clients["shared"] = &ClusterClient{
		Name: "shared", Client: firstClient, Connected: true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "shared", NamespaceFilter: []string{"first"}},
	}
	secondManager.clients["shared"] = &ClusterClient{
		Name: "shared", Client: secondClient, Connected: true,
		ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "shared", NamespaceFilter: []string{"second"}},
	}
	if firstManager.clients["shared"].Client == secondManager.clients["shared"].Client ||
		firstManager.clients["shared"].ClusterCfg.NamespaceFilter[0] == secondManager.clients["shared"].ClusterCfg.NamespaceFilter[0] {
		t.Fatal("Dashboard-scoped remote client/configuration state was contaminated")
	}
}

func TestResourceWatchersEnqueueEveryDashboard(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboards := []client.Object{
		&homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "one", Namespace: "apps"}},
		&homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "two", Namespace: "platform"}},
	}
	reconciler := &DashboardReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(dashboards...).Build(),
	}
	expected := map[client.ObjectKey]bool{
		{Name: "one", Namespace: "apps"}:     true,
		{Name: "two", Namespace: "platform"}: true,
	}

	resources := []struct {
		name string
		obj  client.Object
		mapf func(context.Context, client.Object) []ctrl.Request
	}{
		{name: "Ingress", obj: &networkingv1.Ingress{}, mapf: reconciler.findDashboardsForIngress},
		{name: "Service", obj: &corev1.Service{}, mapf: reconciler.findDashboardsForService},
		{name: "HTTPRoute", obj: &gatewayv1.HTTPRoute{}, mapf: reconciler.findDashboardsForHTTPRoute},
		{name: "Gateway", obj: &gatewayv1.Gateway{}, mapf: reconciler.findDashboardsForGateway},
	}
	for _, resource := range resources {
		t.Run(resource.name, func(t *testing.T) {
			assertRequests(t, resource.mapf(context.Background(), resource.obj), expected)
		})
	}
}

func TestCleanupResourcesReturnsGetErrors(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := &homerv1alpha1.Dashboard{ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default", UID: "uid"}}
	wantErr := errors.New("API unavailable")
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &DashboardReconciler{Client: interceptor.NewClient(baseClient, interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return wantErr
		},
	})}
	if err := reconciler.cleanupResources(context.Background(), dashboard); !errors.Is(err, wantErr) {
		t.Fatalf("cleanupResources() error = %v, want %v", err, wantErr)
	}
}

func TestCleanupResourcesReturnsAssetConfigMapGetErrors(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default", UID: "uid"},
		Spec: homerv1alpha1.DashboardSpec{Assets: &homerv1alpha1.AssetsConfig{
			ConfigMapRef: &homerv1alpha1.AssetConfigMapRef{Name: "assets"},
		}},
	}
	wantErr := errors.New("asset API unavailable")
	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &DashboardReconciler{Client: interceptor.NewClient(baseClient, interceptor.Funcs{
		Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if key.Name == "assets" {
				return wantErr
			}
			return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
		},
	})}
	if err := reconciler.cleanupResources(context.Background(), dashboard); !errors.Is(err, wantErr) {
		t.Fatalf("cleanupResources() asset error = %v, want %v", err, wantErr)
	}
}

func TestNormalizeManagedPodSpecDoesNotMutateInput(t *testing.T) {
	deployment := homer.CreateDeployment("dashboard", "default", nil, nil, nil)
	original := deployment.Spec.Template.Spec.DeepCopy()
	_ = normalizeManagedPodSpec(deployment.Spec.Template.Spec)
	if !reflect.DeepEqual(original, &deployment.Spec.Template.Spec) {
		t.Fatal("normalizeManagedPodSpec mutated its input")
	}
}

func runtimeAuditScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		appsv1.AddToScheme,
		networkingv1.AddToScheme,
		homerv1alpha1.AddToScheme,
		gatewayv1.Install,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return scheme
}

func TestSortedClusterNamesAndDashboardAggregation(t *testing.T) {
	if got, want := sortedClusterNames(map[string]int{"zulu": 1, "alpha": 2, "local": 3}), []string{"alpha", "local", "zulu"}; !equalStrings(got, want) {
		t.Fatalf("sortedClusterNames() = %v, want %v", got, want)
	}

	scheme := runtimeAuditScheme(t)
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testIngress("local-ingress")).Build()
	alphaClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testIngress("alpha-ingress")).Build()
	zuluClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testIngress("zulu-ingress")).Build()

	manager := NewClusterManager(localClient, scheme)
	manager.clients = map[string]*ClusterClient{
		"zulu":  {Name: "zulu", Client: zuluClient, Connected: true, ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "zulu"}},
		"local": {Name: localClusterName, Client: localClient, Connected: true},
		"alpha": {Name: "alpha", Client: alphaClient, Connected: true, ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "alpha"}},
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{
				{Name: "zulu", Enabled: true},
				{Name: "alpha", Enabled: true},
			},
		},
	}

	reconciler := &DashboardReconciler{ClusterManager: manager}
	allIngresses, _, err := reconciler.getMultiClusterFilteredIngresses(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("getMultiClusterFilteredIngresses() returned error: %v", err)
	}

	gotNames := make([]string, 0, len(allIngresses.Items))
	for _, ingress := range allIngresses.Items {
		gotNames = append(gotNames, ingress.Name)
	}
	if want := []string{"alpha-ingress", "local-ingress", "zulu-ingress"}; !equalStrings(gotNames, want) {
		t.Fatalf("aggregated ingress order = %v, want %v", gotNames, want)
	}

	manager.clients["zulu"].LastError = errors.New("unavailable")
	manager.clients["zulu"].Connected = false
	statuses := manager.GetClusterStatuses()
	if got, want := []string{statuses[0].Name, statuses[1].Name}, []string{"alpha", "zulu"}; !equalStrings(got, want) {
		t.Fatalf("cluster status order = %v, want %v", got, want)
	}
}

func TestDiscoveryPermissionErrorDoesNotDisconnectOtherResourceTypes(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps", Labels: map[string]string{"discover": "yes"}},
	}).Build()
	remoteClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		List: func(ctx context.Context, underlying client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, denied := list.(*networkingv1.IngressList); denied {
				return apierrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "ingresses"}, "all", errors.New("RBAC denied"))
			}
			return underlying.List(ctx, list, opts...)
		},
	})
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"discover": "yes"}}
	manager := NewClusterManager(baseClient, scheme)
	manager.clients = map[string]*ClusterClient{
		"remote": {
			Name: "remote", Client: remoteClient, Connected: true,
			ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote", ServiceSelector: selector},
		},
	}
	dashboard := &homerv1alpha1.Dashboard{Spec: homerv1alpha1.DashboardSpec{ServiceSelector: selector}}

	if _, err := manager.DiscoverIngresses(context.Background(), dashboard); err != nil {
		t.Fatalf("DiscoverIngresses() error = %v", err)
	}
	if !manager.clients["remote"].Connected {
		t.Fatal("Ingress permission error disconnected the remote cluster")
	}

	services, err := manager.DiscoverServices(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("DiscoverServices() error = %v", err)
	}
	if got := len(services["remote"]); got != 1 {
		t.Fatalf("remote Services = %d, want one permitted Service", got)
	}
}

func TestClusterManagerRetriesDisconnectedConnectionWithUnchangedSecret(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-kubeconfig", Namespace: "default"},
		Data:       map[string][]byte{"kubeconfig": []byte("not parsed by the injected connector")},
	}).Build()

	manager := NewClusterManager(localClient, scheme)
	attempts := 0
	manager.createClusterClientFn = func(_ context.Context, _ *homerv1alpha1.Dashboard, cfg *homerv1alpha1.RemoteCluster) (*ClusterClient, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary remote API failure")
		}
		return &ClusterClient{
			Name:       cfg.Name,
			Client:     localClient,
			Connected:  true,
			ClusterCfg: cfg,
		}, nil
	}

	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{RemoteClusters: []homerv1alpha1.RemoteCluster{{
			Name: "remote", Enabled: true,
			SecretRef: homerv1alpha1.KubeconfigSecretRef{Name: "remote-kubeconfig"},
		}}},
	}

	if err := manager.UpdateClusters(context.Background(), dashboard); err != nil {
		t.Fatalf("first UpdateClusters() returned error: %v", err)
	}
	if manager.clients["remote"].Connected {
		t.Fatal("remote cluster should be disconnected after the first failed attempt")
	}

	if err := manager.UpdateClusters(context.Background(), dashboard); err != nil {
		t.Fatalf("second UpdateClusters() returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("connector attempts = %d, want 2", attempts)
	}
	if !manager.clients["remote"].Connected {
		t.Fatal("remote cluster should reconnect on the second reconcile")
	}
}

func TestDisabledRemoteClusterRemainsInStatusAsDisconnected(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := NewClusterManager(localClient, scheme)
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{RemoteClusters: []homerv1alpha1.RemoteCluster{{
			Name: "disabled", Enabled: false,
		}}},
	}

	if err := manager.UpdateClusters(context.Background(), dashboard); err != nil {
		t.Fatalf("UpdateClusters() returned error: %v", err)
	}
	statuses := manager.GetClusterStatuses()
	if len(statuses) != 1 || statuses[0].Name != "disabled" || statuses[0].Connected {
		t.Fatalf("disabled cluster status = %#v, want one disconnected configured cluster", statuses)
	}
}

func TestClusterManagerHealthProbeUsesVersionWithoutNamespaceList(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		if request.URL.Path != "/version" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gitVersion":"v1.30.0"}`))
	}))
	defer server.Close()

	manager := &ClusterManager{}
	if err := manager.testConnection(context.Background(), &rest.Config{Host: server.URL}); err != nil {
		t.Fatalf("testConnection() returned error: %v", err)
	}
	if !equalStrings(paths, []string{"/version"}) {
		t.Fatalf("health probe paths = %v, want only /version", paths)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "remote unavailable", http.StatusServiceUnavailable)
	}))
	defer errorServer.Close()
	if err := manager.testConnection(context.Background(), &rest.Config{Host: errorServer.URL}); err == nil || !strings.Contains(err.Error(), "currently unable to handle the request") {
		t.Fatalf("testConnection() error = %v, want preserved probe error", err)
	}
}

func TestEffectiveExternalConfigControlsValidationAndPWADefaults(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "default"},
		Data:       map[string]string{"config.yml": "title: External title\nsubtitle: External subtitle\n"},
	}).Build()
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			ConfigMap:   homerv1alpha1.ConfigMap{Name: "external"},
			HomerConfig: homer.HomerConfig{Defaults: homer.DefaultConfig{Layout: "invalid-inline-value"}},
			Assets:      &homerv1alpha1.AssetsConfig{PWA: &homerv1alpha1.PWAConfig{Enabled: true}},
		},
	}
	reconciler := &DashboardReconciler{Client: client}
	effective, err := reconciler.buildHomerConfig(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("buildHomerConfig() returned error: %v", err)
	}
	if err := reconciler.validateDashboardConfig(effective); err != nil {
		t.Fatalf("stale inline config was validated instead of external config: %v", err)
	}

	manifest := reconciler.generatePWAManifest(dashboard, effective)
	var decoded struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		StartURL    string `json:"start_url"`
		Scope       string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(manifest), &decoded); err != nil {
		t.Fatalf("PWA manifest is not valid JSON: %v", err)
	}
	if decoded.Name != "External title" || decoded.Description != "External subtitle" || decoded.StartURL != "../" || decoded.Scope != "../" {
		t.Fatalf("PWA defaults from effective config = %#v", decoded)
	}
}

func TestPWARelativeCustomIconsRequireAssetStaging(t *testing.T) {
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{Assets: &homerv1alpha1.AssetsConfig{
			Icons: &homerv1alpha1.IconConfig{PWAIcon192: "custom-192.png", PWAIcon512: "custom-512.png"},
			PWA:   &homerv1alpha1.PWAConfig{Enabled: true},
		}},
	}
	manifest := (&DashboardReconciler{}).generatePWAManifest(dashboard, &homer.HomerConfig{Title: "Dashboard"})
	if strings.Contains(manifest, "custom-192.png") || strings.Contains(manifest, "custom-512.png") || !strings.Contains(manifest, "icons/pwa-192x192.png") || !strings.Contains(manifest, "icons/pwa-512x512.png") {
		t.Fatalf("unstaged relative PWA icon was emitted or defaults were lost: %s", manifest)
	}

	dashboard.Spec.Assets.Icons.PWAIcon512 = "https://cdn.example/512.png"
	dashboard.Spec.Assets.ConfigMapRef = &homerv1alpha1.AssetConfigMapRef{Name: "assets"}
	manifest = (&DashboardReconciler{}).generatePWAManifest(dashboard, &homer.HomerConfig{Title: "Dashboard"})
	if !strings.Contains(manifest, "icons/pwa-192x192.png") || !strings.Contains(manifest, "https://cdn.example/512.png") {
		t.Fatalf("staged/absolute PWA icons were not emitted: %s", manifest)
	}
}

func TestCreateOrUpdateResourcesPreservesServiceFieldsAndResourceVersion(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	policy := corev1.IPFamilyPolicySingleStack
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "dashboard-homer",
			Namespace:       "default",
			ResourceVersion: "42",
		},
		Spec: corev1.ServiceSpec{
			Type:           corev1.ServiceTypeLoadBalancer,
			ClusterIP:      "10.0.0.10",
			ClusterIPs:     []string{"10.0.0.10"},
			IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol},
			IPFamilyPolicy: &policy,
			Selector:       map[string]string{"old": "selector"},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstrFromInt(8080),
				NodePort:   30080,
			}},
		},
	}

	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	stored := &corev1.Service{}
	if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(existing), stored); err != nil {
		t.Fatal(err)
	}

	var updated *corev1.Service
	wrappedClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		Update: func(ctx context.Context, underlying client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if service, ok := obj.(*corev1.Service); ok {
				updated = service.DeepCopy()
			}
			return underlying.Update(ctx, obj, opts...)
		},
	})

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: existing.Name, Namespace: existing.Namespace},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"dashboard": "dashboard"},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstrFromInt(8080),
			}},
		},
	}

	reconciler := &DashboardReconciler{Client: wrappedClient}
	if err := reconciler.createOrUpdateResources(context.Background(), []client.Object{desired}, "dashboard"); err != nil {
		t.Fatalf("createOrUpdateResources() returned error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected Service Update to be called")
	}
	if updated.ResourceVersion != stored.ResourceVersion {
		t.Fatalf("updated resourceVersion = %q, want %q", updated.ResourceVersion, stored.ResourceVersion)
	}
	if updated.Spec.Type != existing.Spec.Type || updated.Spec.ClusterIP != existing.Spec.ClusterIP || updated.Spec.Ports[0].NodePort != 30080 {
		t.Fatalf("server-assigned Service fields were not preserved: %#v", updated.Spec)
	}
	if updated.Spec.Selector["dashboard"] != "dashboard" {
		t.Fatalf("desired selector was not applied: %#v", updated.Spec.Selector)
	}
}

func TestUpdateStatusPropagatesNonNotFoundError(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(dashboard).
		WithObjects(dashboard).
		Build()
	statusErr := errors.New("status API unavailable")
	wrappedClient := failingStatusClient{Client: baseClient, err: statusErr}

	reconciler := &DashboardReconciler{Client: wrappedClient}
	err := reconciler.updateStatus(context.Background(), dashboard, nil)
	if !errors.Is(err, statusErr) {
		t.Fatalf("updateStatus() error = %v, want %v", err, statusErr)
	}
}

type failingStatusClient struct {
	client.Client
	err error
}

func (c failingStatusClient) Status() client.SubResourceWriter {
	return failingStatusWriter{err: c.err}
}

type failingStatusWriter struct {
	err error
}

func (w failingStatusWriter) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return w.err
}

func (w failingStatusWriter) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return w.err
}

func (w failingStatusWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return w.err
}

func TestMultiClusterCreateConfigMapDoesNotApplyLocalHTTPRouteFiltersRemotely(t *testing.T) {
	scheme := runtimeAuditScheme(t)
	localRoute := testHTTPRoute("local-route", "local.example.com")
	remoteRoute := testHTTPRoute("remote-route", "remote.example.com")
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localRoute).Build()
	remoteClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(remoteRoute).Build()

	manager := NewClusterManager(localClient, scheme)
	manager.clients = map[string]*ClusterClient{
		localClusterName: {Name: localClusterName, Client: localClient, Connected: true},
		"remote": {
			Name: "remote", Client: remoteClient, Connected: true,
			ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote"},
		},
	}
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			DomainFilters:  []string{"local.example.com"},
			RemoteClusters: []homerv1alpha1.RemoteCluster{{Name: "remote", Enabled: true}},
			HomerConfig:    homer.HomerConfig{Title: "Dashboard"},
		},
	}
	reconciler := &DashboardReconciler{
		Client:           localClient,
		ClusterManager:   manager,
		EnableGatewayAPI: true,
	}

	configMap, _, err := reconciler.createConfigMap(context.Background(), &homer.HomerConfig{Title: "Dashboard"}, dashboard, networkingv1.IngressList{}, nil)
	if err != nil {
		t.Fatalf("createConfigMap() returned error: %v", err)
	}
	yamlConfig := configMap.Data["config.yml"]
	if !strings.Contains(yamlConfig, "local-route") {
		t.Fatalf("local route was unexpectedly filtered: %s", yamlConfig)
	}
	if !strings.Contains(yamlConfig, "remote-route") {
		t.Fatalf("remote route outside the local domain filter was filtered: %s", yamlConfig)
	}
}

func testIngress(name string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: name + ".example.com"}}},
	}
}

func testHTTPRoute(name, hostname string) *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(hostname)}},
	}
}

func intstrFromInt(value int) intstr.IntOrString {
	return intstr.FromInt(value)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
