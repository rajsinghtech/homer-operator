package homer

// +kubebuilder:object:generate=true

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ConflictStrategy defines how to handle conflicts when merging items
type ConflictStrategy string

const (
	ConflictStrategyReplace ConflictStrategy = "replace"
	ConflictStrategyMerge   ConflictStrategy = "merge"
	ConflictStrategyError   ConflictStrategy = "error"
)

// ServiceGroupingStrategy defines how services are grouped
type ServiceGroupingStrategy string

const (
	ServiceGroupingNamespace ServiceGroupingStrategy = "namespace"
	ServiceGroupingLabel     ServiceGroupingStrategy = "label"
	ServiceGroupingCustom    ServiceGroupingStrategy = "custom"
)

// ValidationLevel defines the strictness of validation
type ValidationLevel string

const (
	ValidationLevelStrict ValidationLevel = "strict"
	ValidationLevelWarn   ValidationLevel = "warn"
	ValidationLevelNone   ValidationLevel = "none"
)

// Common repeated strings
const (
	NamespaceIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/" +
		"ns-128.png"
	GenericType = "Generic"
	NameField   = "name"
)

// HomerConfig contains base configuration for Homer dashboard.
type HomerConfig struct {
	Title             string        `json:"title,omitempty" yaml:"title,omitempty"`
	Subtitle          string        `json:"subtitle,omitempty" yaml:"subtitle,omitempty"`
	DocumentTitle     string        `json:"documentTitle,omitempty" yaml:"documentTitle,omitempty"`
	Logo              string        `json:"logo,omitempty" yaml:"logo,omitempty"`
	Icon              string        `json:"icon,omitempty" yaml:"icon,omitempty"`
	Header            bool          `json:"header" yaml:"header"`
	Footer            string        `json:"footer,omitempty" yaml:"footer,omitempty"`
	Columns           string        `json:"columns,omitempty" yaml:"columns,omitempty"`
	ConnectivityCheck bool          `json:"connectivityCheck,omitempty" yaml:"connectivityCheck,omitempty"`
	Hotkey            HotkeyConfig  `json:"hotkey,omitempty" yaml:"hotkey,omitempty"`
	Theme             string        `json:"theme,omitempty" yaml:"theme,omitempty"`
	Stylesheet        []string      `json:"stylesheet,omitempty" yaml:"stylesheet,omitempty"`
	Colors            ColorConfig   `json:"colors,omitempty" yaml:"colors,omitempty"`
	Defaults          DefaultConfig `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Proxy             ProxyConfig   `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	Message           MessageConfig `json:"message,omitempty" yaml:"message,omitempty"`
	Links             []Link        `json:"links,omitempty" yaml:"links,omitempty"`
	Services          []Service     `json:"services,omitempty" yaml:"services,omitempty"`
	ExternalConfig    string        `json:"externalConfig,omitempty" yaml:"externalConfig,omitempty"`
}

// ProxyConfig contains configuration for proxy settings.
type ProxyConfig struct {
	UseCredentials bool              `json:"useCredentials,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

// DefaultConfig contains default settings for the Homer dashboard.
type DefaultConfig struct {
	// Layout is the layout of the dashboard.
	Layout string `json:"layout,omitempty"`
	// ColorTheme is the name of the color theme to be used.
	ColorTheme string `json:"colorTheme,omitempty"`
}

type Service struct {
	Name  string `json:"name,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Logo  string `json:"logo,omitempty"`
	Class string `json:"class,omitempty"`
	Items []Item `json:"items,omitempty"`
}

type Item struct {
	Name       string `json:"name,omitempty"`
	Logo       string `json:"logo,omitempty"`
	Icon       string `json:"icon,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Tagstyle   string `json:"tagstyle,omitempty"`
	Keywords   string `json:"keywords,omitempty"`
	Url        string `json:"url,omitempty"`
	Target     string `json:"target,omitempty"`
	Class      string `json:"class,omitempty"`
	Background string `json:"background,omitempty"`
	// Smart card properties
	Type           string            `json:"type,omitempty"`
	Apikey         string            `json:"apikey,omitempty"`
	Endpoint       string            `json:"endpoint,omitempty"`
	UseCredentials bool              `json:"useCredentials,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	// Service-specific fields
	Node         string `json:"node,omitempty"`
	Legacyapi    string `json:"legacyApi,omitempty"`
	Librarytype  string `json:"libraryType,omitempty"`
	Warningvalue string `json:"warning_value,omitempty"`
	Dangervalue  string `json:"danger_value,omitempty"`
	// Metadata for conflict detection
	Source     string `json:"-"` // Source ingress/httproute name
	Namespace  string `json:"-"` // Source namespace
	LastUpdate string `json:"-"` // Last update timestamp
}

type Link struct {
	Name   string `json:"name,omitempty"`
	Icon   string `json:"icon,omitempty"`
	Url    string `json:"url,omitempty"`
	Target string `json:"target,omitempty"`
}

// HotkeyConfig contains hotkey configuration
type HotkeyConfig struct {
	Search string `json:"search,omitempty"`
}

// ColorConfig contains color scheme configuration
type ColorConfig struct {
	Light ThemeColors `json:"light,omitempty"`
	Dark  ThemeColors `json:"dark,omitempty"`
}

// ThemeColors contains color definitions for a theme
type ThemeColors struct {
	HighlightPrimary   string `json:"highlight-primary,omitempty" yaml:"highlight-primary,omitempty"`
	HighlightSecondary string `json:"highlight-secondary,omitempty" yaml:"highlight-secondary,omitempty"`
	HighlightHover     string `json:"highlight-hover,omitempty" yaml:"highlight-hover,omitempty"`
	Background         string `json:"background,omitempty" yaml:"background,omitempty"`
	CardBackground     string `json:"card-background,omitempty" yaml:"card-background,omitempty"`
	Text               string `json:"text,omitempty" yaml:"text,omitempty"`
	TextHeader         string `json:"text-header,omitempty" yaml:"text-header,omitempty"`
	TextTitle          string `json:"text-title,omitempty" yaml:"text-title,omitempty"`
	TextSubtitle       string `json:"text-subtitle,omitempty" yaml:"text-subtitle,omitempty"`
	CardShadow         string `json:"card-shadow,omitempty" yaml:"card-shadow,omitempty"`
	Link               string `json:"link,omitempty" yaml:"link,omitempty"`
	LinkHover          string `json:"link-hover,omitempty" yaml:"link-hover,omitempty"`
	BackgroundImage    string `json:"background-image,omitempty" yaml:"background-image,omitempty"`
}

// MessageConfig contains dynamic message configuration
type MessageConfig struct {
	Url             string            `json:"url,omitempty"`
	Mapping         map[string]string `json:"mapping,omitempty"`
	RefreshInterval int               `json:"refreshInterval,omitempty"`
	Style           string            `json:"style,omitempty"`
	Title           string            `json:"title,omitempty"`
	Icon            string            `json:"icon,omitempty"`
	Content         string            `json:"content,omitempty"`
}

