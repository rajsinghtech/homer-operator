package homer

// +kubebuilder:object:generate=true

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// HomerConfig contains base configuration for Homer dashboard.
type HomerConfig struct {
	// Title to which is displayed on the dashboard.
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	// Subtitle
	Subtitle string `json:"subtitle,omitempty" yaml:"subtitle,omitempty"`
	// DocumentTitle sets the browser tab title
	DocumentTitle string `json:"documentTitle,omitempty" yaml:"documentTitle,omitempty"`
	// Logo used within dashboard.
	Logo string `json:"logo,omitempty" yaml:"logo,omitempty"`
	// Icon alternative to logo using FontAwesome classes
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// Header show/hide header
	Header bool `json:"header" yaml:"header"`
	// Footer to be displayed on the dashboard.
	Footer string `json:"footer,omitempty" yaml:"footer,omitempty"`
	// Columns layout configuration
	Columns string `json:"columns,omitempty" yaml:"columns,omitempty"`
	// ConnectivityCheck enables VPN/connectivity monitoring
	ConnectivityCheck bool `json:"connectivityCheck,omitempty" yaml:"connectivityCheck,omitempty"`
	// Hotkey configuration
	Hotkey HotkeyConfig `json:"hotkey,omitempty" yaml:"hotkey,omitempty"`
	// Theme name from themes directory
	Theme string `json:"theme,omitempty" yaml:"theme,omitempty"`
	// Stylesheet additional CSS files
	Stylesheet []string `json:"stylesheet,omitempty" yaml:"stylesheet,omitempty"`
	// Colors extensive color scheme support
	Colors ColorConfig `json:"colors,omitempty" yaml:"colors,omitempty"`
	// Defaults are your default settings for the dashboard.
	Defaults DefaultConfig `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	// Proxy configuration
	Proxy ProxyConfig `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	// Message dynamic message support
	Message MessageConfig `json:"message,omitempty" yaml:"message,omitempty"`
	// Links contains any additional links (static) to be displayed on the dashboard.
	Links []Link `json:"links,omitempty" yaml:"links,omitempty"`
	// List of Services to be displayed on the dashboard.
	Services []Service `json:"services,omitempty" yaml:"services,omitempty"`
	// ExternalConfig URL to load config from external source
	ExternalConfig string `json:"externalConfig,omitempty" yaml:"externalConfig,omitempty"`
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

// LoadConfigFromFile loads HomerConfig from a YAML file.
func LoadConfigFromFile(filename string) (*HomerConfig, error) {
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

func CreateConfigMap(config *HomerConfig, name string, namespace string, ingresses networkingv1.IngressList, owner client.Object) corev1.ConfigMap {
	UpdateHomerConfig(config, ingresses)

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
func CreateConfigMapWithHTTPRoutes(config *HomerConfig, name string, namespace string, ingresses networkingv1.IngressList, httproutes []gatewayv1.HTTPRoute, owner client.Object) corev1.ConfigMap {
	UpdateHomerConfig(config, ingresses)
	// Update config with HTTPRoutes
	for _, httproute := range httproutes {
		UpdateHomerConfigHTTPRoute(config, &httproute)
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

func CreateDeployment(name string, namespace string, replicas *int32, owner client.Object) appsv1.Deployment {
	var defaultReplicas int32 = 1
	if replicas == nil {
		replicas = &defaultReplicas
	}
	image := "b4bz/homer"
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
								"cp /config/config.yml /www/assets/config.yml",
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/config",
								},
								{
									Name:      "assets-volume",
									MountPath: "/www/assets",
								},
							},
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
					Volumes: []corev1.Volume{
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
					},
				},
			},
		},
	}
	return *d
}

