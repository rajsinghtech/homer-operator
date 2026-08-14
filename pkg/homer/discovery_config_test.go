package homer

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const testSecretHeaderValue = "secret-value"

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

func TestSecretHeadersPropagateToDiscoveredItems(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "default"},
		Data:       map[string][]byte{"header": []byte(testSecretHeaderValue)},
	}).Build()

	config := &HomerConfig{Services: []Service{{
		Name:  "configured",
		Items: []Item{{Name: "foundation"}},
	}}}
	foundation := &config.Services[0].Items[0]
	if err := ResolveHeaderFromSecret(context.Background(), k8sClient, foundation, "X-Secret", &SecretKeyRef{
		Name: "credentials", Key: "header",
	}, "default"); err != nil {
		t.Fatalf("resolve header secret: %v", err)
	}

	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "apps",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/headers.X-Secret": "discovered",
			},
		},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "web.example.com"}}},
	}
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
	}
	route := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
		Spec:       gatewayv1.HTTPRouteSpec{Hostnames: []gatewayv1.Hostname{"route.example.com"}},
	}

	if _, err := CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{Items: []networkingv1.Ingress{ingress}},
		[]gatewayv1.HTTPRoute{route}, []corev1.Service{svc}, nil, nil, nil,
	); err != nil {
		t.Fatalf("create discovered config: %v", err)
	}

	wantSources := map[string]bool{"ingress/web": false, "svc/api": false, "httproute/route": false}
	for _, service := range config.Services {
		for _, item := range service.Items {
			if _, ok := wantSources[item.Source]; !ok {
				continue
			}
			wantSources[item.Source] = true
			if got := item.Headers["X-Secret"]; got != testSecretHeaderValue {
				t.Errorf("%s header = %#v, want %s", item.Source, got, testSecretHeaderValue)
			}
		}
	}
	for source, found := range wantSources {
		if !found {
			t.Errorf("discovered source %q was not found in %#v", source, config.Services)
		}
	}
}

func TestSecretHeadersApplyWithoutConfiguredFoundation(t *testing.T) {
	config := &HomerConfig{}
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "apps",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/headers.X-Secret": "discovered",
			},
		},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "web.example.com"}}},
	}

	if _, err := CreateConfigMapWithDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{Items: []networkingv1.Ingress{ingress}},
		nil, nil, &DiscoveryConfig{SecretHeaders: map[string]string{"X-Secret": testSecretHeaderValue}},
	); err != nil {
		t.Fatalf("create discovered config: %v", err)
	}

	if len(config.Services) != 1 || len(config.Services[0].Items) != 1 {
		t.Fatalf("discovered config = %#v, want one service and item", config.Services)
	}
	if got := config.Services[0].Items[0].Headers["X-Secret"]; got != testSecretHeaderValue {
		t.Fatalf("secret header = %#v, want %s", got, testSecretHeaderValue)
	}
}

func TestConfiguredSecretHeadersOverrideDirectAndDiscoveredHeadersCaseInsensitively(t *testing.T) {
	config := &HomerConfig{Services: []Service{{Items: []Item{{
		Name:                  "configured",
		Headers:               map[string]any{"AuThOrIzAtIoN": testSecretHeaderValue},
		resolvedSecretHeaders: "Authorization",
	}}}}}
	discovered := []Item{{
		Source:  "ingress/web",
		Name:    "web",
		Headers: map[string]any{"authorization": "discovered"},
	}}

	applyConfiguredSecretHeaders(config, discovered)
	if got, ok := getHeaderValue(discovered[0].Headers, "AUTHORIZATION"); !ok || got != testSecretHeaderValue {
		t.Fatalf("configured Secret header = %#v, want %s", got, testSecretHeaderValue)
	}
	if len(discovered[0].Headers) != 1 {
		t.Fatalf("configured Secret header casing produced duplicates: %#v", discovered[0].Headers)
	}

	existing := &Item{
		Source:  CRDSource,
		Name:    "web",
		Headers: map[string]any{"AUTHORIZATION": "direct"},
	}
	smartMergeItems(existing, &discovered[0])

	if got, ok := getHeaderValue(existing.Headers, "authorization"); !ok || got != testSecretHeaderValue {
		t.Fatalf("merged Secret header = %#v, want %s", got, testSecretHeaderValue)
	}
	if len(existing.Headers) != 1 {
		t.Fatalf("merged Secret header casing produced duplicates: %#v", existing.Headers)
	}
}

func TestSecretHeaderValueSurvivesCaseVariantNormalization(t *testing.T) {
	existing := &Item{
		Source: CRDSource,
		Headers: map[string]any{
			"Authorization": "direct",
			"authorization": "secret",
		},
		resolvedSecretHeaders: "authorization",
	}
	incoming := &Item{
		Source:  "ingress/web",
		Headers: map[string]any{"AUTHORIZATION": "discovered"},
	}

	smartMergeItems(existing, incoming)

	if got, ok := getHeaderValue(existing.Headers, "Authorization"); !ok || got != "secret" {
		t.Fatalf("case-variant Secret header = %#v, want secret", existing.Headers)
	}
	if len(existing.Headers) != 1 {
		t.Fatalf("case-variant Secret header produced duplicates: %#v", existing.Headers)
	}
}

func TestHeaderPrecedenceIsDirectThenDiscoveredThenHealth(t *testing.T) {
	config := &HomerConfig{Services: []Service{{
		Name: "apps",
		Items: []Item{{
			Name:    "web-one.example.com",
			URL:     "https://configured.example.com",
			Headers: map[string]any{"X-Shared": "direct"},
		}},
	}}}
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "apps",
			Annotations: map[string]string{
				"item.homer.rajsingh.info/headers.X-Shared": "discovered",
			},
		},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{
			{Host: "one.example.com"},
			{Host: "two.example.com"},
		}},
	}
	discovery := &DiscoveryConfig{
		HealthCheck: &ServiceHealthConfig{
			Enabled:    true,
			HealthPath: "/health",
			Headers: map[string]string{
				"X-Shared": "health",
				"X-Health": "health",
			},
		},
	}

	if _, err := CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
		config, "dashboard", "default", networkingv1.IngressList{Items: []networkingv1.Ingress{ingress}},
		nil, nil, nil, nil, discovery,
	); err != nil {
		t.Fatalf("create precedence config: %v", err)
	}

	items := make(map[string]Item)
	for _, service := range config.Services {
		for _, item := range service.Items {
			items[getItemName(&item)] = item
		}
	}
	for name, wantShared := range map[string]string{
		"web-one.example.com": "direct",
		"web-two.example.com": "discovered",
	} {
		item, ok := items[name]
		if !ok {
			t.Fatalf("item %q not found in %#v", name, items)
		}
		if got := item.Headers["X-Shared"]; got != wantShared {
			t.Errorf("%s X-Shared = %#v, want %q", name, got, wantShared)
		}
		if got := item.Headers["X-Health"]; got != "health" {
			t.Errorf("%s X-Health = %#v, want health", name, got)
		}
	}
}

func TestLegacyHeaderParametersKeepCRDPrecedence(t *testing.T) {
	existing := &Item{
		Source:     CRDSource,
		Name:       "web",
		Parameters: map[string]string{"headers": "X-Shared: legacy"},
	}
	discovered := &Item{
		Source:  "ingress/web",
		Name:    "web",
		Headers: map[string]any{"X-Shared": "discovered"},
	}

	smartMergeItems(existing, discovered)

	if got := existing.Headers["X-Shared"]; got != "legacy" {
		t.Fatalf("legacy CRD header = %#v, want legacy", got)
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
