package homer

// +kubebuilder:object:generate=true

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/rajsinghtech/homer-operator/pkg/utils"
	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	IngressIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/" +
		"ing-128.png"
	ServiceIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/" +
		"svc-128.png"
	GenericType       = "Generic"
	DefaultNamespace  = "default"
	NameField         = "name"
	URLField          = "url"
	CRDSource         = "crd"
	WarningValueField = "warning_value"
	DangerValueField  = "danger_value"
	BooleanTrue       = "true"
	BooleanFalse      = "false"
)

// Convention-based type detection patterns
var (
	// Boolean patterns - parameters that should be treated as booleans
	booleanSuffixes = []string{"_enabled", "_flag"}
	booleanNames    = []string{"usecredentials", "legacyapi"}

	// Integer patterns - parameters that should be treated as integers
	integerSuffixes = []string{"Interval", "interval", "_value"}
	integerNames    = []string{"timeout", "limit", WarningValueField, DangerValueField}

	// Known object parameters - these are always objects regardless of content
	objectParameters = []string{"headers", "mapping", "customHeaders"}
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

// Service represents a Homer service group configuration
type Service struct {
	// Items in this service group
	Items []Item `json:"items,omitempty"`

	// Common service fields (for CRD compatibility and user convenience)
	Name string `json:"name,omitempty"`
	Icon string `json:"icon,omitempty"`
	Logo string `json:"logo,omitempty"`

	// Dynamic parameters for annotation-driven configuration
	Parameters map[string]string `json:"parameters,omitempty"`
	// Nested objects for complex configuration (e.g., headers)
	NestedObjects map[string]map[string]string `json:"nestedObjects,omitempty"`
}

// Item represents a Homer dashboard item configuration
type Item struct {
	// Common item fields (for CRD compatibility and user convenience)
	Name     string `json:"name,omitempty"`
	Logo     string `json:"logo,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url,omitempty"`
	Tag      string `json:"tag,omitempty"`
	TagStyle string `json:"tagstyle,omitempty"`
	Target   string `json:"target,omitempty"`
	Keywords string `json:"keywords,omitempty"`
	Type     string `json:"type,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`

	// Dynamic parameters for annotation-driven configuration
	Parameters map[string]string `json:"parameters,omitempty"`
	// Nested objects for complex configuration (e.g., customHeaders)
	NestedObjects map[string]map[string]string `json:"nestedObjects,omitempty"`

	// Internal metadata for service discovery (not serialized to Homer config)
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

// Helper functions for consistent field access

// getServiceName returns the service name from Parameters map
func getServiceName(service *Service) string {
	if service.Parameters != nil {
		return service.Parameters["name"]
	}
	return ""
}

// getItemName returns the item name from Parameters map
func getItemName(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["name"]
	}
	return ""
}

// getItemURL returns the item URL from Parameters map
func getItemURL(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["url"]
	}
	return ""
}

// getItemType returns the item type from Parameters map
func getItemType(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["type"]
	}
	return ""
}

// getItemEndpoint returns the item endpoint from Parameters map
func getItemEndpoint(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["endpoint"]
	}
	return ""
}

// setItemParameter sets any item parameter in Parameters map
func setItemParameter(item *Item, key, value string) {
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters[key] = value
}

// setServiceParameter sets any service parameter in Parameters map
func setServiceParameter(service *Service, key, value string) {
	if service.Parameters == nil {
		service.Parameters = make(map[string]string)
	}
	service.Parameters[key] = value
}

// cleanupHomerConfig cleans up any invalid services/items that might come from the Dashboard CRD
func cleanupHomerConfig(config *HomerConfig) {
	// Filter out services with empty names or invalid structure
	validServices := make([]Service, 0, len(config.Services))
	for _, service := range config.Services {
		// Convert and validate the service
		normalizedService := normalizeCRDService(&service)

		// Check if service has a valid name after normalization
		serviceName := getServiceName(&normalizedService)
		if serviceName == "" {
			// Skip services with no name
			continue
		}

		// Clean up items in the service
		var validItems []Item
		for _, item := range normalizedService.Items {
			// Convert and validate the item
			normalizedItem := normalizeCRDItem(&item)

			// Check if item has a valid name after normalization
			itemName := getItemName(&normalizedItem)
			if itemName == "" {
				// Skip items with no name
				continue
			}

			// Mark as CRD source for conflict detection
			normalizedItem.Source = CRDSource
			normalizedItem.Namespace = "dashboard"
			normalizedItem.LastUpdate = "crd-defined"

			validItems = append(validItems, normalizedItem)
		}

		normalizedService.Items = validItems
		validServices = append(validServices, normalizedService)
	}

	config.Services = validServices
}

