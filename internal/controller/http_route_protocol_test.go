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

func TestSetHTTPRouteProtocolUsesSelectedGatewayListener(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	sectionName := gatewayv1.SectionName("tls")
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{
				Name:        "public",
				SectionName: &sectionName,
			}}},
			Hostnames: []gatewayv1.Hostname{"app.example.com"},
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{
			Name:     sectionName,
			Protocol: gatewayv1.HTTPSProtocolType,
			Port:     443,
		}}},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway).Build()

	setHTTPRouteProtocol(context.Background(), reader, route)

	if got := route.Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTPS {
		t.Fatalf("resolved protocol = %q, want %q", got, homer.ProtocolHTTPS)
	}
}

func TestSetHTTPRouteProtocolPreservesResolvedProtocolWhenGatewayIsUnavailable(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				homer.HTTPRouteProtocolAnnotation: homer.ProtocolHTTPS,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{Name: "remote-only"}}},
		},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).Build()

	setHTTPRouteProtocol(context.Background(), reader, route)

	if got := route.Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTPS {
		t.Fatalf("resolved protocol = %q, want existing protocol %q", got, homer.ProtocolHTTPS)
	}
}

func TestSetHTTPRouteProtocolHonorsParentRefPort(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	port := gatewayv1.PortNumber(80)
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{
				Name: "public",
				Port: &port,
			}}},
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{
			{Name: "web", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
			{Name: "tls", Protocol: gatewayv1.HTTPSProtocolType, Port: 443},
		}},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway).Build()

	setHTTPRouteProtocol(context.Background(), reader, route)

	if got := route.Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTP {
		t.Fatalf("resolved protocol = %q, want %q for parentRef port 80", got, homer.ProtocolHTTP)
	}
}

func TestSetHTTPRouteProtocolDoesNotGuessFromHostname(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{Name: "public"}}},
			Hostnames:       []gatewayv1.Hostname{"app.example.com"},
		},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{
			Name:     "web",
			Protocol: gatewayv1.HTTPProtocolType,
			Port:     80,
		}}},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway).Build()

	setHTTPRouteProtocol(context.Background(), reader, route)

	if got := route.Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTP {
		t.Fatalf("resolved protocol = %q, want %q", got, homer.ProtocolHTTP)
	}
}

func TestSetHTTPRouteProtocolHonorsAllowedRouteNamespaces(t *testing.T) {
	tests := []struct {
		name             string
		from             gatewayv1.FromNamespaces
		useAllowedRoutes bool
		namespaceLabels  map[string]string
		selector         *metav1.LabelSelector
		wantProtocol     string
		includeNamespace bool
	}{
		{
			name:         "default same-namespace policy rejects cross-namespace route",
			from:         gatewayv1.NamespacesFromSame,
			wantProtocol: "",
		},
		{
			name:             "all namespaces permits cross-namespace route",
			from:             gatewayv1.NamespacesFromAll,
			useAllowedRoutes: true,
			wantProtocol:     homer.ProtocolHTTPS,
		},
		{
			name:             "selector permits a labeled route namespace",
			from:             gatewayv1.NamespacesFromSelector,
			useAllowedRoutes: true,
			namespaceLabels:  map[string]string{"gateway-access": "true"},
			selector:         &metav1.LabelSelector{MatchLabels: map[string]string{"gateway-access": "true"}},
			wantProtocol:     homer.ProtocolHTTPS,
			includeNamespace: true,
		},
		{
			name:             "selector rejects an unlabeled route namespace",
			from:             gatewayv1.NamespacesFromSelector,
			useAllowedRoutes: true,
			selector:         &metav1.LabelSelector{MatchLabels: map[string]string{"gateway-access": "true"}},
			wantProtocol:     "",
			includeNamespace: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := gatewayv1.Install(scheme); err != nil {
				t.Fatalf("add Gateway API to scheme: %v", err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("add core API to scheme: %v", err)
			}

			parentNamespace := gatewayv1.Namespace("gateway-system")
			parentRef := gatewayv1.ParentReference{Name: "public", Namespace: &parentNamespace}
			var allowedRoutes *gatewayv1.AllowedRoutes
			if tt.useAllowedRoutes {
				from := tt.from
				allowedRoutes = &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{
					From:     &from,
					Selector: tt.selector,
				}}
			}
			gateway := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "gateway-system"},
				Spec: gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{
					Name:          "tls",
					Protocol:      gatewayv1.HTTPSProtocolType,
					Port:          443,
					AllowedRoutes: allowedRoutes,
				}}},
			}
			route := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "apps"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parentRef}},
				},
				Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{
					Parents: []gatewayv1.RouteParentStatus{acceptedRouteParentStatus(parentRef)},
				}},
			}

			objects := []runtime.Object{gateway, route}
			if tt.includeNamespace {
				objects = append(objects, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "apps", Labels: tt.namespaceLabels}})
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()

			setHTTPRouteProtocol(context.Background(), reader, route)

			got := ""
			if route.Annotations != nil {
				got = route.Annotations[homer.HTTPRouteProtocolAnnotation]
			}
			if got != tt.wantProtocol {
				t.Fatalf("resolved protocol = %q, want %q", got, tt.wantProtocol)
			}
		})
	}
}

