package homer

import (
	"os"
	"reflect"
	"strings"

	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type HomerConfig struct {
	Title    string        `json:"title,omitempty"`
	Subtitle string        `json:"subtitle,omitempty"`
	Logo     string        `json:"logo,omitempty"`
	Header   string        `json:"header,omitempty"`
	Services []Service     `json:"services,omitempty"`
	Footer   string        `json:"footer,omitempty"`
	Defaults DefaultConfig `json:"defaults,omitempty"`
	Links    []Link        `json:"links,omitempty"`
}

type ProxyConfig struct {
	UseCredentials bool `json:"useCredentials,omitempty"`
}

type DefaultConfig struct {
	Layout     string `json:"layout,omitempty"`
	ColorTheme string `json:"colorTheme,omitempty"`
}

type Service struct {
	Name  string `json:"name,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Logo  string `json:"logo,omitempty"`
	Items []Item `json:"items,omitempty"`
}

type Item struct {
	Name         string `json:"name,omitempty"`
	Logo         string `json:"logo,omitempty"`
	Subtitle     string `json:"subtitle,omitempty"`
	Tag          string `json:"tag,omitempty"`
	Keywords     string `json:"keywords,omitempty"`
	Url          string `json:"url,omitempty"`
	Target       string `json:"target,omitempty"`
	Tagstyle     string `json:"tagstyle,omitempty"`
	Type         string `json:"type,omitempty"`
	Class        string `json:"class,omitempty"`
	Background   string `json:"background,omitempty"`
	Apikey       string `json:"apikey,omitempty"`
	Node      	 string `json:"node,omitempty"`
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

func CreateConfigMap(config HomerConfig, name string, namespace string, ingresses networkingv1.IngressList) corev1.ConfigMap {
	UpdateHomerConfig(&config, ingresses)
	objYAML, err := yaml.Marshal(config)
	if err != nil {
		return corev1.ConfigMap{}
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
		},
		Data: map[string]string{
			"config.yml": string(objYAML),
		},
	}
	return *cm
}

func CreateDeployment(name string, namespace string) appsv1.Deployment {
	var replicas int32 = 1
	image := "b4bz/homer"
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
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
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: image,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/www/assets",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
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
										Name: name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return *d
}

func CreateService(name string, namespace string) corev1.Service {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": name,
			},
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
			for key, value := range ingress.ObjectMeta.Annotations {
				if strings.HasPrefix(key, "item.homer.rajsingh.info/") {
					fieldName := strings.TrimPrefix(key, "item.homer.rajsingh.info/")
					reflect.ValueOf(&item).Elem().FieldByName(fieldName).SetString(value)
				}
				if strings.HasPrefix(key, "service.homer.rajsingh.info/") {
					fieldName := strings.TrimPrefix(key, "service.homer.rajsingh.info/")
					reflect.ValueOf(&service).Elem().FieldByName(fieldName).SetString(value)
				}
			}
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
	if len(ingress.Spec.TLS) > 0 {
		item.Url = "https://" + ingress.Spec.Rules[0].Host
	} else {
		item.Url = "http://" + ingress.Spec.Rules[0].Host
	}
	item.Logo = "https://raw.githubusercontent.com/kubernetes/community/master/icons/png/resources/labeled/ing-128.png"
	item.Subtitle = ingress.Spec.Rules[0].Host
	for key, value := range ingress.ObjectMeta.Annotations {
		if strings.HasPrefix(key, "item.homer.rajsingh.info/") {
			fieldName := strings.TrimPrefix(key, "item.homer.rajsingh.info/")
			reflect.ValueOf(&item).Elem().FieldByName(fieldName).SetString(value)
		}
		if strings.HasPrefix(key, "service.homer.rajsingh.info/") {
			fieldName := strings.TrimPrefix(key, "service.homer.rajsingh.info/")
			reflect.ValueOf(&service).Elem().FieldByName(fieldName).SetString(value)
		}
	}
	for sx, s := range homerConfig.Services {
		if s.Name == service.Name {
			for ix, i := range s.Items {
				if i.Name == item.Name {
					homerConfig.Services[sx].Items[ix] = item
					return
				}
			}
			homerConfig.Services[sx].Items = append(homerConfig.Services[sx].Items, item)
		}
	}
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