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
// Any existing marker is discarded first because it may have come from an
// untrusted resource annotation. A protocol is only retained when this call
// resolves it from the current Gateway state. Callers that need to distinguish
// a transient read failure should use setHTTPRouteProtocolWithError.
func setHTTPRouteProtocol(ctx context.Context, reader client.Reader, route *gatewayv1.HTTPRoute) {
	_ = setHTTPRouteProtocolWithError(ctx, reader, route)
}

func setHTTPRouteProtocolWithError(ctx context.Context, reader client.Reader, route *gatewayv1.HTTPRoute) error {
	if route == nil {
		return nil
	}

	if route.Annotations != nil {
		delete(route.Annotations, homer.HTTPRouteProtocolAnnotation)
	}
	protocol, err := resolveHTTPRouteProtocol(ctx, reader, route)
	if err != nil {
		log.FromContext(ctx).V(1).Info("could not resolve HTTPRoute Gateway protocol; defaulting to HTTP",
			"httproute", route.Namespace+"/"+route.Name, "error", err)
		return err
	}
	if protocol == "" {
		return nil
	}
	if route.Annotations == nil {
		route.Annotations = make(map[string]string)
	}
	route.Annotations[homer.HTTPRouteProtocolAnnotation] = protocol
	return nil
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

		controllerName, eligible, err := gatewayEligibility(ctx, reader, gateway)
		if err != nil {
			return "", err
		}
		if !eligible {
			continue
		}

		candidate, ok, err := protocolFromGatewayListeners(ctx, reader, gateway, route, parentRef, controllerName)
		if err != nil {
			return "", err
		}
		if ok {
			protocol = preferredHTTPRouteProtocol(protocol, candidate)
		}
	}

	return protocol, nil
}