// LoadHomerConfigFromFile loads HomerConfig from a YAML file.
func LoadHomerConfigFromFile(filename string) (*HomerConfig, error) {
	config := HomerConfig{}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func CreateConfigMap(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	owner client.Object,
) corev1.ConfigMap {
	_ = UpdateHomerConfig(config, ingresses, nil)

	// Validate configuration before creating ConfigMap
	if err := ValidateHomerConfig(config); err != nil {
		// Log validation error but continue with potentially invalid config
		// In production, you might want to handle this differently
		fmt.Printf("Warning: Homer config validation failed: %v\n", err)
	}

	// Set default values if not specified
	normalizeHomerConfig(config)

	objYAML, err := marshalHomerConfigToYAML(config)
	if err != nil {
		return corev1.ConfigMap{}
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-homer",
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
			OwnerReferences: getOwnerReferences(owner),
		},
		Data: map[string]string{
			"config.yml": string(objYAML),
		},
	}
	return *cm
}

// CreateConfigMapWithHTTPRoutes creates a ConfigMap with both Ingress and HTTPRoute resources
func CreateConfigMapWithHTTPRoutes(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	httproutes []gatewayv1.HTTPRoute,
	owner client.Object,
	domainFilters []string,
) corev1.ConfigMap {
	return createConfigMapWithHTTPRoutesAndHealth(
		config, name, namespace, ingresses, httproutes, owner, domainFilters, nil)
}

// createConfigMapWithHTTPRoutesAndHealth creates a ConfigMap with advanced aggregation features
func createConfigMapWithHTTPRoutesAndHealth(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	httproutes []gatewayv1.HTTPRoute,
	owner client.Object,
	domainFilters []string,
	healthConfig *ServiceHealthConfig,
) corev1.ConfigMap {
	_ = UpdateHomerConfig(config, ingresses, domainFilters)
	// Update config with HTTPRoutes
	for _, httproute := range httproutes {
		UpdateHomerConfigHTTPRoute(config, &httproute, domainFilters)
	}

	// Enhance config with aggregation features if health config provided
	if healthConfig != nil {
		enhanceHomerConfigWithAggregation(config, healthConfig)
	}

	// Validate configuration before creating ConfigMap
	if err := ValidateHomerConfig(config); err != nil {
		// Log validation error but continue with potentially invalid config
		fmt.Printf("Warning: Homer config validation failed: %v\n", err)
	}

	// Set default values if not specified
	normalizeHomerConfig(config)

	objYAML, err := marshalHomerConfigToYAML(config)
	if err != nil {
		return corev1.ConfigMap{}
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-homer",
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
			OwnerReferences: getOwnerReferences(owner),
		},
		Data: map[string]string{
			"config.yml": string(objYAML),
		},
	}
	return *cm
}

// DeploymentConfig contains all configuration options for creating a Homer deployment
type DeploymentConfig struct {
	AssetsConfigMapName string
	PWAManifest         string
	DNSPolicy           string
	DNSConfig           string
}

// CreateDeployment creates a Homer deployment with all optional configuration
func CreateDeployment(
	name string, namespace string, replicas *int32, owner client.Object, config *DeploymentConfig,
) appsv1.Deployment {
	if config == nil {
		config = &DeploymentConfig{}
	}
	return createDeploymentInternal(name, namespace, replicas, owner, config)
}

func createDeploymentInternal(
	name string, namespace string, replicas *int32, owner client.Object, config *DeploymentConfig,
) appsv1.Deployment {
	var defaultReplicas int32 = 1
	if replicas == nil {
		replicas = &defaultReplicas
	}
	image := "b4bz/homer"

	// Base volumes
	volumes := []corev1.Volume{
		{
			Name: "config-volume",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-homer",
					},
				},
			},
		},
		{
			Name: "assets-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// Base volume mounts for init container
	initVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-volume",
			MountPath: "/config",
		},
		{
			Name:      "assets-volume",
			MountPath: "/www/assets",
		},
	}

	// Base init command
	initCommand := "cp /config/config.yml /www/assets/config.yml"

	// If custom assets ConfigMap is provided, add it as a volume and copy assets
	if config.AssetsConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: config.AssetsConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: config.AssetsConfigMapName,
					},
				},
			},
		})

		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name:      config.AssetsConfigMapName,
			MountPath: "/custom-assets",
		})

		// Update init command to also copy custom assets (dereference symlinks to copy actual files)
		initCommand = "cp /config/config.yml /www/assets/config.yml && " +
			"cp -rL /custom-assets/* /www/assets/ 2>/dev/null || true"
	}

	// Add PWA manifest creation if provided
	if config.PWAManifest != "" {
		initCommand += " && cat > /www/assets/manifest.json << 'EOF'\n" + config.PWAManifest + "\nEOF"
	}

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-homer",
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
			OwnerReferences: getOwnerReferences(owner),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"dashboard.homer.rajsingh.info/name": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"dashboard.homer.rajsingh.info/name": name,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{1000}[0],
						RunAsGroup:   &[]int64{1000}[0],
						FSGroup:      &[]int64{1000}[0],
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "init-assets",
							Image: "busybox:1.35",
							Command: []string{
								"sh",
								"-c",
								initCommand,
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								RunAsNonRoot:             &[]bool{true}[0],
								RunAsUser:                &[]int64{1000}[0],
								RunAsGroup:               &[]int64{1000}[0],
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
							VolumeMounts: initVolumeMounts,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: image,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								RunAsNonRoot:             &[]bool{true}[0],
								RunAsUser:                &[]int64{1000}[0],
								RunAsGroup:               &[]int64{1000}[0],
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "assets-volume",
									MountPath: "/www/assets",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "INIT_ASSETS",
									Value: "1",
								},
								{
									Name:  "PORT",
									Value: "8080",
								},
								{
									Name:  "IPV6_DISABLE",
									Value: "0",
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	// Add DNS configuration if provided
	if config.DNSPolicy != "" {
		d.Spec.Template.Spec.DNSPolicy = corev1.DNSPolicy(config.DNSPolicy)
	}
	// DNSConfig would require JSON parsing if needed
	// For now, skipping complex DNS config parsing

	return *d
}

// CreateDeploymentWithAssets creates a Deployment with custom asset support and PWA manifest
func CreateDeploymentWithAssets(
	name string,
	namespace string,
	replicas *int32,
	owner client.Object,
	assetsConfigMapName string,
	pwaManifest string,
) appsv1.Deployment {
	return CreateDeployment(name, namespace, replicas, owner, &DeploymentConfig{
		AssetsConfigMapName: assetsConfigMapName,
		PWAManifest:         pwaManifest,
	})
}

// CreateDeploymentWithDNS creates a Deployment with DNS configuration
func CreateDeploymentWithDNS(
	name string,
	namespace string,
	replicas *int32,
	owner client.Object,
	dnsPolicy *corev1.DNSPolicy,
	dnsConfig *corev1.PodDNSConfig,
) appsv1.Deployment {
	config := &DeploymentConfig{}
	if dnsPolicy != nil {
		config.DNSPolicy = string(*dnsPolicy)
	}
	// Note: DNSConfig is complex and would require JSON serialization for full support
	return CreateDeployment(name, namespace, replicas, owner, config)
}

// ValidateTheme validates that the theme name is supported by Homer
func ValidateTheme(theme string) error {
	if theme == "" {
		return nil // Empty theme is valid (uses default)
	}

	validThemes := []string{"default", "neon", "walkxcode"}
	if slices.Contains(validThemes, theme) {
		return nil
	}
	return fmt.Errorf("unsupported theme '%s'. Valid themes are: %v", theme, validThemes)
}