// ValidateTheme validates that the theme name is supported by Homer
func ValidateTheme(theme string) error {
	if theme == "" {
		return nil // Empty theme is valid (uses default)
	}

	validThemes := []string{"default", "neon", "walkxcode"}
	for _, valid := range validThemes {
		if theme == valid {
			return nil
		}
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
func ResolveAPIKeyFromSecret(ctx context.Context, k8sClient client.Client, item *Item, secretRef *SecretKeyRef, defaultNamespace string) error {
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
func GeneratePWAManifest(title, description, themeColor, backgroundColor, display, startURL string, icons map[string]string) string {
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
		truncateString(title, 12), // Short name max 12 chars
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

// CreateDeploymentWithAssets creates a Deployment with custom asset support and PWA manifest
func CreateDeploymentWithAssets(name string, namespace string, replicas *int32, owner client.Object, assetsConfigMapName string, pwaManifest string) appsv1.Deployment {
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
	if assetsConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "custom-assets",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: assetsConfigMapName,
					},
				},
			},
		})

		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name:      "custom-assets",
			MountPath: "/custom-assets",
		})

		// Update init command to also copy custom assets
		initCommand = "cp /config/config.yml /www/assets/config.yml && " +
			"cp -r /custom-assets/* /www/assets/ 2>/dev/null || true"
	}

	// Add PWA manifest creation if provided
	if pwaManifest != "" {
		initCommand += " && cat > /www/assets/manifest.json << 'EOF'\n" + pwaManifest + "\nEOF"
	}

	// Complete init command (FSGroup handles permissions)
	// No chmod needed - FSGroup=1000 ensures proper volume permissions

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
	return *d
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
func UpdateHomerConfig(config *HomerConfig, ingresses networkingv1.IngressList) error {
	var services []Service
	// iterate over all ingresses and add them to the dashboard
	for _, ingress := range ingresses.Items {
		for _, rule := range ingress.Spec.Rules {
			item := Item{}
			service := Service{}
			service.Name = ingress.ObjectMeta.Namespace
			item.Name = ingress.ObjectMeta.Name
			service.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ns-128.png"
			if len(ingress.Spec.TLS) > 0 {
				item.Url = "https://" + rule.Host
			} else {
				item.Url = "http://" + rule.Host
			}
			item.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ing-128.png"
			item.Subtitle = rule.Host
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
func UpdateHomerConfigIngress(homerConfig *HomerConfig, ingress networkingv1.Ingress) {
	service := Service{}
	item := Item{}
	service.Name = ingress.ObjectMeta.Namespace
	item.Name = ingress.ObjectMeta.Name
	service.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ns-128.png"

	// Check if there are any rules before accessing them
	if len(ingress.Spec.Rules) == 0 {
		// Skip Ingress resources without rules
		return
	}

	host := ingress.Spec.Rules[0].Host
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
	for sx, s := range homerConfig.Services {
		if s.Name == service.Name {
			for ix, i := range s.Items {
				if i.Name == item.Name {
					homerConfig.Services[sx].Items[ix] = item
					return
				}
			}
			homerConfig.Services[sx].Items = append(homerConfig.Services[sx].Items, item)
			return
		}
	}
	// Service not found, add it
	service.Items = []Item{item}
	homerConfig.Services = append(homerConfig.Services, service)
}

func UpdateConfigMapIngress(cm *corev1.ConfigMap, ingress networkingv1.Ingress) {
	homerConfig := HomerConfig{}
	err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig)
	if err != nil {
		return
	}
	UpdateHomerConfigIngress(&homerConfig, ingress)
	objYAML, err := yaml.Marshal(homerConfig)
	if err != nil {
		return
	}
	cm.Data["config.yml"] = string(objYAML)
}

// UpdateHomerConfigHTTPRoute updates the HomerConfig with HTTPRoute information
func UpdateHomerConfigHTTPRoute(homerConfig *HomerConfig, httproute *gatewayv1.HTTPRoute) {
	service := Service{}
	item := Item{}
	service.Name = httproute.ObjectMeta.Namespace
	item.Name = httproute.ObjectMeta.Name
	service.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ns-128.png"

	// Get the first hostname if available
	hostname := ""
	if len(httproute.Spec.Hostnames) > 0 {
		hostname = string(httproute.Spec.Hostnames[0])
	}

	// Determine protocol based on parent Gateway listener configuration
	protocol := determineProtocolFromHTTPRoute(httproute)

	item.Url = protocol + "://" + hostname
	item.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/svc-128.png"
	item.Subtitle = hostname

	// Process annotations safely
	processItemAnnotations(&item, httproute.ObjectMeta.Annotations)
	processServiceAnnotations(&service, httproute.ObjectMeta.Annotations)

	// Update or add the service
	for sx, s := range homerConfig.Services {
		if s.Name == service.Name {
			for ix, i := range s.Items {
				if i.Name == item.Name {
					homerConfig.Services[sx].Items[ix] = item
					return
				}
			}
			homerConfig.Services[sx].Items = append(homerConfig.Services[sx].Items, item)
			return
		}
	}
	// Service not found, add it
	service.Items = []Item{item}
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
	for key, value := range annotations {
		if strings.HasPrefix(key, "item.homer.rajsingh.info/") {
			fieldName := strings.TrimPrefix(key, "item.homer.rajsingh.info/")
			switch strings.ToLower(fieldName) {
			case "name":
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
				item.Keywords = value
			case "url":
				item.Url = value
			case "target":
				item.Target = value
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
				item.Warningvalue = value
			case "danger_value":
				item.Dangervalue = value
			case "usecredentials":
				item.UseCredentials = strings.ToLower(value) == "true"
			}
		}
	}
}

// processServiceAnnotations safely processes service annotations without reflection
func processServiceAnnotations(service *Service, annotations map[string]string) {
	for key, value := range annotations {
		if strings.HasPrefix(key, "service.homer.rajsingh.info/") {
			fieldName := strings.TrimPrefix(key, "service.homer.rajsingh.info/")
			switch strings.ToLower(fieldName) {
			case "name":
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
func UpdateConfigMapHTTPRoute(cm *corev1.ConfigMap, httproute *gatewayv1.HTTPRoute) {
	homerConfig := HomerConfig{}
	err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig)
	if err != nil {
		return
	}
	UpdateHomerConfigHTTPRoute(&homerConfig, httproute)
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
		if config.Defaults.ColorTheme != "auto" && config.Defaults.ColorTheme != "light" && config.Defaults.ColorTheme != "dark" {
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
	commonColors := []string{"red", "blue", "green", "yellow", "orange", "purple", "pink", "brown", "black", "white", "gray", "grey"}
	for _, commonColor := range commonColors {
		if strings.ToLower(color) == commonColor {
			return true
		}
	}

	// Check for rgb/rgba format (basic check)
	if strings.HasPrefix(strings.ToLower(color), "rgb") {
		return true
	}

	return false
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
func CreateAssetConfigMap(name string, namespace string, assets map[string][]byte, owner client.Object) corev1.ConfigMap {
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

// marshalHomerConfigToYAML creates properly formatted YAML for Homer
func marshalHomerConfigToYAML(config *HomerConfig) ([]byte, error) {
	// Create a map with proper field names for Homer
	configMap := map[string]interface{}{
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
		configMap["hotkey"] = map[string]interface{}{
			"search": config.Hotkey.Search,
		}
	}

	// Add colors with proper field names
	if config.Colors.Light.HighlightPrimary != "" || config.Colors.Dark.HighlightPrimary != "" {
		colors := map[string]interface{}{}

		if config.Colors.Light.HighlightPrimary != "" {
			light := map[string]interface{}{}
			if config.Colors.Light.HighlightPrimary != "" {
				light["highlight-primary"] = config.Colors.Light.HighlightPrimary
			}
			if config.Colors.Light.HighlightSecondary != "" {
				light["highlight-secondary"] = config.Colors.Light.HighlightSecondary
			}
			if config.Colors.Light.HighlightHover != "" {
				light["highlight-hover"] = config.Colors.Light.HighlightHover
			}
			if config.Colors.Light.Background != "" {
				light["background"] = config.Colors.Light.Background
			}
			if config.Colors.Light.CardBackground != "" {
				light["card-background"] = config.Colors.Light.CardBackground
			}
			if config.Colors.Light.Text != "" {
				light["text"] = config.Colors.Light.Text
			}
			if config.Colors.Light.TextHeader != "" {
				light["text-header"] = config.Colors.Light.TextHeader
			}
			if config.Colors.Light.TextTitle != "" {
				light["text-title"] = config.Colors.Light.TextTitle
			}
			if config.Colors.Light.TextSubtitle != "" {
				light["text-subtitle"] = config.Colors.Light.TextSubtitle
			}
			if config.Colors.Light.CardShadow != "" {
				light["card-shadow"] = config.Colors.Light.CardShadow
			}
			if config.Colors.Light.Link != "" {
				light["link"] = config.Colors.Light.Link
			}
			if config.Colors.Light.LinkHover != "" {
				light["link-hover"] = config.Colors.Light.LinkHover
			}
			if config.Colors.Light.BackgroundImage != "" {
				light["background-image"] = config.Colors.Light.BackgroundImage
			}
			if len(light) > 0 {
				colors["light"] = light
			}
		}

		if config.Colors.Dark.HighlightPrimary != "" {
			dark := map[string]interface{}{}
			if config.Colors.Dark.HighlightPrimary != "" {
				dark["highlight-primary"] = config.Colors.Dark.HighlightPrimary
			}
			if config.Colors.Dark.HighlightSecondary != "" {
				dark["highlight-secondary"] = config.Colors.Dark.HighlightSecondary
			}
			if config.Colors.Dark.HighlightHover != "" {
				dark["highlight-hover"] = config.Colors.Dark.HighlightHover
			}
			if config.Colors.Dark.Background != "" {
				dark["background"] = config.Colors.Dark.Background
			}
			if config.Colors.Dark.CardBackground != "" {
				dark["card-background"] = config.Colors.Dark.CardBackground
			}
			if config.Colors.Dark.Text != "" {
				dark["text"] = config.Colors.Dark.Text
			}
			if config.Colors.Dark.TextHeader != "" {
				dark["text-header"] = config.Colors.Dark.TextHeader
			}
			if config.Colors.Dark.TextTitle != "" {
				dark["text-title"] = config.Colors.Dark.TextTitle
			}
			if config.Colors.Dark.TextSubtitle != "" {
				dark["text-subtitle"] = config.Colors.Dark.TextSubtitle
			}
			if config.Colors.Dark.CardShadow != "" {
				dark["card-shadow"] = config.Colors.Dark.CardShadow
			}
			if config.Colors.Dark.Link != "" {
				dark["link"] = config.Colors.Dark.Link
			}
			if config.Colors.Dark.LinkHover != "" {
				dark["link-hover"] = config.Colors.Dark.LinkHover
			}
			if config.Colors.Dark.BackgroundImage != "" {
				dark["background-image"] = config.Colors.Dark.BackgroundImage
			}
			if len(dark) > 0 {
				colors["dark"] = dark
			}
		}

		if len(colors) > 0 {
			configMap["colors"] = colors
		}
	}

	// Add defaults
	if config.Defaults.Layout != "" || config.Defaults.ColorTheme != "" {
		defaults := map[string]interface{}{}
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
		proxy := map[string]interface{}{}
		proxy["useCredentials"] = config.Proxy.UseCredentials
		if len(config.Proxy.Headers) > 0 {
			proxy["headers"] = config.Proxy.Headers
		}
		configMap["proxy"] = proxy
	}

	// Add message if configured
	if config.Message.Title != "" || config.Message.Content != "" {
		message := map[string]interface{}{}
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
