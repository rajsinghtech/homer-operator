package controller

import (
	"context"
	"strings"

	"github.com/rajsinghtech/homer-operator/pkg/homer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// setHTTPRouteProtocol resolves the protocol from the Gateway listener that
// accepts the HTTPRoute. The route is a copy during full dashboard rebuilds,
// and is only annotated in memory; the source HTTPRoute is never persisted.
func setHTTPRouteProtocol(ctx context.Context, reader client.Reader, route *gatewayv1.HTTPRoute) {
	if route == nil {
		return
	}

	protocol, err := resolveHTTPRouteProtocol(ctx, reader, route)
	if err != nil {
		log.FromContext(ctx).V(1).Info("could not resolve HTTPRoute Gateway protocol; defaulting to HTTP",
			"httproute", route.Namespace+"/"+route.Name, "error", err)
		protocol = ""
	}
	if protocol == "" {
		if route.Annotations != nil {
			delete(route.Annotations, homer.HTTPRouteProtocolAnnotation)
		}
		return
	}
	if route.Annotations == nil {
		route.Annotations = make(map[string]string)
	}
	route.Annotations[homer.HTTPRouteProtocolAnnotation] = protocol
}

func resolveHTTPRouteProtocol(ctx context.Context, reader client.Reader, route *gatewayv1.HTTPRoute) (string, error) {
	for _, parentRef := range route.Spec.ParentRefs {
		if parentRef.Kind != nil && string(*parentRef.Kind) != "Gateway" {
			continue
		}

		namespace := route.Namespace
		if parentRef.Namespace != nil {
			namespace = string(*parentRef.Namespace)
		}
		gateway := &gatewayv1.Gateway{}
		if err := reader.Get(ctx, client.ObjectKey{Name: string(parentRef.Name), Namespace: namespace}, gateway); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return "", err
		}

		if protocol, ok := protocolFromGatewayListeners(gateway, route, parentRef.SectionName); ok {
			return protocol, nil
		}
	}

	return "", nil
}

func protocolFromGatewayListeners(gateway *gatewayv1.Gateway, route *gatewayv1.HTTPRoute, sectionName *gatewayv1.SectionName) (string, bool) {
	if sectionName != nil {
		for _, listener := range gateway.Spec.Listeners {
			if listener.Name != *sectionName {
				continue
			}
			return protocolFromGatewayListener(listener)
		}
		return "", false
	}

	// A parent reference without a section can match more than one listener.
	// Prefer HTTPS when both protocols accept the route, and otherwise use the
	// only compatible HTTP(S) listener.
	protocol := ""
	for _, listener := range gateway.Spec.Listeners {
		if !gatewayListenerMatchesRoute(listener, route) {
			continue
		}
		candidate, ok := protocolFromGatewayListener(listener)
		if !ok {
			continue
		}
		if candidate == homer.ProtocolHTTPS {
			return candidate, true
		}
		protocol = candidate
	}
	return protocol, protocol != ""
}

func protocolFromGatewayListener(listener gatewayv1.Listener) (string, bool) {
	switch listener.Protocol {
	case gatewayv1.HTTPProtocolType:
		return homer.ProtocolHTTP, true
	case gatewayv1.HTTPSProtocolType:
		return homer.ProtocolHTTPS, true
	default:
		return "", false
	}
}

func gatewayListenerMatchesRoute(listener gatewayv1.Listener, route *gatewayv1.HTTPRoute) bool {
	if listener.Hostname == nil || strings.TrimSpace(string(*listener.Hostname)) == "" || len(route.Spec.Hostnames) == 0 {
		return true
	}

	for _, routeHostname := range route.Spec.Hostnames {
		if hostnamesOverlap(string(*listener.Hostname), string(routeHostname)) {
			return true
		}
	}
	return false
}

func hostnamesOverlap(left, right string) bool {
	left = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(left), "."))
	right = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(right), "."))
	if left == right {
		return true
	}
	if strings.HasPrefix(left, "*.") {
		return wildcardMatchesHost(left, right)
	}
	if strings.HasPrefix(right, "*.") {
		return wildcardMatchesHost(right, left)
	}
	return false
}

func wildcardMatchesHost(pattern, host string) bool {
	suffix := strings.TrimPrefix(pattern, "*")
	return strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(pattern, "*.")
}