// SecretKeyRef represents a reference to a key in a Secret (local type to avoid circular imports)
type SecretKeyRef struct {
	Name      string
	Key       string
	Namespace string
}

// ResolveAPIKeyFromSecret resolves an API key from a Kubernetes Secret and updates the item
func ResolveAPIKeyFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) error {
	if secretRef == nil || item.Type == "" {
		return nil // No secret to resolve or not a smart card
	}

	secretNamespace := defaultNamespace
	if secretRef.Namespace != "" {
		secretNamespace = secretRef.Namespace
	}

	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      secretRef.Name,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, secretRef.Name, err)
	}

	value, exists := secret.Data[secretRef.Key]
	if !exists {
		return fmt.Errorf("key %s not found in secret %s/%s", secretRef.Key, secretNamespace, secretRef.Name)
	}

	// Set the API key in the item
	item.Apikey = string(value)
	return nil
}

// GeneratePWAManifest generates a PWA manifest.json from configuration
func GeneratePWAManifest(
	title, shortName, description, themeColor, backgroundColor, display, startURL string,
	icons map[string]string,
) string {
	// Default values
	if display == "" {
		display = "standalone"
	}
	if startURL == "" {
		startURL = "/"
	}
	if themeColor == "" {
		themeColor = "#3367d6"
	}
	if backgroundColor == "" {
		backgroundColor = "#ffffff"
	}

	manifest := fmt.Sprintf(`{
  "name": "%s",
  "short_name": "%s",
  "description": "%s",
  "start_url": "%s",
  "display": "%s",
  "theme_color": "%s",
  "background_color": "%s",
  "icons": [`,
		title,
		func() string {
			if shortName != "" {
				return truncateString(shortName, 12)
			}
			return truncateString(title, 12)
		}(), // Short name max 12 chars
		description,
		startURL,
		display,
		themeColor,
		backgroundColor)

	iconEntries := []string{}

	// Add default icons if not overridden
	if icons["192"] != "" {
		iconEntries = append(iconEntries, fmt.Sprintf(`    {
      "src": "%s",
      "sizes": "192x192",
      "type": "image/png",
      "purpose": "any maskable"
    }`, icons["192"]))
	}

	if icons["512"] != "" {
		iconEntries = append(iconEntries, fmt.Sprintf(`    {
      "src": "%s", 
      "sizes": "512x512",
      "type": "image/png",
      "purpose": "any maskable"
    }`, icons["512"]))
	}

	// Add default Homer icons if no custom icons provided
	if len(iconEntries) == 0 {
		iconEntries = append(iconEntries,
			`    {
      "src": "assets/icons/pwa-192x192.png",
      "sizes": "192x192", 
      "type": "image/png",
      "purpose": "any maskable"
    }`,
			`    {
      "src": "assets/icons/pwa-512x512.png",
      "sizes": "512x512",
      "type": "image/png", 
      "purpose": "any maskable"
    }`)
	}

	manifest += strings.Join(iconEntries, ",\n") + `
  ]
}`

	return manifest
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func CreateService(name string, namespace string, owner client.Object) corev1.Service {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-homer",
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
			OwnerReferences: getOwnerReferences(owner),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"dashboard.homer.rajsingh.info/name": name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
	return *s
}
func UpdateHomerConfig(config *HomerConfig, ingresses networkingv1.IngressList, domainFilters []string) error {
	var services []Service
	// iterate over all ingresses and add them to the dashboard
	for _, ingress := range ingresses.Items {
		for _, rule := range ingress.Spec.Rules {
			host := rule.Host
			if host == "" {
				continue // Skip rules without hostnames
			}

			// Apply domain filtering
			if !matchesIngressDomainFilters(host, domainFilters) {
				continue // Skip hosts that don't match domain filters
			}

			item := Item{}
			service := Service{}
			service.Name = ingress.ObjectMeta.Namespace
			item.Name = ingress.ObjectMeta.Name
			service.Logo = NamespaceIconURL
			if len(ingress.Spec.TLS) > 0 {
				item.Url = "https://" + host
			} else {
				item.Url = "http://" + host
			}
			item.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ing-128.png"
			item.Subtitle = host
			// Process annotations safely
			processItemAnnotations(&item, ingress.ObjectMeta.Annotations)
			processServiceAnnotations(&service, ingress.ObjectMeta.Annotations)
			service.Items = append(service.Items, item)
			services = append(services, service)
		}
	}
	for _, s1 := range services {
		complete := false
		for j, s2 := range config.Services {
			if s1.Name == s2.Name {
				config.Services[j].Items = append(s2.Items, s1.Items[0])
				complete = true
				break
			}
		}
		if !complete {
			config.Services = append(config.Services, s1)
		}
	}
	return nil
}
func UpdateHomerConfigIngress(homerConfig *HomerConfig, ingress networkingv1.Ingress, domainFilters []string) {
	UpdateHomerConfigIngressWithGrouping(homerConfig, ingress, domainFilters, nil)
}

// UpdateHomerConfigIngressWithGrouping updates Homer config with custom grouping strategy
func UpdateHomerConfigIngressWithGrouping(
	homerConfig *HomerConfig,
	ingress networkingv1.Ingress,
	domainFilters []string,
	groupingConfig *ServiceGroupingConfig,
) {
	service := Service{}
	item := Item{}

	// Determine service group using flexible grouping
	service.Name = determineServiceGroup(
		ingress.ObjectMeta.Namespace,
		ingress.ObjectMeta.Labels,
		ingress.ObjectMeta.Annotations,
		groupingConfig,
	)
	item.Name = ingress.ObjectMeta.Name
	service.Logo = NamespaceIconURL

	// Check if there are any rules before accessing them
	if len(ingress.Spec.Rules) == 0 {
		// Skip Ingress resources without rules
		return
	}

	// FIRST: Remove any existing items from this Ingress source to ensure clean slate
	removeItemsFromIngressSource(homerConfig, ingress.ObjectMeta.Name, ingress.ObjectMeta.Namespace)

	// Process service-level annotations
	processServiceAnnotations(&service, ingress.ObjectMeta.Annotations)

	// Process each rule's host with domain filtering
	items := make([]Item, 0, len(ingress.Spec.Rules))
	validRuleCount := 0
	// First pass: count valid rules
	for _, rule := range ingress.Spec.Rules {
		host := rule.Host
		if host == "" {
			continue // Skip rules without hostnames
		}
		// Apply domain filtering
		if !matchesIngressDomainFilters(host, domainFilters) {
			continue // Skip hosts that don't match domain filters
		}
		validRuleCount++
	}
	for _, rule := range ingress.Spec.Rules {
		host := rule.Host
		if host == "" {
			continue // Skip rules without hostnames
		}

		// Apply domain filtering
		if !matchesIngressDomainFilters(host, domainFilters) {
			continue // Skip hosts that don't match domain filters
		}

		item := Item{}
		item.Name = ingress.ObjectMeta.Name
		item.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ing-128.png"

		// If multiple valid hosts, append hostname to make names unique
		if validRuleCount > 1 {
			item.Name = ingress.ObjectMeta.Name + "-" + host
		}

		if len(ingress.Spec.TLS) > 0 {
			item.Url = "https://" + host
		} else {
			item.Url = "http://" + host
		}
		item.Subtitle = host

		// Set metadata for conflict detection
		item.Source = ingress.ObjectMeta.Name
		item.Namespace = ingress.ObjectMeta.Namespace
		item.LastUpdate = ingress.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

		// Process annotations safely
		processItemAnnotations(&item, ingress.ObjectMeta.Annotations)
		items = append(items, item)
	}

	// Only update the service if we have matching items
	if len(items) > 0 {
		updateOrAddServiceItems(homerConfig, service, items)
	}
}

func UpdateConfigMapIngress(cm *corev1.ConfigMap, ingress networkingv1.Ingress, domainFilters []string) {
	homerConfig := HomerConfig{}
	err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig)
	if err != nil {
		return
	}
	UpdateHomerConfigIngress(&homerConfig, ingress, domainFilters)
	objYAML, err := yaml.Marshal(homerConfig)
	if err != nil {
		return
	}
	cm.Data["config.yml"] = string(objYAML)
}

