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
	homer "github.com/rajsinghtech/homer-operator.git/pkg/homer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DashboardSpec defines the desired state of Dashboard
type DashboardSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of dashboard
	// Important: Run "make" to regenerate code after modifying this file

	// ConfigMap is where you want said homer configuration stored.
	ConfigMap ConfigMap `json:"configMap,omitempty"`

	// HomerConfig is base/default Homer configuration.
	HomerConfig homer.HomerConfig `json:"homerConfig,omitempty"`

	// Replicas is the number of desired pods for the Homer dashboard deployment.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

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
}

// DashboardStatus defines the observed state of Dashboard
type DashboardStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
	// ThemeColor defines the theme color for the PWA
	ThemeColor string `json:"themeColor,omitempty"`
	// BackgroundColor defines the background color for the PWA
	BackgroundColor string `json:"backgroundColor,omitempty"`
	// Display mode for the PWA (standalone, fullscreen, minimal-ui, browser)
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
