package homer

// +kubebuilder:object:generate=true

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	neturl "net/url"
	"os"
	"path"
	"reflect"
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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
	LocalCluster        = "local"
	NameField           = "name"
	IconField           = "icon"
	LogoField           = "logo"
	ClassField          = "class"
	SubtitleField       = "subtitle"
	TagField            = "tag"
	KeywordsField       = "keywords"
	URLField            = "url"
	TargetField         = "target"
	TagStyleField       = "tagstyle"
	TypeField           = "type"
	BackgroundField     = "background"
	EndpointField       = "endpoint"
	WarningValueField   = "warning_value"
	DangerValueField    = "danger_value"
	BooleanTrue         = "true"
	BooleanFalse        = "false"
	JSONNullValue       = "null"
	FooterHidden        = "__FOOTER_HIDDEN__"
	ProtocolHTTPS       = "https"
	ProtocolHTTP        = "http"
	// HTTPRouteProtocolAnnotation is populated by the controller after it
	// resolves the selected Gateway listener. It is intentionally not part of
	// the user-facing Homer annotation surface.
	HTTPRouteProtocolAnnotation = "homer.rajsingh.info/discovered-protocol"
	NamespaceIconURL            = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/" +
		"resources/labeled/ns-128.png"
	IngressIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/" +
		"resources/labeled/ing-128.png"
	ServiceIconURL = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/" +
		"resources/labeled/svc-128.png"
	assetVolumeNamePrefix = "asset-volume-"
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
	// +nullable
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	// +nullable
	Subtitle string `json:"subtitle,omitempty" yaml:"subtitle,omitempty"`
	// +nullable
	DocumentTitle string `json:"documentTitle,omitempty" yaml:"documentTitle,omitempty"`
	// +nullable
	Logo string `json:"logo,omitempty" yaml:"logo,omitempty"`
	// +nullable
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// +nullable
	Header bool `json:"header,omitempty" yaml:"header,omitempty"`
	// Footer can be false to hide the footer or a string containing HTML content.
	// +kubebuilder:validation:Type=""
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	Footer string `json:"footer,omitempty" yaml:"footer,omitempty"`
	// Columns accepts both the numeric and string forms supported by Homer
	// (for example, 3 and "3").
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	Columns any `json:"columns,omitempty" yaml:"columns,omitempty"`
	// +nullable
	ConnectivityCheck *bool `json:"connectivityCheck,omitempty" yaml:"connectivityCheck,omitempty"`
	// +nullable
	Hotkey HotkeyConfig `json:"hotkey,omitempty" yaml:"hotkey,omitempty"`
	// +nullable
	Theme string `json:"theme,omitempty" yaml:"theme,omitempty"`
	// Stylesheet accepts either one path or an array of paths, matching
	// upstream Homer. Keep the value open so both forms (and explicit null)
	// survive CRD and external-config round trips.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	Stylesheet any `json:"stylesheet,omitempty" yaml:"stylesheet,omitempty"`
	// +nullable
	Colors ColorConfig `json:"colors,omitempty" yaml:"colors,omitempty"`
	// +nullable
	Defaults DefaultConfig `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	// +nullable
	Proxy ProxyConfig `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	// +nullable
	Message MessageConfig `json:"message,omitempty" yaml:"message,omitempty"`
	// +nullable
	Links []Link `json:"links,omitempty" yaml:"links,omitempty"`
	// +nullable
	Services []Service `json:"services,omitempty" yaml:"services,omitempty"`
	// +nullable
	ExternalConfig string `json:"externalConfig,omitempty" yaml:"externalConfig,omitempty"`
	// UpdateIntervalMs is the default refresh interval for Homer smart cards.
	// A value of zero disables automatic refresh, as in upstream Homer.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	UpdateIntervalMs any `json:"updateIntervalMs,omitempty" yaml:"updateIntervalMs,omitempty"`

	// RawFields preserves root-level Homer options that are newer than the
	// operator's typed model. Homer intentionally treats configuration as an
	// open-ended document, so dropping an unknown field would be a parity bug.
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
	// presentFields retains the complete input document's root fields. Unlike
	// RawFields, it includes modeled fields so explicit empty arrays, empty
	// strings, and null values can be emitted again when the config is written
	// as Homer YAML.
	presentFields map[string]json.RawMessage `json:"-" yaml:"-"`

	// These flags retain whether a value was explicitly present in YAML/JSON.
	// They are intentionally private so they do not become CRD fields. This is
	// needed for fields whose zero value is meaningful, such as header: false
	// and updateIntervalMs: 0.
	headerSet         bool `json:"-" yaml:"-"`
	updateIntervalSet bool `json:"-" yaml:"-"`
	footerSet         bool `json:"-" yaml:"-"`
	footerValueSet    bool `json:"-" yaml:"-"`
	footerValue       any  `json:"-" yaml:"-"`
	proxySet          bool `json:"-" yaml:"-"`
	messageSet        bool `json:"-" yaml:"-"`
}

// UnmarshalYAML custom unmarshaler to handle footer: false
func (c *HomerConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type Alias HomerConfig
	aux := &struct {
		Footer         any   `yaml:"footer,omitempty"`
		Header         *bool `yaml:"header,omitempty"`
		UpdateInterval any   `yaml:"updateIntervalMs,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	rawYAML, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(rawYAML, (*Alias)(c)); err != nil {
		return err
	}
	if err := yaml.Unmarshal(rawYAML, aux); err != nil {
		return err
	}
	switch v := aux.Footer.(type) {
	case bool:
		c.footerValue = v
		if !v {
			c.Footer = FooterHidden
		} else {
			c.Footer = ""
		}
	case string:
		c.footerValue = v
		c.Footer = v
	}
	if aux.Header != nil {
		c.Header = *aux.Header
		c.headerSet = true
	}
	c.headerSet = hasYAMLKey(raw, "header")
	c.updateIntervalSet = hasYAMLKey(raw, "updateIntervalMs")
	c.footerSet = hasYAMLKey(raw, "footer")
	c.footerValueSet = c.footerSet
	c.proxySet = hasYAMLKey(raw, "proxy")
	c.messageSet = hasYAMLKey(raw, "message")
	if err := captureYAMLFields(raw, &c.presentFields); err != nil {
		return err
	}
	return captureUnknownFields(raw, homerConfigJSONFields, &c.RawFields)
}

// UnmarshalJSON custom unmarshaler to handle footer: false
func (c *HomerConfig) UnmarshalJSON(data []byte) error {
	type Alias HomerConfig
	aux := &struct {
		Footer         any   `json:"footer,omitempty"`
		Header         *bool `json:"header,omitempty"`
		UpdateInterval any   `json:"updateIntervalMs,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	switch v := aux.Footer.(type) {
	case bool:
		c.footerValue = v
		if !v {
			c.Footer = FooterHidden
		} else {
			c.Footer = ""
		}
	case string:
		c.footerValue = v
		c.Footer = v
	}
	if aux.Header != nil {
		c.Header = *aux.Header
		c.headerSet = true
	}
	if aux.UpdateInterval != nil {
		c.UpdateIntervalMs = aux.UpdateInterval
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	_, c.headerSet = raw["header"]
	_, c.updateIntervalSet = raw["updateIntervalMs"]
	_, c.footerSet = raw["footer"]
	c.footerValueSet = c.footerSet
	_, c.proxySet = raw["proxy"]
	_, c.messageSet = raw["message"]
	if err := captureJSONFields(data, &c.presentFields); err != nil {
		return err
	}
	return captureUnknownJSONFields(data, homerConfigJSONFields, &c.RawFields)
}

// MarshalJSON emits the upstream representation, including the special
// boolean form of footer: false and explicitly supplied zero/false values.
func (c HomerConfig) MarshalJSON() ([]byte, error) {
	type alias HomerConfig
	encoded, err := json.Marshal(alias(c))
	if err != nil {
		return nil, err
	}

	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	if c.Footer == FooterHidden {
		fields["footer"] = json.RawMessage("false")
	} else if c.footerValueSet {
		value, err := json.Marshal(c.footerValue)
		if err != nil {
			return nil, err
		}
		fields["footer"] = value
	}
	if c.headerSet {
		if raw, ok := c.presentFields["header"]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
			fields["header"] = append(json.RawMessage(nil), raw...)
		} else {
			value, err := json.Marshal(c.Header)
			if err != nil {
				return nil, err
			}
			fields["header"] = value
		}
	}
	if c.proxySet {
		if _, exists := fields["proxy"]; !exists {
			fields["proxy"] = json.RawMessage(JSONNullValue)
		}
	}
	if c.messageSet {
		if _, exists := fields["message"]; !exists {
			fields["message"] = json.RawMessage(JSONNullValue)
		}
	}
	if c.updateIntervalSet {
		if _, exists := fields["updateIntervalMs"]; !exists {
			value, err := json.Marshal(c.UpdateIntervalMs)
			if err != nil {
				return nil, err
			}
			fields["updateIntervalMs"] = value
		}
	}
	restoreExplicitRootJSONFields(fields, c)
	for key, value := range c.RawFields {
		if _, exists := fields[key]; !exists {
			fields[key] = append(json.RawMessage(nil), value...)
		}
	}
	return json.Marshal(fields)
}

func restoreExplicitRootJSONFields(fields map[string]json.RawMessage, config HomerConfig) {
	restoreExplicitRootStringFields(fields, config)
	restoreExplicitRootNullableFields(fields, config)
	restoreExplicitRootCollectionFields(fields, config)
	restoreExplicitRootObjectFields(fields, config)
}

func restoreExplicitRootStringFields(fields map[string]json.RawMessage, config HomerConfig) {
	values := []struct {
		key   string
		value string
	}{
		{"title", config.Title},
		{"subtitle", config.Subtitle},
		{"documentTitle", config.DocumentTitle},
		{"logo", config.Logo},
		{"icon", config.Icon},
		{"theme", config.Theme},
		{"externalConfig", config.ExternalConfig},
	}
	for _, field := range values {
		if config.rootFieldPresent(field.key) && field.value == "" {
			fields[field.key] = explicitZeroJSONValue(config, field.key, field.value)
		}
	}
}

func restoreExplicitRootNullableFields(fields map[string]json.RawMessage, config HomerConfig) {
	if config.rootFieldPresent("columns") && config.Columns == nil {
		fields["columns"] = explicitZeroJSONValue(config, "columns", nil)
	}
	if config.rootFieldPresent("connectivityCheck") && config.ConnectivityCheck == nil {
		fields["connectivityCheck"] = explicitZeroJSONValue(config, "connectivityCheck", nil)
	}
}

func restoreExplicitRootCollectionFields(fields map[string]json.RawMessage, config HomerConfig) {
	collections := []struct {
		key   string
		empty bool
		value any
	}{
		{"stylesheet", isEmptyStylesheet(config.Stylesheet), []string{}},
		{"links", len(config.Links) == 0, []Link{}},
		{"services", len(config.Services) == 0, []Service{}},
	}
	for _, field := range collections {
		if config.rootFieldPresent(field.key) && field.empty {
			fields[field.key] = explicitZeroJSONValue(config, field.key, field.value)
		}
	}
}

func restoreExplicitRootObjectFields(fields map[string]json.RawMessage, config HomerConfig) {
	objects := []struct {
		key   string
		zero  bool
		value any
	}{
		{"hotkey", isZeroHotkey(config.Hotkey), map[string]any{}},
		{"colors", isZeroColors(config.Colors), map[string]any{}},
		{"defaults", isZeroDefaults(config.Defaults), map[string]any{}},
		{"proxy", isZeroProxy(config.Proxy), map[string]any{}},
		{"message", isZeroMessage(config.Message), map[string]any{}},
	}
	for _, field := range objects {
		if config.rootFieldPresent(field.key) && field.zero {
			fields[field.key] = explicitZeroJSONValue(config, field.key, field.value)
		}
	}
}

func (c HomerConfig) rootFieldPresent(key string) bool {
	_, ok := c.presentFields[key]
	return ok
}

func explicitZeroJSONValue(config HomerConfig, key string, current any) json.RawMessage {
	if raw, ok := config.presentFields[key]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
		return append(json.RawMessage(nil), raw...)
	}
	value, err := json.Marshal(current)
	if err != nil {
		return json.RawMessage(JSONNullValue)
	}
	return value
}

func isZeroHotkey(config HotkeyConfig) bool {
	return config.Search == "" && len(config.RawFields) == 0
}

func isZeroColors(config ColorConfig) bool {
	return !themeColorsConfigured(config.Light) && !themeColorsConfigured(config.Dark) && len(config.RawFields) == 0
}

func isZeroDefaults(config DefaultConfig) bool {
	return config.Layout == "" && config.ColorTheme == "" && len(config.RawFields) == 0
}

func isZeroProxy(config ProxyConfig) bool {
	return !config.UseCredentials && len(config.Headers) == 0 && len(config.RawFields) == 0
}

func isZeroMessage(config MessageConfig) bool {
	return config.Url == "" && len(config.Mapping) == 0 && config.RefreshInterval == nil &&
		config.Style == "" && config.Title == "" && config.Icon == "" && config.Content == "" &&
		len(config.RawFields) == 0
}

func isEmptyStylesheet(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Slice:
		return reflected.Len() == 0
	default:
		return false
	}
}

