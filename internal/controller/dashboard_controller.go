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

package controller

import (
	// "fmt"
	"context"
	"strings"
	"reflect"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	// "os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DashboardReconciler reconciles a Dashboard object
type DashboardReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=dashboards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=dashboards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=dashboards/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Dashboard object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *DashboardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var dashboard homerv1alpha1.Dashboard
	// Resource Deleted - Clean up all resources
	if err := r.Get(ctx, req.NamespacedName, &dashboard); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Dashboard", "dashboard", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// Delete all deployments with "homer.rajsingh.info/name": dashboard.Name annotation
		deployments := &appsv1.DeploymentList{}
		if err := r.List(ctx, deployments, client.MatchingLabels{"homer.rajsingh.info/name": req.NamespacedName.Name}); err != nil {
			log.Error(err, "unable to list Deployments", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		for _, deployment := range deployments.Items {
			if err := r.Delete(ctx, &deployment); err != nil {
				log.Error(err, "unable to delete Deployment", "dashboard", req.NamespacedName)
				return ctrl.Result{}, err
			}
			log.Info("Deployment deleted", "deployment", deployment)
		}
		services := &corev1.ServiceList{}
		if err := r.List(ctx, services, client.MatchingLabels{"homer.rajsingh.info/name": req.NamespacedName.Name}); err != nil {
			log.Error(err, "unable to list Services", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		for _, service := range services.Items {
			if err := r.Delete(ctx, &service); err != nil {
				log.Error(err, "unable to delete Service", "dashboard", req.NamespacedName)
				return ctrl.Result{}, err
			}
			log.Info("Service deleted", "service", service)
		}
		configMaps := &corev1.ConfigMapList{}
		if err := r.List(ctx, configMaps, client.MatchingLabels{"homer.rajsingh.info/name": req.NamespacedName.Name}); err != nil {
			log.Error(err, "unable to list ConfigMaps", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		for _, configMap := range configMaps.Items {
			if err := r.Delete(ctx, &configMap); err != nil {
				log.Error(err, "unable to delete ConfigMap", "dashboard", req.NamespacedName)
				return ctrl.Result{}, err
			}
			log.Info("ConfigMap deleted", "configMap", configMap)
		}
		return ctrl.Result{}, nil
	}
	// Get Ingresses
	ingresses := &networkingv1.IngressList{}
	if err := r.List(ctx, ingresses); err != nil {
		log.Error(err, "unable to list Ingresses", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// Generate Dashboard
	dashboard = updateHomerConfig(dashboard, *ingresses)
	// Resource Created/Updated - Create/Update resources
	configMapSpec, err := r.createConfigMap(dashboard)
	if err != nil {
		log.Error(err, "unable to create ConfigMap", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// Check if the ConfigMap already exists
	if err := r.Get(ctx, client.ObjectKey{Namespace: configMapSpec.Namespace, Name: configMapSpec.Name}, &corev1.ConfigMap{}); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch ConfigMap", "dashboard", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// If not found, create the ConfigMap
		if err := r.Create(ctx, &configMapSpec); err != nil {
			log.Error(err, "unable to create ConfigMap", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("ConfigMap created", "ConfigMap", configMapSpec)
	} else {
		// If found, update the ConfigMap
		if err := r.Update(ctx, &configMapSpec); err != nil {
			log.Error(err, "unable to update ConfigMap", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("ConfigMap updated", "ConfigMap", configMapSpec)
	}
	deploymentSpec, err := r.createDeployment(dashboard)
	if err != nil {
		log.Error(err, "unable to create Deployment", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// Check if the Deployment already exists
	if err := r.Get(ctx, client.ObjectKey{Namespace: deploymentSpec.Namespace, Name: deploymentSpec.Name}, &appsv1.Deployment{}); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Deployment", "dashboard", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// If not found, create the Deployment
		if err := r.Create(ctx, &deploymentSpec); err != nil {
			log.Error(err, "unable to create Deployment", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("Deployment created", "deployment", deploymentSpec)
	} else {
		// If found, update the Deployment
		if err := r.Update(ctx, &deploymentSpec); err != nil {
			log.Error(err, "unable to update Deployment", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("Deployment updated", "deployment", deploymentSpec)
	}
	serviceSpec, err := r.createService(dashboard)
	if err != nil {
		log.Error(err, "unable to create Service", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// Check if the Service already exists
	if err := r.Get(ctx, client.ObjectKey{Namespace: serviceSpec.Namespace, Name: serviceSpec.Name}, &corev1.Service{}); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Service", "dashboard", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// If not found, create the Service
		if err := r.Create(ctx, &serviceSpec); err != nil {
			log.Error(err, "unable to create Service", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("Service created", "service", serviceSpec)
	} else {
		// If found, update the Service
		if err := r.Update(ctx, &serviceSpec); err != nil {
			log.Error(err, "unable to update Service", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("Service updated", "service", serviceSpec)
	}	
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Dashboard{}).
		Complete(r)
}

func updateHomerConfig(dashboard homerv1alpha1.Dashboard, ingresses networkingv1.IngressList) homerv1alpha1.Dashboard {
	var services []homerv1alpha1.Service = dashboard.Spec.HomerConfig.Services
	// iterate over all ingresses and add them to the dashboard
	for _, ingress := range ingresses.Items {
		for _, rule := range ingress.Spec.Rules {
			item := homerv1alpha1.Item{}
			service := homerv1alpha1.Service{}
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
				if strings.HasPrefix(key, "item.homer.rajsingh.info/"){
					fieldName := strings.TrimPrefix(key, "item.homer.rajsingh.info/")
					reflect.ValueOf(&item).Elem().FieldByName(fieldName).SetString(value)
				}
				if strings.HasPrefix(key, "service.homer.rajsingh.info/"){
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
		for j, s2 := range dashboard.Spec.HomerConfig.Services {
			if s1.Name == s2.Name {
				dashboard.Spec.HomerConfig.Services[j].Items = append(s2.Items, s1.Items[0])
				complete = true
				break
			}
		}
		if !complete {
			dashboard.Spec.HomerConfig.Services = append(dashboard.Spec.HomerConfig.Services, s1)
		}
	}
	return dashboard
}

func (r *DashboardReconciler) createConfigMap(dashboard homerv1alpha1.Dashboard) (corev1.ConfigMap, error) {
	objYAML, err := yaml.Marshal(dashboard.Spec.HomerConfig)
	if err != nil {
		return corev1.ConfigMap{}, err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dashboard.Spec.ConfigMap.Name,
			Namespace: dashboard.Namespace,
			Labels: map[string]string{
				"managed-by":               "homer-operator",
				"homer.rajsingh.info/name": dashboard.Name,
			},
		},
		Data: map[string]string{
			"config.yml": string(objYAML),
		},
	}
	return *cm, nil
}

func (r *DashboardReconciler) createDeployment(dashboard homerv1alpha1.Dashboard) (appsv1.Deployment, error) {
	var replicas int32 = 1
	image := "b4bz/homer"
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dashboard.Name,
			Namespace: dashboard.Namespace,
			Labels: map[string]string{
				"managed-by":               "homer-operator",
				"homer.rajsingh.info/name": dashboard.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"homer.rajsingh.info/name": dashboard.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"homer.rajsingh.info/name": dashboard.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  dashboard.Name,
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
										Name: dashboard.Spec.ConfigMap.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return *d, nil
}
func (r *DashboardReconciler) createService(dashboard homerv1alpha1.Dashboard) (corev1.Service, error) {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dashboard.Name,
			Namespace: dashboard.Namespace,
			Labels: map[string]string{
				"managed-by":               "homer-operator",
				"homer.rajsingh.info/name": dashboard.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"homer.rajsingh.info/name": dashboard.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
	return *s, nil
}