// UpdateHomerConfigHTTPRoute updates the HomerConfig with HTTPRoute information
func UpdateHomerConfigHTTPRoute(homerConfig *HomerConfig, httproute *gatewayv1.HTTPRoute, domainFilters []string) {
	updateHomerConfigWithHTTPRoutes(homerConfig, httproute, domainFilters, nil)
}

// UpdateHomerConfigHTTPRouteWithGrouping updates Homer config with custom grouping strategy
func updateHomerConfigWithHTTPRoutes(
	homerConfig *HomerConfig,
	httproute *gatewayv1.HTTPRoute,
	domainFilters []string,
	groupingConfig *ServiceGroupingConfig,
) {
	service := Service{}

	// Determine service group using flexible grouping
	service.Name = determineServiceGroup(
		httproute.ObjectMeta.Namespace,
		httproute.ObjectMeta.Labels,
		httproute.ObjectMeta.Annotations,
		groupingConfig,
	)
	service.Logo = NamespaceIconURL

	// Process service-level annotations
	processServiceAnnotations(&service, httproute.ObjectMeta.Annotations)

	// FIRST: Remove any existing items from this HTTPRoute source to ensure clean slate
	removeItemsFromHTTPRouteSource(homerConfig, httproute.ObjectMeta.Name, httproute.ObjectMeta.Namespace)

	// Determine protocol based on parent Gateway listener configuration
	protocol := determineProtocolFromHTTPRoute(httproute)

	// Handle multiple hostnames by creating separate items (similar to Ingress approach)
	var items []Item
	if len(httproute.Spec.Hostnames) == 0 {
		// No hostnames specified - don't create any items
		// This allows for cleanup when all hostnames are removed
		return
	} else {
		// Create separate item for each hostname that matches domain filters
		var filteredHostnames []gatewayv1.Hostname
		for _, hostname := range httproute.Spec.Hostnames {
			hostStr := string(hostname)
			if matchesHTTPRouteDomainFilters(hostStr, domainFilters) {
				filteredHostnames = append(filteredHostnames, hostname)
			}
		}

		// Only process hostnames that match the domain filters
		for _, hostname := range filteredHostnames {
			hostStr := string(hostname)
			item := createHTTPRouteItem(httproute, hostStr, protocol)

			// If multiple hostnames, append hostname to make names unique
			if len(filteredHostnames) > 1 {
				item.Name = httproute.ObjectMeta.Name + "-" + hostStr
			} else {
				// Single hostname, use base name
				item.Name = httproute.ObjectMeta.Name
			}

			// Set metadata for conflict detection
			item.Source = httproute.ObjectMeta.Name
			item.Namespace = httproute.ObjectMeta.Namespace
			item.LastUpdate = httproute.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

			processItemAnnotations(&item, httproute.ObjectMeta.Annotations)
			items = append(items, item)
		}
	}

	// Update or add the service and items (this will add the new current items)
	if len(items) > 0 {
		updateOrAddServiceItems(homerConfig, service, items)
	}
	// Note: if len(items) == 0, we've already removed the old items above,
	// so the service will be cleaned up by removeEmptyServices()
}

// matchesHTTPRouteDomainFilters checks if a hostname matches the domain filters
func matchesHTTPRouteDomainFilters(hostname string, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true // No filters means include all
	}

	for _, filter := range domainFilters {
		// Support exact match or subdomain match
		if hostname == filter || strings.HasSuffix(hostname, "."+filter) {
			return true
		}
	}

	return false
}

// matchesIngressDomainFilters checks if a hostname matches the domain filters for Ingress
func matchesIngressDomainFilters(hostname string, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true // No filters means include all
	}

	for _, filter := range domainFilters {
		// Support exact match or subdomain match
		if hostname == filter || strings.HasSuffix(hostname, "."+filter) {
			return true
		}
	}

	return false
}

// createHTTPRouteItem creates a dashboard item for a specific hostname
func createHTTPRouteItem(httproute *gatewayv1.HTTPRoute, hostname, protocol string) Item {
	item := Item{}
	item.Name = httproute.ObjectMeta.Name
	item.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/svc-128.png"

	if hostname != "" {
		item.Url = protocol + "://" + hostname
		item.Subtitle = hostname
	} else {
		// Handle case where no hostname is specified
		item.Url = ""
		item.Subtitle = ""
	}

	return item
}

// updateOrAddServiceItems updates existing items or adds new ones to the service
func updateOrAddServiceItems(homerConfig *HomerConfig, service Service, items []Item) {
	// Find existing service
	for sx, s := range homerConfig.Services {
		if s.Name == service.Name {
			// Service exists, update/add items
			for _, newItem := range items {
				updated := false
				// Check if item already exists and update it
				for ix, existingItem := range s.Items {
					if existingItem.Name == newItem.Name {
						homerConfig.Services[sx].Items[ix] = newItem
						updated = true
						break
					}
				}
				// If item doesn't exist, add it
				if !updated {
					homerConfig.Services[sx].Items = append(homerConfig.Services[sx].Items, newItem)
				}
			}
			return
		}
	}

	// Service not found, create new service with all items
	service.Items = items
	homerConfig.Services = append(homerConfig.Services, service)
}

// determineProtocolFromHTTPRoute determines the protocol based on HTTPRoute configuration
func determineProtocolFromHTTPRoute(httproute *gatewayv1.HTTPRoute) string {
	// Check if any parent references indicate TLS
	for _, parentRef := range httproute.Spec.ParentRefs {
		// If the parent reference specifies a section name that typically indicates HTTPS
		if parentRef.SectionName != nil {
			sectionName := string(*parentRef.SectionName)
			if strings.Contains(strings.ToLower(sectionName), "https") ||
				strings.Contains(strings.ToLower(sectionName), "tls") ||
				strings.Contains(strings.ToLower(sectionName), "ssl") {
				return "https"
			}
		}
	}

	// Check if any hostnames look like they should be HTTPS (common patterns)
	for _, hostname := range httproute.Spec.Hostnames {
		hostStr := string(hostname)
		// Common patterns that suggest HTTPS
		if strings.Contains(hostStr, "api.") ||
			strings.Contains(hostStr, "secure.") ||
			strings.Contains(hostStr, "admin.") ||
			strings.HasSuffix(hostStr, ".com") ||
			strings.HasSuffix(hostStr, ".org") ||
			strings.HasSuffix(hostStr, ".net") {
			return "https"
		}
	}

	// Default to HTTP for local/development environments
	return "http"
}

