package utils

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestMatchesHostDomainFilters(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		domainFilters []string
		expected      bool
	}{
		{
			name:          "no filters - should match",
			hostname:      "example.com",
			domainFilters: []string{},
			expected:      true,
		},
		{
			name:          "exact match",
			hostname:      "example.com",
			domainFilters: []string{"example.com"},
			expected:      true,
		},
		{
			name:          "subdomain match",
			hostname:      "api.example.com",
			domainFilters: []string{"example.com"},
			expected:      true,
		},
		{
			name:          "no match",
			hostname:      "other.com",
			domainFilters: []string{"example.com"},
			expected:      false,
		},
		{
			name:          "multiple filters - match first",
			hostname:      "api.example.com",
			domainFilters: []string{"example.com", "other.com"},
			expected:      true,
		},
		{
			name:          "multiple filters - match second",
			hostname:      "api.other.com",
			domainFilters: []string{"example.com", "other.com"},
			expected:      true,
		},
		{
			name:          "multiple filters - no match",
			hostname:      "unmatched.com",
			domainFilters: []string{"example.com", "other.com"},
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesHostDomainFilters(tt.hostname, tt.domainFilters)
			if result != tt.expected {
				t.Errorf("MatchesHostDomainFilters(%q, %v) = %v; want %v",
					tt.hostname, tt.domainFilters, result, tt.expected)
			}
		})
	}
}

func TestMatchesIngressDomainFilters(t *testing.T) {
	// Create helper function for Ingress creation
	createIngress := func(hosts ...string) *networkingv1.Ingress {
		var rules []networkingv1.IngressRule
		for _, host := range hosts {
			rules = append(rules, networkingv1.IngressRule{Host: host})
		}
		return &networkingv1.Ingress{
			Spec: networkingv1.IngressSpec{Rules: rules},
		}
	}

	tests := []struct {
		name          string
		ingress       *networkingv1.Ingress
		domainFilters []string
		expected      bool
	}{
		{
			name:          "ingress with matching host",
			ingress:       createIngress("api.example.com"),
			domainFilters: []string{"example.com"},
			expected:      true,
		},
		{
			name:          "ingress with non-matching host",
			ingress:       createIngress("api.other.com"),
			domainFilters: []string{"example.com"},
			expected:      false,
		},
		{
			name:          "ingress with empty host",
			ingress:       createIngress(""),
			domainFilters: []string{"example.com"},
			expected:      false,
		},
		{
			name:          "ingress with multiple hosts - one matches",
			ingress:       createIngress("api.other.com", "api.example.com"),
			domainFilters: []string{"example.com"},
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesIngressDomainFilters(tt.ingress, tt.domainFilters)
			if result != tt.expected {
				t.Errorf("MatchesIngressDomainFilters() = %v; want %v", result, tt.expected)
			}
		})
	}
}

func TestMatchesHTTPRouteDomainFilters(t *testing.T) {
	tests := []struct {
		name          string
		hostnames     []gatewayv1.Hostname
		domainFilters []string
		expected      bool
	}{
		{
			name:          "matching hostname",
			hostnames:     []gatewayv1.Hostname{"api.example.com"},
			domainFilters: []string{"example.com"},
			expected:      true,
		},
		{
			name:          "non-matching hostname",
			hostnames:     []gatewayv1.Hostname{"api.other.com"},
			domainFilters: []string{"example.com"},
			expected:      false,
		},
		{
			name:          "multiple hostnames - one matches",
			hostnames:     []gatewayv1.Hostname{"api.other.com", "api.example.com"},
			domainFilters: []string{"example.com"},
			expected:      true,
		},
		{
			name:          "no hostnames",
			hostnames:     []gatewayv1.Hostname{},
			domainFilters: []string{"example.com"},
			expected:      false,
		},
		{
			name:          "no filters",
			hostnames:     []gatewayv1.Hostname{"api.example.com"},
			domainFilters: []string{},
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesHTTPRouteDomainFilters(tt.hostnames, tt.domainFilters)
			if result != tt.expected {
				t.Errorf("MatchesHTTPRouteDomainFilters() = %v; want %v", result, tt.expected)
			}
		})
	}
}