func TestSetHTTPRouteProtocolRejectsExplicitlyUnacceptedParent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	parentRef := gatewayv1.ParentReference{Name: "public"}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app",
			Namespace: "default",
			Annotations: map[string]string{
				homer.HTTPRouteProtocolAnnotation: homer.ProtocolHTTPS,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parentRef}}},
		Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{
			rejectedRouteParentStatus(parentRef),
		}}},
	}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{
			Name:     "tls",
			Protocol: gatewayv1.HTTPSProtocolType,
			Port:     443,
		}}},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway).Build()

	setHTTPRouteProtocol(context.Background(), reader, route)

	if _, exists := route.Annotations[homer.HTTPRouteProtocolAnnotation]; exists {
		t.Fatalf("rejected route retained protocol annotation %q", route.Annotations[homer.HTTPRouteProtocolAnnotation])
	}
}

func TestSetHTTPRouteProtocolPrefersHTTPSAcrossParentsRegardlessOfOrder(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	parentHTTP := gatewayv1.ParentReference{Name: "public-http"}
	parentHTTPS := gatewayv1.ParentReference{Name: "public-https"}
	objects := []runtime.Object{
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "public-http", Namespace: "default"},
			Spec:       gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{Name: "web", Protocol: gatewayv1.HTTPProtocolType, Port: 80}}},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "public-https", Namespace: "default"},
			Spec:       gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{Name: "tls", Protocol: gatewayv1.HTTPSProtocolType, Port: 443}}},
		},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()

	for _, parentRefs := range [][]gatewayv1.ParentReference{{parentHTTP, parentHTTPS}, {parentHTTPS, parentHTTP}} {
		route := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec:       gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: parentRefs}},
			Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{
				acceptedRouteParentStatus(parentHTTP),
				acceptedRouteParentStatus(parentHTTPS),
			}}},
		}

		setHTTPRouteProtocol(context.Background(), reader, route)

		if got := route.Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTPS {
			t.Fatalf("parent order %v resolved protocol = %q, want %q", parentRefs, got, homer.ProtocolHTTPS)
		}
	}
}