// processItemAnnotations safely processes item annotations without reflection
func processItemAnnotations(item *Item, annotations map[string]string) {
	processItemAnnotationsWithValidation(item, annotations, ValidationLevelNone)
}

// processItemAnnotationsWithValidation processes item annotations with validation
func processItemAnnotationsWithValidation(item *Item, annotations map[string]string, validationLevel ValidationLevel) {
	for key, value := range annotations {
		if fieldName, ok := strings.CutPrefix(key, "item.homer.rajsingh.info/"); ok {
			processItemField(item, strings.ToLower(fieldName), value, validationLevel)
		}
	}
}

// processItemField processes a single item field based on its name
func processItemField(item *Item, fieldName, value string, validationLevel ValidationLevel) {
	switch fieldName {
	case NameField:
		item.Name = value
	case "logo":
		item.Logo = value
	case "icon":
		item.Icon = value
	case "subtitle":
		item.Subtitle = value
	case "tag":
		item.Tag = value
	case "tagstyle":
		item.Tagstyle = value
	case "keywords":
		processKeywordsField(item, value)
	case "url":
		processValidatedField(fieldName, value, validationLevel, func() { item.Url = value })
	case "target":
		processValidatedField(fieldName, value, validationLevel, func() { item.Target = value })
	case "class":
		item.Class = value
	case "background":
		item.Background = value
	case "type":
		item.Type = value
	case "apikey":
		item.Apikey = value
	case "endpoint":
		item.Endpoint = value
	case "node":
		item.Node = value
	case "legacyapi":
		item.Legacyapi = value
	case "librarytype":
		item.Librarytype = value
	case "warning_value":
		processValidatedField(fieldName, value, validationLevel, func() { item.Warningvalue = value })
	case "danger_value":
		processValidatedField(fieldName, value, validationLevel, func() { item.Dangervalue = value })
	case "usecredentials":
		item.UseCredentials = parseBooleanValue(value)
	case "headers":
		processHeadersField(item, value)
	default:
		processHeaderPrefixField(item, fieldName, value)
	}
}

// processKeywordsField processes the keywords field with validation and cleanup
func processKeywordsField(item *Item, value string) {
	if strings.Contains(value, ",") {
		keywords := strings.Split(value, ",")
		var cleanKeywords []string
		for _, keyword := range keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" {
				cleanKeywords = append(cleanKeywords, keyword)
			}
		}
		item.Keywords = strings.Join(cleanKeywords, ",")
	} else {
		item.Keywords = strings.TrimSpace(value)
	}
}

// processValidatedField processes a field that requires validation
func processValidatedField(fieldName, value string, validationLevel ValidationLevel, setFunc func()) {
	if err := validateAnnotationValue(fieldName, value, validationLevel); err != nil &&
		validationLevel == ValidationLevelStrict {
		return
	}
	setFunc()
}

// processHeadersField processes the headers field
func processHeadersField(item *Item, value string) {
	if item.Headers == nil {
		item.Headers = make(map[string]string)
	}
	parseHeadersAnnotation(item.Headers, value)
}

// processHeaderPrefixField processes header fields with prefix notation
func processHeaderPrefixField(item *Item, fieldName, value string) {
	if headerName, ok := strings.CutPrefix(fieldName, "headers."); ok {
		if item.Headers == nil {
			item.Headers = make(map[string]string)
		}
		item.Headers[headerName] = value
	}
}

// parseBooleanValue parses various boolean representations
func parseBooleanValue(value string) bool {
	val := strings.ToLower(strings.TrimSpace(value))
	return val == "true" || val == "1" || val == "yes" || val == "on"
}

// parseHeadersAnnotation parses comma-separated key:value pairs into headers map
func parseHeadersAnnotation(headers map[string]string, value string) {
	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		part := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(part) == 2 {
			headers[strings.TrimSpace(part[0])] = strings.TrimSpace(part[1])
		}
	}
}

// validateAnnotationValue validates annotation values based on field type
func validateAnnotationValue(fieldName, value string, level ValidationLevel) error {
	switch strings.ToLower(fieldName) {
	case "url":
		if value != "" && !isValidURL(value) {
			if level == ValidationLevelStrict {
				return fmt.Errorf("invalid URL format: %s", value)
			}
			if level == ValidationLevelWarn {
				fmt.Printf("Warning: potentially invalid URL format: %s\n", value)
			}
		}
	case "target":
		if value != "" && value != "_blank" && value != "_self" && value != "_parent" && value != "_top" {
			if level == ValidationLevelStrict {
				return fmt.Errorf("invalid target value: %s, must be one of: _blank, _self, _parent, _top", value)
			}
			if level == ValidationLevelWarn {
				fmt.Printf("Warning: potentially invalid target value: %s\n", value)
			}
		}
	case "warning_value", "danger_value":
		if value != "" {
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				if level == ValidationLevelStrict {
					return fmt.Errorf("invalid numeric value for %s: %s", fieldName, value)
				}
				if level == ValidationLevelWarn {
					fmt.Printf("Warning: potentially invalid numeric value for %s: %s\n", fieldName, value)
				}
			}
		}
	}
	return nil
}

// ServiceGroupingConfig defines how services should be grouped
type ServiceGroupingConfig struct {
	Strategy    ServiceGroupingStrategy `json:"strategy,omitempty"`
	LabelKey    string                  `json:"labelKey,omitempty"`
	CustomRules []GroupingRule          `json:"customRules,omitempty"`
}

// GroupingRule defines a custom grouping rule
type GroupingRule struct {
	Name      string            `json:"name"`
	Condition map[string]string `json:"condition"`
	Priority  int               `json:"priority"`
}

// determineServiceGroup determines the service group name based on strategy
func determineServiceGroup(
	namespace string,
	labels map[string]string,
	annotations map[string]string,
	config *ServiceGroupingConfig,
) string {
	if config == nil {
		config = &ServiceGroupingConfig{Strategy: ServiceGroupingNamespace}
	}

	// Check for explicit service name annotation first
	if serviceName := getServiceNameFromAnnotations(annotations); serviceName != "" {
		return serviceName
	}

	switch config.Strategy {
	case ServiceGroupingLabel:
		if config.LabelKey != "" {
			if labelValue, exists := labels[config.LabelKey]; exists {
				return labelValue
			}
		}
		// Fallback to namespace if label not found
		return namespace

	case ServiceGroupingCustom:
		for _, rule := range config.CustomRules {
			if matchesCondition(labels, annotations, rule.Condition) {
				return rule.Name
			}
		}
		// Fallback to namespace if no rules match
		return namespace

	default: // ServiceGroupingNamespace
		return namespace
	}
}

// getServiceNameFromAnnotations extracts service name from annotations
func getServiceNameFromAnnotations(annotations map[string]string) string {
	for key, value := range annotations {
		if fieldName, ok := strings.CutPrefix(key, "service.homer.rajsingh.info/"); ok {
			if strings.ToLower(fieldName) == "name" {
				return value
			}
		}
	}
	return ""
}

