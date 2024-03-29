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
	"reflect"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator.git/pkg/homer"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
		labelSelector := client.MatchingLabels{"dashboard.homer.rajsingh.info/name": req.NamespacedName.Name}
		// List of resources to delete
		resourceTypes := []struct {
			list     client.ObjectList
			resource string
		}{
			{&appsv1.DeploymentList{}, "Deployment"},
			{&corev1.ServiceList{}, "Service"},
			{&corev1.ConfigMapList{}, "ConfigMap"},
		}

		for _, resourceType := range resourceTypes {
			if err := r.List(ctx, resourceType.list, labelSelector); err != nil {
				log.Error(err, "unable to list resources", "dashboard", req.NamespacedName)
				return ctrl.Result{}, err
			}
			items := reflect.ValueOf(resourceType.list).Elem().FieldByName("Items")
			for i := 0; i < items.Len(); i++ {
				item := items.Index(i).Addr().Interface().(client.Object)
				if err := r.Delete(ctx, item); err != nil {
					return ctrl.Result{}, err
				}
				log.Info("Resource deleted", "resource", item.GetName())
			}
		}
		return ctrl.Result{}, nil
	}
	ingresses := &networkingv1.IngressList{}
	if err := r.List(ctx, ingresses); err != nil {
		log.Error(err, "unable to list Ingresses", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// Resource Created - Create all resources
	deployment := homer.CreateDeployment(dashboard.Name, dashboard.Namespace)
	service := homer.CreateService(dashboard.Name, dashboard.Namespace)
	configMap := homer.CreateConfigMap(dashboard.Spec.HomerConfig, dashboard.Name, dashboard.Namespace, *ingresses)
	// List of resources
	resources := []client.Object{&deployment, &service, &configMap}

	for _, resource := range resources {
		newResource := reflect.New(reflect.TypeOf(resource).Elem()).Interface().(client.Object)
		err := r.Get(ctx, client.ObjectKey{Namespace: resource.GetNamespace(), Name: resource.GetName()}, newResource)
		switch {
		case err != nil:
			err = r.Create(ctx, resource)
			if err != nil {
				log.Error(err, "unable to create resource", "resource", resource)
				return ctrl.Result{}, err
			}
			log.Info("Resource created", "resource", resource)
		case client.IgnoreNotFound(err) != nil:
			log.Error(err, "unable to fetch resource", "resource", resource)
			return ctrl.Result{}, err
		default:
			err = r.Update(ctx, resource)
			if err != nil {
				log.Error(err, "unable to update resource", "resource", resource)
				return ctrl.Result{}, err
			}
			log.Info("Resource updated", "resource", resource)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Dashboard{}).
		Complete(r)
}
