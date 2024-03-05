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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DashboardSpec defines the desired state of Dashboard
type DashboardSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Dashboard. Edit dashboard_types.go to remove/update
	ConfigMap   ConfigMap   `json:"configMap,omitempty"`
	HomerConfig HomerConfig `json:"homerConfig,omitempty"`
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
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type HomerConfig struct {
	Title    string    `json:"title,omitempty"`
	Subtitle string    `json:"subtitle,omitempty"`
	Logo     string    `json:"logo,omitempty"`
	Header   string    `json:"header,omitempty"`
	Services []Service `json:"services,omitempty"`
	Footer   string    `json:"footer"`
	// Columns          string        `json:"columns"` // Consider changing this to int if it's always a number
	// ConnectivityCheck bool          `json:"connectivityCheck"`
	// Proxy            ProxyConfig   `json:"proxy"`
	Defaults DefaultConfig `json:"defaults,omitempty"`
	Links   []Link        `json:"links,omitempty"`
	// Theme            string        `json:"theme"`
	// Colors           struct {
	// 	Light struct {
	// 		HighlightPrimary   string `json:"highlight-primary"`
	// 		HighlightSecondary string `json:"highlight-secondary"`
	// 		HighlightHover     string `json:"highlight-hover"`
	// 		Background         string `json:"background"`
	// 		CardBackground     string `json:"card-background"`
	// 		Text               string `json:"text"`
	// 		TextHeader         string `json:"text-header"`
	// 		TextTitle          string `json:"text-title"`
	// 		TextSubtitle       string `json:"text-subtitle"`
	// 		CardShadow         string `json:"card-shadow"`
	// 		Link               string `json:"link"`
	// 		LinkHover          string `json:"link-hover"`
	// 		BackgroundImage    string `json:"background-image"`
	// 	} `json:"light"`
	// 	Dark struct {
	// 		HighlightPrimary   string `json:"highlight-primary"`
	// 		HighlightSecondary string `json:"highlight-secondary"`
	// 		HighlightHover     string `json:"highlight-hover"`
	// 		Background         string `json:"background"`
	// 		CardBackground     string `json:"card-background"`
	// 		Text               string `json:"text"`
	// 		TextHeader         string `json:"text-header"`
	// 		TextTitle          string `json:"text-title"`
	// 		TextSubtitle       string `json:"text-subtitle"`
	// 		CardShadow         string `json:"card-shadow"`
	// 		Link               string `json:"link"`
	// 		LinkHover          string `json:"link-hover"`
	// 		BackgroundImage    string `json:"background-image"`
	// 	} `json:"dark"`
	// } `json:"colors"`
	// Message struct {
	// 	Style   string `json:"style"`
	// 	Title   string `json:"title"`
	// 	Icon    string `json:"icon"`
	// 	Content string `json:"content"`
	// } `json:"message"`

}

type ProxyConfig struct {
	UseCredentials bool `json:"useCredentials"`
}

type DefaultConfig struct {
	Layout     string `json:"layout"`
	ColorTheme string `json:"colorTheme"`
}

type Service struct {
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Logo  string `json:"logo"`
	Items []Item `json:"items"`
}

type Item struct {
	Name       string `json:"name"`
	Logo       string `json:"logo,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Keywords   string `json:"keywords,omitempty"`
	Url        string `json:"url"`
	Target     string `json:"target,omitempty"`
	Tagstyle   string `json:"tagstyle,omitempty"`
	Type       string `json:"type,omitempty"`
	Class      string `json:"class,omitempty"`
	Background string `json:"background,omitempty"`
}

type Link struct {
	Name   string `json:"name"`
	Icon   string `json:"icon,omitempty"`
	Url    string `json:"url"`
	Target string `json:"target,omitempty"`
}