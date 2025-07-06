package utils

import (
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// MatchesIngressDomainFilters checks if any Ingress rule host matches the domain filters
func MatchesIngressDomainFilters(ingress *networkingv1.Ingress, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}

		if MatchesHostDomainFilters(rule.Host, domainFilters) {
			return true
		}
	}

	return false
}

// MatchesHTTPRouteDomainFilters checks if any HTTPRoute hostname matches the domain filters
func MatchesHTTPRouteDomainFilters(hostnames []gatewayv1.Hostname, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, hostname := range hostnames {
		hostnameStr := string(hostname)
		if MatchesHostDomainFilters(hostnameStr, domainFilters) {
			return true
		}
	}

	return false
}

// MatchesHostDomainFilters checks if a single hostname matches the domain filters
// This is the core filtering logic used by both Ingress and HTTPRoute filtering
func MatchesHostDomainFilters(hostname string, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, filter := range domainFilters {
		// Support exact match or subdomain match
		if hostname == filter || strings.HasSuffix(hostname, "."+filter) {
			return true
		}
	}

	return false
}