var homerConfigJSONFields = map[string]struct{}{
	"title": {}, "subtitle": {}, "documentTitle": {}, "logo": {}, "icon": {},
	"header": {}, "footer": {}, "columns": {}, "connectivityCheck": {},
	"hotkey": {}, "theme": {}, "stylesheet": {}, "colors": {}, "defaults": {},
	"proxy": {}, "message": {}, "links": {}, "services": {}, "externalConfig": {},
	"updateIntervalMs": {},
}

func hasYAMLKey(raw map[string]any, key string) bool {
	_, ok := raw[key]
	return ok
}

// ProxyConfig contains configuration for proxy settings.
// +kubebuilder:object:generate=false
// +kubebuilder:pruning:PreserveUnknownFields
type ProxyConfig struct {
	// +nullable
	UseCredentials bool `json:"useCredentials,omitempty" yaml:"useCredentials,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	Headers map[string]any `json:"headers,omitempty" yaml:"headers,omitempty"`
	// RawFields retains explicit empty/false values and newer proxy options
	// that upstream Homer may add before the operator models them.
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
}

func (p *ProxyConfig) UnmarshalJSON(data []byte) error {
	type alias ProxyConfig
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ProxyConfig(decoded)
	return captureJSONFields(data, &p.RawFields)
}

func (p ProxyConfig) MarshalJSON() ([]byte, error) {
	type alias ProxyConfig
	encoded, err := json.Marshal(alias(p))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, p.RawFields)
}

func (p *ProxyConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type alias ProxyConfig
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*p = ProxyConfig(decoded)
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &p.RawFields)
}

// DefaultConfig contains optional Homer default settings.
// +kubebuilder:pruning:PreserveUnknownFields
type DefaultConfig struct {
	// +nullable
	Layout string `json:"layout,omitempty" yaml:"layout,omitempty"`
	// +nullable
	ColorTheme string                     `json:"colorTheme,omitempty" yaml:"colorTheme,omitempty"`
	RawFields  map[string]json.RawMessage `json:"-" yaml:"-"`
}

func (d *DefaultConfig) UnmarshalJSON(data []byte) error {
	type alias DefaultConfig
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = DefaultConfig(decoded)
	return captureJSONFields(data, &d.RawFields)
}

func (d DefaultConfig) MarshalJSON() ([]byte, error) {
	type alias DefaultConfig
	encoded, err := json.Marshal(alias(d))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, d.RawFields)
}

func (d *DefaultConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type alias DefaultConfig
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*d = DefaultConfig(decoded)
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &d.RawFields)
}

func (d DefaultConfig) MarshalYAML() (any, error) {
	fields := map[string]any{}
	for key, value := range d.RawFields {
		fields[key] = decodeRawField(value)
	}
	if d.Layout != "" {
		fields["layout"] = d.Layout
	}
	if d.ColorTheme != "" {
		fields["colorTheme"] = d.ColorTheme
	}
	return fields, nil
}

// Service represents a Homer service group. The direct fields mirror
// upstream Homer. Parameters/NestedObjects remain supported for annotations
// and backwards compatibility with earlier operator releases.
// +kubebuilder:pruning:PreserveUnknownFields
type Service struct {
	// +nullable
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// +nullable
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// +nullable
	Logo string `json:"logo,omitempty" yaml:"logo,omitempty"`
	// +nullable
	Class string `json:"class,omitempty" yaml:"class,omitempty"`
	// +nullable
	Items []Item `json:"items,omitempty" yaml:"items,omitempty"`
	// +nullable
	Parameters map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	// +nullable
	NestedObjects    map[string]map[string]string `json:"nestedObjects,omitempty" yaml:"nestedObjects,omitempty"`
	RawFields        map[string]json.RawMessage   `json:"-" yaml:"-"`
	legacyParameters bool                         `json:"-" yaml:"-"`
	objectSet        bool                         `json:"-" yaml:"-"`
}

// QuickLink is an upstream Homer quick link entry.
// +kubebuilder:pruning:PreserveUnknownFields
type QuickLink struct {
	// +nullable
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// +nullable
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// +nullable
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// +nullable
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
	// +nullable
	Color     string                     `json:"color,omitempty" yaml:"color,omitempty"`
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
	objectSet bool                       `json:"-" yaml:"-"`
}

func (q *QuickLink) UnmarshalJSON(data []byte) error {
	type alias QuickLink
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*q = QuickLink(decoded)
	q.objectSet = true
	return captureJSONFields(data, &q.RawFields)
}

func (q QuickLink) MarshalJSON() ([]byte, error) {
	type alias QuickLink
	encoded, err := json.Marshal(alias(q))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, q.RawFields)
}

func (q *QuickLink) UnmarshalYAML(unmarshal func(any) error) error {
	type alias QuickLink
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*q = QuickLink(decoded)
	q.objectSet = true
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &q.RawFields)
}

func (q QuickLink) MarshalYAML() (any, error) {
	fields := map[string]any{}
	for key, value := range q.RawFields {
		fields[key] = decodeRawField(value)
	}
	if q.Name != "" {
		fields["name"] = q.Name
	}
	if q.Icon != "" {
		fields["icon"] = q.Icon
	}
	if q.URL != "" {
		fields["url"] = q.URL
	}
	if q.Target != "" {
		fields["target"] = q.Target
	}
	if q.Color != "" {
		fields["color"] = q.Color
	}
	return fields, nil
}

// Item represents a Homer service item. Common upstream fields are modeled
// directly; RawFields preserves smart-card-specific and future Homer fields.
// +kubebuilder:pruning:PreserveUnknownFields
type Item struct {
	// +nullable
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// +nullable
	Logo string `json:"logo,omitempty" yaml:"logo,omitempty"`
	// +nullable
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// +nullable
	Subtitle string `json:"subtitle,omitempty" yaml:"subtitle,omitempty"`
	// +nullable
	Tag string `json:"tag,omitempty" yaml:"tag,omitempty"`
	// +nullable
	Keywords string `json:"keywords,omitempty" yaml:"keywords,omitempty"`
	// +nullable
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// +nullable
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
	// +nullable
	TagStyle string `json:"tagstyle,omitempty" yaml:"tagstyle,omitempty"`
	// +nullable
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	// +nullable
	Background string `json:"background,omitempty" yaml:"background,omitempty"`
	// +nullable
	Class string `json:"class,omitempty" yaml:"class,omitempty"`
	// +nullable
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	// +nullable
	UseCredentials *bool `json:"useCredentials,omitempty" yaml:"useCredentials,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	Headers map[string]any `json:"headers,omitempty" yaml:"headers,omitempty"`
	// +nullable
	SuccessCodes []int `json:"successCodes,omitempty" yaml:"successCodes,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	UpdateIntervalMs any `json:"updateIntervalMs,omitempty" yaml:"updateIntervalMs,omitempty"`
	// +nullable
	Quick []QuickLink `json:"quick,omitempty" yaml:"quick,omitempty"`
	// +nullable
	Parameters map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	// +nullable
	NestedObjects map[string]map[string]string `json:"nestedObjects,omitempty" yaml:"nestedObjects,omitempty"`
	// +nullable
	ArrayObjects map[string][]map[string]string `json:"arrayObjects,omitempty" yaml:"arrayObjects,omitempty"`
	RawFields    map[string]json.RawMessage     `json:"-" yaml:"-"`
	Source       string                         `json:"-" yaml:"-"`
	Namespace    string                         `json:"-" yaml:"-"`
	LastUpdate   string                         `json:"-" yaml:"-"`
	// crdFoundation remains true after a discovered item enhances a CRD item.
	// The source is then allowed to identify that enhanced item without
	// allowing a second same-named resource to merge into it.
	crdFoundation      bool `json:"-" yaml:"-"`
	updateIntervalSet  bool `json:"-" yaml:"-"`
	legacyParameters   bool `json:"-" yaml:"-"`
	parametersInjected bool `json:"-" yaml:"-"`
	objectSet          bool `json:"-" yaml:"-"`
}

// UnmarshalJSON captures unknown upstream item fields so Kubernetes and
// external configuration can carry smart-card-specific options unchanged.
func (s *Service) UnmarshalJSON(data []byte) error {
	type alias Service
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = Service(decoded)
	s.objectSet = true
	if jsonFieldPresent(data, "parameters") {
		s.legacyParameters = true
	}
	return captureJSONFields(data, &s.RawFields)
}

// UnmarshalYAML captures unknown upstream service fields from file-based
// configuration while retaining the normal yaml.v2 decoding behavior.
func (s *Service) UnmarshalYAML(unmarshal func(any) error) error {
	type alias Service
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*s = Service(decoded)
	s.objectSet = true
	var rawKeys map[string]any
	if err := unmarshal(&rawKeys); err != nil {
		return err
	}
	if _, ok := rawKeys["parameters"]; ok {
		s.legacyParameters = true
	}
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &s.RawFields)
}

// UnmarshalJSON captures unknown upstream item fields so arbitrary Homer
// smart-card options survive a CRD round trip.
func (i *Item) UnmarshalJSON(data []byte) error {
	type alias Item
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*i = Item(decoded)
	i.objectSet = true
	if err := captureJSONFields(data, &i.RawFields); err != nil {
		return err
	}
	if jsonFieldPresent(data, "updateIntervalMs") {
		i.updateIntervalSet = true
	}
	if jsonFieldPresent(data, "parameters") {
		i.legacyParameters = true
	}
	return nil
}

// MarshalJSON merges preserved unknown fields back into an upstream service
// object. This keeps smart-card fields intact through CRD JSON round trips.
func (s Service) MarshalJSON() ([]byte, error) {
	type alias Service
	encoded, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for key, value := range s.RawFields {
		if _, exists := fields[key]; !exists {
			fields[key] = append(json.RawMessage(nil), value...)
		}
	}
	return json.Marshal(fields)
}

// MarshalJSON merges preserved unknown fields back into an upstream item.
func (i Item) MarshalJSON() ([]byte, error) {
	type alias Item
	encoded, err := json.Marshal(alias(i))
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for key, value := range i.RawFields {
		if _, exists := fields[key]; !exists {
			fields[key] = append(json.RawMessage(nil), value...)
		}
	}
	return json.Marshal(fields)
}

// UnmarshalYAML captures unknown upstream item fields from file-based
// configuration while retaining the normal yaml.v2 decoding behavior.
func (i *Item) UnmarshalYAML(unmarshal func(any) error) error {
	type alias Item
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*i = Item(decoded)
	i.objectSet = true
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if err := captureYAMLFields(raw, &i.RawFields); err != nil {
		return err
	}
	if _, ok := raw["updateIntervalMs"]; ok {
		i.updateIntervalSet = true
	}
	if _, ok := raw["parameters"]; ok {
		i.legacyParameters = true
	}
	return nil
}

func jsonFieldPresent(data []byte, field string) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	_, ok := raw[field]
	return ok
}

func captureUnknownJSONFields(data []byte, known map[string]struct{}, target *map[string]json.RawMessage) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	values := make(map[string]json.RawMessage)
	for key, value := range raw {
		if _, ok := known[key]; ok {
			continue
		}
		values[key] = append(json.RawMessage(nil), value...)
	}
	*target = values
	return nil
}

func captureUnknownFields(raw map[string]any, known map[string]struct{}, target *map[string]json.RawMessage) error {
	values := make(map[string]json.RawMessage)
	for key, value := range raw {
		if _, ok := known[key]; ok {
			continue
		}
		encoded, err := json.Marshal(normalizeYAMLValue(value))
		if err != nil {
			return err
		}
		values[key] = encoded
	}
	*target = values
	return nil
}

func normalizeYAMLValue(value any) any {
	switch value := value.(type) {
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, nested := range value {
			result[fmt.Sprint(key)] = normalizeYAMLValue(nested)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, nested := range value {
			result[key] = normalizeYAMLValue(nested)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, nested := range value {
			result[i] = normalizeYAMLValue(nested)
		}
		return result
	default:
		return value
	}
}

func decodeRawField(value json.RawMessage) any {
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return string(value)
	}
	return decoded
}

// Link describes a link in a Homer service configuration.
// +kubebuilder:pruning:PreserveUnknownFields
type Link struct {
	// +nullable
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// +nullable
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// +nullable
	Url string `json:"url,omitempty" yaml:"url,omitempty"`
	// +nullable
	Target    string                     `json:"target,omitempty" yaml:"target,omitempty"`
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
	objectSet bool                       `json:"-" yaml:"-"`
}

func (l *Link) UnmarshalJSON(data []byte) error {
	type alias Link
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*l = Link(decoded)
	l.objectSet = true
	return captureJSONFields(data, &l.RawFields)
}

func (l Link) MarshalJSON() ([]byte, error) {
	type alias Link
	encoded, err := json.Marshal(alias(l))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, l.RawFields)
}

func (l *Link) UnmarshalYAML(unmarshal func(any) error) error {
	type alias Link
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*l = Link(decoded)
	l.objectSet = true
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &l.RawFields)
}

func (l Link) MarshalYAML() (any, error) {
	fields := map[string]any{}
	for key, value := range l.RawFields {
		fields[key] = decodeRawField(value)
	}
	if l.Name != "" {
		fields["name"] = l.Name
	}
	if l.Icon != "" {
		fields["icon"] = l.Icon
	}
	if l.Url != "" {
		fields["url"] = l.Url
	}
	if l.Target != "" {
		fields["target"] = l.Target
	}
	return fields, nil
}

func getParameter(params map[string]string, key string) string {
	if params != nil {
		return params[key]
	}
	return ""
}

func getServiceName(service *Service) string {
	if service.Name != "" {
		return service.Name
	}
	if service.Parameters != nil {
		if name, ok := service.Parameters["name"]; ok {
			return name
		}
	}
	return service.Name
}

func getItemName(item *Item) string {
	if item.Name != "" {
		return item.Name
	}
	if item.Parameters != nil {
		if name, ok := item.Parameters["name"]; ok {
			return name
		}
	}
	return item.Name
}

func getItemURL(item *Item) string {
	if item.URL != "" {
		return item.URL
	}
	if item.Parameters != nil {
		if value, ok := item.Parameters["url"]; ok {
			return value
		}
	}
	return item.URL
}

func getItemTag(item *Item) string {
	if item.Tag != "" {
		return item.Tag
	}
	if item.Parameters != nil {
		if value, ok := item.Parameters["tag"]; ok {
			return value
		}
	}
	return item.Tag
}

func getItemKeywords(item *Item) string {
	if item.Keywords != "" {
		return item.Keywords
	}
	if item.Parameters != nil {
		if value, ok := item.Parameters["keywords"]; ok {
			return value
		}
	}
	return item.Keywords
}

func getItemSubtitle(item *Item) string {
	if item.Subtitle != "" {
		return item.Subtitle
	}
	if item.Parameters != nil {
		if value, ok := item.Parameters["subtitle"]; ok {
			return value
		}
	}
	return item.Subtitle
}

func getItemType(item *Item) string {
	if item.Type != "" {
		return item.Type
	}
	if item.Parameters != nil {
		if value, ok := item.Parameters["type"]; ok {
			return value
		}
	}
	return item.Type
}

func getItemEndpoint(item *Item) string {
	if item.Endpoint != "" {
		return item.Endpoint
	}
	if item.Parameters != nil {
		if value, ok := item.Parameters["endpoint"]; ok {
			return value
		}
	}
	return item.Endpoint
}

func setServiceParameter(service *Service, key, value string) {
	service.legacyParameters = true
	switch strings.ToLower(key) {
	case NameField:
		service.Name = value
	case IconField:
		service.Icon = value
	case LogoField:
		service.Logo = value
	case ClassField:
		service.Class = value
	}
	if service.Parameters == nil {
		service.Parameters = make(map[string]string)
	}
	service.Parameters[key] = value
}

func setItemParameter(item *Item, key, value string) {
	item.legacyParameters = true
	switch strings.ToLower(key) {
	case NameField:
		item.Name = value
	case LogoField:
		item.Logo = value
	case IconField:
		item.Icon = value
	case SubtitleField:
		item.Subtitle = value
	case TagField:
		item.Tag = value
	case KeywordsField:
		item.Keywords = value
	case URLField:
		item.URL = value
	case TargetField:
		item.Target = value
	case TagStyleField:
		item.TagStyle = value
	case TypeField:
		item.Type = value
	case BackgroundField:
		item.Background = value
	case ClassField:
		item.Class = value
	case EndpointField:
		item.Endpoint = value
	}
	if item.Parameters == nil {
		item.Parameters = make(map[string]string)
	}
	item.Parameters[key] = value
}

func cleanupHomerConfig(config *HomerConfig) {
	validServices := make([]Service, 0, len(config.Services))
	for _, service := range config.Services {
		// Keep direct upstream objects in declaration order. Parameter-map
		// objects are the operator's legacy representation and retain their
		// historical deterministic sorting behavior.
		ensureParameterMaps(&service.Parameters, &service.NestedObjects)

		var validItems []Item
		for _, item := range service.Items {
			ensureParameterMaps(&item.Parameters, &item.NestedObjects)
			if getItemName(&item) == "" && !itemHasConfiguration(&item) {
				continue
			}

			item.Source = "crd"
			item.Namespace = "dashboard"
			item.LastUpdate = "crd-defined"
			item.crdFoundation = false

			validItems = append(validItems, item)
		}

		service.Items = validItems
		if len(service.Items) == 0 {
			// Keep an explicitly configured empty service group. Homer permits
			// groups without a name and treats the items key as optional.
			if !service.objectSet && len(service.RawFields) == 0 && service.Name == "" && getParameter(service.Parameters, "name") == "" {
				continue
			}
		}
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

func itemHasConfiguration(item *Item) bool {
	if item == nil {
		return false
	}
	return item.objectSet || item.Logo != "" || item.Icon != "" || item.Subtitle != "" ||
		item.Tag != "" || item.Keywords != "" || item.URL != "" ||
		item.Target != "" || item.TagStyle != "" || item.Type != "" ||
		item.Background != "" || item.Class != "" || item.Endpoint != "" ||
		item.UseCredentials != nil || len(item.Headers) > 0 ||
		len(item.SuccessCodes) > 0 || item.UpdateIntervalMs != nil ||
		item.updateIntervalSet || len(item.Quick) > 0 || len(item.RawFields) > 0 ||
		len(item.Parameters) > 0 || len(item.NestedObjects) > 0 || len(item.ArrayObjects) > 0
}

// HotkeyConfig contains Homer keyboard shortcut settings.
// +kubebuilder:pruning:PreserveUnknownFields
type HotkeyConfig struct {
	// +nullable
	Search    string                     `json:"search,omitempty" yaml:"search,omitempty"`
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
}

func (h *HotkeyConfig) UnmarshalJSON(data []byte) error {
	type alias HotkeyConfig
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*h = HotkeyConfig(decoded)
	return captureJSONFields(data, &h.RawFields)
}

func (h HotkeyConfig) MarshalJSON() ([]byte, error) {
	type alias HotkeyConfig
	encoded, err := json.Marshal(alias(h))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, h.RawFields)
}

func (h *HotkeyConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type alias HotkeyConfig
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*h = HotkeyConfig(decoded)
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &h.RawFields)
}

// ColorConfig contains color scheme configuration.
// +kubebuilder:pruning:PreserveUnknownFields
type ColorConfig struct {
	// +nullable
	Light ThemeColors `json:"light,omitempty" yaml:"light,omitempty"`
	// +nullable
	Dark      ThemeColors                `json:"dark,omitempty" yaml:"dark,omitempty"`
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
}

func (c *ColorConfig) UnmarshalJSON(data []byte) error {
	type alias ColorConfig
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ColorConfig(decoded)
	return captureJSONFields(data, &c.RawFields)
}

func (c ColorConfig) MarshalJSON() ([]byte, error) {
	type alias ColorConfig
	encoded, err := json.Marshal(alias(c))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, c.RawFields)
}

func (c *ColorConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type alias ColorConfig
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*c = ColorConfig(decoded)
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &c.RawFields)
}

// ThemeColors contains color definitions for a theme.
// +kubebuilder:pruning:PreserveUnknownFields
type ThemeColors struct {
	// +nullable
	HighlightPrimary string `json:"highlight-primary,omitempty" yaml:"highlight-primary,omitempty"`
	// +nullable
	HighlightSecondary string `json:"highlight-secondary,omitempty" yaml:"highlight-secondary,omitempty"`
	// +nullable
	HighlightHover string `json:"highlight-hover,omitempty" yaml:"highlight-hover,omitempty"`
	// +nullable
	Background string `json:"background,omitempty" yaml:"background,omitempty"`
	// +nullable
	CardBackground string `json:"card-background,omitempty" yaml:"card-background,omitempty"`
	// +nullable
	Text string `json:"text,omitempty" yaml:"text,omitempty"`
	// +nullable
	TextHeader string `json:"text-header,omitempty" yaml:"text-header,omitempty"`
	// +nullable
	TextTitle string `json:"text-title,omitempty" yaml:"text-title,omitempty"`
	// +nullable
	TextSubtitle string `json:"text-subtitle,omitempty" yaml:"text-subtitle,omitempty"`
	// +nullable
	CardShadow string `json:"card-shadow,omitempty" yaml:"card-shadow,omitempty"`
	// +nullable
	Link string `json:"link,omitempty" yaml:"link,omitempty"`
	// +nullable
	LinkHover string `json:"link-hover,omitempty" yaml:"link-hover,omitempty"`
	// +nullable
	BackgroundImage string                     `json:"background-image,omitempty" yaml:"background-image,omitempty"`
	RawFields       map[string]json.RawMessage `json:"-" yaml:"-"`
}

func (c *ThemeColors) UnmarshalJSON(data []byte) error {
	type alias ThemeColors
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ThemeColors(decoded)
	return captureJSONFields(data, &c.RawFields)
}

func (c ThemeColors) MarshalJSON() ([]byte, error) {
	type alias ThemeColors
	encoded, err := json.Marshal(alias(c))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, c.RawFields)
}

func (c *ThemeColors) UnmarshalYAML(unmarshal func(any) error) error {
	type alias ThemeColors
	var decoded alias
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	*c = ThemeColors(decoded)
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	return captureYAMLFields(raw, &c.RawFields)
}

// MessageConfig contains dynamic message configuration.
// +kubebuilder:object:generate=false
// +kubebuilder:pruning:PreserveUnknownFields
type MessageConfig struct {
	// +nullable
	Url string `json:"url,omitempty" yaml:"url,omitempty"`
	// Mapping is an open object in upstream Homer. Values are normally string
	// property names, but preserving arbitrary JSON keeps newer mappings valid.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	Mapping map[string]any `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	// RefreshInterval is intentionally open-ended. Homer passes this value to
	// JavaScript's setTimeout, so numeric, numeric-string, false, and null
	// values all have meaningful upstream behavior.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	RefreshInterval any `json:"refreshInterval,omitempty" yaml:"refreshInterval,omitempty"`
	// +nullable
	Style string `json:"style,omitempty" yaml:"style,omitempty"`
	// +nullable
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	// +nullable
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`
	// +nullable
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
	// RawFields retains explicit empty/zero values and newer message options
	// that upstream Homer may add before the operator models them.
	RawFields map[string]json.RawMessage `json:"-" yaml:"-"`
}

func (m *MessageConfig) UnmarshalJSON(data []byte) error {
	type alias MessageConfig
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = MessageConfig(decoded)
	return captureJSONFields(data, &m.RawFields)
}

func (m MessageConfig) MarshalJSON() ([]byte, error) {
	type alias MessageConfig
	encoded, err := json.Marshal(alias(m))
	if err != nil {
		return nil, err
	}
	return mergeRawFields(encoded, m.RawFields)
}

func (m *MessageConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type alias MessageConfig
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	var decoded alias
	encoded, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	*m = MessageConfig(decoded)
	return captureYAMLFields(raw, &m.RawFields)
}

func captureJSONFields(data []byte, target *map[string]json.RawMessage) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	values := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		values[key] = append(json.RawMessage(nil), value...)
	}
	*target = values
	return nil
}

func captureYAMLFields(raw map[string]any, target *map[string]json.RawMessage) error {
	values := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		encoded, err := json.Marshal(normalizeYAMLValue(value))
		if err != nil {
			return err
		}
		values[key] = encoded
	}
	*target = values
	return nil
}

func mergeRawFields(encoded []byte, rawFields map[string]json.RawMessage) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for key, value := range rawFields {
		if _, exists := fields[key]; !exists {
			fields[key] = append(json.RawMessage(nil), value...)
		}
	}
	return json.Marshal(fields)
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
	services []corev1.Service,
	owner client.Object,
) (corev1.ConfigMap, error) {
	return CreateConfigMapWithDiscoveryConfig(config, name, namespace, ingresses, services, owner, nil)
}

// DiscoveryConfig controls how discovered Kubernetes resources are converted
// into Homer services and items.
type DiscoveryConfig struct {
	ServiceGrouping *ServiceGroupingConfig
	HealthCheck     *ServiceHealthConfig
	ValidationLevel ValidationLevel
	// IngressDomainFilters contains filters authorized for each discovered
	// Ingress. A single Ingress may contain both matching and non-matching
	// hosts, so filtering the resource before generation is not sufficient.
	IngressDomainFilters map[string][]string
	// HTTPRouteDomainFilters contains filters explicitly authorized by the
	// ClusterManager. Keys must be produced with HTTPRouteDomainFilterKey;
	// arbitrary resource annotations are never trusted as filter authority.
	HTTPRouteDomainFilters map[string][]string
}

// HTTPRouteDomainFilterKey returns the stable in-memory key used to associate
// ClusterManager-authorized domain filters with a discovered HTTPRoute.
func HTTPRouteDomainFilterKey(httproute *gatewayv1.HTTPRoute) string {
	if httproute == nil {
		return ""
	}
	return httproute.Namespace + "\x00" + discoveredResourceSource("httproute/"+httproute.Name, httproute.Annotations)
}

// IngressDomainFilterKey returns the stable in-memory key used to associate
// Dashboard-authorized domain filters with a discovered Ingress.
func IngressDomainFilterKey(ingress *networkingv1.Ingress) string {
	if ingress == nil {
		return ""
	}
	return ingress.Namespace + "\x00" + discoveredResourceSource("ingress/"+ingress.Name, ingress.Annotations)
}

// CreateConfigMapWithDiscoveryConfig creates a ConfigMap while applying the
// Dashboard's discovery feature configuration.
func CreateConfigMapWithDiscoveryConfig(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	services []corev1.Service,
	owner client.Object,
	discoveryConfig *DiscoveryConfig,
) (corev1.ConfigMap, error) {
	cleanupHomerConfig(config)

	for _, ingress := range ingresses.Items {
		updateHomerConfigIngress(config, ingress, nil, discoveryConfig)
	}

	for _, svc := range services {
		updateHomerConfigService(config, svc, discoveryConfig)
	}

	if discoveryConfig != nil && discoveryConfig.HealthCheck != nil && discoveryConfig.HealthCheck.Enabled {
		enhanceHomerConfigWithAggregation(config, discoveryConfig.HealthCheck)
	}

	if err := validateHomerConfig(config, discoveryValidationLevel(discoveryConfig)); err != nil {
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
	services []corev1.Service,
	owner client.Object,
	domainFilters []string,
) (corev1.ConfigMap, error) {
	return CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
		config, name, namespace, ingresses, httproutes, services, owner, domainFilters, nil)
}

// CreateConfigMapWithHTTPRoutesAndDiscoveryConfig creates a ConfigMap with
// Ingress, HTTPRoute, and Service discovery feature configuration applied.
func CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
	config *HomerConfig,
	name string,
	namespace string,
	ingresses networkingv1.IngressList,
	httproutes []gatewayv1.HTTPRoute,
	services []corev1.Service,
	owner client.Object,
	domainFilters []string,
	discoveryConfig *DiscoveryConfig,
) (corev1.ConfigMap, error) {
	originalConfig := *config

	cleanupHomerConfig(config)

	for _, ingress := range ingresses.Items {
		updateHomerConfigIngress(config, ingress, domainFilters, discoveryConfig)
	}
	for _, httproute := range httproutes {
		updateHomerConfigWithHTTPRoutes(config, &httproute, domainFilters, discoveryConfig)
	}

	for _, svc := range services {
		updateHomerConfigService(config, svc, discoveryConfig)
	}

	if err := validateCRDServicePreservation(&originalConfig, config); err != nil {
		slog.Warn("CRD service preservation check failed", "error", err)
	}

	if discoveryConfig != nil && discoveryConfig.HealthCheck != nil && discoveryConfig.HealthCheck.Enabled {
		enhanceHomerConfigWithAggregation(config, discoveryConfig.HealthCheck)
	}

	// Validate configuration before creating ConfigMap
	if err := validateHomerConfig(config, discoveryValidationLevel(discoveryConfig)); err != nil {
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
	HomerImage          string
	ConfigSyncImage     string
	// IconAliases maps one source asset to every Homer icon destination it
	// should populate. A single source file is commonly used for both the
	// favicon and Apple touch icon, so a scalar destination loses information.
	IconAliases    map[string][]string
	PageConfigKeys []string
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
	image := config.HomerImage
	if image == "" {
		image = "b4bz/homer:latest"
	}
	configSyncImage := config.ConfigSyncImage
	if configSyncImage == "" {
		configSyncImage = "alpine:3.18"
	}
	assetVolumeName := AssetVolumeName(config.AssetsConfigMapName)

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
		{
			Name: "operator-state-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// Add custom assets ConfigMap volume if provided
	if config.AssetsConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: assetVolumeName,
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
							Image: configSyncImage,
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
			slog.Warn("failed to parse DNSConfig", "error", err)
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

	if config != nil {
		for _, pageKey := range config.PageConfigKeys {
			if !isSafeRelativeAssetPath(pageKey) {
				slog.Warn("skipping unsafe Homer page asset path", "path", pageKey)
				continue
			}
			cmd += "ln -sf " + shellQuote("/config/"+pageKey) + " " + shellQuote("/www/assets/"+pageKey) + " && "
		}
	}

	cmd += "echo 'Initial setup complete. Starting config watch...' && "

	// Keep the symlinks and staged assets current using only basic POSIX
	// utilities. ConfigMap volumes are projected atomically, so refreshing on
	// each poll avoids depending on readlink/find/sha256sum being present in a
	// user-supplied config-sync image.
	cmd += "while true; do " +
		"ln -sf /config/config.yml /www/assets/config.yml && "
	if config != nil {
		for _, pageKey := range config.PageConfigKeys {
			if !isSafeRelativeAssetPath(pageKey) {
				continue
			}
			cmd += "ln -sf " + shellQuote("/config/"+pageKey) + " " + shellQuote("/www/assets/"+pageKey) + " && "
		}
	}
	cmd += buildAssetRefreshCommand(config) + "sleep 5; done"

	return cmd
}

// buildAssetRefreshCommand returns a shell fragment that replaces every
// asset previously staged by this operator and copies the current projected
// ConfigMap contents. The state file lets updates remove files that no longer
// exist in the ConfigMap, while keeping Homer-owned default assets intact.
func buildAssetRefreshCommand(config *DeploymentConfig) string {
	if config == nil {
		return "true && "
	}

	const stateDir = "/operator-state/.homer-operator-state"
	const stateFile = stateDir + "/staged"
	const backupDir = stateDir + "/backups"

	cmd := "true && "
	if config.AssetsConfigMapName != "" {
		cmd += "mkdir -p " + shellQuote(stateDir) + " && " +
			"if [ -f " + shellQuote(stateFile) + " ]; then " +
			"while IFS= read -r relative; do " +
			"if [ -n \"$relative\" ]; then " +
			"backup=\"" + backupDir + "/$relative\"; destination=\"/www/assets/$relative\"; " +
			"if [ -f \"$backup\" ]; then mkdir -p \"${destination%/*}\" && cp \"$backup\" \"$destination\"; elif [ -f \"$backup.missing\" ]; then rm -f \"$destination\"; else rm -f \"$destination\"; fi; " +
			"fi; " +
			"done < " + shellQuote(stateFile) + "; " +
			"fi && " +
			"rm -rf " + shellQuote(backupDir) + " && mkdir -p " + shellQuote(backupDir) + " && " +
			"rm -f " + shellQuote(stateFile) + " && touch " + shellQuote(stateFile) + " && " +
			"stage_asset() { " +
			"source=\"$1\"; relative=\"$2\"; " +
			"case \"$relative\" in " + assetRefreshReservedPaths(config) + ") return 0 ;; esac; " +
			"case \"$relative\" in ''|.|..|/*|../*|*'\\\\'*) return 0 ;; esac; " +
			"destination=\"/www/assets/$relative\"; backup=\"" + backupDir + "/$relative\"; " +
			"if [ ! -e \"$backup\" ] && [ ! -e \"$backup.missing\" ]; then " +
			"if [ -e \"$destination\" ]; then mkdir -p \"${backup%/*}\" && cp \"$destination\" \"$backup\"; else mkdir -p \"${backup%/*}\" && : > \"$backup.missing\"; fi; " +
			"fi; " +
			"mkdir -p \"${destination%/*}\" && cp \"$source\" \"$destination\" && printf '%s\\n' \"$relative\" >> " + shellQuote(stateFile) + "; " +
			"} && " +
			"if [ -d /custom-assets ]; then " +
			"stage_tree() { " +
			"for entry in \"$1\"/* \"$1\"/.[!.]* \"$1\"/..?*; do " +
			"[ -f \"$entry\" ] || [ -d \"$entry\" ] || continue; " +
			"if [ -d \"$entry\" ]; then stage_tree \"$entry\" \"$2${entry##*/}/\"; " +
			"else stage_asset \"$entry\" \"$2${entry##*/}\"; fi; " +
			"done; " +
			"}; stage_tree /custom-assets ''; fi && "
	}

	aliasSources := make([]string, 0, len(config.IconAliases))
	for source := range config.IconAliases {
		aliasSources = append(aliasSources, source)
	}
	slices.Sort(aliasSources)
	for _, source := range aliasSources {
		if !isSafeRelativeAssetPath(source) {
			slog.Warn("skipping unsafe Homer icon alias source", "source", source)
			continue
		}
		destinations := append([]string(nil), config.IconAliases[source]...)
		slices.Sort(destinations)
		for _, destination := range destinations {
			if !isSafeRelativeAssetPath(destination) {
				slog.Warn("skipping unsafe Homer icon alias destination", "source", source, "destination", destination)
				continue
			}
			if config.AssetsConfigMapName != "" {
				cmd += "if [ -f " + shellQuote("/custom-assets/"+source) + " ]; then stage_asset " +
					shellQuote("/custom-assets/"+source) + " " + shellQuote(destination) + "; fi && "
			}
		}
	}

	if config.PWAManifest != "" {
		cmd += "printf '%s' " + shellQuote(config.PWAManifest) + " > " + shellQuote("/www/assets/manifest.json") + " && "
	}

	return cmd
}

func assetRefreshReservedPaths(config *DeploymentConfig) string {
	paths := []string{"config.yml"}
	if config != nil && config.PWAManifest != "" {
		paths = append(paths, "manifest.json")
	}
	if config != nil {
		for _, pageKey := range config.PageConfigKeys {
			if isSafeRelativeAssetPath(pageKey) {
				paths = append(paths, pageKey)
			}
		}
	}
	return strings.Join(paths, "|")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func isSafeRelativeAssetPath(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") {
		return false
	}
	clean := path.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
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
		{
			Name:      "operator-state-volume",
			MountPath: "/operator-state",
		},
	}

	// Add custom assets mount if configured
	if config != nil && config.AssetsConfigMapName != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      AssetVolumeName(config.AssetsConfigMapName),
			MountPath: "/custom-assets",
		})
	}

	return mounts
}

// AssetVolumeName returns the stable Pod volume name used for an asset
// ConfigMap. ConfigMap names are DNS subdomains and can exceed the stricter
// DNS-1123 label constraints for volume and volume-mount names, so the source
// name is represented by a deterministic hash. The reserved prefix keeps the
// derived name separate from the fixed operator volumes.
func AssetVolumeName(configMapName string) string {
	if configMapName == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("homer-operator/assets/" + configMapName))
	return assetVolumeNamePrefix + hex.EncodeToString(digest[:])[:48]
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
			slog.Warn("failed to serialize DNSConfig", "error", err)
		}
	}
	return CreateDeployment(name, namespace, replicas, owner, config)
}

// ValidateTheme validates that the theme name is supported by Homer
func ValidateTheme(theme string) error {
	// Homer builds the CSS class as `theme-${theme}` and intentionally leaves
	// room for community/custom themes. Only the frontend knows which theme
	// stylesheet is available, so rejecting names here would prevent valid
	// upstream Homer configurations from being used.
	return nil
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
	item.parametersInjected = true
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
	item.parametersInjected = true
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
	item.parametersInjected = true
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
		startURL = "../"
	}
	if themeColor == "" {
		themeColor = "#3367d6"
	}
	if backgroundColor == "" {
		backgroundColor = "#ffffff"
	}

	type manifestIcon struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose"`
	}
	type manifest struct {
		Name            string         `json:"name"`
		ShortName       string         `json:"short_name"`
		Description     string         `json:"description"`
		StartURL        string         `json:"start_url"`
		Scope           string         `json:"scope"`
		Display         string         `json:"display"`
		ThemeColor      string         `json:"theme_color"`
		BackgroundColor string         `json:"background_color"`
		Icons           []manifestIcon `json:"icons"`
	}

	iconEntries := make([]manifestIcon, 0, 2)

	// Add default icons if not overridden
	if icons["192"] != "" {
		iconEntries = append(iconEntries, manifestIcon{
			Src: icons["192"], Sizes: "192x192", Type: "image/png", Purpose: "any maskable",
		})
	}

	if icons["512"] != "" {
		iconEntries = append(iconEntries, manifestIcon{
			Src: icons["512"], Sizes: "512x512", Type: "image/png", Purpose: "any maskable",
		})
	}

	// Add default Homer icons if no custom icons provided
	if len(iconEntries) == 0 {
		iconEntries = append(iconEntries,
			manifestIcon{Src: "icons/pwa-192x192.png", Sizes: "192x192", Type: "image/png", Purpose: "any maskable"},
			manifestIcon{Src: "icons/pwa-512x512.png", Sizes: "512x512", Type: "image/png", Purpose: "any maskable"},
		)
	}

	result, err := json.MarshalIndent(manifest{
		Name: title,
		ShortName: func() string {
			if shortName != "" {
				return truncateString(shortName, 12)
			}
			return truncateString(title, 12)
		}(),
		Description: description, StartURL: startURL, Scope: "../", Display: display,
		ThemeColor: themeColor, BackgroundColor: backgroundColor, Icons: iconEntries,
	}, "", "  ")
	if err != nil {
		return ""
	}
	return string(result)
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
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
	updateHomerConfigIngress(homerConfig, ingress, domainFilters, nil)
}

