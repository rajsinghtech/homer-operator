/*
Copyright 2024 RajSingh.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	homer "github.com/rajsinghtech/homer-operator/pkg/homer"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DashboardSpec defines the desired state of Dashboard
type DashboardSpec struct {

	// ConfigMap is where you want said homer configuration stored.
	ConfigMap ConfigMap `json:"configMap,omitempty"`

	// HomerConfig is base/default Homer configuration.
	HomerConfig homer.HomerConfig `json:"homerConfig,omitempty"`

	// Replicas is the number of desired pods for the Homer dashboard deployment.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// DNSPolicy defines the DNS policy for the Homer dashboard deployment pods.
	// +kubebuilder:validation:Enum=ClusterFirst;ClusterFirstWithHostNet;Default;None
	// +kubebuilder:default="ClusterFirst"
	DNSPolicy string `json:"dnsPolicy,omitempty"`

	// DNSConfig defines the DNS parameters for the Homer dashboard deployment pods.
	// Only applicable when DNSPolicy is set to "None".
	// This field accepts a JSON string representation of PodDNSConfig
	DNSConfig string `json:"dnsConfig,omitempty"`

	// Resources defines resource requirements for the Homer container.
	// If not specified, sensible defaults will be applied.
	// +kubebuilder:validation:Optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// Assets configures custom assets (logos, icons, CSS) for the dashboard.
	Assets *AssetsConfig `json:"assets,omitempty"`

	// Secrets configures Secret references for sensitive smart card data.
	Secrets *SmartCardSecrets `json:"secrets,omitempty"`

	// GatewaySelector optionally filters HTTPRoutes by Gateway labels. If not specified, all HTTPRoutes are included.
	GatewaySelector *metav1.LabelSelector `json:"gatewaySelector,omitempty"`

	// HTTPRouteSelector optionally filters HTTPRoutes by labels. If not specified, all HTTPRoutes are included.
	HTTPRouteSelector *metav1.LabelSelector `json:"httpRouteSelector,omitempty"`

	// IngressSelector optionally filters Ingresses by labels. If not specified, all Ingresses are included.
	IngressSelector *metav1.LabelSelector `json:"ingressSelector,omitempty"`

	// DomainFilters optionally filters HTTPRoutes and Ingresses by domain names. If not specified, all domains are included.
	DomainFilters []string `json:"domainFilters,omitempty"`

	// ServiceGrouping configures how services are grouped in the dashboard.
	ServiceGrouping *ServiceGroupingConfig `json:"serviceGrouping,omitempty"`

	// ValidationLevel defines the strictness of annotation validation.
	// +kubebuilder:validation:Enum=strict;warn;none
	// +kubebuilder:default="warn"
	ValidationLevel string `json:"validationLevel,omitempty"`

	// HealthCheck configures health checking for discovered services.
	HealthCheck *ServiceHealthConfig `json:"healthCheck,omitempty"`

	// Advanced configures advanced aggregation and analysis features.
	Advanced *AdvancedConfig `json:"advanced,omitempty"`
}

// DashboardStatus defines the observed state of Dashboard
type DashboardStatus struct {
	// Ready indicates if the Homer dashboard deployment is ready
	Ready bool `json:"ready"`

	// Replicas is the desired number of replicas
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas is the number of available replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Conditions represent the latest available observations of the Dashboard's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.readyReplicas`,description="Ready replicas"
//+kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.status.replicas`,description="Desired replicas"
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.availableReplicas`,description="Available replicas"
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Dashboard is the Schema for the dashboards API
type Dashboard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DashboardSpec   `json:"spec,omitempty"`
	Status DashboardStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DashboardList contains a list of Dashboard
type DashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Dashboard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Dashboard{}, &DashboardList{})
}

type ConfigMap struct {
	// Name is the ConfigMap name where Homer configuration is stored.
	Name string `json:"name,omitempty"`
	// Key is the key in the ConfigMap where Homer configuration is stored.
	Key string `json:"key,omitempty"`
}

// AssetsConfig defines custom assets for the Homer dashboard
type AssetsConfig struct {
	// ConfigMapRef references a ConfigMap containing custom assets
	ConfigMapRef *AssetConfigMapRef `json:"configMapRef,omitempty"`
	// Icons configures custom icon assets
	Icons *IconConfig `json:"icons,omitempty"`
	// PWA configures Progressive Web App manifest generation
	PWA *PWAConfig `json:"pwa,omitempty"`
}

// AssetConfigMapRef references a ConfigMap containing custom assets
type AssetConfigMapRef struct {
	// Name of the ConfigMap containing assets
	Name string `json:"name"`
	// Optional namespace (defaults to Dashboard namespace)
	Namespace string `json:"namespace,omitempty"`
}

// IconConfig defines custom icon configuration
type IconConfig struct {
	// Favicon custom favicon.ico file
	Favicon string `json:"favicon,omitempty"`
	// AppleTouchIcon custom apple-touch-icon.png file
	AppleTouchIcon string `json:"appleTouchIcon,omitempty"`
	// PWAIcon192 custom pwa-192x192.png file
	PWAIcon192 string `json:"pwaIcon192,omitempty"`
	// PWAIcon512 custom pwa-512x512.png file
	PWAIcon512 string `json:"pwaIcon512,omitempty"`
}

// PWAConfig defines Progressive Web App configuration
type PWAConfig struct {
	// Enabled controls whether PWA manifest should be generated
	Enabled bool `json:"enabled,omitempty"`
	// Name is the full name of the PWA
	Name string `json:"name,omitempty"`
	// ShortName is the short name of the PWA (for home screen)
	ShortName string `json:"shortName,omitempty"`
	// Description describes the PWA
	Description string `json:"description,omitempty"`
	// ThemeColor defines the theme color
	ThemeColor string `json:"themeColor,omitempty"`
	// BackgroundColor defines the background color
	BackgroundColor string `json:"backgroundColor,omitempty"`
	// Display mode for the PWA (standalone, fullscreen, minimal-ui, browser)
	// +kubebuilder:validation:Enum=standalone;fullscreen;minimal-ui;browser
	// +kubebuilder:default="standalone"
	Display string `json:"display,omitempty"`
	// StartURL is the start URL for the PWA
	StartURL string `json:"startUrl,omitempty"`
}

// SecretKeyRef references a key in a Secret
type SecretKeyRef struct {
	// Name of the Secret
	Name string `json:"name"`
	// Key in the Secret to use
	Key string `json:"key"`
	// Optional namespace (defaults to Dashboard namespace)
	Namespace string `json:"namespace,omitempty"`
}

// SmartCardSecrets defines Secret references for smart card sensitive data
type SmartCardSecrets struct {
	// APIKey references a Secret containing the API key
	APIKey *SecretKeyRef `json:"apiKey,omitempty"`
	// Token references a Secret containing an authentication token
	Token *SecretKeyRef `json:"token,omitempty"`
	// Password references a Secret containing a password
	Password *SecretKeyRef `json:"password,omitempty"`
	// Username references a Secret containing a username
	Username *SecretKeyRef `json:"username,omitempty"`
	// Headers references Secrets for custom authentication headers
	Headers map[string]*SecretKeyRef `json:"headers,omitempty"`
}

// ServiceGroupingConfig defines how services are grouped in the dashboard
type ServiceGroupingConfig struct {
	// Strategy defines the grouping strategy (namespace, label, custom)
	// +kubebuilder:validation:Enum=namespace;label;custom
	// +kubebuilder:default="namespace"
	Strategy string `json:"strategy,omitempty"`

	// LabelKey specifies which label to use for grouping when strategy is 'label'
	LabelKey string `json:"labelKey,omitempty"`

	// CustomRules defines custom grouping rules when strategy is 'custom'
	CustomRules []GroupingRule `json:"customRules,omitempty"`
}

// GroupingRule defines a custom grouping rule
type GroupingRule struct {
	// Name of the service group this rule creates
	Name string `json:"name"`

	// Condition defines labels/annotations that must match for this rule to apply
	Condition map[string]string `json:"condition"`

	// Priority determines rule evaluation order (higher priority evaluated first)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Priority int `json:"priority,omitempty"`
}

// ResourceRequirements describes resource requirements for the Homer container
type ResourceRequirements struct {
	// Limits describes the maximum amount of compute resources allowed.
	// +kubebuilder:validation:Optional
	Limits map[string]resource.Quantity `json:"limits,omitempty"`

	// Requests describes the minimum amount of compute resources required.
	// +kubebuilder:validation:Optional
	Requests map[string]resource.Quantity `json:"requests,omitempty"`
}

// ServiceHealthConfig defines health checking configuration for services
type ServiceHealthConfig struct {
	// Enabled controls whether health checking is enabled
	Enabled bool `json:"enabled,omitempty"`

	// Interval between health checks
	// +kubebuilder:default="30s"
	Interval string `json:"interval,omitempty"`

	// Timeout for health check requests
	// +kubebuilder:default="10s"
	Timeout string `json:"timeout,omitempty"`

	// HealthPath is the path for health checks
	// +kubebuilder:default="/health"
	HealthPath string `json:"healthPath,omitempty"`

	// ExpectedCode is the expected HTTP status code
	// +kubebuilder:default=200
	ExpectedCode int `json:"expectedCode,omitempty"`

	// Headers to include in health check requests
	Headers map[string]string `json:"headers,omitempty"`
}

// AdvancedConfig configures advanced aggregation and analysis features
type AdvancedConfig struct {
	// EnableDependencyAnalysis enables automatic service dependency detection
	EnableDependencyAnalysis bool `json:"enableDependencyAnalysis,omitempty"`

	// EnableMetricsAggregation enables service metrics collection and display
	EnableMetricsAggregation bool `json:"enableMetricsAggregation,omitempty"`

	// EnableLayoutOptimization enables automatic service layout optimization
	EnableLayoutOptimization bool `json:"enableLayoutOptimization,omitempty"`

	// MaxServicesPerGroup limits the number of services per group (0 = unlimited)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxServicesPerGroup int `json:"maxServicesPerGroup,omitempty"`

	// MaxItemsPerService limits the number of items per service (0 = unlimited)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxItemsPerService int `json:"maxItemsPerService,omitempty"`
}