// normalizeCRDService converts CRD-style service to modern Parameters format
func normalizeCRDService(service *Service) Service {
	normalized := *service

	// Initialize Parameters map if not exists
	if normalized.Parameters == nil {
		normalized.Parameters = make(map[string]string)
	}

	// Initialize NestedObjects map if not exists
	if normalized.NestedObjects == nil {
		normalized.NestedObjects = make(map[string]map[string]string)
	}

	// Migrate struct fields to Parameters map (for consistency with dynamic approach)
	if normalized.Name != "" && normalized.Parameters["name"] == "" {
		normalized.Parameters["name"] = normalized.Name
	}
	if normalized.Icon != "" && normalized.Parameters["icon"] == "" {
		normalized.Parameters["icon"] = normalized.Icon
	}
	if normalized.Logo != "" && normalized.Parameters["logo"] == "" {
		normalized.Parameters["logo"] = normalized.Logo
	}

	return normalized
}

// normalizeCRDItem converts CRD-style item to modern Parameters format
func normalizeCRDItem(item *Item) Item {
	normalized := *item

	// Initialize Parameters map if not exists
	if normalized.Parameters == nil {
		normalized.Parameters = make(map[string]string)
	}

	// Initialize NestedObjects map if not exists
	if normalized.NestedObjects == nil {
		normalized.NestedObjects = make(map[string]map[string]string)
	}

	// Migrate struct fields to Parameters map (for consistency with dynamic approach)
	if normalized.Name != "" && normalized.Parameters["name"] == "" {
		normalized.Parameters["name"] = normalized.Name
	}
	if normalized.Logo != "" && normalized.Parameters["logo"] == "" {
		normalized.Parameters["logo"] = normalized.Logo
	}
	if normalized.Icon != "" && normalized.Parameters["icon"] == "" {
		normalized.Parameters["icon"] = normalized.Icon
	}
	if normalized.Subtitle != "" && normalized.Parameters["subtitle"] == "" {
		normalized.Parameters["subtitle"] = normalized.Subtitle
	}
	if normalized.URL != "" && normalized.Parameters["url"] == "" {
		normalized.Parameters["url"] = normalized.URL
	}
	if normalized.Tag != "" && normalized.Parameters["tag"] == "" {
		normalized.Parameters["tag"] = normalized.Tag
	}
	if normalized.TagStyle != "" && normalized.Parameters["tagstyle"] == "" {
		normalized.Parameters["tagstyle"] = normalized.TagStyle
	}
	if normalized.Target != "" && normalized.Parameters["target"] == "" {
		normalized.Parameters["target"] = normalized.Target
	}
	if normalized.Keywords != "" && normalized.Parameters["keywords"] == "" {
		normalized.Parameters["keywords"] = normalized.Keywords
	}
	if normalized.Type != "" && normalized.Parameters["type"] == "" {
		normalized.Parameters["type"] = normalized.Type
	}
	if normalized.Endpoint != "" && normalized.Parameters["endpoint"] == "" {
		normalized.Parameters["endpoint"] = normalized.Endpoint
	}

	return normalized
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
) (corev1.ConfigMap, error) {
	// Clean up any invalid services from the initial config
	cleanupHomerConfig(config)

	// Process each ingress individually using smart merging (CRD foundation + discoveries enhance)
	for _, ingress := range ingresses.Items {
		UpdateHomerConfigIngress(config, ingress, nil)
	}

	// Validate configuration before creating ConfigMap
	if err := ValidateHomerConfig(config); err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("homer config validation failed: %w", err)
	}

	// Set default values if not specified
	normalizeHomerConfig(config)

	objYAML, err := marshalHomerConfigToYAML(config)
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("failed to marshal homer config to YAML: %w", err)
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
	return *cm, nil
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
) (corev1.ConfigMap, error) {
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
) (corev1.ConfigMap, error) {
	// Clean up any invalid services from the initial config
	cleanupHomerConfig(config)

	// Process each ingress individually using smart merging (CRD foundation + discoveries enhance)
	for _, ingress := range ingresses.Items {
		UpdateHomerConfigIngress(config, ingress, domainFilters)
	}
	// Update config with HTTPRoutes using smart merging
	for _, httproute := range httproutes {
		UpdateHomerConfigHTTPRoute(config, &httproute, domainFilters)
	}

	// Enhance config with aggregation features if health config provided
	if healthConfig != nil {
		enhanceHomerConfigWithAggregation(config, healthConfig)
	}

	// Validate configuration before creating ConfigMap
	if err := ValidateHomerConfig(config); err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("homer config validation failed: %w", err)
	}

	// Set default values if not specified
	normalizeHomerConfig(config)

	objYAML, err := marshalHomerConfigToYAML(config)
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("failed to marshal homer config to YAML: %w", err)
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
	return *cm, nil
}