// UpdateHomerConfigIngressWithGrouping updates Homer config with custom grouping strategy
func UpdateHomerConfigIngressWithGrouping(
	homerConfig *HomerConfig,
	ingress networkingv1.Ingress,
	domainFilters []string,
	groupingConfig *ServiceGroupingConfig,
) {
	updateHomerConfigIngress(homerConfig, ingress, domainFilters, &DiscoveryConfig{ServiceGrouping: groupingConfig})
}

func updateHomerConfigIngress(
	homerConfig *HomerConfig,
	ingress networkingv1.Ingress,
	domainFilters []string,
	discoveryConfig *DiscoveryConfig,
) {
	var groupingConfig *ServiceGroupingConfig
	if discoveryConfig != nil {
		groupingConfig = discoveryConfig.ServiceGrouping
	}
	// Remove existing items before validating rules so an update that removes
	// every rule also removes the old discovered cards.
	removeItemsFromSource(
		homerConfig,
		discoveredResourceSource("ingress/"+ingress.ObjectMeta.Name, ingress.ObjectMeta.Annotations),
		ingress.ObjectMeta.Namespace,
	)

	effectiveDomainFilters := domainFilters
	if discoveryConfig != nil {
		if authorizedFilters, ok := discoveryConfig.IngressDomainFilters[IngressDomainFilterKey(&ingress)]; ok {
			effectiveDomainFilters = authorizedFilters
		}
	}

	// Setup service configuration
	service := setupIngressService(homerConfig, ingress, groupingConfig)

	// Validate ingress has rules
	if len(ingress.Spec.Rules) == 0 {
		return // The source was removed above; there is nothing to add.
	}

	processServiceAnnotations(&service, ingress.ObjectMeta.Annotations)

	// Create items from ingress rules
	items := createIngressItems(ingress, effectiveDomainFilters, discoveryValidationLevel(discoveryConfig))

	// Update service if we have matching items
	if len(items) > 0 {
		updateOrAddServiceItems(homerConfig, service, items)
	}
}

