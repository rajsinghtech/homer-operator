package homer

// +kubebuilder:object:generate=true

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

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

type ServiceGroupingStrategy string
type ValidationLevel string

const (
	ResourceSuffix      = "-homer"
	DefaultHomerPort    = 8080
	DefaultServicePort  = 80
	DefaultContainerUID = 1000
	DefaultContainerGID = 1000
	DefaultNamespace    = "default"
	GenericType         = "Generic"
	CRDSource           = "crd"
	NameField           = "name"
	URLField            = "url"
	WarningValueField   = "warning_value"
	DangerValueField    = "danger_value"
	BooleanTrue         = "true"
	BooleanFalse        = "false"
	FooterHidden        = "__FOOTER_HIDDEN__"
	NamespaceIconURL    = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/" +
		"resources/labeled/ns-128.png"
	IngressIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/" +
		"resources/labeled/ing-128.png"
	ServiceIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/" +
		"resources/labeled/svc-128.png"
)

const (
	ValidationLevelStrict ValidationLevel = "strict"
	ValidationLevelWarn   ValidationLevel = "warn"
	ValidationLevelNone   ValidationLevel = "none"
)

const (
	ServiceGroupingNamespace ServiceGroupingStrategy = "namespace"
	ServiceGroupingLabel     ServiceGroupingStrategy = "label"
	ServiceGroupingCustom    ServiceGroupingStrategy = "custom"
)

var (
	configMutex    sync.Mutex
	configMapMutex sync.Mutex
)

