package controller

import (
	"context"
	"testing"

	"github.com/rajsinghtech/homer-operator/pkg/homer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestSetHTTPRouteProtocolUsesSelectedGatewayListener(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	sectionName := gatewayv1.SectionName("tls")
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{ParentRefs: []gatewayv1.ParentReference{{
			Name:        "public",
			SectionName: &sectionName,
		}}, Hostnames: []gatewayv1.Hostname{"app.example.com"}},
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

func TestSetHTTPRouteProtocolDoesNotGuessFromHostname(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := gatewayv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Gateway API to scheme: %v", err)
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{{Name: "public"}},
			Hostnames:  []gatewayv1.Hostname{"app.example.com"},
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