// matchesCondition checks if labels/annotations match a grouping condition
func matchesCondition(labels map[string]string, annotations map[string]string, condition map[string]string) bool {
	for key, expectedValue := range condition {
		// Check labels first
		if actualValue, exists := labels[key]; exists {
			if !matchesPattern(actualValue, expectedValue) {
				return false
			}
			continue
		}

		// Check annotations
		if actualValue, exists := annotations[key]; exists {
			if !matchesPattern(actualValue, expectedValue) {
				return false
			}
			continue
		}

		// Key not found in either labels or annotations
		return false
	}
	return true
}

// matchesPattern checks if a value matches a pattern (supports wildcards)
func matchesPattern(value, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "*") {
		// Simple wildcard matching
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return value == pattern
}

// processServiceAnnotations safely processes service annotations without reflection
func processServiceAnnotations(service *Service, annotations map[string]string) {
	for key, value := range annotations {
		if fieldName, ok := strings.CutPrefix(key, "service.homer.rajsingh.info/"); ok {
			switch strings.ToLower(fieldName) {
			case NameField:
				service.Name = value
			case "icon":
				service.Icon = value
			case "logo":
				service.Logo = value
			case "class":
				service.Class = value
			}
		}
	}
}

// UpdateConfigMapHTTPRoute updates the ConfigMap with HTTPRoute information
func UpdateConfigMapHTTPRoute(cm *corev1.ConfigMap, httproute *gatewayv1.HTTPRoute, domainFilters []string) {
	homerConfig := HomerConfig{}
	err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig)
	if err != nil {
		return
	}
	UpdateHomerConfigHTTPRoute(&homerConfig, httproute, domainFilters)
	objYAML, err := yaml.Marshal(homerConfig)
	if err != nil {
		return
	}
	cm.Data["config.yml"] = string(objYAML)
}

// ValidateHomerConfig validates the Homer configuration for common issues
func ValidateHomerConfig(config *HomerConfig) error {
	if config == nil {
		return errors.New("homer config cannot be nil")
	}

	// Validate title is not empty for user experience
	if config.Title == "" {
		return errors.New("title is required for dashboard")
	}

	// Validate color themes if specified
	if config.Colors.Light.Background != "" || config.Colors.Dark.Background != "" {
		if config.Colors.Light.Background != "" && !isValidColor(config.Colors.Light.Background) {
			return fmt.Errorf("invalid light background color: %s", config.Colors.Light.Background)
		}
		if config.Colors.Dark.Background != "" && !isValidColor(config.Colors.Dark.Background) {
			return fmt.Errorf("invalid dark background color: %s", config.Colors.Dark.Background)
		}
	}

	// Validate layout options
	if config.Defaults.Layout != "" {
		if config.Defaults.Layout != "columns" && config.Defaults.Layout != "list" {
			return fmt.Errorf("invalid layout '%s', must be 'columns' or 'list'", config.Defaults.Layout)
		}
	}

	// Validate color theme options
	if config.Defaults.ColorTheme != "" {
		if config.Defaults.ColorTheme != "auto" && config.Defaults.ColorTheme != "light" &&
			config.Defaults.ColorTheme != "dark" {
			return fmt.Errorf("invalid colorTheme '%s', must be 'auto', 'light', or 'dark'", config.Defaults.ColorTheme)
		}
	}

	// Validate services and items
	for i, service := range config.Services {
		if service.Name == "" {
			return fmt.Errorf("service at index %d is missing name", i)
		}
		for j, item := range service.Items {
			if item.Name == "" {
				return fmt.Errorf("item at index %d in service '%s' is missing name", j, service.Name)
			}
			if item.Url != "" && !isValidURL(item.Url) {
				return fmt.Errorf("invalid URL '%s' for item '%s'", item.Url, item.Name)
			}
		}
	}

	return nil
}

// isValidColor checks if a color string is valid (basic validation)
func isValidColor(color string) bool {
	// Check for hex colors (#rgb, #rrggbb)
	if strings.HasPrefix(color, "#") {
		color = color[1:]
		if len(color) != 3 && len(color) != 6 {
			return false
		}
		for _, c := range color {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}

	// Check for common CSS color names
	commonColors := []string{
		"red", "blue", "green", "yellow", "orange", "purple",
		"pink", "brown", "black", "white", "gray", "grey",
	}
	if slices.Contains(commonColors, strings.ToLower(color)) {
		return true
	}

	// Check for rgb/rgba format
	return strings.HasPrefix(strings.ToLower(color), "rgb")
}

// isValidURL checks if a URL string has basic valid format
func isValidURL(url string) bool {
	if url == "" {
		return true // empty URLs are valid (optional)
	}

	// Basic URL validation - must start with protocol
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "ftp://")
}

// getOwnerReferences safely creates owner references with proper GVK
func getOwnerReferences(owner client.Object) []metav1.OwnerReference {
	if owner == nil {
		return nil
	}

	// Try to get GVK from ObjectKind first
	gvk := owner.GetObjectKind().GroupVersionKind()

	// If GVK is empty (common in tests), try to infer it from the object type
	if gvk.Empty() {
		// For Dashboard objects, manually set the GVK
		if _, ok := owner.(interface{ GetName() string }); ok {
			// This is likely a Dashboard object based on the interface
			gvk = schema.GroupVersionKind{
				Group:   "homer.rajsingh.info",
				Version: "v1alpha1",
				Kind:    "Dashboard",
			}
		}
	}

	// If we still don't have a valid GVK, return empty (safer than invalid owner reference)
	if gvk.Empty() {
		return nil
	}

	return []metav1.OwnerReference{
		*metav1.NewControllerRef(owner, gvk),
	}
}

// AssetConfig contains configuration for asset management
type AssetConfig struct {
	// BaseURL is the base URL for serving assets
	BaseURL string `json:"baseURL,omitempty"`
	// UseLocal indicates whether to use local asset serving
	UseLocal bool `json:"useLocal,omitempty"`
	// CustomLogos maps service names to logo URLs
	CustomLogos map[string]string `json:"customLogos,omitempty"`
	// CustomIcons maps service names to icon classes
	CustomIcons map[string]string `json:"customIcons,omitempty"`
}

// CreateAssetConfigMap creates a ConfigMap for custom assets
func CreateAssetConfigMap(
	name string,
	namespace string,
	assets map[string][]byte,
	owner client.Object,
) corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-homer-assets",
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
				"homer.rajsingh.info/type":           "assets",
			},
			OwnerReferences: getOwnerReferences(owner),
		},
		BinaryData: assets,
	}
	return *cm
}

// GetAssetURL returns the appropriate asset URL based on configuration
func GetAssetURL(assetConfig *AssetConfig, assetName string, fallbackURL string) string {
	if assetConfig == nil {
		return fallbackURL
	}

	// Check for custom logos first
	if customURL, exists := assetConfig.CustomLogos[assetName]; exists {
		return customURL
	}

	// If using local assets, construct local URL
	if assetConfig.UseLocal && assetConfig.BaseURL != "" {
		return assetConfig.BaseURL + "/" + assetName
	}

	// Fall back to provided URL
	return fallbackURL
}

