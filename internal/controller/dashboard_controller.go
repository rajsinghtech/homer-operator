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
	"context"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
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
	if err := r.Get(ctx, req.NamespacedName, &dashboard); err != nil {

		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Dashboard", "dashboard", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// Delete all resources with "homer.rajsingh.info/name": dashboard.Name annotation
		deployments := &appsv1.DeploymentList{}
		err = r.List(ctx, deployments, client.MatchingLabels{"homer.rajsingh.info/name": req.NamespacedName.Name})
		if err != nil {
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
		return ctrl.Result{}, nil
	}

	configMapSpec, err := r.createConfigMap(dashboard)
	if err != nil {
		log.Error(err, "unable to create ConfigMap", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// Check if the ConfigMap already exists
	existingConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Namespace: configMapSpec.Namespace, Name: configMapSpec.Name}, existingConfigMap)
	if err != nil {
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
		existingConfigMap.Data = configMapSpec.Data
		if err := r.Update(ctx, existingConfigMap); err != nil {
			log.Error(err, "unable to update ConfigMap", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("ConfigMap updated", "ConfigMap", existingConfigMap)
	}

	deploymentSpec, err := r.createDeployment(dashboard)
	if err != nil {
		log.Error(err, "unable to create Deployment", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Check if the Deployment already exists
	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Namespace: deploymentSpec.Namespace, Name: deploymentSpec.Name}, existingDeployment)
	if err != nil {
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
		existingDeployment.Spec = deploymentSpec.Spec
		if err := r.Update(ctx, existingDeployment); err != nil {
			log.Error(err, "unable to update Deployment", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}
		log.Info("Deployment updated", "deployment", existingDeployment)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Dashboard{}).
		Complete(r)
}

func (r *DashboardReconciler) createConfigMap(dashboard homerv1alpha1.Dashboard) (corev1.ConfigMap, error) {
	yamlFile, err := os.ReadFile("../homer/config.yml")
	if err != nil {
		return corev1.ConfigMap{}, err
	}
	obj := make(map[string]interface{})
	err = yaml.Unmarshal(yamlFile, obj)
	if err != nil {
		return corev1.ConfigMap{}, err
	}
	obj["title"] = dashboard.Spec.Title
	obj["subtitle"] = dashboard.Spec.Subtitle
	// Marshal the obj into YAML
	objYAML, err := yaml.Marshal(obj)
	if err != nil {
		return corev1.ConfigMap{}, err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dashboard.Spec.ConfigMap.Name,
			Namespace: dashboard.Namespace,
			Annotations: map[string]string{
				"managed-by":               "homer-operator",
				"homer.rajsingh.info/name": dashboard.Name,
			},
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
			Annotations: map[string]string{
				"managed-by":               "homer-operator",
				"homer.rajsingh.info/name": dashboard.Name,
			},
			Labels: map[string]string{
				"managed-by":               "homer-operator",
				"homer.rajsingh.info/name": dashboard.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": dashboard.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": dashboard.Name,
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