// UpdateHomerConfigService updates Homer config from a Kubernetes Service resource
func UpdateHomerConfigService(homerConfig *HomerConfig, svc corev1.Service) {
	updateHomerConfigService(homerConfig, svc, nil)
}

// UpdateHomerConfigServiceWithGrouping updates Homer config from a K8s Service with custom grouping
func UpdateHomerConfigServiceWithGrouping(
	homerConfig *HomerConfig,
	svc corev1.Service,
	groupingConfig *ServiceGroupingConfig,
) {
	updateHomerConfigService(homerConfig, svc, &DiscoveryConfig{ServiceGrouping: groupingConfig})
}

func updateHomerConfigService(
	homerConfig *HomerConfig,
	svc corev1.Service,
	discoveryConfig *DiscoveryConfig,
) {
	var groupingConfig *ServiceGroupingConfig
	if discoveryConfig != nil {
		groupingConfig = discoveryConfig.ServiceGrouping
	}
	serviceGroup := setupK8sServiceGroup(homerConfig, svc, groupingConfig)

	removeItemsFromSource(
		homerConfig,
		discoveredResourceSource("svc/"+svc.Name, svc.Annotations),
		svc.Namespace,
	)
	processServiceAnnotations(&serviceGroup, svc.Annotations)

	item := createK8sServiceItem(svc)
	processItemAnnotationsWithValidation(&item, svc.Annotations, discoveryValidationLevel(discoveryConfig))

	if clusterName := svc.Annotations["homer.rajsingh.info/cluster"]; clusterName != "" && clusterName != LocalCluster && !hasExplicitServiceURL(svc.Annotations) {
		// A cluster-internal DNS name for a remote Service resolves in the
		// operator's cluster, not the source cluster. Keep the discovered item
		// visible, but remove the misleading default link unless the user
		// supplied an explicit URL (or a CRD foundation later supplies one).
		slog.Warn("omitting default URL for remote Service without an explicit item URL annotation",
			"service", svc.Namespace+"/"+svc.Name, "cluster", clusterName,
			"annotation", "item.homer.rajsingh.info/url")
		item.URL = ""
		delete(item.Parameters, URLField)
	}

	// Apply cluster-name-suffix after annotation processing so it takes precedence
	if clusterName, ok := svc.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
		if suffix, hasSuffix := svc.Labels["cluster-name-suffix"]; hasSuffix && suffix != "" {
			if currentName, hasName := item.Parameters["name"]; hasName && currentName != "" {
				setItemParameter(&item, "name", currentName+suffix)
			}
		}
	}

	if isItemHidden(&item) {
		return
	}

	updateOrAddServiceItems(homerConfig, serviceGroup, []Item{item})
}

