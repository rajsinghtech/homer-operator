package homer

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestUpstreamStyleConfigurationIsPreserved(t *testing.T) {
	configYAML := `
title: Demo
header: false
theme: classic
updateIntervalMs: 30000
links:
  - name: Page two
    url: "#page2"
services:
  - name: First group
    icon: fa-solid fa-server
    class: highlight-purple
    items:
      - name: First item
        url: "#"
        background: circle
        quick:
          - name: Docs
            url: https://example.com/docs
            target: _blank
      - name: Relative item
        url: /relative/path
`

	var config HomerConfig
	if err := yaml.Unmarshal([]byte(configYAML), &config); err != nil {
		t.Fatalf("unmarshal upstream-style config: %v", err)
	}
	if config.Header {
		t.Fatal("header: false was not preserved")
	}

	cm, err := CreateConfigMap(&config, "demo", "default", networkingv1.IngressList{}, nil, nil)
	if err != nil {
		t.Fatalf("create config map: %v", err)
	}
	output := cm.Data["config.yml"]
	for _, expected := range []string{
		"header: false",
		"theme: classic",
		"updateIntervalMs: 30000",
		"url: '#page2'",
		"url: '#'",
		"background: circle",
		"quick:",
		"target: _blank",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("generated config does not contain %q:\n%s", expected, output)
		}
	}
}

