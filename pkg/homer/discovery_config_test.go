package homer

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const probeHeaderValue = "yes"

func TestCreateConfigMapWithDiscoveryConfigAppliesDashboardFeatures(t *testing.T) {
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app", Namespace: "apps", Labels: map[string]string{"team": "platform"},
			Annotations: map[string]string{"item.homer.rajsingh.info/url": "not a url"},
		},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app.example.com"}}},
	}
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps", Labels: map[string]string{"team": "platform"}},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(8080)}}},
	}
	config := &HomerConfig{Title: "Dashboard"}
	discovery := &DiscoveryConfig{
		ServiceGrouping: &ServiceGroupingConfig{Strategy: ServiceGroupingLabel, LabelKey: "team"},
		ValidationLevel: ValidationLevelStrict,
		HealthCheck: &ServiceHealthConfig{
			Enabled: true, HealthPath: "/healthz", Headers: map[string]string{"X-Probe": probeHeaderValue},
		},
	}

	configMap, err := CreateConfigMapWithDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{Items: []networkingv1.Ingress{ingress}},
		[]corev1.Service{service}, nil, discovery,
	)
	if err != nil {
		t.Fatalf("CreateConfigMapWithDiscoveryConfig() error = %v", err)
	}
	if len(config.Services) != 1 || getServiceName(&config.Services[0]) != "platform" {
		t.Fatalf("service grouping was not applied: %#v", config.Services)
	}
	if len(config.Services[0].Items) != 2 {
		t.Fatalf("discovered item count = %d, want 2", len(config.Services[0].Items))
	}
	for i := range config.Services[0].Items {
		item := &config.Services[0].Items[i]
		if getItemType(item) != GenericType || !strings.HasSuffix(getItemEndpoint(item), "/healthz") {
			t.Fatalf("health check was not applied to item %#v", item)
		}
		if item.Headers["X-Probe"] != probeHeaderValue {
			t.Fatalf("health header was not applied to item %#v", item)
		}
	}

	for i := range config.Services[0].Items {
		item := &config.Services[0].Items[i]
		if item.Source == "app" && getItemURL(item) != "http://app.example.com" {
			t.Fatalf("strict validation retained invalid annotation URL: %q", getItemURL(item))
		}
	}
	if !strings.Contains(configMap.Data["config.yml"], "platform") {
		t.Fatalf("rendered config is missing grouped service: %s", configMap.Data["config.yml"])
	}
}

func TestCreateConfigMapWithHTTPRoutesAppliesValidation(t *testing.T) {
	route := testRouteForDiscoveryConfig()
	config := &HomerConfig{Title: "Dashboard"}
	_, err := CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{}, []gatewayv1.HTTPRoute{route}, nil, nil, nil,
		&DiscoveryConfig{ValidationLevel: ValidationLevelStrict},
	)
	if err != nil {
		t.Fatalf("CreateConfigMapWithHTTPRoutesAndDiscoveryConfig() error = %v", err)
	}
	if got := getItemURL(&config.Services[0].Items[0]); got != "https://route.example.com" {
		t.Fatalf("strict validation retained invalid HTTPRoute annotation URL: %q", got)
	}
}

func TestCreateConfigMapWithDiscoveryWarnRetainsInvalidURL(t *testing.T) {
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "app",
			Namespace:   "apps",
			Annotations: map[string]string{"item.homer.rajsingh.info/url": "not a url"},
		},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "app.example.com"}}},
	}
	config := &HomerConfig{}

	if _, err := CreateConfigMapWithDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{Items: []networkingv1.Ingress{ingress}}, nil, nil,
		&DiscoveryConfig{ValidationLevel: ValidationLevelWarn},
	); err != nil {
		t.Fatalf("warn-level discovery rejected invalid annotation URL: %v", err)
	}
	if got := getItemURL(&config.Services[0].Items[0]); got != "not a url" {
		t.Fatalf("warn-level URL = %q, want invalid value retained", got)
	}

	if err := ValidateHomerConfig(config); err == nil {
		t.Fatal("strict public validation accepted the invalid legacy URL")
	}
}

func TestDiscoveryValidationLevelDefaultsToWarn(t *testing.T) {
	if got := discoveryValidationLevel(&DiscoveryConfig{}); got != ValidationLevelWarn {
		t.Fatalf("empty Dashboard discovery validation level = %q, want %q", got, ValidationLevelWarn)
	}
	if got := discoveryValidationLevel(nil); got != ValidationLevelNone {
		t.Fatalf("legacy nil discovery config validation level = %q, want %q", got, ValidationLevelNone)
	}
}

func testRouteForDiscoveryConfig() gatewayv1.HTTPRoute {
	return gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: "route", Namespace: "apps",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/url": "not a url",
				HTTPRouteProtocolAnnotation:    ProtocolHTTPS,
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{"route.example.com"}},
	}
}

func TestHTTPRouteProtocolDoesNotInferFromHostname(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
		Spec:       gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{"example.com"}},
	}
	config := &HomerConfig{}

	UpdateHomerConfigHTTPRoute(config, &route, nil)

	if got := getItemURL(&config.Services[0].Items[0]); got != "http://example.com" {
		t.Fatalf("HTTPRoute URL = %q, want listener-independent HTTP fallback", got)
	}
}

