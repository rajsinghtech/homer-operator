package controller

import (
	"context"
	"strings"

	"github.com/rajsinghtech/homer-operator/pkg/homer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const httpRouteClusterAnnotation = "homer.rajsingh.info/cluster"

// setHTTPRouteProtocol resolves the protocol from the Gateway listener that
// accepts the HTTPRoute. The route is a copy during full dashboard rebuilds,
// and is only annotated in memory; the source HTTPRoute is never persisted.
func setHTTPRouteProtocol(ctx context.Context, reader client.Reader, route *gatewayv1.HTTPRoute) {
	if route == nil {
		return
	}

	previousProtocol, hadPreviousProtocol := httpRouteProtocolAnnotation(route)
	protocol, err := resolveHTTPRouteProtocol(ctx, reader, route)
	if err != nil {
		if hadPreviousProtocol && !httpRouteHasRejectedParent(route) {
			log.FromContext(ctx).V(1).Info("could not resolve HTTPRoute Gateway protocol; retaining existing protocol",
				"httproute", route.Namespace+"/"+route.Name, "protocol", previousProtocol, "error", err)
			return
		}
		log.FromContext(ctx).V(1).Info("could not resolve HTTPRoute Gateway protocol; defaulting to HTTP",
			"httproute", route.Namespace+"/"+route.Name, "error", err)
		protocol = ""
	}
	if protocol == "" {
		if hadPreviousProtocol && !httpRouteHasRejectedParent(route) {
			return
		}
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
	protocol := ""
	for _, parentRef := range route.Spec.ParentRefs {
		if !isGatewayParentReference(parentRef) {
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

		candidate, ok, err := protocolFromGatewayListeners(ctx, reader, gateway, route, parentRef)
		if err != nil {
			return "", err
		}
		if ok {
			protocol = preferredHTTPRouteProtocol(protocol, candidate)
		}
	}

	return protocol, nil
}

func protocolFromGatewayListeners(ctx context.Context, reader client.Reader, gateway *gatewayv1.Gateway, route *gatewayv1.HTTPRoute, parentRef gatewayv1.ParentReference) (string, bool, error) {
	accepted, statusKnown := httpRouteParentAccepted(route, parentRef)
	if statusKnown && !accepted {
		return "", false, nil
	}

	// A parent reference can match more than one listener. Prefer HTTPS when
	// multiple compatible listeners accept the route, regardless of list order.
	protocol := ""
	for _, listener := range gateway.Spec.Listeners {
		matches, err := gatewayListenerMatchesParent(ctx, reader, gateway, listener, route, parentRef)
		if err != nil {
			return "", false, err
		}
		if !matches {
			continue
		}
		candidate, ok := protocolFromGatewayListener(listener)
		if !ok {
			continue
		}
		protocol = preferredHTTPRouteProtocol(protocol, candidate)
	}
	return protocol, protocol != ""
}

func preferredHTTPRouteProtocol(current, candidate string) string {
	if candidate == homer.ProtocolHTTPS || current == "" {
		return candidate
	}
	return current
}

func gatewayListenerMatchesParent(ctx context.Context, reader client.Reader, gateway *gatewayv1.Gateway, listener gatewayv1.Listener, route *gatewayv1.HTTPRoute, parentRef gatewayv1.ParentReference) (bool, error) {
	if parentRef.SectionName != nil && listener.Name != *parentRef.SectionName {
		return false, nil
	}
	if parentRef.Port != nil && listener.Port != *parentRef.Port {
		return false, nil
	}
	if !gatewayListenerMatchesRoute(listener, route) {
		return false, nil
	}
	allowed, err := gatewayListenerAllowsRoute(ctx, reader, gateway, listener, route)
	if err != nil || !allowed {
		return false, err
	}
	return true, nil
}

func gatewayListenerAllowsRoute(ctx context.Context, reader client.Reader, gateway *gatewayv1.Gateway, listener gatewayv1.Listener, route *gatewayv1.HTTPRoute) (bool, error) {
	if listener.AllowedRoutes == nil || listener.AllowedRoutes.Namespaces == nil {
		return route.Namespace == gateway.Namespace, nil
	}

	namespaces := listener.AllowedRoutes.Namespaces
	from := gatewayv1.NamespacesFromSame
	if namespaces.From != nil {
		from = *namespaces.From
	}

	switch from {
	case gatewayv1.NamespacesFromAll:
		return true, nil
	case gatewayv1.NamespacesFromSelector:
		if namespaces.Selector == nil {
			return false, nil
		}
		selector, err := metav1.LabelSelectorAsSelector(namespaces.Selector)
		if err != nil {
			return false, err
		}
		namespace := &corev1.Namespace{}
		if err := reader.Get(ctx, client.ObjectKey{Name: route.Namespace}, namespace); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return selector.Matches(labels.Set(namespace.Labels)), nil
	case gatewayv1.NamespacesFromNone:
		return false, nil
	case gatewayv1.NamespacesFromSame:
		fallthrough
	default:
		return route.Namespace == gateway.Namespace, nil
	}
}

func httpRouteProtocolAnnotation(route *gatewayv1.HTTPRoute) (string, bool) {
	if route.Annotations == nil {
		return "", false
	}

	protocol := strings.ToLower(strings.TrimSpace(route.Annotations[homer.HTTPRouteProtocolAnnotation]))
	return protocol, protocol == homer.ProtocolHTTP || protocol == homer.ProtocolHTTPS
}

func shouldResolveHTTPRouteProtocolLocally(route *gatewayv1.HTTPRoute) bool {
	if route == nil || route.Annotations == nil {
		return true
	}
	cluster := strings.TrimSpace(route.Annotations[httpRouteClusterAnnotation])
	return cluster == "" || cluster == localClusterName
}

func isGatewayParentReference(parentRef gatewayv1.ParentReference) bool {
	if parentRef.Group != nil && string(*parentRef.Group) != gatewayv1.GroupName {
		return false
	}
	return parentRef.Kind == nil || string(*parentRef.Kind) == gatewayKind
}

func httpRouteParentAccepted(route *gatewayv1.HTTPRoute, parentRef gatewayv1.ParentReference) (bool, bool) {
	for _, parentStatus := range route.Status.Parents {
		if !parentReferencesEqual(parentStatus.ParentRef, parentRef, route.Namespace) {
			continue
		}

		for _, condition := range parentStatus.Conditions {
			if condition.Type == string(gatewayv1.RouteConditionAccepted) {
				return condition.Status == metav1.ConditionTrue, true
			}
		}
		return false, true
	}
	return true, false
}

func httpRouteHasRejectedParent(route *gatewayv1.HTTPRoute) bool {
	for _, parentRef := range route.Spec.ParentRefs {
		if !isGatewayParentReference(parentRef) {
			continue
		}
		accepted, statusKnown := httpRouteParentAccepted(route, parentRef)
		if statusKnown && !accepted {
			return true
		}
	}
	return false
}

func parentReferencesEqual(left, right gatewayv1.ParentReference, defaultNamespace string) bool {
	if string(left.Name) != string(right.Name) ||
		parentReferenceGroup(left) != parentReferenceGroup(right) ||
		parentReferenceKind(left) != parentReferenceKind(right) ||
		parentReferenceNamespace(left, defaultNamespace) != parentReferenceNamespace(right, defaultNamespace) {
		return false
	}
	return optionalSectionNamesEqual(left.SectionName, right.SectionName) && optionalPortsEqual(left.Port, right.Port)
}

func parentReferenceGroup(parentRef gatewayv1.ParentReference) string {
	if parentRef.Group == nil {
		return gatewayv1.GroupName
	}
	return string(*parentRef.Group)
}

func parentReferenceKind(parentRef gatewayv1.ParentReference) string {
	if parentRef.Kind == nil {
		return gatewayKind
	}
	return string(*parentRef.Kind)
}

func parentReferenceNamespace(parentRef gatewayv1.ParentReference, defaultNamespace string) string {
	if parentRef.Namespace == nil || string(*parentRef.Namespace) == "" {
		return defaultNamespace
	}
	return string(*parentRef.Namespace)
}

func optionalSectionNamesEqual(left, right *gatewayv1.SectionName) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func optionalPortsEqual(left, right *gatewayv1.PortNumber) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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