func hasExplicitServiceURL(annotations map[string]string) bool {
	url, ok := annotations["item.homer.rajsingh.info/url"]
	return ok && strings.TrimSpace(url) != ""
}

// setupK8sServiceGroup creates the Homer service group for a K8s Service
func setupK8sServiceGroup(
	homerConfig *HomerConfig,
	svc corev1.Service,
	groupingConfig *ServiceGroupingConfig,
) Service {
	service := Service{}
	serviceName := determineServiceGroupWithCRDRespect(
		homerConfig,
		svc.Namespace,
		svc.Labels,
		svc.Annotations,
		groupingConfig,
	)
	setServiceParameter(&service, "name", serviceName)
	setServiceParameter(&service, "logo", NamespaceIconURL)
	return service
}

// createK8sServiceItem builds a Homer dashboard item from a K8s Service
func createK8sServiceItem(svc corev1.Service) Item {
	item := Item{}

	name := svc.Name
	namespace := svc.Namespace

	setItemParameter(&item, "name", name)
	setItemParameter(&item, "logo", ServiceIconURL)
	setItemParameter(&item, "subtitle", namespace+"/"+name)

	// Build cluster-internal URL
	protocol := ProtocolHTTP
	portSuffix := ""
	if len(svc.Spec.Ports) > 0 {
		port := svc.Spec.Ports[0].Port
		if port == 443 {
			protocol = ProtocolHTTPS
		}
		portSuffix = fmt.Sprintf(":%d", port)
	}
	setItemParameter(&item, "url", fmt.Sprintf("%s://%s.%s.svc.cluster.local%s", protocol, name, namespace, portSuffix))

	// Set source metadata for conflict detection (prefix with "svc/" to avoid collisions with Ingress items)
	item.Source = discoveredResourceSource("svc/"+name, svc.Annotations)
	item.Namespace = namespace
	item.LastUpdate = svc.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

	// Auto-tag with cluster name if cluster-tagstyle label is set
	if clusterName, ok := svc.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
		if tagStyle, hasStyle := svc.Labels["cluster-tagstyle"]; hasStyle && tagStyle != "" {
			setItemParameter(&item, "tag", clusterName)
			setItemParameter(&item, "tagstyle", tagStyle)
		}
	}

	return item
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
func createIngressItems(ingress networkingv1.Ingress, domainFilters []string, validationLevel ValidationLevel) []Item {
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
		processItemAnnotationsWithValidation(&item, ingress.ObjectMeta.Annotations, validationLevel)

		// Append cluster name suffix from label AFTER processing annotations
		// so that it takes precedence over any name annotations
		if clusterName, ok := ingress.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
			if suffix, hasSuffix := ingress.ObjectMeta.Labels["cluster-name-suffix"]; hasSuffix && suffix != "" {
				if currentName, hasName := item.Parameters["name"]; hasName && currentName != "" {
					setItemParameter(&item, "name", currentName+suffix)
				}
			}
		}

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
	// For remote clusters, include cluster name in Source to make it unique
	item.Source = discoveredResourceSource("ingress/"+ingress.ObjectMeta.Name, ingress.ObjectMeta.Annotations)
	item.Namespace = ingress.ObjectMeta.Namespace
	item.LastUpdate = ingress.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

	// Auto-tag with cluster name if cluster-tagstyle label is set
	if clusterName, ok := ingress.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
		// Only add tag if cluster-tagstyle is explicitly set
		if tagStyle, hasStyle := ingress.ObjectMeta.Labels["cluster-tagstyle"]; hasStyle && tagStyle != "" {
			setItemParameter(&item, "tag", clusterName)
			setItemParameter(&item, "tagstyle", tagStyle)
		}
	}

	return item
}