// HomerConfig contains base configuration for Homer dashboard.
type HomerConfig struct {
	Title         string `json:"title,omitempty" yaml:"title,omitempty"`
	Subtitle      string `json:"subtitle,omitempty" yaml:"subtitle,omitempty"`
	DocumentTitle string `json:"documentTitle,omitempty" yaml:"documentTitle,omitempty"`
	Logo          string `json:"logo,omitempty" yaml:"logo,omitempty"`
	Icon          string `json:"icon,omitempty" yaml:"icon,omitempty"`
	Header        bool   `json:"header" yaml:"header"`
	// Footer can be false to hide the footer or a string containing HTML content.
	// +kubebuilder:validation:Type=""
	// +kubebuilder:pruning:PreserveUnknownFields
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

// UnmarshalYAML custom unmarshaler to handle footer: false
func (c *HomerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type Alias HomerConfig
	aux := &struct {
		Footer interface{} `yaml:"footer,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := unmarshal(aux); err != nil {
		return err
	}
	switch v := aux.Footer.(type) {
	case bool:
		if !v {
			c.Footer = FooterHidden
		}
	case string:
		c.Footer = v
	}
	return nil
}

// UnmarshalJSON custom unmarshaler to handle footer: false
func (c *HomerConfig) UnmarshalJSON(data []byte) error {
	type Alias HomerConfig
	aux := &struct {
		Footer interface{} `json:"footer,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	switch v := aux.Footer.(type) {
	case bool:
		if !v {
			c.Footer = FooterHidden
		}
	case string:
		c.Footer = v
	}
	return nil
}

// ProxyConfig contains configuration for proxy settings.
type ProxyConfig struct {
	UseCredentials bool              `json:"useCredentials,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

type DefaultConfig struct {
	Layout     string `json:"layout,omitempty"`
	ColorTheme string `json:"colorTheme,omitempty"`
}

type Service struct {
	Items         []Item                       `json:"items,omitempty"`
	Parameters    map[string]string            `json:"parameters,omitempty"`
	NestedObjects map[string]map[string]string `json:"nestedObjects,omitempty"`
}

type Item struct {
	Parameters    map[string]string            `json:"parameters,omitempty"`
	NestedObjects map[string]map[string]string `json:"nestedObjects,omitempty"`
	Source        string                       `json:"-"`
	Namespace     string                       `json:"-"`
	LastUpdate    string                       `json:"-"`
}

type Link struct {
	Name   string `json:"name,omitempty"`
	Icon   string `json:"icon,omitempty"`
	Url    string `json:"url,omitempty"`
	Target string `json:"target,omitempty"`
}

func getParameter(params map[string]string, key string) string {
	if params != nil {
		return params[key]
	}
	return ""
}

func getServiceName(service *Service) string {
	if service.Parameters != nil {
		return service.Parameters["name"]
	}
	return ""
}

func getItemName(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["name"]
	}
	return ""
}

func getItemURL(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["url"]
	}
	return ""
}

func getItemType(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["type"]
	}
	return ""
}

func getItemEndpoint(item *Item) string {
	if item.Parameters != nil {
		return item.Parameters["endpoint"]
	}
	return ""
}

func setServiceParameter(service *Service, key, value string) {
	if service.Parameters == nil {
		service.Parameters = make(map[string]string)
	}
	service.Parameters[key] = value
}

func setItemParameter(item *Item, key, value string) {
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters[key] = value
}

func cleanupHomerConfig(config *HomerConfig) {
	validServices := make([]Service, 0, len(config.Services))
	for _, service := range config.Services {
		ensureParameterMaps(&service.Parameters, &service.NestedObjects)
		if getParameter(service.Parameters, "name") == "" {
			continue
		}

		var validItems []Item
		for _, item := range service.Items {
			ensureParameterMaps(&item.Parameters, &item.NestedObjects)
			if getParameter(item.Parameters, "name") == "" {
				continue
			}

			item.Source = "crd"
			item.Namespace = "dashboard"
			item.LastUpdate = "crd-defined"

			validItems = append(validItems, item)
		}

		service.Items = validItems
		validServices = append(validServices, service)
	}

	config.Services = validServices
}

func ensureParameterMaps(params *map[string]string, nested *map[string]map[string]string) {
	if *params == nil {
		*params = make(map[string]string)
	}
	if *nested == nil {
		*nested = make(map[string]map[string]string)
	}
}

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

type MessageConfig struct {
	Url             string            `json:"url,omitempty"`
	Mapping         map[string]string `json:"mapping,omitempty"`
	RefreshInterval int               `json:"refreshInterval,omitempty"`
	Style           string            `json:"style,omitempty"`
	Title           string            `json:"title,omitempty"`
	Icon            string            `json:"icon,omitempty"`
	Content         string            `json:"content,omitempty"`
}

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
	cleanupHomerConfig(config)

	for _, ingress := range ingresses.Items {
		UpdateHomerConfigIngress(config, ingress, nil)
	}

	if err := ValidateHomerConfig(config); err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("config validation: %w", err)
	}

	normalizeHomerConfig(config)

	objYAML, err := marshalHomerConfigToYAML(config)
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("marshal config: %w", err)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + ResourceSuffix,
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
	originalConfig := *config

	cleanupHomerConfig(config)

	fmt.Fprintf(os.Stderr, "DEBUG: createConfigMapWithHTTPRoutesAndHealth called with %d HTTPRoutes\n", len(httproutes))
	for i, httproute := range httproutes {
		if clusterAnnot, ok := httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok {
			fmt.Fprintf(os.Stderr, "DEBUG: HTTPRoute[%d] %s has cluster annotation: %s, labels: %v\n", i, httproute.ObjectMeta.Name, clusterAnnot, httproute.ObjectMeta.Labels)
		}
	}

	for _, ingress := range ingresses.Items {
		UpdateHomerConfigIngress(config, ingress, domainFilters)
	}
	for _, httproute := range httproutes {
		UpdateHomerConfigHTTPRoute(config, &httproute, domainFilters)
	}

	if err := validateCRDServicePreservation(&originalConfig, config); err != nil {
		log.Printf("Warning: %v", err)
	}

	if healthConfig != nil {
		enhanceHomerConfigWithAggregation(config, healthConfig)
	}

	// Validate configuration before creating ConfigMap
	if err := ValidateHomerConfig(config); err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("config validation: %w", err)
	}

	normalizeHomerConfig(config)

	objYAML, err := marshalHomerConfigToYAML(config)
	if err != nil {
		return corev1.ConfigMap{}, fmt.Errorf("marshal config: %w", err)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + ResourceSuffix,
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

type DeploymentConfig struct {
	AssetsConfigMapName string
	PWAManifest         string
	DNSPolicy           string
	DNSConfig           string
	Resources           *corev1.ResourceRequirements
}

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
						Name: name + ResourceSuffix,
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
			Name:      name + ResourceSuffix,
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
						RunAsUser:    &[]int64{DefaultContainerUID}[0],
						RunAsGroup:   &[]int64{DefaultContainerGID}[0],
						FSGroup:      &[]int64{DefaultContainerGID}[0],
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
								RunAsUser:                &[]int64{DefaultContainerUID}[0],
								RunAsGroup:               &[]int64{DefaultContainerGID}[0],
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
								RunAsUser:                &[]int64{DefaultContainerUID}[0],
								RunAsGroup:               &[]int64{DefaultContainerGID}[0],
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
									ContainerPort: DefaultHomerPort,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "INIT_ASSETS",
									Value: "1",
								},
								{
									Name:  "PORT",
									Value: strconv.Itoa(DefaultHomerPort),
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

	// Parse and apply DNSConfig if provided
	if config.DNSConfig != "" {
		var dnsConfig corev1.PodDNSConfig
		if err := json.Unmarshal([]byte(config.DNSConfig), &dnsConfig); err != nil {
			// Log error but don't fail deployment - DNS config is optional
			log.Printf("Warning: parse DNSConfig: %v", err)
		} else {
			d.Spec.Template.Spec.DNSConfig = &dnsConfig
		}
	}

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
		// Dynamically copy all files from custom-assets directory
		cmd += "if [ -d /custom-assets ] && [ \"$(ls -A /custom-assets 2>/dev/null)\" ]; then " +
			"cd /custom-assets && " +
			"for file in *; do " +
			"[ -f \"$file\" ] && cp \"$file\" /www/assets/ && echo \"Copied $file\" || true; " +
			"done; " +
			"cd /; fi && "
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
	if dnsConfig != nil {
		// Convert PodDNSConfig to JSON string for consistency with DeploymentConfig
		if dnsConfigJSON, err := json.Marshal(dnsConfig); err == nil {
			config.DNSConfig = string(dnsConfigJSON)
		} else {
			log.Printf("Warning: serialize DNSConfig: %v", err)
		}
	}
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
	return fmt.Errorf("unsupported theme: %s", theme)
}

// SecretKeyRef represents a reference to a key in a Secret (local type to avoid circular imports)
type SecretKeyRef struct {
	Name      string
	Key       string
	Namespace string
}

// resolveSecretValue is a helper function to resolve a secret value
func resolveSecretValue(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) (string, error) {
	// Check if item has a type in Parameters (smart card indicator)
	itemType := getItemType(item)
	if secretRef == nil || itemType == "" {
		return "", nil // No secret to resolve or not a smart card
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
		return "", fmt.Errorf("secret %s/%s: %w", secretNamespace, secretRef.Name, err)
	}

	if secret.Data == nil {
		return "", fmt.Errorf("secret %s/%s: no data", secretNamespace, secretRef.Name)
	}

	value, exists := secret.Data[secretRef.Key]
	if !exists {
		return "", fmt.Errorf("secret %s/%s: key %s not found", secretNamespace, secretRef.Name, secretRef.Key)
	}

	return string(value), nil
}

// ResolveAPIKeyFromSecret resolves an API key from a Kubernetes Secret and updates the item
func ResolveAPIKeyFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) error {
	value, err := resolveSecretValue(ctx, k8sClient, item, secretRef, defaultNamespace)
	if err != nil || value == "" {
		return err
	}

	// Set the API key in the item Parameters
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters["apikey"] = value
	return nil
}

// ResolveTokenFromSecret resolves a Bearer token from a Kubernetes Secret and updates the item
func ResolveTokenFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) error {
	value, err := resolveSecretValue(ctx, k8sClient, item, secretRef, defaultNamespace)
	if err != nil || value == "" {
		return err
	}

	// Set the token in NestedObjects under customHeaders/Authorization
	if item.NestedObjects == nil {
		item.NestedObjects = make(map[string]map[string]string)
	}
	if item.NestedObjects["customHeaders"] == nil {
		item.NestedObjects["customHeaders"] = make(map[string]string)
	}
	item.NestedObjects["customHeaders"]["Authorization"] = fmt.Sprintf("Bearer %s", value)
	return nil
}

// ResolveUsernameFromSecret resolves a username from a Kubernetes Secret and updates the item
func ResolveUsernameFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) error {
	value, err := resolveSecretValue(ctx, k8sClient, item, secretRef, defaultNamespace)
	if err != nil || value == "" {
		return err
	}

	// Set the username in the item Parameters
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters["username"] = value
	return nil
}

// ResolvePasswordFromSecret resolves a password from a Kubernetes Secret and updates the item
func ResolvePasswordFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) error {
	value, err := resolveSecretValue(ctx, k8sClient, item, secretRef, defaultNamespace)
	if err != nil || value == "" {
		return err
	}

	// Set the password in the item Parameters
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters["password"] = value
	return nil
}

// ResolveHeaderFromSecret resolves a custom header value from a Kubernetes Secret and updates the item
func ResolveHeaderFromSecret(
	ctx context.Context,
	k8sClient client.Client,
	item *Item,
	headerName string,
	secretRef *SecretKeyRef,
	defaultNamespace string,
) error {
	value, err := resolveSecretValue(ctx, k8sClient, item, secretRef, defaultNamespace)
	if err != nil || value == "" {
		return err
	}

	// Set the custom header in NestedObjects under customHeaders
	if item.NestedObjects == nil {
		item.NestedObjects = make(map[string]map[string]string)
	}
	if item.NestedObjects["customHeaders"] == nil {
		item.NestedObjects["customHeaders"] = make(map[string]string)
	}
	item.NestedObjects["customHeaders"][headerName] = value
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
			Name:      name + ResourceSuffix,
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
					Port:       DefaultServicePort,
					TargetPort: intstr.FromInt(DefaultHomerPort),
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
	// Setup service configuration
	service := setupIngressService(homerConfig, ingress, groupingConfig)

	// Validate ingress has rules
	if len(ingress.Spec.Rules) == 0 {
		return // Skip Ingress resources without rules
	}

	// Remove existing items and process service annotations
	removeItemsFromIngressSource(homerConfig, ingress.ObjectMeta.Name, ingress.ObjectMeta.Namespace)
	processServiceAnnotations(&service, ingress.ObjectMeta.Annotations)

	// Create items from ingress rules
	items := createIngressItems(ingress, domainFilters)

	// Update service if we have matching items
	if len(items) > 0 {
		updateOrAddServiceItems(homerConfig, service, items)
	}
}

// setupIngressService creates and configures a service for an ingress
func setupIngressService(
	homerConfig *HomerConfig,
	ingress networkingv1.Ingress,
	groupingConfig *ServiceGroupingConfig,
) Service {
	service := Service{}

	// Determine service group using CRD-aware flexible grouping and set parameters
	serviceName := determineServiceGroupWithCRDRespect(
		homerConfig,
		ingress.ObjectMeta.Namespace,
		ingress.ObjectMeta.Labels,
		ingress.ObjectMeta.Annotations,
		groupingConfig,
	)
	setServiceParameter(&service, "name", serviceName)
	setServiceParameter(&service, "logo", NamespaceIconURL)

	return service
}

// createIngressItems creates dashboard items from ingress rules
func createIngressItems(ingress networkingv1.Ingress, domainFilters []string) []Item {
	items := make([]Item, 0, len(ingress.Spec.Rules))

	// First pass: count valid rules for naming
	validRuleCount := countValidIngressRules(ingress, domainFilters)

	// Second pass: create items for valid rules
	for _, rule := range ingress.Spec.Rules {
		host := rule.Host
		if host == "" {
			continue // Skip rules without hostnames
		}

		// Apply domain filtering
		if !utils.MatchesHostDomainFilters(host, domainFilters) {
			continue // Skip hosts that don't match domain filters
		}

		item := createIngressItem(ingress, host, validRuleCount)
		processItemAnnotations(&item, ingress.ObjectMeta.Annotations)

		// Skip items that are marked as hidden
		if isItemHidden(&item) {
			continue
		}

		items = append(items, item)
	}

	return items
}

// countValidIngressRules counts rules that pass domain filtering
func countValidIngressRules(ingress networkingv1.Ingress, domainFilters []string) int {
	validRuleCount := 0
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
	return validRuleCount
}

// createIngressItem creates a single dashboard item for an ingress rule
func createIngressItem(ingress networkingv1.Ingress, host string, validRuleCount int) Item {
	item := Item{}

	// Set default values using helper functions
	name := ingress.ObjectMeta.Name
	if validRuleCount > 1 {
		name = ingress.ObjectMeta.Name + "-" + host
	}

	// Append cluster name suffix from label if set (only for remote clusters)
	if clusterName, ok := ingress.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != "local" {
		fmt.Fprintf(os.Stderr, "DEBUG: Ingress %s from cluster %s, labels: %v\n", ingress.ObjectMeta.Name, clusterName, ingress.ObjectMeta.Labels)
		if suffix, hasSuffix := ingress.ObjectMeta.Labels["cluster-name-suffix"]; hasSuffix && suffix != "" {
			fmt.Fprintf(os.Stderr, "DEBUG: Appending suffix %q to name %q\n", suffix, name)
			name = name + suffix
		} else {
			fmt.Fprintf(os.Stderr, "DEBUG: No cluster-name-suffix label found for %s\n", ingress.ObjectMeta.Name)
		}
	}

	setItemParameter(&item, "name", name)
	setItemParameter(&item, "logo", IngressIconURL)
	setItemParameter(&item, "subtitle", host)

	// Determine protocol based on TLS configuration
	if len(ingress.Spec.TLS) > 0 {
		setItemParameter(&item, "url", "https://"+host)
	} else {
		setItemParameter(&item, "url", "http://"+host)
	}

	// Set metadata for conflict detection
	item.Source = ingress.ObjectMeta.Name
	item.Namespace = ingress.ObjectMeta.Namespace
	item.LastUpdate = ingress.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

	// Auto-tag with cluster name if cluster-tagstyle label is set
	if clusterName, ok := ingress.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != "local" {
		// Only add tag if cluster-tagstyle is explicitly set
		if tagStyle, hasStyle := ingress.ObjectMeta.Labels["cluster-tagstyle"]; hasStyle && tagStyle != "" {
			setItemParameter(&item, "tag", clusterName)
			setItemParameter(&item, "tagstyle", tagStyle)
		}
	}

	return item
}

func UpdateConfigMapIngress(cm *corev1.ConfigMap, ingress networkingv1.Ingress, domainFilters []string) {
	configMapMutex.Lock()
	defer configMapMutex.Unlock()

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

	// Determine service group using CRD-aware flexible grouping and set parameters
	serviceName := determineServiceGroupWithCRDRespect(
		homerConfig,
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
		// Check if HTTPRoute has per-cluster domain filters annotation
		effectiveDomainFilters := domainFilters
		if filterAnnotation, ok := httproute.ObjectMeta.Annotations["homer.rajsingh.info/domain-filters"]; ok && filterAnnotation != "" {
			// Use per-cluster domain filters from annotation
			effectiveDomainFilters = strings.Split(filterAnnotation, ",")
			for i := range effectiveDomainFilters {
				effectiveDomainFilters[i] = strings.TrimSpace(effectiveDomainFilters[i])
			}
		}

		// Create separate item for each hostname that matches domain filters
		var filteredHostnames []gatewayv1.Hostname
		for _, hostname := range httproute.Spec.Hostnames {
			hostStr := string(hostname)
			if utils.MatchesHostDomainFilters(hostStr, effectiveDomainFilters) {
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

			// Append cluster name suffix from label if set (only for remote clusters)
			if clusterName, ok := httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != "local" {
				fmt.Fprintf(os.Stderr, "DEBUG: HTTPRoute %s from cluster %s, labels: %v\n", httproute.ObjectMeta.Name, clusterName, httproute.ObjectMeta.Labels)
				if suffix, hasSuffix := httproute.ObjectMeta.Labels["cluster-name-suffix"]; hasSuffix && suffix != "" {
					fmt.Fprintf(os.Stderr, "DEBUG: Appending suffix %q to name %q\n", suffix, name)
					name = name + suffix
				} else {
					fmt.Fprintf(os.Stderr, "DEBUG: No cluster-name-suffix label found for %s\n", httproute.ObjectMeta.Name)
				}
			}

			setItemParameter(&item, "name", name)

			// Set metadata for conflict detection
			item.Source = httproute.ObjectMeta.Name
			item.Namespace = httproute.ObjectMeta.Namespace
			item.LastUpdate = httproute.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

			processItemAnnotations(&item, httproute.ObjectMeta.Annotations)

			// Skip items that are marked as hidden
			if isItemHidden(&item) {
				continue
			}

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

	// Auto-tag with cluster name if cluster-tagstyle label is set
	if clusterName, ok := httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != "local" {
		// Only add tag if cluster-tagstyle is explicitly set
		if tagStyle, hasStyle := httproute.ObjectMeta.Labels["cluster-tagstyle"]; hasStyle && tagStyle != "" {
			setItemParameter(&item, "tag", clusterName)
			setItemParameter(&item, "tagstyle", tagStyle)
		}
	}

	return item
}

// updateOrAddServiceItems updates existing items or adds new ones using smart merging
// Smart strategy: CRD items = foundation, discovered items = enhancements
func updateOrAddServiceItems(homerConfig *HomerConfig, service Service, items []Item) {
	configMutex.Lock()
	defer configMutex.Unlock()

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
func smartInferType(value string) interface{} {
	value = strings.TrimSpace(value)

	// Boolean detection
	lower := strings.ToLower(value)
	if lower == "true" || lower == "1" || lower == "yes" || lower == "on" {
		return true
	}
	if lower == "false" || lower == "0" || lower == "no" || lower == "off" {
		return false
	}

	// Integer detection
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}

	return value
}

// validateParameterValue validates string values according to their expected type

// validateAnnotationValue validates annotation values based on field type
func validateAnnotationValue(fieldName, value string, level ValidationLevel) error {
	if level != ValidationLevelStrict || value == "" {
		return nil
	}

	switch strings.ToLower(fieldName) {
	case "url":
		if !isValidURL(value) {
			return fmt.Errorf("url: %s", value)
		}
	case "target":
		if value != "_blank" && value != "_self" && value != "_parent" && value != "_top" {
			return fmt.Errorf("target: %s", value)
		}
	case WarningValueField, DangerValueField:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("%s: %s", fieldName, value)
		}
	}
	return nil
}

// isItemHidden checks if an item should be hidden based on annotation
func isItemHidden(item *Item) bool {
	if item.Parameters == nil {
		return false
	}

	// Check for hide parameter
	if hideValue, exists := item.Parameters["hide"]; exists {
		// Use smart type inference to handle boolean values
		hideInterface := smartInferType(hideValue)
		if hideBool, ok := hideInterface.(bool); ok {
			return hideBool
		}
		// If not a boolean, treat non-empty string as true
		return hideValue != ""
	}

	return false
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

// determineServiceGroupWithCRDRespect determines service group while respecting existing CRD groups
func determineServiceGroupWithCRDRespect(
	homerConfig *HomerConfig,
	namespace string,
	labels map[string]string,
	annotations map[string]string,
	config *ServiceGroupingConfig,
) string {
	// Check for explicit service name annotation first
	if serviceName := getServiceNameFromAnnotations(annotations); serviceName != "" {
		return serviceName
	}

	// Try to find a suitable existing CRD service group
	if existingGroup := findBestMatchingCRDServiceGroup(homerConfig, namespace, annotations); existingGroup != "" {
		return existingGroup
	}

	// Fall back to standard determination
	return determineServiceGroup(namespace, labels, annotations, config)
}

// findBestMatchingCRDServiceGroup finds the best existing CRD service group for a discovered service
func findBestMatchingCRDServiceGroup(
	homerConfig *HomerConfig,
	namespace string,
	annotations map[string]string,
) string {
	bestMatch := ""
	bestScore := 0

	// Minimum score threshold to avoid weak matches
	const minScoreThreshold = 30

	for _, service := range homerConfig.Services {
		// Only consider CRD services (services with CRD items)
		if !hasCRDItems(service) {
			continue
		}

		serviceName := getServiceName(&service)
		if serviceName == "" {
			continue
		}

		// Score the match based on various criteria
		score := scoreCRDServiceGroupMatch(serviceName, namespace, annotations)
		if score > bestScore && score >= minScoreThreshold {
			bestScore = score
			bestMatch = serviceName
		}
	}

	return bestMatch
}

// hasCRDItems checks if a service has any items from CRD source
func hasCRDItems(service Service) bool {
	for _, item := range service.Items {
		if item.Source == CRDSource {
			return true
		}
	}
	return false
}

// scoreCRDServiceGroupMatch scores how well a discovered service matches an existing CRD service group
func scoreCRDServiceGroupMatch(
	crdServiceName string,
	discoveredNamespace string,
	discoveredAnnotations map[string]string,
) int {
	score := 0

	// Check for explicit service name annotation first (highest priority)
	if serviceNameAnnotation, exists := discoveredAnnotations["service.homer.rajsingh.info/name"]; exists {
		if strings.EqualFold(serviceNameAnnotation, crdServiceName) {
			score += 200 // Highest priority for explicit service name annotation
		}
		return score // If annotation exists, only consider annotation-based matching
	}

	// Fall back to namespace-based matching
	normalizedCRDName := strings.ToLower(crdServiceName)
	normalizedNamespace := strings.ToLower(discoveredNamespace)

	// Direct name match with namespace
	if normalizedCRDName == normalizedNamespace {
		score += 100
	} else if strings.Contains(normalizedCRDName, normalizedNamespace) ||
		strings.Contains(normalizedNamespace, normalizedCRDName) {
		// Partial name match with namespace (for namespace variations like "kube-system")
		score += 50
	}

	return score
}

// validateCRDServicePreservation validates that CRD services are preserved after discovery
func validateCRDServicePreservation(originalConfig, processedConfig *HomerConfig) error {
	crdServiceNames := make(map[string]bool)
	for _, service := range originalConfig.Services {
		if hasCRDItems(service) {
			if serviceName := getServiceName(&service); serviceName != "" {
				crdServiceNames[serviceName] = true
			}
		}
	}

	preservedCRDServices := make(map[string]bool)
	for _, service := range processedConfig.Services {
		if hasCRDItems(service) {
			if serviceName := getServiceName(&service); serviceName != "" {
				preservedCRDServices[serviceName] = true
			}
		}
	}

	var missingServices []string
	for serviceName := range crdServiceNames {
		if !preservedCRDServices[serviceName] {
			missingServices = append(missingServices, serviceName)
		}
	}

	if len(missingServices) > 0 {
		return fmt.Errorf("CRD services lost: %v", missingServices)
	}

	return nil
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
	configMapMutex.Lock()
	defer configMapMutex.Unlock()

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

// MatchesDomainFilters checks if any of the provided hosts match the domain filters
func MatchesDomainFilters(hosts []string, domainFilters []string) bool {
	// If no domain filters specified, include everything
	if len(domainFilters) == 0 {
		return true
	}

	// Check if any host matches any filter
	for _, host := range hosts {
		if utils.MatchesHostDomainFilters(host, domainFilters) {
			return true
		}
	}

	return false
}

// ValidateHomerConfig validates the Homer configuration for common issues
func ValidateHomerConfig(config *HomerConfig) error {
	if config == nil {
		return fmt.Errorf("config: nil")
	}

	if config.Title == "" {
		return fmt.Errorf("title: required")
	}

	if config.Colors.Light.Background != "" && !isValidColor(config.Colors.Light.Background) {
		return fmt.Errorf("light background color: %s", config.Colors.Light.Background)
	}
	if config.Colors.Dark.Background != "" && !isValidColor(config.Colors.Dark.Background) {
		return fmt.Errorf("dark background color: %s", config.Colors.Dark.Background)
	}

	if config.Defaults.Layout != "" && config.Defaults.Layout != "columns" && config.Defaults.Layout != "list" {
		return fmt.Errorf("layout: %s", config.Defaults.Layout)
	}

	if config.Defaults.ColorTheme != "" {
		validThemes := []string{"auto", "light", "dark"}
		valid := false
		for _, theme := range validThemes {
			if config.Defaults.ColorTheme == theme {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("colorTheme: %s", config.Defaults.ColorTheme)
		}
	}

	for i, service := range config.Services {
		serviceName := getServiceName(&service)
		if serviceName == "" {
			return fmt.Errorf("service[%d]: missing name", i)
		}

		for j, item := range service.Items {
			itemName := getItemName(&item)
			if itemName == "" {
				return fmt.Errorf("service[%d].item[%d]: missing name", i, j)
			}

			itemURL := getItemURL(&item)
			if itemURL != "" && !isValidURL(itemURL) {
				return fmt.Errorf("service[%d].item[%d]: invalid URL %s", i, j, itemURL)
			}
		}
	}

	return nil
}

func isValidColor(color string) bool {
	if strings.HasPrefix(color, "#") {
		hex := color[1:]
		if len(hex) != 3 && len(hex) != 6 {
			return false
		}
		for _, c := range hex {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
		return true
	}

	commonColors := map[string]bool{
		"red": true, "blue": true, "green": true, "yellow": true,
		"orange": true, "purple": true, "pink": true, "brown": true,
		"black": true, "white": true, "gray": true, "grey": true,
	}

	lowerColor := strings.ToLower(color)
	if commonColors[lowerColor] {
		return true
	}

	return strings.HasPrefix(lowerColor, "rgb")
}

func isValidURL(url string) bool {
	if url == "" {
		return true
	}
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

	// Sort services and items alphabetically for consistent ordering
	sortServicesAndItems(config)
}

// sortServicesAndItems sorts services and their items alphabetically by name
func sortServicesAndItems(config *HomerConfig) {
	// Sort services alphabetically by name
	sort.Slice(config.Services, func(i, j int) bool {
		nameI := getServiceName(&config.Services[i])
		nameJ := getServiceName(&config.Services[j])
		return strings.ToLower(nameI) < strings.ToLower(nameJ)
	})

	// Sort items within each service alphabetically by name
	for i := range config.Services {
		sort.Slice(config.Services[i].Items, func(x, y int) bool {
			nameX := getItemName(&config.Services[i].Items[x])
			nameY := getItemName(&config.Services[i].Items[y])
			return strings.ToLower(nameX) < strings.ToLower(nameY)
		})
	}
}

// marshalHomerConfigToYAML creates properly formatted YAML for Homer
func marshalHomerConfigToYAML(config *HomerConfig) ([]byte, error) {
	configMap := make(map[string]interface{})

	addBasicFields(configMap, config)
	addHotkeyConfig(configMap, config)
	addColorsConfig(configMap, config)
	addDefaultsConfig(configMap, config)
	addProxyConfig(configMap, config)
	addMessageConfig(configMap, config)
	addLinksAndServices(configMap, config)

	return yaml.Marshal(configMap)
}

// addBasicFields adds basic configuration fields
func addBasicFields(configMap map[string]interface{}, config *HomerConfig) {
	if config.Title != "" {
		configMap["title"] = config.Title
	}
	if config.Subtitle != "" {
		configMap["subtitle"] = config.Subtitle
	}
	if config.DocumentTitle != "" {
		configMap["documentTitle"] = config.DocumentTitle
	}
	if config.Logo != "" {
		configMap["logo"] = config.Logo
	}
	if config.Icon != "" {
		configMap["icon"] = config.Icon
	}
	configMap["header"] = config.Header
	if config.Footer != "" {
		if config.Footer == FooterHidden {
			configMap["footer"] = false
		} else {
			configMap["footer"] = config.Footer
		}
	}
	if config.Columns != "" {
		configMap["columns"] = config.Columns
	}
	if config.ConnectivityCheck {
		configMap["connectivityCheck"] = config.ConnectivityCheck
	}
	if config.Theme != "" {
		configMap["theme"] = config.Theme
	}
	if len(config.Stylesheet) > 0 {
		configMap["stylesheet"] = config.Stylesheet
	}
	if config.ExternalConfig != "" {
		configMap["externalConfig"] = config.ExternalConfig
	}
}

// addHotkeyConfig adds hotkey configuration
func addHotkeyConfig(configMap map[string]interface{}, config *HomerConfig) {
	if config.Hotkey.Search != "" {
		configMap["hotkey"] = map[string]interface{}{
			"search": config.Hotkey.Search,
		}
	}
}

// addColorsConfig adds colors configuration
func addColorsConfig(configMap map[string]interface{}, config *HomerConfig) {
	if config.Colors.Light != (ThemeColors{}) || config.Colors.Dark != (ThemeColors{}) {
		colorsMap := make(map[string]interface{})
		if config.Colors.Light != (ThemeColors{}) {
			lightMap := make(map[string]interface{})
			addThemeColors(lightMap, config.Colors.Light)
			if len(lightMap) > 0 {
				colorsMap["light"] = lightMap
			}
		}
		if config.Colors.Dark != (ThemeColors{}) {
			darkMap := make(map[string]interface{})
			addThemeColors(darkMap, config.Colors.Dark)
			if len(darkMap) > 0 {
				colorsMap["dark"] = darkMap
			}
		}
		if len(colorsMap) > 0 {
			configMap["colors"] = colorsMap
		}
	}
}

// addDefaultsConfig adds defaults configuration
func addDefaultsConfig(configMap map[string]interface{}, config *HomerConfig) {
	if config.Defaults.ColorTheme != "" || config.Defaults.Layout != "" {
		defaultsMap := make(map[string]interface{})
		if config.Defaults.Layout != "" {
			defaultsMap["layout"] = config.Defaults.Layout
		}
		if config.Defaults.ColorTheme != "" {
			defaultsMap["colorTheme"] = config.Defaults.ColorTheme
		}
		configMap["defaults"] = defaultsMap
	}
}

// addProxyConfig adds proxy configuration
func addProxyConfig(configMap map[string]interface{}, config *HomerConfig) {
	if config.Proxy.UseCredentials || len(config.Proxy.Headers) > 0 {
		proxyMap := make(map[string]interface{})
		if config.Proxy.UseCredentials {
			proxyMap["useCredentials"] = config.Proxy.UseCredentials
		}
		if len(config.Proxy.Headers) > 0 {
			proxyMap["headers"] = config.Proxy.Headers
		}
		configMap["proxy"] = proxyMap
	}
}

// addMessageConfig adds message configuration
func addMessageConfig(configMap map[string]interface{}, config *HomerConfig) {
	if config.Message.Title != "" || config.Message.Content != "" || config.Message.Url != "" {
		messageMap := make(map[string]interface{})
		if config.Message.Title != "" {
			messageMap["title"] = config.Message.Title
		}
		if config.Message.Content != "" {
			messageMap["content"] = config.Message.Content
		}
		if config.Message.Icon != "" {
			messageMap["icon"] = config.Message.Icon
		}
		if config.Message.Style != "" {
			messageMap["style"] = config.Message.Style
		}
		if config.Message.Url != "" {
			messageMap["url"] = config.Message.Url
		}
		if config.Message.RefreshInterval > 0 {
			messageMap["refreshInterval"] = config.Message.RefreshInterval
		}
		if len(config.Message.Mapping) > 0 {
			messageMap["mapping"] = config.Message.Mapping
		}
		configMap["message"] = messageMap
	}
}

// addLinksAndServices adds links and services configuration
func addLinksAndServices(configMap map[string]interface{}, config *HomerConfig) {
	if len(config.Links) > 0 {
		configMap["links"] = config.Links
	}
	if len(config.Services) > 0 {
		configMap["services"] = flattenServicesForYAML(config.Services)
	}
}

// addThemeColors adds theme color fields to a map
func addThemeColors(colorMap map[string]interface{}, colors ThemeColors) {
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
	if colors.BackgroundImage != "" {
		colorMap["background-image"] = colors.BackgroundImage
	}
	if colors.CardBackground != "" {
		colorMap["card-background"] = colors.CardBackground
	}
	if colors.CardShadow != "" {
		colorMap["card-shadow"] = colors.CardShadow
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
	if colors.Link != "" {
		colorMap["link"] = colors.Link
	}
	if colors.LinkHover != "" {
		colorMap["link-hover"] = colors.LinkHover
	}
}

func flattenServicesForYAML(services []Service) []map[string]interface{} {
	if len(services) == 0 {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(services))

	for _, service := range services {
		serviceMap := make(map[string]interface{})

		// Add parameters with smart type inference
		if service.Parameters != nil {
			for key, value := range service.Parameters {
				serviceMap[key] = smartInferType(value)
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
			if flattenedItems := flattenItemsForYAML(service.Items); len(flattenedItems) > 0 {
				serviceMap["items"] = flattenedItems
			}
		}

		if len(serviceMap) > 0 {
			result = append(result, serviceMap)
		}
	}

	return result
}

func flattenItemsForYAML(items []Item) []map[string]interface{} {
	if len(items) == 0 {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(items))

	for _, item := range items {
		itemMap := make(map[string]interface{})

		// Add parameters with smart type inference
		if item.Parameters != nil {
			for key, value := range item.Parameters {
				yamlKey := getYAMLKey(key)
				itemMap[yamlKey] = smartInferType(value)
			}
		}

		// Add nested objects
		if item.NestedObjects != nil {
			for objectName, objectMap := range item.NestedObjects {
				itemMap[objectName] = objectMap
			}
		}

		if len(itemMap) > 0 {
			result = append(result, itemMap)
		}
	}

	return result
}

// getYAMLKey converts parameter keys to proper YAML field names
func getYAMLKey(key string) string {
	switch strings.ToLower(key) {
	case "legacyapi":
		return "legacyApi"
	case "librarytype":
		return "libraryType"
	case "usecredentials":
		return "useCredentials"
	default:
		return key
	}
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
	configMutex.Lock()
	defer configMutex.Unlock()

	for serviceIndex := range homerConfig.Services {
		service := &homerConfig.Services[serviceIndex]

		// Create a new slice for filtered items to avoid race conditions
		filteredItems := make([]Item, 0, len(service.Items))

		// Keep only items that did NOT come from the specified HTTPRoute source
		for _, item := range service.Items {
			// Remove items that match this HTTPRoute source
			if item.Source == sourceName && item.Namespace == sourceNamespace {
				// Skip this item (remove it)
				continue
			}
			// Keep this item by creating a copy
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
	configMutex.Lock()
	defer configMutex.Unlock()

	for serviceIndex := range homerConfig.Services {
		service := &homerConfig.Services[serviceIndex]

		// Create a new slice for filtered items to avoid race conditions
		filteredItems := make([]Item, 0, len(service.Items))

		// Keep items that did NOT come from the specified Ingress source
		for _, item := range service.Items {
			if !(item.Source == sourceName && item.Namespace == sourceNamespace) {
				// Keep this item by creating a copy
				filteredItems = append(filteredItems, item)
			}
		}

		// Update the service with filtered items
		service.Items = filteredItems
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
