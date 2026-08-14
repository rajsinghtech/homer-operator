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
	"strings"
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

const (
	provenanceTestNamespace = "shared"
	localNamespaceDefault   = "local-namespace-default"
	localServiceDefault     = "local-services"
	sourceItemDefault       = "source-item"
	sourceServiceDefault    = "source-services"
)

func TestCreateConfigMapLocalHTTPRouteIgnoresSpoofedRemoteProvenance(t *testing.T) {
	scheme := provenanceTestScheme(t)
	parentRef := gatewayv1.ParentReference{Name: "shared-gateway"}

	localRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-app",
			Namespace: "default",
			Annotations: map[string]string{
				httpRouteClusterAnnotation:        "remote",
				homer.HTTPRouteProtocolAnnotation: homer.ProtocolHTTPS,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parentRef}},
			Hostnames:       []gatewayv1.Hostname{"local.example.com"},
		},
	}
	localGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-gateway", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{GatewayClassName: "managed", Listeners: []gatewayv1.Listener{{
			Name:     "web",
			Protocol: gatewayv1.HTTPProtocolType,
			Port:     80,
		}}},
	}
	remoteClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localRoute, localGateway, testGatewayClass()).Build()

	manager := NewClusterManager(localClient, scheme)
	manager.clients = map[string]*ClusterClient{
		localClusterName: {Name: localClusterName, Client: localClient, Connected: true},
		"remote":         {Name: "remote", Client: remoteClient, Connected: true, ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote", Enabled: true}},
	}
	dashboard := provenanceTestDashboard()
	reconciler := &DashboardReconciler{
		Client:           localClient,
		ClusterManager:   manager,
		EnableGatewayAPI: true,
	}

	configMap, discovered, err := reconciler.createConfigMap(
		context.Background(),
		&homer.HomerConfig{Title: "Dashboard"},
		dashboard,
		networkingv1.IngressList{},
		nil,
	)
	if err != nil {
		t.Fatalf("createConfigMap() returned error: %v", err)
	}

	routes := discovered[localClusterName]
	if len(routes) != 1 {
		t.Fatalf("discovered local HTTPRoutes = %d, want 1", len(routes))
	}
	if got := routes[0].Annotations[httpRouteClusterAnnotation]; got != localClusterName {
		t.Fatalf("local HTTPRoute cluster annotation = %q, want %q", got, localClusterName)
	}
	if got := routes[0].Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTP {
		t.Fatalf("local HTTPRoute protocol = %q, want %q", got, homer.ProtocolHTTP)
	}

	config := configMap.Data["config.yml"]
	if !strings.Contains(config, "http://local.example.com") {
		t.Fatalf("generated config does not use the local Gateway's HTTP protocol: %s", config)
	}
	if strings.Contains(config, "https://local.example.com") {
		t.Fatalf("generated config retained the spoofed remote HTTPS protocol: %s", config)
	}
}

func TestCreateConfigMapRemoteHTTPRouteKeepsSourceNamespaceAnnotations(t *testing.T) {
	scheme := provenanceTestScheme(t)
	parentRef := gatewayv1.ParentReference{Name: "remote-gateway"}
	localNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: provenanceTestNamespace,
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":     "local-item",
				"item.homer.rajsingh.info/subtitle": localNamespaceDefault,
				"service.homer.rajsingh.info/name":  localServiceDefault,
			},
		},
	}
	remoteNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: provenanceTestNamespace,
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":     sourceItemDefault,
				"item.homer.rajsingh.info/subtitle": "source-subtitle",
				"service.homer.rajsingh.info/name":  sourceServiceDefault,
			},
		},
	}
	remoteRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-app", Namespace: provenanceTestNamespace},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parentRef}},
			Hostnames:       []gatewayv1.Hostname{"remote.example.com"},
		},
	}
	remoteGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-gateway", Namespace: provenanceTestNamespace},
		Spec: gatewayv1.GatewaySpec{GatewayClassName: "managed", Listeners: []gatewayv1.Listener{{
			Name:     "web",
			Protocol: gatewayv1.HTTPProtocolType,
			Port:     80,
		}}},
	}

	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localNamespace).Build()
	remoteClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(remoteNamespace, remoteRoute, remoteGateway, testGatewayClass()).Build()
	manager := NewClusterManager(localClient, scheme)
	manager.clients = map[string]*ClusterClient{
		localClusterName: {Name: localClusterName, Client: localClient, Connected: true},
		"remote":         {Name: "remote", Client: remoteClient, Connected: true, ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote", Enabled: true}},
	}
	reconciler := &DashboardReconciler{
		Client:           localClient,
		ClusterManager:   manager,
		EnableGatewayAPI: true,
	}

	configMap, discovered, err := reconciler.createConfigMap(
		context.Background(),
		&homer.HomerConfig{Title: "Dashboard"},
		provenanceTestDashboard(),
		networkingv1.IngressList{},
		nil,
	)
	if err != nil {
		t.Fatalf("createConfigMap() returned error: %v", err)
	}

	routes := discovered["remote"]
	if len(routes) != 1 {
		t.Fatalf("discovered remote HTTPRoutes = %d, want 1", len(routes))
	}
	assertSourceNamespaceAnnotations(t, routes[0].Annotations)

	config := configMap.Data["config.yml"]
	for _, want := range []string{sourceItemDefault, "source-subtitle", sourceServiceDefault} {
		if !strings.Contains(config, want) {
			t.Errorf("generated config does not contain source-cluster namespace value %q: %s", want, config)
		}
	}
	for _, unwanted := range []string{"local-item", localNamespaceDefault, localServiceDefault} {
		if strings.Contains(config, unwanted) {
			t.Errorf("generated config contains local namespace default %q for a remote HTTPRoute: %s", unwanted, config)
		}
	}
}