func TestSetHTTPRouteProtocolPrefersHTTPSAcrossListenersRegardlessOfOrder(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	parentRef := gatewayv1.ParentReference{Name: "public"}
	listenerOrders := [][]gatewayv1.Listener{
		{
			{Name: "web", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
			{Name: "tls", Protocol: gatewayv1.HTTPSProtocolType, Port: 443},
		},
		{
			{Name: "tls", Protocol: gatewayv1.HTTPSProtocolType, Port: 443},
			{Name: "web", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
		},
	}

	for _, listeners := range listenerOrders {
		gateway := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
			Spec:       gatewayv1.GatewaySpec{Listeners: listeners},
		}
		route := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec:       gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parentRef}}},
			Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{
				acceptedRouteParentStatus(parentRef),
			}}},
		}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gateway).Build()

		setHTTPRouteProtocol(context.Background(), reader, route)

		if got := route.Annotations[homer.HTTPRouteProtocolAnnotation]; got != homer.ProtocolHTTPS {
			t.Fatalf("listener order %v resolved protocol = %q, want %q", listeners, got, homer.ProtocolHTTPS)
		}
	}
}

func TestDashboardCreateConfigMapPreservesRemoteHTTPRouteProtocol(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	parentRef := gatewayv1.ParentReference{Name: "shared-gateway"}
	remoteRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-app", Namespace: "default"},
		Spec:       gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{"remote.example.com"}, CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parentRef}}},
		Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{
			acceptedRouteParentStatus(parentRef),
		}}},
	}
	remoteGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-gateway", Namespace: "default"},
		Spec:       gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{Name: "tls", Protocol: gatewayv1.HTTPSProtocolType, Port: 443}}},
	}
	localGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-gateway", Namespace: "default"},
		Spec:       gatewayv1.GatewaySpec{Listeners: []gatewayv1.Listener{{Name: "web", Protocol: gatewayv1.HTTPProtocolType, Port: 80}}},
	}

	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localGateway).Build()
	remoteClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(remoteGateway, remoteRoute).Build()
	manager := NewClusterManager(localClient, scheme)
	manager.clients = map[string]*ClusterClient{
		localClusterName: {Name: localClusterName, Client: localClient, Connected: true},
		"remote": {
			Name: "remote", Client: remoteClient, Connected: true,
			ClusterCfg: &homerv1alpha1.RemoteCluster{Name: "remote", Enabled: true},
		},
	}
	reconciler := &DashboardReconciler{Client: localClient, ClusterManager: manager, EnableGatewayAPI: true}
	dashboard := &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard", Namespace: "default"},
		Spec: homerv1alpha1.DashboardSpec{
			RemoteClusters: []homerv1alpha1.RemoteCluster{{Name: "remote", Enabled: true}},
			HomerConfig:    homer.HomerConfig{Title: "Dashboard"},
		},
	}

	configMap, _, err := reconciler.createConfigMap(context.Background(), &homer.HomerConfig{Title: "Dashboard"}, dashboard, networkingv1.IngressList{}, nil)
	if err != nil {
		t.Fatalf("createConfigMap() returned error: %v", err)
	}
	config := configMap.Data["config.yml"]
	if !strings.Contains(config, "https://remote.example.com") {
		t.Fatalf("remote HTTPS route was not preserved in generated config: %s", config)
	}
	if strings.Contains(config, "http://remote.example.com") {
		t.Fatalf("remote HTTPS route was overwritten by local Gateway: %s", config)
	}
}

func acceptedRouteParentStatus(parentRef gatewayv1.ParentReference) gatewayv1.RouteParentStatus {
	return routeParentStatus(parentRef, metav1.ConditionTrue)
}

func rejectedRouteParentStatus(parentRef gatewayv1.ParentReference) gatewayv1.RouteParentStatus {
	return routeParentStatus(parentRef, metav1.ConditionFalse)
}

func routeParentStatus(parentRef gatewayv1.ParentReference, status metav1.ConditionStatus) gatewayv1.RouteParentStatus {
	return gatewayv1.RouteParentStatus{
		ParentRef:      parentRef,
		ControllerName: gatewayv1.GatewayController("example.com/controller"),
		Conditions: []metav1.Condition{{
			Type:   string(gatewayv1.RouteConditionAccepted),
			Status: status,
		}},
	}
}