func UpdateConfigMapIngress(cm *corev1.ConfigMap, ingress networkingv1.Ingress, domainFilters []string) error {
	configMapMutex.Lock()
	defer configMapMutex.Unlock()

	homerConfig := HomerConfig{}
	if err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig); err != nil {
		return fmt.Errorf("unmarshal config.yml: %w", err)
	}
	UpdateHomerConfigIngress(&homerConfig, ingress, domainFilters)
	objYAML, err := marshalHomerConfigToYAML(&homerConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cm.Data["config.yml"] = string(objYAML)
	return nil
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
	discoveryConfig *DiscoveryConfig,
) {
	var groupingConfig *ServiceGroupingConfig
	if discoveryConfig != nil {
		groupingConfig = discoveryConfig.ServiceGrouping
	}
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
	removeItemsFromSource(
		homerConfig,
		discoveredResourceSource("httproute/"+httproute.ObjectMeta.Name, httproute.ObjectMeta.Annotations),
		httproute.ObjectMeta.Namespace,
	)

	// Determine protocol based on parent Gateway listener configuration
	protocol := determineProtocolFromHTTPRoute(httproute)

	// Handle multiple hostnames by creating separate items (similar to Ingress approach)
	var items []Item
	if len(httproute.Spec.Hostnames) == 0 {
		// No hostnames specified - don't create any items
		// This allows for cleanup when all hostnames are removed
		return
	} else {
		// Only filters explicitly supplied by the ClusterManager are honored.
		// The similarly named resource annotation is user-controlled and must not
		// be able to filter a remote cluster whose configuration omitted filters.
		effectiveDomainFilters := domainFilters
		if discoveryConfig != nil {
			if authorizedFilters, ok := discoveryConfig.HTTPRouteDomainFilters[HTTPRouteDomainFilterKey(httproute)]; ok {
				effectiveDomainFilters = authorizedFilters
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

			setItemParameter(&item, "name", name)

			// Set metadata for conflict detection
			// For remote clusters, include cluster name in Source to make it unique
			item.Source = discoveredResourceSource("httproute/"+httproute.ObjectMeta.Name, httproute.ObjectMeta.Annotations)
			item.Namespace = httproute.ObjectMeta.Namespace
			item.LastUpdate = httproute.ObjectMeta.CreationTimestamp.Time.Format("2006-01-02T15:04:05Z")

			processItemAnnotationsWithValidation(&item, httproute.ObjectMeta.Annotations, discoveryValidationLevel(discoveryConfig))

			// Append cluster name suffix from label AFTER processing annotations
			// so that it takes precedence over any name annotations
			if clusterName, ok := httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
				if suffix, hasSuffix := httproute.ObjectMeta.Labels["cluster-name-suffix"]; hasSuffix && suffix != "" {
					if currentName, hasName := item.Parameters["name"]; hasName && currentName != "" {
						setItemParameter(&item, "name", currentName+suffix)
					}
				}
			}

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
	if clusterName, ok := httproute.ObjectMeta.Annotations["homer.rajsingh.info/cluster"]; ok && clusterName != "" && clusterName != LocalCluster {
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
			crdClaims := make(map[int]string)
			for _, newItem := range items {
				updated := false
				// Get new item name from Parameters map
				newItemName := getItemName(&newItem)

				// Check if item already exists
				for ix, existingItem := range s.Items {
					if itemsRepresentSameResource(existingItem, newItem, newItemName, crdClaims[ix]) {
						if existingItem.Source == CRDSource && newItem.Source != CRDSource && newItem.Source != "" {
							crdClaims[ix] = discoveredItemIdentity(newItem)
						}
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

// itemsRepresentSameResource determines whether a discovered item should
// update an existing item or be added as a separate item. Display names are
// intentionally user-facing and are not guaranteed to be unique across
// clusters, namespaces, or resource types. For discovered resources, Source
// plus Namespace is the stable identity; CRD items are matched by name so a
// discovered resource can still enhance a user-authored foundation item.
func itemsRepresentSameResource(existing, incoming Item, incomingName, crdClaim string) bool {
	if getItemName(&existing) != incomingName {
		return false
	}

	if existing.crdFoundation {
		return existing.Source == incoming.Source && existing.Namespace == incoming.Namespace
	}

	if existing.Source == CRDSource && incoming.Source != CRDSource && incoming.Source != "" {
		return crdClaim == "" || crdClaim == discoveredItemIdentity(incoming)
	}

	if existing.Source != "" && incoming.Source != "" &&
		existing.Source != CRDSource && incoming.Source != CRDSource {
		return existing.Source == incoming.Source && existing.Namespace == incoming.Namespace
	}

	return true
}

func discoveredItemIdentity(item Item) string {
	return item.Source + "\x00" + item.Namespace
}

// smartMergeItems intelligently merges items prioritizing CRD foundation with discovered enhancements
func smartMergeItems(existingItem, newItem *Item) {
	if existingItem.Parameters == nil {
		existingItem.Parameters = make(map[string]string)
	}
	if existingItem.NestedObjects == nil {
		existingItem.NestedObjects = make(map[string]map[string]string)
	}

	isCRDExisting := existingItem.Source == CRDSource || existingItem.crdFoundation
	isDiscoveredNew := newItem.Source != CRDSource && newItem.Source != ""
	mergeLegacyItemParameters(existingItem, newItem, isCRDExisting, isDiscoveredNew)
	mergeDirectItemFields(existingItem, newItem, isCRDExisting, isDiscoveredNew)
	mergeNestedItemObjects(existingItem, newItem, isCRDExisting)
	mergeItemHeaders(existingItem, newItem, isCRDExisting)
	mergeQuickLinks(existingItem, newItem)
	mergeItemTypedFields(existingItem, newItem, isCRDExisting)
	mergeItemRawFields(existingItem, newItem)

	if isDiscoveredNew {
		existingItem.LastUpdate = newItem.LastUpdate
		existingItem.Source = newItem.Source
		existingItem.Namespace = newItem.Namespace
		existingItem.crdFoundation = isCRDExisting
	}
}

func mergeLegacyItemParameters(existingItem, newItem *Item, isCRDExisting, isDiscoveredNew bool) {
	if newItem.Parameters != nil {
		for key, value := range newItem.Parameters {
			existingValue := itemFieldValue(existingItem, key)
			switch key {
			case NameField:
				if !isCRDExisting {
					setItemParameter(existingItem, key, value)
				}
			case URLField, SubtitleField:
				if isDiscoveredNew || existingValue == "" || !isCRDExisting {
					setItemParameter(existingItem, key, value)
				}
			default:
				if isCRDExisting && existingValue != "" {
					continue
				}
				setItemParameter(existingItem, key, value)
			}
		}
	}
}

func mergeDirectItemFields(existingItem, newItem *Item, isCRDExisting, isDiscoveredNew bool) {
	for key, value := range directItemStringFields(*newItem) {
		if value == "" {
			continue
		}
		existingValue := itemFieldValue(existingItem, key)
		if key == NameField && isCRDExisting {
			continue
		}
		if (key == URLField || key == SubtitleField) && isDiscoveredNew {
			setItemParameter(existingItem, key, value)
			continue
		}
		if existingValue == "" || !isCRDExisting {
			setItemParameter(existingItem, key, value)
		}
	}
}

func mergeNestedItemObjects(existingItem, newItem *Item, isCRDExisting bool) {
	if newItem.NestedObjects != nil {
		for objectName, objectMap := range newItem.NestedObjects {
			if existingItem.NestedObjects[objectName] == nil {
				existingItem.NestedObjects[objectName] = make(map[string]string)
			}
			// Additive approach - both CRD and discovered can contribute. A CRD
			// value remains authoritative when both sources define the key.
			for key, value := range objectMap {
				if !isCRDExisting || existingItem.NestedObjects[objectName][key] == "" {
					existingItem.NestedObjects[objectName][key] = value
				}
			}
		}
	}
}

func mergeItemHeaders(existingItem, newItem *Item, isCRDExisting bool) {
	if newItem.Headers != nil {
		if existingItem.Headers == nil {
			existingItem.Headers = make(map[string]any)
		}
		for key, value := range newItem.Headers {
			if _, exists := existingItem.Headers[key]; !exists || !isCRDExisting {
				existingItem.Headers[key] = deepCopyValue(value)
			}
		}
	}
}

func mergeQuickLinks(existingItem, newItem *Item) {
	if newItem.Quick != nil {
		for _, quickLink := range newItem.Quick {
			if !quickLinkExists(existingItem.Quick, quickLink) {
				existingItem.Quick = append(existingItem.Quick, *quickLink.DeepCopy())
			}
		}
	}
}

func quickLinkExists(quickLinks []QuickLink, candidate QuickLink) bool {
	for _, quickLink := range quickLinks {
		if reflect.DeepEqual(quickLink, candidate) {
			return true
		}
	}
	return false
}

func mergeItemTypedFields(existingItem, newItem *Item, isCRDExisting bool) {
	if newItem.SuccessCodes != nil && (len(existingItem.SuccessCodes) == 0 || !isCRDExisting) {
		existingItem.SuccessCodes = append([]int(nil), newItem.SuccessCodes...)
	}
	if newItem.UpdateIntervalMs != nil && (existingItem.UpdateIntervalMs == nil || !isCRDExisting) {
		existingItem.UpdateIntervalMs = deepCopyValue(newItem.UpdateIntervalMs)
		existingItem.updateIntervalSet = newItem.updateIntervalSet
	}
	if newItem.UseCredentials != nil && (existingItem.UseCredentials == nil || !isCRDExisting) {
		existingItem.UseCredentials = new(bool)
		*existingItem.UseCredentials = *newItem.UseCredentials
	}
}

func mergeItemRawFields(existingItem, newItem *Item) {
	for key, value := range newItem.RawFields {
		if _, exists := existingItem.RawFields[key]; !exists {
			if existingItem.RawFields == nil {
				existingItem.RawFields = make(map[string]json.RawMessage)
			}
			existingItem.RawFields[key] = append(json.RawMessage(nil), value...)
		}
	}
}

func itemFieldValue(item *Item, key string) string {
	switch strings.ToLower(key) {
	case NameField:
		return item.Name
	case LogoField:
		return item.Logo
	case IconField:
		return item.Icon
	case SubtitleField:
		return item.Subtitle
	case TagField:
		return item.Tag
	case KeywordsField:
		return item.Keywords
	case URLField:
		return item.URL
	case TargetField:
		return item.Target
	case TagStyleField:
		return item.TagStyle
	case TypeField:
		return item.Type
	case BackgroundField:
		return item.Background
	case ClassField:
		return item.Class
	case EndpointField:
		return item.Endpoint
	default:
		return getParameter(item.Parameters, key)
	}
}

func directItemStringFields(item Item) map[string]string {
	return map[string]string{
		NameField:       item.Name,
		LogoField:       item.Logo,
		IconField:       item.Icon,
		SubtitleField:   item.Subtitle,
		TagField:        item.Tag,
		KeywordsField:   item.Keywords,
		URLField:        item.URL,
		TargetField:     item.Target,
		TagStyleField:   item.TagStyle,
		TypeField:       item.Type,
		BackgroundField: item.Background,
		ClassField:      item.Class,
		EndpointField:   item.Endpoint,
	}
}

// determineProtocolFromHTTPRoute uses the protocol resolved from the selected
// Gateway listener. HTTPRoute objects do not carry listener protocol data, so
// callers that can read Gateways should populate HTTPRouteProtocolAnnotation;
// an unresolved route is conservatively treated as HTTP. Hostname suffixes
// and listener names are not protocol signals.
func determineProtocolFromHTTPRoute(httproute *gatewayv1.HTTPRoute) string {
	if httproute != nil && httproute.Annotations != nil {
		switch strings.ToLower(strings.TrimSpace(httproute.Annotations[HTTPRouteProtocolAnnotation])) {
		case ProtocolHTTPS:
			return ProtocolHTTPS
		case ProtocolHTTP:
			return ProtocolHTTP
		}
	}
	return ProtocolHTTP
}

// processItemAnnotations safely processes item annotations without reflection
func processItemAnnotations(item *Item, annotations map[string]string) {
	processItemAnnotationsWithValidation(item, annotations, ValidationLevelNone)
}

func discoveryValidationLevel(config *DiscoveryConfig) ValidationLevel {
	if config == nil || config.ValidationLevel == "" {
		// Dashboard.spec.validationLevel defaults to "warn" in the CRD. Keep
		// the same behavior for objects constructed in-process or read before
		// API-server defaulting has been applied. The legacy public helpers pass
		// a nil DiscoveryConfig and continue to use ValidationLevelNone.
		if config == nil {
			return ValidationLevelNone
		}
		return ValidationLevelWarn
	}
	return config.ValidationLevel
}

// processItemAnnotationsWithValidation processes item annotations with validation
func processItemAnnotationsWithValidation(item *Item, annotations map[string]string, validationLevel ValidationLevel) {
	item.legacyParameters = true
	for key, value := range annotations {
		if fieldName, ok := strings.CutPrefix(key, "item.homer.rajsingh.info/"); ok {
			processItemField(item, strings.ToLower(fieldName), value, validationLevel)
		}
	}
}

// processItemField processes a single item field using smart convention-based detection
func processItemField(item *Item, fieldName, value string, validationLevel ValidationLevel) {
	// Handle array-of-objects annotations (e.g., quick.0.name, quick.1.url)
	if parts := strings.SplitN(fieldName, ".", 3); len(parts) == 3 {
		if idx, err := strconv.Atoi(parts[1]); err == nil {
			processArrayObjectField(item, parts[0], idx, parts[2], value)
			return
		}
	}

	// Handle nested object annotations (e.g., customHeaders/Authorization)
	if strings.Contains(fieldName, "/") {
		processNestedObjectField(item, fieldName, value)
		return
	}

	// Handle all parameters dynamically using smart type inference
	processDynamicParameter(item, fieldName, value, validationLevel)
}

// processArrayObjectField handles array-of-objects annotations like quick.0.name
func processArrayObjectField(item *Item, arrayName string, index int, property, value string) {
	item.legacyParameters = true
	if item.ArrayObjects == nil {
		item.ArrayObjects = make(map[string][]map[string]string)
	}

	arr := item.ArrayObjects[arrayName]

	// Grow the slice to accommodate the index
	for len(arr) <= index {
		arr = append(arr, make(map[string]string))
	}

	arr[index][property] = value
	item.ArrayObjects[arrayName] = arr
}

// processNestedObjectField handles nested object annotations like customHeaders/Authorization
func processNestedObjectField(item *Item, fieldName, value string) {
	item.legacyParameters = true
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
	item.legacyParameters = true
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
			value = strings.Join(cleanKeywords, ",")
		} else {
			value = strings.TrimSpace(value)
		}
		setKnownItemParameterOrStore(item, fieldName, value)
	case URLField, TargetField, WarningValueField, DangerValueField:
		// Handle validation for these fields
		if err := validateAnnotationValue(fieldName, value, validationLevel); err != nil &&
			validationLevel == ValidationLevelStrict {
			// Don't store invalid values in strict mode
			return
		}
		setKnownItemParameterOrStore(item, fieldName, value)
	default:
		// Store known upstream fields in both representations so annotation
		// overrides remain visible to direct-field consumers. Other fields stay
		// in the open-ended legacy parameter map.
		setKnownItemParameterOrStore(item, fieldName, value)
	}
}

func setKnownItemParameterOrStore(item *Item, fieldName, value string) {
	switch fieldName {
	case NameField, LogoField, IconField, SubtitleField, TagField, KeywordsField, URLField, TargetField, TagStyleField, TypeField, BackgroundField, ClassField, EndpointField:
		setItemParameter(item, fieldName, value)
	default:
		item.Parameters[fieldName] = value
	}
}

// knownArrayParams lists Homer parameters that expect array values.
// When these parameters contain comma-separated values, they are automatically
// split into proper YAML arrays. Other parameters use JSON array syntax [...]
// for explicit array values.
var knownArrayParams = map[string]bool{
	"successCodes": true,
	"successcodes": true,
	"hide":         true,
	"groups":       true,
	"environments": true,
	"stats":        true,
	"items":        true,
	"stylesheet":   true,
}

// smartInferType uses convention-based detection to infer parameter types
func smartInferType(value string) any {
	value = strings.TrimSpace(value)

	// JSON array detection: values wrapped in [...] are parsed as arrays
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		var arr []any
		if err := json.Unmarshal([]byte(value), &arr); err == nil {
			return arr
		}
	}

	// Integer detection FIRST - prevents "0" and "1" from being converted to booleans
	// This is important for fields like updateInterval, timeout, apiVersion, etc.
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}

	// Float detection - for values like danger_value: 95.5
	// Only if it contains a decimal point to avoid converting integers
	if strings.Contains(value, ".") {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}

	// Boolean detection - only for explicit boolean strings
	lower := strings.ToLower(value)
	if lower == "true" || lower == "yes" || lower == "on" {
		return true
	}
	if lower == "false" || lower == "no" || lower == "off" {
		return false
	}

	return value
}

// smartInferTypeForParam infers type with awareness of the parameter name.
// Known array parameters with comma-separated values are split into arrays.
func smartInferTypeForParam(key, value string) any {
	value = strings.TrimSpace(value)

	// JSON array syntax always takes priority regardless of key
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		var arr []any
		if err := json.Unmarshal([]byte(value), &arr); err == nil {
			return arr
		}
	}

	// For known array parameters, split comma-separated values into arrays
	if knownArrayParams[key] && strings.Contains(value, ",") {
		parts := strings.Split(value, ",")
		arr := make([]any, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				arr = append(arr, smartInferType(part))
			}
		}
		if len(arr) > 0 {
			return arr
		}
	}

	return smartInferType(value)
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
		// Use smart type inference to handle boolean and integer values
		hideInterface := smartInferType(hideValue)
		switch v := hideInterface.(type) {
		case bool:
			return v
		case int:
			// Treat 0 as false, non-zero as true (JavaScript truthiness)
			return v != 0
		default:
			// For strings, treat non-empty as true
			return hideValue != ""
		}
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

// hasCRDItems checks if a service has any items from CRD source. A discovered
// item that enhanced a CRD foundation keeps crdFoundation set after its source
// changes, so the service remains eligible for CRD-aware grouping.
func hasCRDItems(service Service) bool {
	for _, item := range service.Items {
		if item.Source == CRDSource || item.crdFoundation {
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
			if strings.ToLower(fieldName) == NameField && value != "" {
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
	service.legacyParameters = true
	// Handle nested object annotations (e.g., customConfig/theme)
	if strings.Contains(fieldName, "/") {
		processServiceNestedObjectField(service, fieldName, value)
		return
	}

	// Don't override existing values with empty values for critical fields
	if strings.ToLower(fieldName) == "name" && value == "" {
		return
	}

	// Store known upstream fields directly as well as in the legacy map. This
	// keeps annotation overrides visible to direct-field consumers.
	fieldName = strings.ToLower(fieldName)
	switch fieldName {
	case "name", "icon", "logo", "class":
		setServiceParameter(service, fieldName, value)
		return
	}

	// Store all other parameters dynamically using lowercase field names
	if service.Parameters == nil {
		service.Parameters = make(map[string]string)
	}
	service.Parameters[fieldName] = value
}

// processServiceNestedObjectField handles nested object annotations for services
func processServiceNestedObjectField(service *Service, fieldName, value string) {
	service.legacyParameters = true
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
func UpdateConfigMapHTTPRoute(cm *corev1.ConfigMap, httproute *gatewayv1.HTTPRoute, domainFilters []string) error {
	configMapMutex.Lock()
	defer configMapMutex.Unlock()

	homerConfig := HomerConfig{}
	if err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &homerConfig); err != nil {
		return fmt.Errorf("unmarshal config.yml: %w", err)
	}
	UpdateHomerConfigHTTPRoute(&homerConfig, httproute, domainFilters)
	objYAML, err := marshalHomerConfigToYAML(&homerConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cm.Data["config.yml"] = string(objYAML)
	return nil
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

// ValidateHomerConfig validates the Homer configuration for common issues.
// It retains the strict behavior used by callers that do not have a discovery
// validation policy.
func ValidateHomerConfig(config *HomerConfig) error {
	return validateHomerConfig(config, ValidationLevelStrict)
}

func validateHomerConfig(config *HomerConfig, validationLevel ValidationLevel) error {
	if config == nil {
		return fmt.Errorf("config: nil")
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

	if config.Defaults.ColorTheme != "" && !slices.Contains([]string{"auto", "light", "dark"}, config.Defaults.ColorTheme) {
		return fmt.Errorf("colorTheme: %s", config.Defaults.ColorTheme)
	}

	for i, service := range config.Services {
		serviceName := getServiceName(&service)

		for j, item := range service.Items {
			itemName := getItemName(&item)

			itemURL := getItemURL(&item)
			// Upstream Homer treats item URLs as browser href strings. Keep
			// strict URL checks for the annotation/parameter compatibility
			// path, where the operator historically promised validation, but
			// do not reject direct upstream-style values such as mailto:, tel:,
			// protocol-relative URLs, or custom schemes.
			legacyItem := service.legacyParameters || item.legacyParameters ||
				len(service.Parameters) > 0 || len(item.Parameters) > 0 ||
				len(item.NestedObjects) > 0 || len(item.ArrayObjects) > 0
			if legacyItem && itemURL != "" && !isValidURL(itemURL) && validationLevel != ValidationLevelWarn {
				return fmt.Errorf("service[%d].item[%d] (%s): invalid URL %s", i, j, serviceName+"/"+itemName, itemURL)
			}
		}
	}

	return nil
}

func isValidColor(color string) bool {
	color = strings.TrimSpace(color)
	if color == "" || strings.ContainsAny(color, "\r\n;{}") {
		return false
	}

	if strings.HasPrefix(color, "#") {
		hex := color[1:]
		if len(hex) != 3 && len(hex) != 4 && len(hex) != 6 && len(hex) != 8 {
			return false
		}
		for _, c := range hex {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
		return true
	}

	lowerColor := strings.ToLower(color)
	if lowerColor == "transparent" || lowerColor == "inherit" || lowerColor == "initial" ||
		lowerColor == "unset" || lowerColor == "revert" || lowerColor == "currentcolor" || lowerColor == "none" {
		return true
	}

	// Homer injects these values directly into CSS custom properties. Accept
	// CSS identifiers (including named colors and custom-property references)
	// and function forms such as rgb(), hsl(), var(), and linear-gradient().
	if isCSSIdentifier(lowerColor) {
		return true
	}
	open := strings.IndexByte(lowerColor, '(')
	return open > 0 && strings.HasSuffix(lowerColor, ")") && isCSSIdentifier(lowerColor[:open])
}

func isCSSIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9' && index > 0) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func isValidURL(url string) bool {
	if url == "" {
		return true
	}
	if strings.HasPrefix(url, "#") || strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") {
		return true
	}
	parsed, err := neturl.Parse(url)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "ftp"
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
	// Homer defaults to showing the header. Preserve an explicitly supplied
	// false value, which is a supported upstream setting.
	if !config.headerSet && !config.Header {
		config.Header = true
	}

	// Homer preserves declaration order. The operator's optional order
	// parameter remains available for users who explicitly request sorting.
	if configNeedsExplicitOrdering(config) {
		sortServicesAndItems(config)
	}
}

func configNeedsExplicitOrdering(config *HomerConfig) bool {
	for _, service := range config.Services {
		// The parameter-map representation is the operator's legacy/discovery
		// representation. Keep its historical deterministic ordering. Direct
		// upstream-style objects are emitted in declaration order unless an
		// order parameter is explicitly supplied.
		if service.legacyParameters || len(service.Parameters) > 0 || hasOrderParameter(service.Parameters) {
			return true
		}
		for _, item := range service.Items {
			if item.legacyParameters || (!item.parametersInjected && len(item.Parameters) > 0) || hasOrderParameter(item.Parameters) {
				return true
			}
		}
	}
	return false
}

func hasOrderParameter(params map[string]string) bool {
	if params == nil {
		return false
	}
	_, ok := params["order"]
	return ok
}

// getOrderAnnotation returns the integer value of the "order" parameter.
// Higher values sort first; unset or invalid defaults to 0.
func getOrderAnnotation(params map[string]string) int {
	if params == nil {
		return 0
	}
	if v, ok := params["order"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

// sortServicesAndItems sorts services and their items.
// Primary key: "order" annotation (higher value = appears first, default 0).
// Tie-break: case-insensitive alphabetical by name.
func sortServicesAndItems(config *HomerConfig) {
	sort.SliceStable(config.Services, func(i, j int) bool {
		orderI := getOrderAnnotation(config.Services[i].Parameters)
		orderJ := getOrderAnnotation(config.Services[j].Parameters)
		if orderI != orderJ {
			return orderI > orderJ
		}
		nameI := getServiceName(&config.Services[i])
		nameJ := getServiceName(&config.Services[j])
		return strings.ToLower(nameI) < strings.ToLower(nameJ)
	})

	for i := range config.Services {
		sort.SliceStable(config.Services[i].Items, func(x, y int) bool {
			orderX := getOrderAnnotation(config.Services[i].Items[x].Parameters)
			orderY := getOrderAnnotation(config.Services[i].Items[y].Parameters)
			if orderX != orderY {
				return orderX > orderY
			}
			nameX := getItemName(&config.Services[i].Items[x])
			nameY := getItemName(&config.Services[i].Items[y])
			return strings.ToLower(nameX) < strings.ToLower(nameY)
		})
	}
}

// marshalHomerConfigToYAML creates properly formatted YAML for Homer
func marshalHomerConfigToYAML(config *HomerConfig) ([]byte, error) {
	configMap := make(map[string]any)

	addBasicFields(configMap, config)
	addHotkeyConfig(configMap, config)
	addColorsConfig(configMap, config)
	addDefaultsConfig(configMap, config)
	addProxyConfig(configMap, config)
	addMessageConfig(configMap, config)
	addLinksAndServices(configMap, config)
	for key, value := range config.presentFields {
		if _, exists := configMap[key]; !exists {
			configMap[key] = decodeRawField(value)
		}
	}

	return yaml.Marshal(configMap)
}

// addBasicFields adds basic configuration fields
func addBasicFields(configMap map[string]any, config *HomerConfig) {
	for key, value := range config.RawFields {
		if _, known := homerConfigJSONFields[key]; known {
			continue
		}
		configMap[key] = decodeRawField(value)
	}
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
	if config.Header || config.headerSet {
		if raw, ok := config.presentFields["header"]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
			configMap["header"] = nil
		} else {
			configMap["header"] = config.Header
		}
	}
	if config.Footer != "" || config.footerSet {
		if config.Footer == FooterHidden {
			configMap["footer"] = false
		} else if config.footerValueSet {
			configMap["footer"] = config.footerValue
		} else {
			configMap["footer"] = config.Footer
		}
	}
	if config.Columns != nil {
		configMap["columns"] = config.Columns
	}
	if config.ConnectivityCheck != nil {
		configMap["connectivityCheck"] = *config.ConnectivityCheck
	}
	if config.Theme != "" {
		configMap["theme"] = config.Theme
	}
	if !isEmptyStylesheet(config.Stylesheet) {
		configMap["stylesheet"] = config.Stylesheet
	}
	if config.ExternalConfig != "" {
		configMap["externalConfig"] = config.ExternalConfig
	}
	if config.UpdateIntervalMs != nil || config.updateIntervalSet {
		configMap["updateIntervalMs"] = config.UpdateIntervalMs
	}
}

// AddPageConfigsToConfigMap stores additional Homer page configurations in
// the same ConfigMap as config.yml. Homer loads a page named "foo" from
// assets/foo.yml when the browser URL contains #foo. Page names are limited
// to safe single-path components because they become ConfigMap keys and
// projected asset filenames.
func AddPageConfigsToConfigMap(configMap *corev1.ConfigMap, pages map[string]apiextensionsv1.JSON) error {
	if configMap == nil || len(pages) == 0 {
		return nil
	}
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	for name, page := range pages {
		if !isValidPageName(name) {
			return fmt.Errorf("invalid Homer page name %q: use only letters, numbers, '.', '_' and '-'", name)
		}
		var pageValue any
		if err := json.Unmarshal(page.Raw, &pageValue); err != nil {
			return fmt.Errorf("parse Homer page %q: %w", name, err)
		}
		data, err := yaml.Marshal(pageValue)
		if err != nil {
			return fmt.Errorf("marshal Homer page %q: %w", name, err)
		}
		configMap.Data[name+".yml"] = string(data)
	}
	return nil
}

func isValidPageName(name string) bool {
	if name == "" || name == "config" || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// addHotkeyConfig adds hotkey configuration
func addHotkeyConfig(configMap map[string]any, config *HomerConfig) {
	if config.Hotkey.Search != "" || len(config.Hotkey.RawFields) > 0 {
		values := make(map[string]any)
		for key, value := range config.Hotkey.RawFields {
			values[key] = decodeRawField(value)
		}
		configMap["hotkey"] = map[string]any{
			"search": config.Hotkey.Search,
		}
		for key, value := range values {
			configMap["hotkey"].(map[string]any)[key] = value
		}
	} else if raw, ok := config.presentFields["hotkey"]; ok {
		configMap["hotkey"] = decodeRawField(raw)
	}
}

// addColorsConfig adds colors configuration
func addColorsConfig(configMap map[string]any, config *HomerConfig) {
	if themeColorsConfigured(config.Colors.Light) || themeColorsConfigured(config.Colors.Dark) || len(config.Colors.RawFields) > 0 {
		if raw, ok := config.presentFields["colors"]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
			configMap["colors"] = nil
			return
		}
		colorsMap := make(map[string]any)
		for key, value := range config.Colors.RawFields {
			colorsMap[key] = decodeRawField(value)
		}
		if themeColorsConfigured(config.Colors.Light) {
			lightMap := make(map[string]any)
			addThemeColors(lightMap, config.Colors.Light)
			for key, value := range config.Colors.Light.RawFields {
				lightMap[key] = decodeRawField(value)
			}
			if len(lightMap) > 0 {
				colorsMap["light"] = lightMap
			}
		}
		if themeColorsConfigured(config.Colors.Dark) {
			darkMap := make(map[string]any)
			addThemeColors(darkMap, config.Colors.Dark)
			for key, value := range config.Colors.Dark.RawFields {
				darkMap[key] = decodeRawField(value)
			}
			if len(darkMap) > 0 {
				colorsMap["dark"] = darkMap
			}
		}
		if len(colorsMap) > 0 {
			configMap["colors"] = colorsMap
		}
	} else if raw, ok := config.presentFields["colors"]; ok {
		configMap["colors"] = decodeRawField(raw)
	}
}

func themeColorsConfigured(colors ThemeColors) bool {
	return colors.HighlightPrimary != "" || colors.HighlightSecondary != "" ||
		colors.HighlightHover != "" || colors.Background != "" ||
		colors.CardBackground != "" || colors.Text != "" ||
		colors.TextHeader != "" || colors.TextTitle != "" ||
		colors.TextSubtitle != "" || colors.CardShadow != "" ||
		colors.Link != "" || colors.LinkHover != "" ||
		colors.BackgroundImage != "" || len(colors.RawFields) > 0
}

// addDefaultsConfig adds defaults configuration
func addDefaultsConfig(configMap map[string]any, config *HomerConfig) {
	if config.Defaults.ColorTheme != "" || config.Defaults.Layout != "" || len(config.Defaults.RawFields) > 0 {
		if raw, ok := config.presentFields["defaults"]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
			configMap["defaults"] = nil
			return
		}
		defaultsMap := make(map[string]any)
		for key, value := range config.Defaults.RawFields {
			defaultsMap[key] = decodeRawField(value)
		}
		if config.Defaults.Layout != "" {
			defaultsMap["layout"] = config.Defaults.Layout
		}
		if config.Defaults.ColorTheme != "" {
			defaultsMap["colorTheme"] = config.Defaults.ColorTheme
		}
		configMap["defaults"] = defaultsMap
	} else if raw, ok := config.presentFields["defaults"]; ok {
		configMap["defaults"] = decodeRawField(raw)
	}
}

// addProxyConfig adds proxy configuration
func addProxyConfig(configMap map[string]any, config *HomerConfig) {
	if config.Proxy.UseCredentials || len(config.Proxy.Headers) > 0 || len(config.Proxy.RawFields) > 0 || config.proxySet {
		if raw, ok := config.presentFields["proxy"]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
			configMap["proxy"] = nil
			return
		}
		proxyMap := make(map[string]any)
		for key, value := range config.Proxy.RawFields {
			proxyMap[key] = decodeRawField(value)
		}
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
func addMessageConfig(configMap map[string]any, config *HomerConfig) {
	if config.Message.Title != "" || config.Message.Content != "" || config.Message.Url != "" ||
		config.Message.Icon != "" || config.Message.Style != "" ||
		config.Message.RefreshInterval != nil || len(config.Message.Mapping) > 0 ||
		len(config.Message.RawFields) > 0 || config.messageSet {
		if raw, ok := config.presentFields["message"]; ok && strings.TrimSpace(string(raw)) == JSONNullValue {
			configMap["message"] = nil
			return
		}
		messageMap := make(map[string]any)
		for key, value := range config.Message.RawFields {
			messageMap[key] = decodeRawField(value)
		}
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
		if config.Message.RefreshInterval != nil {
			messageMap["refreshInterval"] = config.Message.RefreshInterval
		}
		if len(config.Message.Mapping) > 0 {
			messageMap["mapping"] = config.Message.Mapping
		}
		configMap["message"] = messageMap
	}
}

// addLinksAndServices adds links and services configuration
func addLinksAndServices(configMap map[string]any, config *HomerConfig) {
	if len(config.Links) > 0 {
		configMap["links"] = config.Links
	} else if raw, ok := config.presentFields["links"]; ok {
		configMap["links"] = decodeRawField(raw)
	}
	if len(config.Services) > 0 {
		configMap["services"] = flattenServicesForYAML(config.Services)
	} else if raw, ok := config.presentFields["services"]; ok {
		configMap["services"] = decodeRawField(raw)
	}
}

// addThemeColors adds theme color fields to a map
func addThemeColors(colorMap map[string]any, colors ThemeColors) {
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

func flattenServicesForYAML(services []Service) []map[string]any {
	if len(services) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(services))

	for _, service := range services {
		serviceMap := make(map[string]any)
		if service.Name != "" {
			serviceMap["name"] = service.Name
		}
		if service.Icon != "" {
			serviceMap["icon"] = service.Icon
		}
		if service.Logo != "" {
			serviceMap["logo"] = service.Logo
		}
		if service.Class != "" {
			serviceMap["class"] = service.Class
		}
		for key, value := range service.RawFields {
			if _, exists := serviceMap[key]; !exists {
				serviceMap[key] = decodeRawField(value)
			}
		}

		// Add parameters with smart type inference and YAML key conversion
		if service.Parameters != nil {
			for key, value := range service.Parameters {
				yamlKey := getYAMLKey(key)
				serviceMap[yamlKey] = smartInferTypeForParam(yamlKey, value)
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

		if len(serviceMap) > 0 || service.objectSet {
			result = append(result, serviceMap)
		}
	}

	return result
}

func flattenItemsForYAML(items []Item) []map[string]any {
	if len(items) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(items))

	for _, item := range items {
		itemMap := make(map[string]any)
		addDirectItemFields(itemMap, item)
		for key, value := range item.RawFields {
			if _, exists := itemMap[key]; !exists {
				itemMap[key] = decodeRawField(value)
			}
		}

		// Add parameters with smart type inference (key-aware for array detection)
		if item.Parameters != nil {
			for key, value := range item.Parameters {
				yamlKey := getYAMLKey(key)
				itemMap[yamlKey] = smartInferTypeForParam(yamlKey, value)
			}
		}

		// Add nested objects
		if item.NestedObjects != nil {
			for objectName, objectMap := range item.NestedObjects {
				itemMap[objectName] = objectMap
			}
		}

		// Add array objects (e.g., quick links)
		if item.ArrayObjects != nil {
			for arrayName, arrayMaps := range item.ArrayObjects {
				inferredArray := make([]any, 0, len(arrayMaps))
				for _, objMap := range arrayMaps {
					inferredObj := make(map[string]any)
					for k, v := range objMap {
						inferredObj[k] = smartInferType(v)
					}
					inferredArray = append(inferredArray, inferredObj)
				}
				itemMap[arrayName] = inferredArray
			}
		}

		if len(itemMap) > 0 || item.objectSet {
			result = append(result, itemMap)
		}
	}

	return result
}

func addDirectItemFields(itemMap map[string]any, item Item) {
	if item.Name != "" {
		itemMap["name"] = item.Name
	}
	if item.Logo != "" {
		itemMap["logo"] = item.Logo
	}
	if item.Icon != "" {
		itemMap["icon"] = item.Icon
	}
	if item.Subtitle != "" {
		itemMap["subtitle"] = item.Subtitle
	}
	if item.Tag != "" {
		itemMap["tag"] = item.Tag
	}
	if item.Keywords != "" {
		itemMap["keywords"] = item.Keywords
	}
	if item.URL != "" {
		itemMap["url"] = item.URL
	}
	if item.Target != "" {
		itemMap["target"] = item.Target
	}
	if item.TagStyle != "" {
		itemMap["tagstyle"] = item.TagStyle
	}
	if item.Type != "" {
		itemMap["type"] = item.Type
	}
	if item.Background != "" {
		itemMap["background"] = item.Background
	}
	if item.Class != "" {
		itemMap["class"] = item.Class
	}
	if item.Endpoint != "" {
		itemMap["endpoint"] = item.Endpoint
	}
	if item.UseCredentials != nil {
		itemMap["useCredentials"] = *item.UseCredentials
	}
	if len(item.Headers) > 0 {
		itemMap["headers"] = item.Headers
	}
	if len(item.SuccessCodes) > 0 {
		itemMap["successCodes"] = item.SuccessCodes
	}
	if item.UpdateIntervalMs != nil || item.updateIntervalSet {
		itemMap["updateIntervalMs"] = item.UpdateIntervalMs
	}
	if len(item.Quick) > 0 {
		itemMap["quick"] = item.Quick
	}
}

// getYAMLKey converts parameter keys to proper YAML field names
// Homer uses camelCase for JavaScript property access, so we need to ensure
// annotation keys (which may be lowercase) are converted to proper camelCase
func getYAMLKey(key string) string {
	switch strings.ToLower(key) {
	// Service/Item parameters from Homer service components
	case "legacyapi":
		return "legacyApi"
	case "librarytype":
		return "libraryType"
	case "usecredentials":
		return "useCredentials"
	case "apiversion":
		return "apiVersion"
	case "checkinterval":
		return "checkInterval"
	case "updateinterval":
		return "updateInterval"
	case "updateintervalms":
		return "updateIntervalMs"
	case "refreshinterval":
		return "refreshInterval"
	case "successcodes":
		return "successCodes"
	case "rateinterval":
		return "rateInterval"
	case "torrentinterval":
		return "torrentInterval"
	case "downloadinterval":
		return "downloadInterval"
	case "hideaverages":
		return "hideaverages" // Homer uses lowercase
	case "locationid":
		return "locationId" // OpenWeather
	case "api_token":
		return "api_token" // Homer uses underscore
	case "warning_value":
		return "warning_value" // Homer uses underscore
	case "danger_value":
		return "danger_value" // Homer uses underscore
	case "hide_decimals":
		return "hide_decimals" // Homer uses underscore
	case "small_font_on_small_screens":
		return "small_font_on_small_screens" // Homer uses underscore
	case "small_font_on_desktop":
		return "small_font_on_desktop" // Homer uses underscore

	// Global config fields
	case "documenttitle":
		return "documentTitle"
	case "colortheme":
		return "colorTheme"
	case "connectivitycheck":
		return "connectivityCheck"
	case "externalconfig":
		return "externalConfig"
	case "tagstyle":
		return "tagstyle" // Homer uses lowercase

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

	usesLegacyParameters := item.legacyParameters || item.parametersInjected || len(item.Parameters) > 0

	// Add health check URL if not already a smart card. Direct upstream fields
	// are the source of truth for direct CRD items; generated/discovered items
	// retain the legacy parameter representation for compatibility.
	if getItemType(item) == "" {
		if usesLegacyParameters {
			setItemParameter(item, "type", GenericType)
		} else {
			item.Type = GenericType
		}
	}

	// Set health endpoint.
	if healthConfig.HealthPath != "" && getItemEndpoint(item) == "" {
		if url := getItemURL(item); url != "" {
			endpoint := url + healthConfig.HealthPath
			if usesLegacyParameters {
				setItemParameter(item, "endpoint", endpoint)
			} else {
				item.Endpoint = endpoint
			}
		}
	}

	// Merge health check headers. Homer accepts direct item headers; the
	// nested representation remains available for annotation-generated items.
	if healthConfig.Headers != nil {
		if usesLegacyParameters {
			if item.NestedObjects == nil {
				item.NestedObjects = make(map[string]map[string]string)
			}
			if item.NestedObjects["headers"] == nil {
				item.NestedObjects["headers"] = make(map[string]string)
			}
			for key, value := range healthConfig.Headers {
				if _, exists := item.NestedObjects["headers"][key]; !exists {
					item.NestedObjects["headers"][key] = value
				}
			}
		} else {
			if item.Headers == nil {
				item.Headers = make(map[string]any)
			}
			for key, value := range healthConfig.Headers {
				if _, exists := item.Headers[key]; !exists {
					item.Headers[key] = value
				}
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
		if getItemURL(&item) != "" {
			count++
		}
	}
	return count
}

// countItemsWithTags counts items that have tags
func countItemsWithTags(items []Item) int {
	count := 0
	for _, item := range items {
		if getItemTag(&item) != "" {
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
		serviceName := getServiceName(&service)
		if serviceName == "" {
			continue
		}

		for _, item := range service.Items {
			// Get item name from Parameters map
			itemName := getItemName(&item)

			// Process keywords dependencies from direct upstream fields with
			// legacy parameter fallback.
			if keywords := getItemKeywords(&item); keywords != "" {
				dependencies = append(dependencies,
					findKeywordDependencies(keywords, services, serviceName, itemName)...)
			}

			if subtitle := getItemSubtitle(&item); subtitle != "" {
				dependencies = append(dependencies,
					findSubtitleDependencies(subtitle, services, serviceName, itemName)...)
			}
		}
	}

	return dependencies
}

// findKeywordDependencies finds dependencies based on keywords
func findKeywordDependencies(keywords string, services []Service, serviceName, itemName string) []ServiceDependency {
	var dependencies []ServiceDependency
	for keyword := range strings.SplitSeq(keywords, ",") {
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
	optimizedServices := make([]Service, len(services))
	copy(optimizedServices, services)

	sort.Slice(optimizedServices, func(i, j int) bool {
		if len(optimizedServices[i].Items) != len(optimizedServices[j].Items) {
			return len(optimizedServices[i].Items) > len(optimizedServices[j].Items)
		}
		return getServiceName(&optimizedServices[i]) < getServiceName(&optimizedServices[j])
	})

	return optimizedServices
}

// discoveredResourceSource returns the stable source key used by discovered
// resources. Remote cluster names are part of the key so resources with the
// same name and namespace in different clusters remain independent.
func discoveredResourceSource(resourceName string, annotations map[string]string) string {
	clusterName := annotations["homer.rajsingh.info/cluster"]
	if clusterName != "" && clusterName != LocalCluster {
		return resourceName + "@" + clusterName
	}
	return resourceName
}

// removeItemsFromSource removes all items that originated from a specific source
func removeItemsFromSource(homerConfig *HomerConfig, sourceName, sourceNamespace string) {
	configMutex.Lock()
	defer configMutex.Unlock()

	for serviceIndex := range homerConfig.Services {
		service := &homerConfig.Services[serviceIndex]
		filteredItems := make([]Item, 0, len(service.Items))
		for _, item := range service.Items {
			if item.Source == sourceName && item.Namespace == sourceNamespace {
				continue
			}
			filteredItems = append(filteredItems, item)
		}
		service.Items = filteredItems
	}

	removeEmptyServices(homerConfig)
}

// removeEmptyServices removes services that have no items
func removeEmptyServices(homerConfig *HomerConfig) {
	// Use in-place filtering to avoid allocations
	filteredCount := 0
	for i, service := range homerConfig.Services {
		if len(service.Items) > 0 || preserveEmptyService(&service) {
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

// preserveEmptyService keeps user-authored upstream service groups while
// allowing discovery-created groups to disappear after their last item is
// removed. Services decoded from YAML/JSON carry objectSet; discovery groups
// are built through the legacy parameter path and do not.
func preserveEmptyService(service *Service) bool {
	return service.objectSet || !service.legacyParameters
}

// enhanceHomerConfigWithAggregation enhances Homer config with advanced aggregation features
func enhanceHomerConfigWithAggregation(config *HomerConfig, healthConfig *ServiceHealthConfig) {
	// Enhance items with health checking
	for i := range config.Services {
		for j := range config.Services[i].Items {
			enhanceItemWithHealthCheck(&config.Services[i].Items[j], healthConfig)
		}
	}

	// Find dependencies and optimize layout
	dependencies := findServiceDependencies(config.Services)
	if len(dependencies) > 0 {
		slog.Debug("found service dependencies", "count", len(dependencies))
	}
	config.Services = optimizeServiceLayout(config.Services, dependencies)
}