func TestDisabledHealthCheckPreservesServiceOrder(t *testing.T) {
	config := &HomerConfig{Services: []Service{
		{Name: "z-last", Items: []Item{{Name: "one", URL: "https://one.example.com"}}},
		{Name: "a-first", Items: []Item{{Name: "two", URL: "https://two.example.com"}, {Name: "three", URL: "https://three.example.com"}}},
	}}

	_, err := CreateConfigMapWithDiscoveryConfig(
		config, "dashboard", "apps", networkingv1.IngressList{}, nil, nil,
		&DiscoveryConfig{HealthCheck: &ServiceHealthConfig{Enabled: false, HealthPath: "/health"}},
	)
	if err != nil {
		t.Fatalf("CreateConfigMapWithDiscoveryConfig() error = %v", err)
	}
	if got := getServiceName(&config.Services[0]); got != "z-last" {
		t.Fatalf("first service = %q, want disabled health check to preserve order", got)
	}
	if got := getItemEndpoint(&config.Services[0].Items[0]); got != "" {
		t.Fatalf("disabled health endpoint = %q, want empty", got)
	}
}

func TestRemoteServiceOmitsUnsafeDefaultURL(t *testing.T) {
	remote := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api", Namespace: "apps",
			Annotations: map[string]string{"homer.rajsingh.info/cluster": "remote"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}
	config := &HomerConfig{}
	UpdateHomerConfigService(config, remote)
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("remote Service without URL was not discovered: %#v", config.Services)
	}
	if got := getItemURL(&config.Services[0].Items[0]); got != "" {
		t.Fatalf("remote Service default URL = %q, want empty", got)
	}

	remote.Annotations["item.homer.rajsingh.info/url"] = "https://remote.example/api"
	UpdateHomerConfigService(config, remote)
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("remote Service with explicit URL was not discovered: %#v", config.Services)
	}
	if got := getItemURL(&config.Services[0].Items[0]); got != "https://remote.example/api" {
		t.Fatalf("remote Service URL = %q, want explicit URL", got)
	}
}

func TestHTTPRouteUserDomainFilterAnnotationHasNoAuthority(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: "remote", Namespace: "apps",
			Annotations: map[string]string{
				"homer.rajsingh.info/cluster":        "remote",
				"homer.rajsingh.info/domain-filters": "blocked.example.com",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{"allowed.example.com"}},
	}
	config := &HomerConfig{}
	_, err := CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{}, []gatewayv1.HTTPRoute{route}, nil, nil,
		nil, &DiscoveryConfig{},
	)
	if err != nil {
		t.Fatalf("CreateConfigMapWithHTTPRoutesAndDiscoveryConfig() error = %v", err)
	}
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("user-controlled domain filter removed an unconfigured route: %#v", config.Services)
	}
}

func TestAuthorizedDomainFiltersApplyPerIngressHostname(t *testing.T) {
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed", Namespace: "apps", Annotations: map[string]string{
			"homer.rajsingh.info/cluster": "remote",
		}},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{
			{Host: "blocked.example.net"},
			{Host: "allowed.example.com"},
		}},
	}
	config := &HomerConfig{}
	discovery := &DiscoveryConfig{
		IngressDomainFilters: map[string][]string{
			IngressDomainFilterKey(&ingress): {"example.com"},
		},
	}
	if _, err := CreateConfigMapWithDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{Items: []networkingv1.Ingress{ingress}}, nil, nil, discovery,
	); err != nil {
		t.Fatalf("CreateConfigMapWithDiscoveryConfig() error = %v", err)
	}
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("filtered Ingress items = %#v, want one matching hostname", config.Services)
	}
	if got := getItemURL(&config.Services[0].Items[0]); got != "http://allowed.example.com" {
		t.Fatalf("filtered Ingress URL = %q, want allowed hostname", got)
	}
}

func TestAuthorizedDomainFiltersApplyPerHTTPRouteHostname(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed", Namespace: "apps", Annotations: map[string]string{
			"homer.rajsingh.info/cluster": "remote",
		}},
		Spec: gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{
			"blocked.example.net", "allowed.example.com",
		}},
	}
	config := &HomerConfig{}
	discovery := &DiscoveryConfig{
		HTTPRouteDomainFilters: map[string][]string{
			HTTPRouteDomainFilterKey(&route): {"example.com"},
		},
	}
	if _, err := CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{}, []gatewayv1.HTTPRoute{route}, nil, nil, nil, discovery,
	); err != nil {
		t.Fatalf("CreateConfigMapWithHTTPRoutesAndDiscoveryConfig() error = %v", err)
	}
	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("filtered HTTPRoute items = %#v, want one matching hostname", config.Services)
	}
	if got := getItemURL(&config.Services[0].Items[0]); got != "http://allowed.example.com" {
		t.Fatalf("filtered HTTPRoute URL = %q, want allowed hostname", got)
	}
}