func protocolFromGatewayListeners(ctx context.Context, reader client.Reader, gateway *gatewayv1.Gateway, route *gatewayv1.HTTPRoute, parentRef gatewayv1.ParentReference, controllerName gatewayv1.GatewayController) (string, bool, error) {
	accepted, statusKnown := httpRouteParentAccepted(route, parentRef, controllerName)
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
	return protocol, protocol != "", nil
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
	if len(gateway.Status.Listeners) > 0 {
		status, ok := gatewayListenerStatus(gateway, listener.Name)
		if !ok || !gatewayListenerStatusAllowsHTTPRoute(status, gateway.Generation) {
			return false, nil
		}
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

func gatewayEligibility(ctx context.Context, reader client.Reader, gateway *gatewayv1.Gateway) (gatewayv1.GatewayController, bool, error) {
	if !conditionsAllowGateway(gateway.Status.Conditions, gateway.Generation) {
		return "", false, nil
	}
	// GatewayClassName is required by the Gateway API. A malformed object must
	// not bypass GatewayClass ownership checks and authorize an HTTPS URL.
	if gateway.Spec.GatewayClassName == "" {
		return "", false, nil
	}

	gatewayClass := &gatewayv1.GatewayClass{}
	if err := reader.Get(ctx, client.ObjectKey{Name: string(gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
		if apierrors.IsNotFound(err) {
			// GatewayClass ownership is required to know which controller's
			// status is authoritative. A missing class is an authoritative
			// ineligible Gateway, while an unreadable class must be surfaced to the
			// caller rather than silently replacing a cached HTTPS route.
			return "", false, nil
		}
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return "", false, err
		}
		return "", false, err
	}
	acceptedConditionFound := false
	for _, condition := range gatewayClass.Status.Conditions {
		if condition.Type != string(gatewayv1.GatewayClassConditionStatusAccepted) {
			continue
		}
		if !conditionObservedGenerationCurrent(condition, gatewayClass.Generation) {
			return "", false, nil
		}
		acceptedConditionFound = true
		if condition.Status != metav1.ConditionTrue {
			return "", false, nil
		}
	}
	if !acceptedConditionFound || gatewayClass.Spec.ControllerName == "" {
		return "", false, nil
	}
	return gatewayClass.Spec.ControllerName, true, nil
}

func conditionsAllowGateway(conditions []metav1.Condition, generation int64) bool {
	for _, condition := range conditions {
		switch condition.Type {
		case string(gatewayv1.GatewayConditionAccepted), string(gatewayv1.GatewayConditionProgrammed), "Ready":
			if !conditionObservedGenerationCurrent(condition, generation) {
				return false
			}
			if condition.Status != metav1.ConditionTrue {
				return false
			}
		}
	}
	return true
}

func gatewayListenerStatus(gateway *gatewayv1.Gateway, name gatewayv1.SectionName) (gatewayv1.ListenerStatus, bool) {
	for _, status := range gateway.Status.Listeners {
		if status.Name == name {
			return status, true
		}
	}
	return gatewayv1.ListenerStatus{}, false
}

func gatewayListenerStatusAllowsHTTPRoute(status gatewayv1.ListenerStatus, generation int64) bool {
	for _, condition := range status.Conditions {
		switch condition.Type {
		case string(gatewayv1.ListenerConditionAccepted), string(gatewayv1.ListenerConditionResolvedRefs), string(gatewayv1.ListenerConditionProgrammed):
			if !conditionObservedGenerationCurrent(condition, generation) {
				return false
			}
			if condition.Status != metav1.ConditionTrue {
				return false
			}
		case string(gatewayv1.ListenerConditionConflicted):
			if !conditionObservedGenerationCurrent(condition, generation) {
				return false
			}
			if condition.Status != metav1.ConditionFalse {
				return false
			}
		}
	}
	if len(status.SupportedKinds) == 0 {
		return true
	}
	for _, routeKind := range status.SupportedKinds {
		group := gatewayv1.GroupName
		if routeKind.Group != nil {
			group = string(*routeKind.Group)
		}
		if group == gatewayv1.GroupName && string(routeKind.Kind) == "HTTPRoute" {
			return true
		}
	}
	return false
}

func gatewayListenerAllowsRoute(ctx context.Context, reader client.Reader, gateway *gatewayv1.Gateway, listener gatewayv1.Listener, route *gatewayv1.HTTPRoute) (bool, error) {
	if listener.AllowedRoutes != nil && len(listener.AllowedRoutes.Kinds) > 0 && !gatewayListenerAllowsHTTPRoute(listener) {
		return false, nil
	}
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

func gatewayListenerAllowsHTTPRoute(listener gatewayv1.Listener) bool {
	if listener.AllowedRoutes == nil || len(listener.AllowedRoutes.Kinds) == 0 {
		return true
	}

	for _, routeKind := range listener.AllowedRoutes.Kinds {
		group := gatewayv1.GroupName
		if routeKind.Group != nil {
			group = string(*routeKind.Group)
		}
		if group == gatewayv1.GroupName && string(routeKind.Kind) == "HTTPRoute" {
			return true
		}
	}
	return false
}

func isGatewayParentReference(parentRef gatewayv1.ParentReference) bool {
	if parentRef.Group != nil && string(*parentRef.Group) != gatewayv1.GroupName {
		return false
	}
	return parentRef.Kind == nil || string(*parentRef.Kind) == gatewayKind
}

func httpRouteParentAccepted(route *gatewayv1.HTTPRoute, parentRef gatewayv1.ParentReference, controllerName gatewayv1.GatewayController) (bool, bool) {
	matched := false
	accepted := false
	acceptedConditionFound := false
	for _, parentStatus := range route.Status.Parents {
		if !parentReferencesEqual(parentStatus.ParentRef, parentRef, route.Namespace) {
			continue
		}
		if controllerName != "" && parentStatus.ControllerName != controllerName {
			continue
		}
		matched = true

		for _, condition := range parentStatus.Conditions {
			if condition.Type == string(gatewayv1.RouteConditionAccepted) {
				if !conditionObservedGenerationCurrent(condition, route.Generation) {
					continue
				}
				acceptedConditionFound = true
				if condition.Status == metav1.ConditionFalse {
					return false, true
				}
				if condition.Status == metav1.ConditionTrue {
					accepted = true
				}
			}
		}
	}
	if matched && acceptedConditionFound {
		return accepted, true
	}
	return true, false
}

func conditionObservedGenerationCurrent(condition metav1.Condition, generation int64) bool {
	// Older Gateway API implementations did not always populate
	// ObservedGeneration. A non-zero value is authoritative when present; zero
	// remains compatible with those implementations.
	return condition.ObservedGeneration == 0 || (generation > 0 && condition.ObservedGeneration == generation)
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
	leftWildcard := strings.HasPrefix(left, "*.")
	rightWildcard := strings.HasPrefix(right, "*.")
	if leftWildcard && rightWildcard {
		leftSuffix := strings.TrimPrefix(left, "*.")
		rightSuffix := strings.TrimPrefix(right, "*.")
		return leftSuffix == rightSuffix ||
			strings.HasSuffix(leftSuffix, "."+rightSuffix) ||
			strings.HasSuffix(rightSuffix, "."+leftSuffix)
	}
	if leftWildcard {
		return wildcardMatchesHost(left, right)
	}
	if rightWildcard {
		return wildcardMatchesHost(right, left)
	}
	return false
}

func wildcardMatchesHost(pattern, host string) bool {
	suffix := strings.TrimPrefix(pattern, "*.")
	return strings.HasSuffix(host, "."+suffix) && host != suffix
}