func TestDirectConfigurationPreservesDeclarationOrder(t *testing.T) {
	config := &HomerConfig{
		Title: "Order",
		Services: []Service{
			{Name: "Second", Items: []Item{{Name: "B"}}},
			{Name: "First", Items: []Item{{Name: "A"}}},
		},
	}
	cm, err := CreateConfigMap(config, "order", "default", networkingv1.IngressList{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Services []struct {
			Name  string `yaml:"name"`
			Items []struct {
				Name string `yaml:"name"`
			} `yaml:"items"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(cm.Data["config.yml"]), &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Services[0].Name; got != "Second" {
		t.Errorf("first service = %q, want declaration order", got)
	}
	if got := decoded.Services[0].Items[0].Name; got != "B" {
		t.Errorf("first item = %q, want declaration order", got)
	}
}

func TestAdditionalPagesAreMountedAsHomerAssets(t *testing.T) {
	page := apiextensionsv1.JSON{Raw: []byte(`{"subtitle":"Second page","services":[{"items":[{"name":"App","url":"https://example.com"}]}]}`)}
	cm := &corev1.ConfigMap{}
	if err := AddPageConfigsToConfigMap(cm, map[string]apiextensionsv1.JSON{"page2": page}); err != nil {
		t.Fatal(err)
	}
	if got := cm.Data["page2.yml"]; !strings.Contains(got, "subtitle: Second page") {
		t.Fatalf("page YAML was not written: %s", got)
	}
	if err := AddPageConfigsToConfigMap(cm, map[string]apiextensionsv1.JSON{"../bad": page}); err == nil {
		t.Fatal("expected invalid page name to be rejected")
	}
}

func TestRelativeURLsAndUnnamedGroupsValidate(t *testing.T) {
	config := &HomerConfig{
		Title:    "Flexible",
		Services: []Service{{Items: []Item{{URL: "#page2"}, {URL: "/health"}}}},
	}
	if err := ValidateHomerConfig(config); err != nil {
		t.Fatalf("relative URLs and unnamed groups should validate: %v", err)
	}
}

func TestConfigSyncImageAndIconAliases(t *testing.T) {
	deployment := CreateDeployment("demo", "default", nil, nil, &DeploymentConfig{
		ConfigSyncImage:     "registry.example/config-sync:v1",
		AssetsConfigMapName: "assets",
		IconAliases:         map[string][]string{"brand.ico": {"icons/favicon.ico", "icons/apple-touch-icon.png"}},
		PageConfigKeys:      []string{"page2.yml"},
	})
	if got := deployment.Spec.Template.Spec.Containers[0].Image; got != "registry.example/config-sync:v1" {
		t.Errorf("config-sync image = %q", got)
	}
	command := deployment.Spec.Template.Spec.Containers[0].Command[2]
	for _, expected := range []string{"/config/page2.yml", "/custom-assets/brand.ico", "'icons/favicon.ico'", "'icons/apple-touch-icon.png'"} {
		if !strings.Contains(command, expected) {
			t.Errorf("sidecar command does not contain %q: %s", expected, command)
		}
	}
	if !strings.Contains(command, "stage_tree") || !strings.Contains(command, `"$2${entry##*/}/"`) {
		t.Fatalf("sidecar command does not recursively stage nested assets: %s", command)
	}
	if err := exec.Command("sh", "-n", "-c", command).Run(); err != nil {
		t.Fatalf("generated config-sync command is not valid POSIX shell: %v\n%s", err, command)
	}
	var hasStateMount bool
	for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == "operator-state-volume" && mount.MountPath == "/operator-state" {
			hasStateMount = true
		}
	}
	if !hasStateMount {
		t.Fatal("config-sync sidecar should mount persistent operator state")
	}
}

func TestNestedCustomAssetsAreStagedByRefreshCommand(t *testing.T) {
	customAssets := t.TempDir()
	wwwAssets := t.TempDir()
	operatorState := t.TempDir()
	if err := os.MkdirAll(filepath.Join(customAssets, "themes", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customAssets, "themes", "nested", "custom.css"), []byte("body { color: red; }"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := buildAssetRefreshCommand(&DeploymentConfig{AssetsConfigMapName: "assets"})
	command = strings.ReplaceAll(command, "/custom-assets", customAssets)
	command = strings.ReplaceAll(command, "/www/assets", wwwAssets)
	command = strings.ReplaceAll(command, "/operator-state", operatorState)
	if err := exec.Command("sh", "-c", command+" true").Run(); err != nil {
		t.Fatalf("nested asset refresh failed: %v\n%s", err, command)
	}
	staged, err := os.ReadFile(filepath.Join(wwwAssets, "themes", "nested", "custom.css"))
	if err != nil {
		t.Fatalf("nested asset was not staged: %v", err)
	}
	if string(staged) != "body { color: red; }" {
		t.Fatalf("staged nested asset = %q", staged)
	}
}

func TestDirectFieldsParticipateInOperatorFeatures(t *testing.T) {
	frontend := Service{
		Name: "Frontend",
		Items: []Item{{
			Name:     "Portal",
			URL:      "https://portal.example.com",
			Tag:      "public",
			Keywords: "database,public",
			Subtitle: "Uses Database",
		}},
	}
	database := Service{
		Name: "Database",
		Items: []Item{{
			Name:     "Postgres",
			Type:     "Generic",
			Endpoint: "https://database.example.com/health",
		}},
	}

	if got := countItemsWithUrls(frontend.Items); got != 1 {
		t.Fatalf("direct URL count = %d, want 1", got)
	}
	if got := countItemsWithTags(frontend.Items); got != 1 {
		t.Fatalf("direct tag count = %d, want 1", got)
	}
	if got := countSmartCards(database.Items); got != 1 {
		t.Fatalf("direct smart-card count = %d, want 1", got)
	}
	metrics := aggregateServiceMetrics(&database)
	if metrics.HealthyItems != 1 {
		t.Fatalf("direct healthy-item count = %d, want 1", metrics.HealthyItems)
	}
	dependencies := findServiceDependencies([]Service{frontend, database})
	if len(dependencies) == 0 || dependencies[0].ServiceName != "Database" {
		t.Fatalf("direct dependency detection = %#v, want Database dependency", dependencies)
	}

	health := &ServiceHealthConfig{Enabled: true, HealthPath: "/health", Headers: map[string]string{"X-Health": "true"}}
	item := Item{URL: "https://direct.example.com"}
	enhanceItemWithHealthCheck(&item, health)
	if item.Type != GenericType || item.Endpoint != "https://direct.example.com/health" {
		t.Fatalf("direct health enhancement = %#v", item)
	}
	if item.Headers["X-Health"] != "true" {
		t.Fatalf("direct health headers = %#v", item.Headers)
	}
}

func TestSmartMergePreservesDirectCRDFieldsAndRawFields(t *testing.T) {
	existing := &Item{
		Source:    CRDSource,
		Name:      "Portal",
		URL:       "https://configured.example.com",
		Type:      "Generic",
		RawFields: map[string]json.RawMessage{"future": []byte(`{"enabled":true}`)},
		Headers:   map[string]any{"Authorization": "configured"},
	}
	discovered := &Item{
		Source:  "route/portal",
		Name:    "Portal",
		URL:     "https://discovered.example.com",
		Headers: map[string]any{"X-Discovered": "yes"},
		RawFields: map[string]json.RawMessage{
			"discoveredOnly": []byte(`true`),
		},
	}

	smartMergeItems(existing, discovered)
	if existing.URL != discovered.URL {
		t.Fatalf("merged direct URL = %q, want discovered URL %q", existing.URL, discovered.URL)
	}
	if existing.Type != "Generic" {
		t.Fatalf("merged direct type = %q, want CRD value", existing.Type)
	}
	if string(existing.RawFields["future"]) != `{"enabled":true}` || string(existing.RawFields["discoveredOnly"]) != "true" {
		t.Fatalf("merged raw fields = %#v", existing.RawFields)
	}
	if existing.Headers["Authorization"] != "configured" || existing.Headers["X-Discovered"] != "yes" {
		t.Fatalf("merged headers = %#v", existing.Headers)
	}
}

func TestPageJSONRoundTrip(t *testing.T) {
	page := apiextensionsv1.JSON{Raw: []byte(`{"title":"Page"}`)}
	dashboard := map[string]any{"pages": map[string]apiextensionsv1.JSON{"page": page}}
	data, err := json.Marshal(dashboard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"pages"`) {
		t.Fatal("pages were not JSON encoded")
	}
}

func TestUpstreamOpenFieldsAndFalsyValuesSurviveRoundTrip(t *testing.T) {
	input := []byte(`{
  "header": false,
  "footer": false,
  "columns": 3,
  "updateIntervalMs": false,
  "futureRoot": {"enabled": true},
  "services": [{
    "name": "Group",
    "futureService": [1, true],
    "items": [{
      "name": "Card",
      "type": "Generic",
      "updateIntervalMs": 0,
      "customFlag": true,
      "customValues": [1, "two", {"three": 3}]
    }]
  }]
}`)

	var config HomerConfig
	if err := json.Unmarshal(input, &config); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	for key := range map[string]bool{"futureRoot": true, "futureService": true, "customFlag": true, "customValues": true} {
		if _, ok := got[key]; ok {
			continue
		}
		if key == "futureService" {
			service := got["services"].([]any)[0].(map[string]any)
			if _, ok := service[key]; ok {
				continue
			}
		}
		if key == "customFlag" || key == "customValues" {
			item := got["services"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
			if _, ok := item[key]; ok {
				continue
			}
		}
		t.Errorf("round trip dropped %q", key)
	}
	if got["header"] != false || got["footer"] != false || got["updateIntervalMs"] != false {
		t.Fatalf("falsy root values were not preserved: %#v", got)
	}
	item := got["services"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["updateIntervalMs"] != float64(0) {
		t.Fatalf("falsy item update interval was not preserved: %#v", item["updateIntervalMs"])
	}
}

func TestUpstreamNestedFalsyValuesSurviveRoundTrip(t *testing.T) {
	input := []byte(`{
  "proxy": {"useCredentials": false, "future": "kept"},
  "message": {"refreshInterval": false, "future": 0},
  "defaults": {"layout": "", "future": true},
  "links": [{"name": "", "url": "#", "future": false}],
  "services": [{"items": [{"name": "Card", "quick": [{"name": "", "url": "#", "future": null}]}]}]
}`)

	var config HomerConfig
	if err := json.Unmarshal(input, &config); err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalHomerConfigToYAML(&config)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, expected := range []string{
		"useCredentials: false", "future: kept", "refreshInterval: false",
		"layout: \"\"", "future: true", "future: false", "future: null",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("nested upstream value %q was dropped:\n%s", expected, output)
		}
	}
}

func TestExplicitEmptyAndNullRootFieldsSurviveYAMLEmission(t *testing.T) {
	input := []byte(`{
  "title": "",
  "columns": null,
  "proxy": null,
  "message": null,
  "colors": {},
  "defaults": null,
  "links": [],
  "services": []
}`)

	var config HomerConfig
	if err := json.Unmarshal(input, &config); err != nil {
		t.Fatal(err)
	}
	output, err := marshalHomerConfigToYAML(&config)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(output, &got); err != nil {
		t.Fatalf("generated YAML is invalid: %v\n%s", err, output)
	}
	for key, want := range map[string]any{
		"title":    "",
		"columns":  nil,
		"proxy":    nil,
		"message":  nil,
		"colors":   map[any]any{},
		"defaults": nil,
		"links":    []any{},
		"services": []any{},
	} {
		gotValue, ok := got[key]
		if !ok {
			t.Errorf("generated YAML dropped explicit %s: %s", key, output)
			continue
		}
		if key == "colors" {
			if _, ok := gotValue.(map[any]any); !ok {
				t.Errorf("colors = %#v, want empty map", gotValue)
			}
			continue
		}
		if key == "links" || key == "services" {
			if values, ok := gotValue.([]any); !ok || len(values) != 0 {
				t.Errorf("%s = %#v, want empty array", key, gotValue)
			}
			continue
		}
		if gotValue != want {
			t.Errorf("%s = %#v, want %#v", key, gotValue, want)
		}
	}
}

func TestMessageMappingAcceptsArbitraryJSONValues(t *testing.T) {
	input := []byte(`{"message":{"mapping":{"title":{"path":["data","title"]},"content":false}}}`)
	var config HomerConfig
	if err := json.Unmarshal(input, &config); err != nil {
		t.Fatal(err)
	}
	output, err := marshalHomerConfigToYAML(&config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "content: false") || !strings.Contains(string(output), "path:") {
		t.Fatalf("arbitrary mapping values were not preserved:\n%s", output)
	}
}

func TestPWAManifestEscapesValuesAndUsesHomerAssetPaths(t *testing.T) {
	manifest := GeneratePWAManifest(`A "quoted" dashboard`, "short", "line\nbreak", "#123", "#456", "standalone", "/#home", map[string]string{
		"192": `icons/custom".png`,
	})
	var decoded map[string]any
	if err := json.Unmarshal([]byte(manifest), &decoded); err != nil {
		t.Fatalf("manifest is not valid JSON: %v\n%s", err, manifest)
	}
	if got := decoded["name"]; got != `A "quoted" dashboard` {
		t.Fatalf("name = %#v", got)
	}
	if !strings.Contains(manifest, `icons/custom\".png`) {
		t.Fatalf("custom icon path was not escaped: %s", manifest)
	}
	defaults := GeneratePWAManifest("Dashboard", "", "", "", "", "", "", nil)
	if strings.Contains(defaults, "assets/icons/") || !strings.Contains(defaults, "icons/pwa-192x192.png") {
		t.Fatalf("default Homer icon path is wrong: %s", defaults)
	}
}
