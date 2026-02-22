package homer

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func makeService(name, namespace string, port int32, annotations map[string]string) corev1.Service {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       port,
					TargetPort: intstr.FromInt32(port),
				},
			},
		},
	}
	return svc
}

func TestUpdateHomerConfigService_BasicItem(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("my-app", "default", 8080, nil)

	UpdateHomerConfigService(config, svc)

	if len(config.Services) == 0 {
		t.Fatal("expected at least one service group")
	}

	found := false
	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "my-app" {
				found = true
				expectedURL := "http://my-app.default.svc.cluster.local:8080"
				if item.Parameters["url"] != expectedURL {
					t.Errorf("url = %q, want %q", item.Parameters["url"], expectedURL)
				}
				if item.Parameters["subtitle"] != "default/my-app" {
					t.Errorf("subtitle = %q, want %q", item.Parameters["subtitle"], "default/my-app")
				}
				if item.Parameters["logo"] != ServiceIconURL {
					t.Errorf("logo = %q, want %q", item.Parameters["logo"], ServiceIconURL)
				}
			}
		}
	}
	if !found {
		t.Error("item 'my-app' not found in config")
	}
}

func TestUpdateHomerConfigService_HTTPSPort443(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("secure-app", "prod", 443, nil)

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "secure-app" {
				expected := "https://secure-app.prod.svc.cluster.local:443"
				if item.Parameters["url"] != expected {
					t.Errorf("url = %q, want %q", item.Parameters["url"], expected)
				}
				return
			}
		}
	}
	t.Error("item 'secure-app' not found")
}

func TestUpdateHomerConfigService_AnnotationURLOverride(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("my-app", "default", 8080, map[string]string{
		"item.homer.rajsingh.info/url": "https://myapp.example.com",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "my-app" {
				if item.Parameters["url"] != "https://myapp.example.com" {
					t.Errorf("url = %q, want annotation override", item.Parameters["url"])
				}
				return
			}
		}
	}
	t.Error("item 'my-app' not found")
}

func TestUpdateHomerConfigService_CustomAnnotations(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("my-app", "default", 8080, map[string]string{
		"item.homer.rajsingh.info/name":     "Custom Name",
		"item.homer.rajsingh.info/subtitle": "Custom Subtitle",
		"service.homer.rajsingh.info/name":  "My Group",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		sgName := getServiceName(&sg)
		if sgName == "My Group" {
			for _, item := range sg.Items {
				if item.Parameters["name"] != "Custom Name" {
					t.Errorf("name = %q, want 'Custom Name'", item.Parameters["name"])
				}
				if item.Parameters["subtitle"] != "Custom Subtitle" {
					t.Errorf("subtitle = %q, want 'Custom Subtitle'", item.Parameters["subtitle"])
				}
				return
			}
		}
	}
	t.Error("service group 'My Group' not found")
}

func TestUpdateHomerConfigService_NoPorts(t *testing.T) {
	config := &HomerConfig{}
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-ports",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{},
	}

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "no-ports" {
				expected := "http://no-ports.default.svc.cluster.local"
				if item.Parameters["url"] != expected {
					t.Errorf("url = %q, want %q", item.Parameters["url"], expected)
				}
				return
			}
		}
	}
	t.Error("item 'no-ports' not found")
}

func TestUpdateHomerConfigService_HiddenItem(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("hidden-app", "default", 8080, map[string]string{
		"item.homer.rajsingh.info/hide": "true",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "hidden-app" {
				t.Error("hidden item should not appear in config")
			}
		}
	}
}

func TestUpdateHomerConfigService_MultipleServices(t *testing.T) {
	config := &HomerConfig{}

	for i := 0; i < 3; i++ {
		svc := makeService(fmt.Sprintf("app-%d", i), "default", int32(8080+i), nil)
		UpdateHomerConfigService(config, svc)
	}

	totalItems := 0
	for _, sg := range config.Services {
		totalItems += len(sg.Items)
	}
	if totalItems != 3 {
		t.Errorf("expected 3 items, got %d", totalItems)
	}
}

func TestUpdateHomerConfigService_ClusterAnnotation(t *testing.T) {
	config := &HomerConfig{}
	svc := makeService("remote-app", "default", 8080, map[string]string{
		"homer.rajsingh.info/cluster": "prod-cluster",
	})

	UpdateHomerConfigService(config, svc)

	for _, sg := range config.Services {
		for _, item := range sg.Items {
			if item.Parameters["name"] == "remote-app" {
				if item.Source != "svc/remote-app@prod-cluster" {
					t.Errorf("source = %q, want 'svc/remote-app@prod-cluster'", item.Source)
				}
				return
			}
		}
	}
	t.Error("item 'remote-app' not found")
}