func TestCreateConfigMapRemoteIngressAndServiceIgnoreLocalNamespaceDefaults(t *testing.T) {
	scheme := provenanceTestScheme(t)
	localNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: provenanceTestNamespace,
			Annotations: map[string]string{
				"item.homer.rajsingh.info/subtitle": localNamespaceDefault,
				"service.homer.rajsingh.info/name":  localServiceDefault,
			},
		},
	}
	remoteNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: provenanceTestNamespace,
			Annotations: map[string]string{
				"item.homer.rajsingh.info/name":     sourceItemDefault,
				"item.homer.rajsingh.info/subtitle": "source-subtitle",
				"service.homer.rajsingh.info/name":  sourceServiceDefault,
			},
		},
	}
	remoteIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-ingress", Namespace: provenanceTestNamespace},
		Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "ingress.example.com"}}},
	}
	remoteService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remote-service",
			Namespace: provenanceTestNamespace,
			Labels:    map[string]string{"dashboard": "true"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}

	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localNamespace).Build()
	remoteClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(remoteNamespace, remoteIngress, remoteService).Build()
	manager := NewClusterManager(localClient, scheme)
	manager.clients = map[string]*ClusterClient{
		localClusterName: {Name: localClusterName, Client: localClient, Connected: true},
		"remote":         {Name: "remote", Client: remoteClient, Connected: true, ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote", Enabled: true}},
	}
	dashboard := provenanceTestDashboard()
	dashboard.Spec.ServiceSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"dashboard": "true"}}
	reconciler := &DashboardReconciler{Client: localClient, ClusterManager: manager}

	ingresses, err := manager.DiscoverIngresses(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("DiscoverIngresses() returned error: %v", err)
	}
	services, err := manager.DiscoverServices(context.Background(), dashboard)
	if err != nil {
		t.Fatalf("DiscoverServices() returned error: %v", err)
	}
	if len(ingresses["remote"]) != 1 || len(services["remote"]) != 1 {
		t.Fatalf("discovered remote resources = %d Ingresses, %d Services; want one of each", len(ingresses["remote"]), len(services["remote"]))
	}
	assertSourceNamespaceAnnotations(t, ingresses["remote"][0].Annotations)
	assertSourceNamespaceAnnotations(t, services["remote"][0].Annotations)

	configMap, _, err := reconciler.createConfigMap(
		context.Background(),
		&homer.HomerConfig{Title: "Dashboard"},
		dashboard,
		networkingv1.IngressList{Items: ingresses["remote"]},
		services["remote"],
	)
	if err != nil {
		t.Fatalf("createConfigMap() returned error: %v", err)
	}

	config := configMap.Data["config.yml"]
	for _, want := range []string{sourceItemDefault, "source-subtitle", sourceServiceDefault} {
		if !strings.Contains(config, want) {
			t.Errorf("generated config does not contain source-cluster namespace value %q: %s", want, config)
		}
	}
	for _, unwanted := range []string{localNamespaceDefault, localServiceDefault} {
		if strings.Contains(config, unwanted) {
			t.Fatalf("generated config contains the local namespace default %q for remote Ingress/Service resources: %s", unwanted, config)
		}
	}
}

func provenanceTestDashboard() *homerv1alpha1.Dashboard {
	return &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{{Name: "remote", Enabled: true}},
		},
	}
}

func provenanceTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	for name, addToScheme := range map[string]func(*runtime.Scheme) error{
		"core":       corev1.AddToScheme,
		"networking": networkingv1.AddToScheme,
		"homer":      homerv1alpha1.AddToScheme,
		"gateway":    gatewayv1.Install,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatalf("add %s API to scheme: %v", name, err)
		}
	}
	return scheme
}

func assertSourceNamespaceAnnotations(t *testing.T, annotations map[string]string) {
	t.Helper()
	for key, want := range map[string]string{
		"item.homer.rajsingh.info/name":     sourceItemDefault,
		"item.homer.rajsingh.info/subtitle": "source-subtitle",
		"service.homer.rajsingh.info/name":  sourceServiceDefault,
		httpRouteClusterAnnotation:          "remote",
	} {
		if got := annotations[key]; got != want {
			t.Errorf("annotation %q = %q, want source-cluster value %q", key, got, want)
		}
	}
}