// normalizeHomerConfig sets default values and ensures proper field formatting
func normalizeHomerConfig(config *HomerConfig) {
	// Set header to true by default if not explicitly set
	if !config.Header {
		config.Header = true
	}
}

// buildThemeColorsMap converts ThemeColors struct to map for YAML output
func buildThemeColorsMap(colors ThemeColors) map[string]any {
	colorMap := map[string]any{}
	if colors.HighlightPrimary != "" {
		colorMap["highlight-primary"] = colors.HighlightPrimary
	}
	if colors.HighlightSecondary != "" {
		colorMap["highlight-secondary"] = colors.HighlightSecondary
	}
	if colors.HighlightHover != "" {
		colorMap["highlight-hover"] = colors.HighlightHover
	}
	if colors.Background != "" {
		colorMap["background"] = colors.Background
	}
	if colors.CardBackground != "" {
		colorMap["card-background"] = colors.CardBackground
	}
	if colors.Text != "" {
		colorMap["text"] = colors.Text
	}
	if colors.TextHeader != "" {
		colorMap["text-header"] = colors.TextHeader
	}
	if colors.TextTitle != "" {
		colorMap["text-title"] = colors.TextTitle
	}
	if colors.TextSubtitle != "" {
		colorMap["text-subtitle"] = colors.TextSubtitle
	}
	if colors.CardShadow != "" {
		colorMap["card-shadow"] = colors.CardShadow
	}
	if colors.Link != "" {
		colorMap["link"] = colors.Link
	}
	if colors.LinkHover != "" {
		colorMap["link-hover"] = colors.LinkHover
	}
	if colors.BackgroundImage != "" {
		colorMap["background-image"] = colors.BackgroundImage
	}
	return colorMap
}

// marshalHomerConfigToYAML creates properly formatted YAML for Homer
func marshalHomerConfigToYAML(config *HomerConfig) ([]byte, error) {
	// Create a map with proper field names for Homer
	configMap := map[string]any{
		"title":             config.Title,
		"subtitle":          config.Subtitle,
		"documentTitle":     config.DocumentTitle,
		"logo":              config.Logo,
		"icon":              config.Icon,
		"header":            config.Header,
		"footer":            config.Footer,
		"columns":           config.Columns,
		"connectivityCheck": config.ConnectivityCheck,
		"theme":             config.Theme,
		"stylesheet":        config.Stylesheet,
		"externalConfig":    config.ExternalConfig,
	}

	// Add hotkey if configured
	if config.Hotkey.Search != "" {
		configMap["hotkey"] = map[string]any{
			"search": config.Hotkey.Search,
		}
	}

	// Add colors with proper field names
	if config.Colors.Light.HighlightPrimary != "" || config.Colors.Dark.HighlightPrimary != "" {
		colors := map[string]any{}

		if light := buildThemeColorsMap(config.Colors.Light); len(light) > 0 {
			colors["light"] = light
		}

		if dark := buildThemeColorsMap(config.Colors.Dark); len(dark) > 0 {
			colors["dark"] = dark
		}

		if len(colors) > 0 {
			configMap["colors"] = colors
		}
	}

	// Add defaults
	if config.Defaults.Layout != "" || config.Defaults.ColorTheme != "" {
		defaults := map[string]any{}
		if config.Defaults.Layout != "" {
			defaults["layout"] = config.Defaults.Layout
		}
		if config.Defaults.ColorTheme != "" {
			defaults["colorTheme"] = config.Defaults.ColorTheme
		}
		configMap["defaults"] = defaults
	}

	// Add proxy if configured
	if config.Proxy.UseCredentials || len(config.Proxy.Headers) > 0 {
		proxy := map[string]any{}
		proxy["useCredentials"] = config.Proxy.UseCredentials
		if len(config.Proxy.Headers) > 0 {
			proxy["headers"] = config.Proxy.Headers
		}
		configMap["proxy"] = proxy
	}

	// Add message if configured
	if config.Message.Title != "" || config.Message.Content != "" {
		message := map[string]any{}
		if config.Message.Url != "" {
			message["url"] = config.Message.Url
		}
		if len(config.Message.Mapping) > 0 {
			message["mapping"] = config.Message.Mapping
		}
		if config.Message.RefreshInterval > 0 {
			message["refreshInterval"] = config.Message.RefreshInterval
		}
		if config.Message.Style != "" {
			message["style"] = config.Message.Style
		}
		if config.Message.Title != "" {
			message["title"] = config.Message.Title
		}
		if config.Message.Icon != "" {
			message["icon"] = config.Message.Icon
		}
		if config.Message.Content != "" {
			message["content"] = config.Message.Content
		}
		configMap["message"] = message
	}

	// Add links
	if len(config.Links) > 0 {
		configMap["links"] = config.Links
	}

	// Add services
	if len(config.Services) > 0 {
		configMap["services"] = config.Services
	}

	return yaml.Marshal(configMap)
}

// ServiceHealthConfig defines health checking configuration for services
type ServiceHealthConfig struct {
	Enabled      bool              `json:"enabled,omitempty"`
	Interval     string            `json:"interval,omitempty"`     // e.g., "30s", "5m"
	Timeout      string            `json:"timeout,omitempty"`      // e.g., "10s"
	HealthPath   string            `json:"healthPath,omitempty"`   // e.g., "/health"
	ExpectedCode int               `json:"expectedCode,omitempty"` // e.g., 200
	Headers      map[string]string `json:"headers,omitempty"`
}

// ServiceDependency represents a dependency between services
type ServiceDependency struct {
	ServiceName string `json:"serviceName"`
	ItemName    string `json:"itemName,omitempty"` // Optional specific item
	Type        string `json:"type"`               // "hard", "soft", "circular"
}

// ServiceMetrics contains aggregated metrics for a service
type ServiceMetrics struct {
	TotalItems     int               `json:"totalItems"`
	HealthyItems   int               `json:"healthyItems"`
	UnhealthyItems int               `json:"unhealthyItems"`
	LastUpdated    string            `json:"lastUpdated"`
	CustomMetrics  map[string]string `json:"customMetrics,omitempty"`
}

// enhanceItemWithHealthCheck adds health checking capabilities to an item
func enhanceItemWithHealthCheck(item *Item, healthConfig *ServiceHealthConfig) {
	if healthConfig == nil || !healthConfig.Enabled {
		return
	}

	// Add health check URL if not already a smart card
	if item.Type == "" {
		item.Type = GenericType
	}

	// Set health endpoint
	if healthConfig.HealthPath != "" && item.Endpoint == "" {
		if item.Url != "" {
			item.Endpoint = item.Url + healthConfig.HealthPath
		}
	}

	// Merge health check headers
	if healthConfig.Headers != nil {
		if item.Headers == nil {
			item.Headers = make(map[string]string)
		}
		for k, v := range healthConfig.Headers {
			if _, exists := item.Headers[k]; !exists {
				item.Headers[k] = v
			}
		}
	}
}

