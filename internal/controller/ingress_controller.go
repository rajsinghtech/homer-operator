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
	homer "github.com/rajsinghtech/homer-operator.git/pkg/homer"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var ingress networkingv1.Ingress
	if err := r.Get(ctx, req.NamespacedName, &ingress); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Ingress", "ingress", req.NamespacedName)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}
	dashboardList, error := getAllDashboard(ctx, r)
	if error != nil {
		log.Error(error, "unable to fetch DashboardList")
		return ctrl.Result{}, error
	}
	for _, dashboard := range dashboardList.Items {
		// Check if dashboard annotations are a subset of the ingress annotations
		delete(dashboard.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if isSubset(ingress.Annotations, dashboard.Annotations) {
			configMap := corev1.ConfigMap{}
			log.Info("Dashboard annotations are a subset of the ingress annotations", "dashboard", dashboard.Name)
			if error := r.Get(ctx, client.ObjectKey{Namespace: dashboard.Namespace, Name: dashboard.Name}, &configMap); error != nil {
				log.Error(error, "unable to fetch ConfigMap", "configmap", dashboard.Name)
				return ctrl.Result{}, error
			}
			homer.UpdateConfigMapIngress(&configMap, ingress)
			if error := r.Update(ctx, &configMap); error != nil {
				log.Error(error, "unable to update ConfigMap", "configmap", dashboard.Name)
				return ctrl.Result{}, error
			}
			log.Info("Updated ConfigMap", "configmap", dashboard.Name)
		}
	}

	return ctrl.Result{}, nil
}

// isSubset checks if the first map is a subset of the second map
func isSubset(map1, map2 map[string]string) bool {
	for key, value := range map2 {
		if map1[key] != value {
			return false
		}
	}
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}

func getAllDashboard(ctx context.Context, r *IngressReconciler) (*homerv1alpha1.DashboardList, error) {
	var dashboardList homerv1alpha1.DashboardList
	if err := r.List(ctx, &dashboardList); err != nil {
		return nil, err
	}
	return &dashboardList, nil
}