// DeploymentConfig contains all configuration options for creating a Homer deployment
type DeploymentConfig struct {
	AssetsConfigMapName string
	PWAManifest         string
	DNSPolicy           string
	DNSConfig           string
	// Resource configuration for Homer container
	Resources *corev1.ResourceRequirements
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

	// Add custom assets ConfigMap volume if provided
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
					InitContainers: []corev1.Container{},
					Containers: []corev1.Container{
						{
							Name:  "config-sync",
							Image: "alpine:3.18",
							Command: []string{
								"sh",
								"-c",
								buildSidecarCommand(config),
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
							VolumeMounts: buildSidecarVolumeMounts(config),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("5m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						},
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
								{
									Name:      "config-volume",
									MountPath: "/config",
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
							Resources: getContainerResources(config),
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

// getContainerResources returns resource requirements for the Homer container
func getContainerResources(config *DeploymentConfig) corev1.ResourceRequirements {
	// Use provided resources if specified
	if config != nil && config.Resources != nil {
		return *config.Resources
	}

	// Return sensible defaults for Homer
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10m"),
			corev1.ResourceMemory: resource.MustParse("32Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
}

// buildSidecarCommand creates the command for the config-sync sidecar container
func buildSidecarCommand(config *DeploymentConfig) string {
	// Initial setup: wait for Homer to initialize, then set up config and assets
	cmd := "echo 'Waiting for Homer to initialize assets...' && sleep 10 && "

	// Set up config.yml symlink
	cmd += "ln -sf /config/config.yml /www/assets/config.yml && "
	cmd += "echo 'Config symlink created' && "

	// Copy custom assets if ConfigMap is provided
	if config != nil && config.AssetsConfigMapName != "" {
		cmd += "echo 'Setting up custom assets...' && "
		cmd += "for file in favicon.ico apple-touch-icon.png pwa-192x192.png pwa-512x512.png; do " +
			"[ -f /custom-assets/$file ] && cp /custom-assets/$file /www/assets/ || true; done && "
	}

	// Add PWA manifest if provided
	if config != nil && config.PWAManifest != "" {
		escapedManifest := strings.ReplaceAll(config.PWAManifest, "'", "'\"'\"'")
		cmd += "echo 'Creating PWA manifest...' && " +
			"cat > /www/assets/manifest.json << 'EOF'\n" + escapedManifest + "\nEOF && "
	}

	cmd += "echo 'Initial setup complete. Starting config watch...' && "

	// Watch for ConfigMap changes using polling approach - no package installation needed
	cmd += "last_config_link='' && " +
		"while true; do " +
		"current_config_link=$(readlink /config/config.yml 2>/dev/null || echo 'none') && " +
		"if [ \"$current_config_link\" != \"$last_config_link\" ]; then " +
		"echo 'Config change detected, updating symlink...' && " +
		"ln -sf /config/config.yml /www/assets/config.yml && " +
		"echo \"Config updated at $(date)\" && " +
		"last_config_link=\"$current_config_link\"; " +
		"fi; " +
		"sleep 5; " +
		"done"

	return cmd
}

// buildSidecarVolumeMounts creates volume mounts for the config-sync sidecar
func buildSidecarVolumeMounts(config *DeploymentConfig) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "config-volume",
			MountPath: "/config",
		},
		{
			Name:      "assets-volume",
			MountPath: "/www/assets",
		},
	}

	// Add custom assets mount if configured
	if config != nil && config.AssetsConfigMapName != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      config.AssetsConfigMapName,
			MountPath: "/custom-assets",
		})
	}

	return mounts
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
	// Check if item has a type in Parameters (smart card indicator)
	itemType := getItemType(item)
	if secretRef == nil || itemType == "" {
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

	// Set the API key in the item Parameters
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters["apikey"] = string(value)
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

	// Determine service group using flexible grouping and set parameters
	serviceName := determineServiceGroup(
		ingress.ObjectMeta.Namespace,
		ingress.ObjectMeta.Labels,
		ingress.ObjectMeta.Annotations,
		groupingConfig,
	)
	setServiceParameter(&service, "name", serviceName)
	setServiceParameter(&service, "logo", NamespaceIconURL)

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
		if !utils.MatchesHostDomainFilters(host, domainFilters) {
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
		if !utils.MatchesHostDomainFilters(host, domainFilters) {
			continue // Skip hosts that don't match domain filters
		}

		item := Item{}

		// Set default values using helper functions
		name := ingress.ObjectMeta.Name
		if validRuleCount > 1 {
			name = ingress.ObjectMeta.Name + "-" + host
		}
		setItemParameter(&item, "name", name)
		setItemParameter(&item, "logo", IngressIconURL)
		setItemParameter(&item, "subtitle", host)

		if len(ingress.Spec.TLS) > 0 {
			setItemParameter(&item, "url", "https://"+host)
		} else {
			setItemParameter(&item, "url", "http://"+host)
		}

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
	objYAML, err := marshalHomerConfigToYAML(&homerConfig)
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

	// Determine service group using flexible grouping and set parameters
	serviceName := determineServiceGroup(
		httproute.ObjectMeta.Namespace,
		httproute.ObjectMeta.Labels,
		httproute.ObjectMeta.Annotations,
		groupingConfig,
	)
	setServiceParameter(&service, "name", serviceName)
	setServiceParameter(&service, "logo", NamespaceIconURL)

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
			if utils.MatchesHostDomainFilters(hostStr, domainFilters) {
				filteredHostnames = append(filteredHostnames, hostname)
			}
		}

		// Only process hostnames that match the domain filters
		for _, hostname := range filteredHostnames {
			hostStr := string(hostname)
			item := createHTTPRouteItem(httproute, hostStr, protocol)

			// If multiple hostnames, append hostname to make names unique
			name := httproute.ObjectMeta.Name
			if len(filteredHostnames) > 1 {
				name = httproute.ObjectMeta.Name + "-" + hostStr
			}
			setItemParameter(&item, "name", name)

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

// createHTTPRouteItem creates a dashboard item for a specific hostname
func createHTTPRouteItem(httproute *gatewayv1.HTTPRoute, hostname, protocol string) Item {
	item := Item{}

	// Set default values using helper functions
	setItemParameter(&item, "name", httproute.ObjectMeta.Name)
	setItemParameter(&item, "logo", ServiceIconURL)

	if hostname != "" {
		setItemParameter(&item, "url", protocol+"://"+hostname)
		setItemParameter(&item, "subtitle", hostname)
	} else {
		// Handle case where no hostname is specified
		setItemParameter(&item, "url", "")
		setItemParameter(&item, "subtitle", "")
	}

	return item
}

// updateOrAddServiceItems updates existing items or adds new ones using smart merging
// Smart strategy: CRD items = foundation, discovered items = enhancements
func updateOrAddServiceItems(homerConfig *HomerConfig, service Service, items []Item) {
	// Get service name from Parameters only
	serviceName := getServiceName(&service)

	// Find existing service
	for sx, s := range homerConfig.Services {
		existingServiceName := getServiceName(&s)

		if existingServiceName == serviceName {
			// Service exists, smart merge items
			for _, newItem := range items {
				updated := false
				// Get new item name from Parameters map
				newItemName := getItemName(&newItem)

				// Check if item already exists
				for ix, existingItem := range s.Items {
					existingItemName := getItemName(&existingItem)

					if existingItemName == newItemName {
						// Smart merge: preserve CRD foundation, enhance with discovered data
						smartMergeItems(&homerConfig.Services[sx].Items[ix], &newItem)
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

// smartMergeItems intelligently merges items prioritizing CRD foundation with discovered enhancements
func smartMergeItems(existingItem, newItem *Item) {
	// Initialize maps if they don't exist
	if existingItem.Parameters == nil {
		existingItem.Parameters = make(map[string]string)
	}
	if existingItem.NestedObjects == nil {
		existingItem.NestedObjects = make(map[string]map[string]string)
	}

	// Smart merging rules based on item source
	isCRDExisting := existingItem.Source == CRDSource
	isDiscoveredNew := newItem.Source != CRDSource && newItem.Source != ""

	if newItem.Parameters != nil {
		for key, value := range newItem.Parameters {
			// Smart precedence rules
			switch key {
			case NameField:
				// CRD name always wins (foundation principle)
				if !isCRDExisting {
					existingItem.Parameters[key] = value
				}
			case URLField, "subtitle":
				// Discovered items provide runtime URLs and subtitles (they know the actual endpoints)
				if isDiscoveredNew {
					existingItem.Parameters[key] = value
				} else if existingItem.Parameters[key] == "" || !isCRDExisting {
					// Fill in if empty OR if existing item is not from CRD (allow updates)
					existingItem.Parameters[key] = value
				}
			default:
				// For other fields, CRD takes precedence, discovered fills gaps
				if isCRDExisting && existingItem.Parameters[key] != "" {
					// Keep CRD value
					continue
				}
				// Use new value (either CRD is empty or new item is CRD)
				existingItem.Parameters[key] = value
			}
		}
	}

	// Merge nested objects (additive - both sources contribute)
	if newItem.NestedObjects != nil {
		for objectName, objectMap := range newItem.NestedObjects {
			if existingItem.NestedObjects[objectName] == nil {
				existingItem.NestedObjects[objectName] = make(map[string]string)
			}
			for key, value := range objectMap {
				// Additive approach - both CRD and discovered can contribute
				existingItem.NestedObjects[objectName][key] = value
			}
		}
	}

	// Update metadata intelligently
	if isDiscoveredNew {
		// Discovered items bring fresh runtime data
		existingItem.LastUpdate = newItem.LastUpdate
		// But preserve the fact that this was originally from CRD if applicable
		if !isCRDExisting {
			existingItem.Source = newItem.Source
			existingItem.Namespace = newItem.Namespace
		}
	}
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

// processItemField processes a single item field using smart convention-based detection
func processItemField(item *Item, fieldName, value string, validationLevel ValidationLevel) {
	// Handle nested object annotations (e.g., customHeaders/Authorization)
	if strings.Contains(fieldName, "/") {
		processNestedObjectField(item, fieldName, value)
		return
	}

	// Handle all parameters dynamically using smart type inference
	processDynamicParameter(item, fieldName, value, validationLevel)
}

// processNestedObjectField handles nested object annotations like customHeaders/Authorization
func processNestedObjectField(item *Item, fieldName, value string) {
	// Split the field name on "/" to get object and property
	parts := strings.SplitN(fieldName, "/", 2)
	if len(parts) != 2 {
		return // Invalid nested format
	}

	objectName := parts[0]
	propertyName := parts[1]

	// Initialize NestedObjects map if not exists
	if item.NestedObjects == nil {
		item.NestedObjects = make(map[string]map[string]string)
	}

	// Initialize the specific object map if not exists
	if item.NestedObjects[objectName] == nil {
		item.NestedObjects[objectName] = make(map[string]string)
	}

	// Store the property
	item.NestedObjects[objectName][propertyName] = value
}

// processDynamicParameter handles all parameters dynamically
func processDynamicParameter(item *Item, fieldName, value string, validationLevel ValidationLevel) {
	// Initialize Parameters map if not exists
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}

	// Special handling for certain fields
	switch fieldName {
	case "keywords":
		// Clean keywords (remove spaces, trim)
		if strings.Contains(value, ",") {
			keywords := strings.Split(value, ",")
			var cleanKeywords []string
			for _, keyword := range keywords {
				keyword = strings.TrimSpace(keyword)
				if keyword != "" {
					cleanKeywords = append(cleanKeywords, keyword)
				}
			}
			item.Parameters[fieldName] = strings.Join(cleanKeywords, ",")
		} else {
			item.Parameters[fieldName] = strings.TrimSpace(value)
		}
	case "url", "target", WarningValueField, DangerValueField:
		// Handle validation for these fields
		if err := validateAnnotationValue(fieldName, value, validationLevel); err != nil &&
			validationLevel == ValidationLevelStrict {
			// Don't store invalid values in strict mode
			return
		}
		item.Parameters[fieldName] = value
	default:
		// Store all other parameters as-is
		item.Parameters[fieldName] = value
	}
}

// smartInferType uses convention-based detection to infer parameter types
func smartInferType(key, value string) interface{} {
	// Check for boolean patterns first
	if isBooleanParameter(key) {
		return parseBooleanValue(value)
	}

	// Check for integer patterns
	if isIntegerParameter(key) {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
		// If conversion fails, fall through to other types
	}

	// Check for known object parameters
	if isObjectParameter(key) {
		result := make(map[string]string)
		parseHeadersAnnotation(result, value)
		return result
	}

	// Smart detection based on value content

	// URLs should always remain as strings, regardless of content
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "ftp://") {
		return value
	}

	// Try boolean detection by value
	val := strings.ToLower(strings.TrimSpace(value))
	if val == BooleanTrue || val == BooleanFalse || val == "yes" || val == "no" ||
		val == "on" || val == "off" || val == "1" || val == "0" {
		return parseBooleanValue(value)
	}

	// Try integer detection (but only if it looks like a number)
	if strings.TrimSpace(value) != "" && !strings.Contains(value, ".") {
		if i, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return i
		}
	}

	// Try array detection (comma-separated values) - but exclude keywords which should be strings
	if strings.Contains(value, ",") && !strings.Contains(value, ":") && key != "keywords" {
		items := strings.Split(value, ",")
		if len(items) > 1 {
			var result []string
			for _, item := range items {
				trimmed := strings.TrimSpace(item)
				if trimmed != "" {
					result = append(result, trimmed)
				}
			}
			return result
		}
	}

	// Try object detection (key:value pairs)
	if strings.Contains(value, ":") && (strings.Contains(value, ",") || strings.Count(value, ":") == 1) {
		result := make(map[string]string)
		parseHeadersAnnotation(result, value)
		return result
	}

	// Default to string
	return value
}

// isBooleanParameter checks if a parameter should be treated as boolean
func isBooleanParameter(key string) bool {
	// Check exact matches
	for _, name := range booleanNames {
		if strings.EqualFold(key, name) {
			return true
		}
	}

	// Check suffixes
	for _, suffix := range booleanSuffixes {
		if strings.HasSuffix(strings.ToLower(key), strings.ToLower(suffix)) {
			return true
		}
	}

	return false
}

// isIntegerParameter checks if a parameter should be treated as integer
func isIntegerParameter(key string) bool {
	// Check exact matches
	for _, name := range integerNames {
		if strings.EqualFold(key, name) {
			return true
		}
	}

	// Check suffixes
	for _, suffix := range integerSuffixes {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}

	return false
}

// isObjectParameter checks if a parameter should be treated as object
func isObjectParameter(key string) bool {
	for _, name := range objectParameters {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

// validateParameterValue validates string values according to their expected type

// parseBooleanValue parses various boolean representations
func parseBooleanValue(value string) bool {
	val := strings.ToLower(strings.TrimSpace(value))
	return val == BooleanTrue || val == "1" || val == "yes" || val == "on"
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
				log.Printf("Warning: potentially invalid URL format: %s", value)
			}
		}
	case "target":
		if value != "" && value != "_blank" && value != "_self" && value != "_parent" && value != "_top" {
			if level == ValidationLevelStrict {
				return fmt.Errorf("invalid target value: %s, must be one of: _blank, _self, _parent, _top", value)
			}
			if level == ValidationLevelWarn {
				log.Printf("Warning: potentially invalid target value: %s", value)
			}
		}
	case WarningValueField, DangerValueField:
		if value != "" {
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				if level == ValidationLevelStrict {
					return fmt.Errorf("invalid numeric value for %s: %s", fieldName, value)
				}
				if level == ValidationLevelWarn {
					log.Printf("Warning: potentially invalid numeric value for %s: %s", fieldName, value)
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
			if labelValue, exists := labels[config.LabelKey]; exists && labelValue != "" {
				return labelValue
			}
		}
		// Fallback to namespace if label not found
		return getNamespaceOrDefault(namespace)

	case ServiceGroupingCustom:
		for _, rule := range config.CustomRules {
			if matchesCondition(labels, annotations, rule.Condition) {
				return rule.Name
			}
		}
		// Fallback to namespace if no rules match
		return getNamespaceOrDefault(namespace)

	default: // ServiceGroupingNamespace
		return getNamespaceOrDefault(namespace)
	}
}

// getNamespaceOrDefault returns the namespace if it's not empty, otherwise returns a default name
func getNamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}

// getServiceNameFromAnnotations extracts service name from annotations
func getServiceNameFromAnnotations(annotations map[string]string) string {
	for key, value := range annotations {
		if fieldName, ok := strings.CutPrefix(key, "service.homer.rajsingh.info/"); ok {
			if strings.ToLower(fieldName) == "name" && value != "" {
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

// processServiceAnnotations processes service annotations using smart convention-based detection
func processServiceAnnotations(service *Service, annotations map[string]string) {
	for key, value := range annotations {
		if fieldName, ok := strings.CutPrefix(key, "service.homer.rajsingh.info/"); ok {
			processServiceField(service, fieldName, value)
		}
	}
}

// processServiceField processes a single service field using smart convention-based detection
func processServiceField(service *Service, fieldName, value string) {
	// Handle nested object annotations (e.g., customConfig/theme)
	if strings.Contains(fieldName, "/") {
		processServiceNestedObjectField(service, fieldName, value)
		return
	}

	// Don't override existing values with empty values for critical fields
	if strings.ToLower(fieldName) == "name" && value == "" {
		return
	}

	// Store all parameters dynamically using lowercase field names
	if service.Parameters == nil {
		service.Parameters = make(map[string]string)
	}
	service.Parameters[strings.ToLower(fieldName)] = value
}

// processServiceNestedObjectField handles nested object annotations for services
func processServiceNestedObjectField(service *Service, fieldName, value string) {
	// Split the field name on "/" to get object and property
	parts := strings.SplitN(fieldName, "/", 2)
	if len(parts) != 2 {
		return // Invalid nested format
	}

	objectName := parts[0]
	propertyName := parts[1]

	// Initialize NestedObjects map if not exists
	if service.NestedObjects == nil {
		service.NestedObjects = make(map[string]map[string]string)
	}

	// Initialize the specific object map if not exists
	if service.NestedObjects[objectName] == nil {
		service.NestedObjects[objectName] = make(map[string]string)
	}

	// Store the property
	service.NestedObjects[objectName][propertyName] = value
}

// UpdateConfigMapHTTPRoute updates the ConfigMap with HTTPRoute information
func UpdateConfigMapHTTPRoute(cm *corev1.ConfigMap, httproute *gatewayv1.HTTPRoute, domainFilters []string) {
	homerConfig := HomerConfig{}
	err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig)
	if err != nil {
		return
	}
	UpdateHomerConfigHTTPRoute(&homerConfig, httproute, domainFilters)
	objYAML, err := marshalHomerConfigToYAML(&homerConfig)
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
		// Get service name from Parameters map
		serviceName := getServiceName(&service)
		if serviceName == "" {
			// Debug: print the service to see what's wrong
			log.Printf("DEBUG: Service at index %d has no name in Parameters. Service: %+v", i, service)
			return fmt.Errorf("service at index %d is missing name", i)
		}

		for j, item := range service.Items {
			// Get item name from Parameters map
			itemName := getItemName(&item)
			if itemName == "" {
				return fmt.Errorf("item at index %d in service '%s' is missing name", j, serviceName)
			}

			// Get item URL from Parameters map
			itemURL := getItemURL(&item)
			if itemURL != "" && !isValidURL(itemURL) {
				return fmt.Errorf("invalid URL '%s' for item '%s'", itemURL, itemName)
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

	// Add services with dynamic parameter flattening
	if len(config.Services) > 0 {
		configMap["services"] = flattenServicesForYAML(config.Services)
	}

	return yaml.Marshal(configMap)
}

// flattenServicesForYAML converts services with dynamic parameters to YAML-friendly maps
func flattenServicesForYAML(services []Service) []map[string]interface{} {
	result := make([]map[string]interface{}, len(services))

	for i, service := range services {
		serviceMap := make(map[string]interface{})

		// Add parameters from Parameters map with smart type inference
		if service.Parameters != nil {
			for key, value := range service.Parameters {
				convertedValue := smartInferType(key, value)
				serviceMap[key] = convertedValue
			}
		}

		// Add nested objects
		if service.NestedObjects != nil {
			for objectName, objectMap := range service.NestedObjects {
				serviceMap[objectName] = objectMap
			}
		}

		// Add items with flattening
		if len(service.Items) > 0 {
			serviceMap["items"] = flattenItemsForYAML(service.Items)
		}

		result[i] = serviceMap
	}

	return result
}

// flattenItemsForYAML converts items with dynamic parameters to YAML-friendly maps
func flattenItemsForYAML(items []Item) []map[string]interface{} {
	result := make([]map[string]interface{}, len(items))

	for i, item := range items {
		itemMap := make(map[string]interface{})

		// Add parameters from Parameters map with smart type inference
		if item.Parameters != nil {
			for key, value := range item.Parameters {
				// Handle special cases that need specific YAML field names
				yamlKey := key
				switch strings.ToLower(key) {
				case "legacyapi":
					yamlKey = "legacyApi"
				case "librarytype":
					yamlKey = "libraryType"
				case "usecredentials":
					yamlKey = "useCredentials"
				}

				// Apply smart type inference
				convertedValue := smartInferType(key, value)
				itemMap[yamlKey] = convertedValue
			}
		}

		// Add nested objects
		if item.NestedObjects != nil {
			for objectName, objectMap := range item.NestedObjects {
				itemMap[objectName] = objectMap
			}
		}

		result[i] = itemMap
	}

	return result
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

	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}

	// Add health check URL if not already a smart card
	if item.Parameters["type"] == "" {
		item.Parameters["type"] = GenericType
	}

	// Set health endpoint
	if healthConfig.HealthPath != "" && item.Parameters["endpoint"] == "" {
		if url := item.Parameters["url"]; url != "" {
			item.Parameters["endpoint"] = url + healthConfig.HealthPath
		}
	}

	// Merge health check headers
	if healthConfig.Headers != nil {
		if item.NestedObjects == nil {
			item.NestedObjects = make(map[string]map[string]string)
		}
		if item.NestedObjects["headers"] == nil {
			item.NestedObjects["headers"] = make(map[string]string)
		}
		for k, v := range healthConfig.Headers {
			if _, exists := item.NestedObjects["headers"][k]; !exists {
				item.NestedObjects["headers"][k] = v
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
		itemType := getItemType(&item)
		endpoint := getItemEndpoint(&item)

		if itemType != "" && endpoint != "" {
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
		// Check Parameters map only
		if item.Parameters != nil && item.Parameters["url"] != "" {
			count++
		}
	}
	return count
}

// countItemsWithTags counts items that have tags
func countItemsWithTags(items []Item) int {
	count := 0
	for _, item := range items {
		// Check Parameters map only
		if item.Parameters != nil && item.Parameters["tag"] != "" {
			count++
		}
	}
	return count
}

// countSmartCards counts smart card items
func countSmartCards(items []Item) int {
	count := 0
	for _, item := range items {
		// Check Parameters map only
		if getItemType(&item) != "" {
			count++
		}
	}
	return count
}

// findServiceDependencies analyzes services to find potential dependencies
func findServiceDependencies(services []Service) []ServiceDependency {
	var dependencies []ServiceDependency

	// Look for dependencies in service names, keywords, or URLs
	for _, service := range services {
		// Get service name from Parameters map only
		serviceName := getServiceName(&service)
		if serviceName == "" {
			continue
		}

		for _, item := range service.Items {
			// Get item name from Parameters map
			itemName := getItemName(&item)

			// Process keywords dependencies
			if item.Parameters != nil && item.Parameters["keywords"] != "" {
				dependencies = append(dependencies,
					findKeywordDependencies(item.Parameters["keywords"], services, serviceName, itemName)...)
			}

			// Process subtitle dependencies
			if item.Parameters != nil && item.Parameters["subtitle"] != "" {
				dependencies = append(dependencies,
					findSubtitleDependencies(item.Parameters["subtitle"], services, serviceName, itemName)...)
			}
		}
	}

	return dependencies
}

// findKeywordDependencies finds dependencies based on keywords
func findKeywordDependencies(keywords string, services []Service, serviceName, itemName string) []ServiceDependency {
	var dependencies []ServiceDependency
	keywordList := strings.Split(keywords, ",")
	for _, keyword := range keywordList {
		keyword = strings.TrimSpace(keyword)
		for _, otherService := range services {
			// Get other service name from Parameters map only
			otherServiceName := getServiceName(&otherService)

			if otherServiceName != "" && otherServiceName != serviceName &&
				strings.Contains(strings.ToLower(keyword), strings.ToLower(otherServiceName)) {
				dependencies = append(dependencies, ServiceDependency{
					ServiceName: otherServiceName,
					ItemName:    itemName,
					Type:        "soft",
				})
			}
		}
	}
	return dependencies
}

// findSubtitleDependencies finds dependencies based on subtitle
func findSubtitleDependencies(subtitle string, services []Service, serviceName, itemName string) []ServiceDependency {
	var dependencies []ServiceDependency
	for _, otherService := range services {
		// Get other service name from Parameters map only
		otherServiceName := getServiceName(&otherService)

		if otherServiceName != "" && otherServiceName != serviceName &&
			strings.Contains(strings.ToLower(subtitle), strings.ToLower(otherServiceName)) {
			dependencies = append(dependencies, ServiceDependency{
				ServiceName: otherServiceName,
				ItemName:    itemName,
				Type:        "soft",
			})
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
				// Get service names from Parameters map only
				serviceNameI := getServiceName(&optimizedServices[i])
				serviceNameJ := getServiceName(&optimizedServices[j])

				if serviceNameI > serviceNameJ {
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
	// Pre-allocate slices to reduce allocations
	for serviceIndex := range homerConfig.Services {
		service := &homerConfig.Services[serviceIndex]

		// Use in-place filtering to avoid extra allocations
		filteredCount := 0
		for i, item := range service.Items {
			// Keep items that did NOT come from the specified Ingress source
			if !(item.Source == sourceName && item.Namespace == sourceNamespace) {
				// Move kept item to the front of the slice
				if filteredCount != i {
					service.Items[filteredCount] = item
				}
				filteredCount++
			}
		}

		// Truncate slice to remove unwanted items
		service.Items = service.Items[:filteredCount]
	}

	// Remove any services that now have no items
	removeEmptyServices(homerConfig)
}

// removeEmptyServices removes services that have no items
func removeEmptyServices(homerConfig *HomerConfig) {
	// Use in-place filtering to avoid allocations
	filteredCount := 0
	for i, service := range homerConfig.Services {
		if len(service.Items) > 0 {
			// Move kept service to the front of the slice
			if filteredCount != i {
				homerConfig.Services[filteredCount] = service
			}
			filteredCount++
		}
	}

	// Truncate slice to remove empty services
	homerConfig.Services = homerConfig.Services[:filteredCount]
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
		log.Printf("Found %d service dependencies", len(dependencies))
	}

	// Optimize service layout
	config.Services = optimizeServiceLayout(config.Services, dependencies)

	// Add service metrics as comments or metadata (if Homer supports it)
	for i := range config.Services {
		metrics := aggregateServiceMetrics(&config.Services[i])
		// Could add metrics to service description or as metadata
		serviceName := getServiceName(&config.Services[i])
		if serviceName != "" {
			log.Printf("Service '%s': %d total items, %d with health checks",
				serviceName, metrics.TotalItems, metrics.HealthyItems)
		}
	}
}