// aggregateServiceMetrics calculates metrics for a service
func aggregateServiceMetrics(service *Service) ServiceMetrics {
	metrics := ServiceMetrics{
		TotalItems:    len(service.Items),
		LastUpdated:   "unknown",
		CustomMetrics: make(map[string]string),
	}

	// Count healthy vs unhealthy items (basic heuristic)
	for _, item := range service.Items {
		if item.Type != "" && item.Endpoint != "" {
			// Assume items with endpoints can be health-checked
			metrics.HealthyItems++
		} else {
			// Items without health check capabilities
			metrics.UnhealthyItems++
		}

		// Find the most recent update
		if item.LastUpdate != "" && (metrics.LastUpdated == "unknown" || item.LastUpdate > metrics.LastUpdated) {
			metrics.LastUpdated = item.LastUpdate
		}
	}

	// Add custom metrics
	metrics.CustomMetrics["itemsWithUrls"] = fmt.Sprintf("%d", countItemsWithUrls(service.Items))
	metrics.CustomMetrics["itemsWithTags"] = fmt.Sprintf("%d", countItemsWithTags(service.Items))
	metrics.CustomMetrics["smartCards"] = fmt.Sprintf("%d", countSmartCards(service.Items))

	return metrics
}

// countItemsWithUrls counts items that have URLs
func countItemsWithUrls(items []Item) int {
	count := 0
	for _, item := range items {
		if item.Url != "" {
			count++
		}
	}
	return count
}

// countItemsWithTags counts items that have tags
func countItemsWithTags(items []Item) int {
	count := 0
	for _, item := range items {
		if item.Tag != "" {
			count++
		}
	}
	return count
}

// countSmartCards counts smart card items
func countSmartCards(items []Item) int {
	count := 0
	for _, item := range items {
		if item.Type != "" {
			count++
		}
	}
	return count
}

// findServiceDependencies analyzes services to find potential dependencies
func findServiceDependencies(services []Service) []ServiceDependency {
	var dependencies []ServiceDependency

	// Create a map of service URLs for quick lookup
	urlToService := make(map[string]string)
	for _, service := range services {
		for _, item := range service.Items {
			if item.Url != "" {
				urlToService[item.Url] = service.Name
			}
		}
	}

	// Look for dependencies in service names, keywords, or URLs
	for _, service := range services {
		for _, item := range service.Items {
			// Check if keywords reference other services
			if item.Keywords != "" {
				keywords := strings.Split(item.Keywords, ",")
				for _, keyword := range keywords {
					keyword = strings.TrimSpace(keyword)
					for _, otherService := range services {
						if otherService.Name != service.Name &&
							strings.Contains(strings.ToLower(keyword), strings.ToLower(otherService.Name)) {
							dependencies = append(dependencies, ServiceDependency{
								ServiceName: otherService.Name,
								ItemName:    item.Name,
								Type:        "soft",
							})
						}
					}
				}
			}

			// Check if item subtitle references other services
			if item.Subtitle != "" {
				for _, otherService := range services {
					if otherService.Name != service.Name &&
						strings.Contains(strings.ToLower(item.Subtitle), strings.ToLower(otherService.Name)) {
						dependencies = append(dependencies, ServiceDependency{
							ServiceName: otherService.Name,
							ItemName:    item.Name,
							Type:        "soft",
						})
					}
				}
			}
		}
	}

	return dependencies
}

// optimizeServiceLayout optimizes service ordering based on dependencies and usage patterns
func optimizeServiceLayout(services []Service, _ []ServiceDependency) []Service {
	// Create a copy to avoid modifying the original
	optimizedServices := make([]Service, len(services))
	copy(optimizedServices, services)

	// Simple optimization: sort by number of items (descending) and then by name
	// More complex dependency-based sorting could be implemented here
	for i := 0; i < len(optimizedServices)-1; i++ {
		for j := i + 1; j < len(optimizedServices); j++ {
			// Sort by item count first (descending)
			if len(optimizedServices[i].Items) < len(optimizedServices[j].Items) {
				optimizedServices[i], optimizedServices[j] = optimizedServices[j], optimizedServices[i]
			} else if len(optimizedServices[i].Items) == len(optimizedServices[j].Items) {
				// If same item count, sort by name (ascending)
				if optimizedServices[i].Name > optimizedServices[j].Name {
					optimizedServices[i], optimizedServices[j] = optimizedServices[j], optimizedServices[i]
				}
			}
		}
	}

	return optimizedServices
}

// removeItemsFromHTTPRouteSource removes all items that originated from a specific HTTPRoute
func removeItemsFromHTTPRouteSource(homerConfig *HomerConfig, sourceName, sourceNamespace string) {
	for serviceIndex := range homerConfig.Services {
		service := &homerConfig.Services[serviceIndex]
		var filteredItems []Item

		// Keep only items that did NOT come from the specified HTTPRoute source
		for _, item := range service.Items {
			// Remove items that match this HTTPRoute source
			if item.Source == sourceName && item.Namespace == sourceNamespace {
				// Skip this item (remove it)
				continue
			}
			// Keep this item
			filteredItems = append(filteredItems, item)
		}

		// Update the service with filtered items
		service.Items = filteredItems
	}

	// Remove any services that now have no items
	removeEmptyServices(homerConfig)
}

// removeItemsFromIngressSource removes all items that originated from a specific Ingress
func removeItemsFromIngressSource(homerConfig *HomerConfig, sourceName, sourceNamespace string) {
	for serviceIndex := range homerConfig.Services {
		service := &homerConfig.Services[serviceIndex]
		var filteredItems []Item

		// Keep only items that did NOT come from the specified Ingress source
		for _, item := range service.Items {
			// Remove items that match this Ingress source
			if item.Source == sourceName && item.Namespace == sourceNamespace {
				// Skip this item (remove it)
				continue
			}
			// Keep this item
			filteredItems = append(filteredItems, item)
		}

		// Update the service with filtered items
		service.Items = filteredItems
	}

	// Remove any services that now have no items
	removeEmptyServices(homerConfig)
}

// removeEmptyServices removes services that have no items
func removeEmptyServices(homerConfig *HomerConfig) {
	var filteredServices []Service

	for _, service := range homerConfig.Services {
		if len(service.Items) > 0 {
			filteredServices = append(filteredServices, service)
		}
	}

	homerConfig.Services = filteredServices
}

// enhanceHomerConfigWithAggregation enhances Homer config with advanced aggregation features
func enhanceHomerConfigWithAggregation(config *HomerConfig, healthConfig *ServiceHealthConfig) {
	// Enhance items with health checking
	for i := range config.Services {
		for j := range config.Services[i].Items {
			enhanceItemWithHealthCheck(&config.Services[i].Items[j], healthConfig)
		}
	}

	// Find and log dependencies
	dependencies := findServiceDependencies(config.Services)
	if len(dependencies) > 0 {
		fmt.Printf("Found %d service dependencies\n", len(dependencies))
	}

	// Optimize service layout
	config.Services = optimizeServiceLayout(config.Services, dependencies)

	// Add service metrics as comments or metadata (if Homer supports it)
	for i := range config.Services {
		metrics := aggregateServiceMetrics(&config.Services[i])
		// Could add metrics to service description or as metadata
		if config.Services[i].Name != "" {
			fmt.Printf("Service '%s': %d total items, %d with health checks\n",
				config.Services[i].Name, metrics.TotalItems, metrics.HealthyItems)
		}
	}
}
