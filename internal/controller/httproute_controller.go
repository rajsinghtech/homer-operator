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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator.git/pkg/homer"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the HTTPRoute instance
	httproute := &gatewayv1.HTTPRoute{}
	if err := r.Get(ctx, req.NamespacedName, httproute); err != nil {
		log.Error(err, "unable to fetch HTTPRoute", "httproute", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// List all Dashboard CRs
	dashboardList := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboardList); err != nil {
		log.Error(err, "unable to list Dashboards", "httproute", req.NamespacedName)
		return ctrl.Result{}, err
	}

	for _, dashboard := range dashboardList.Items {
		// Check if dashboard annotations are a subset of the HTTPRoute annotations
		delete(dashboard.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if isSubset(httproute.Annotations, dashboard.Annotations) {
			configMap := corev1.ConfigMap{}
			log.Info("Dashboard annotations are a subset of the HTTPRoute annotations", "dashboard", dashboard.Name)
			if err := r.Get(ctx, client.ObjectKey{Namespace: dashboard.Namespace, Name: dashboard.Name + "-homer"}, &configMap); err != nil {
				log.Error(err, "unable to fetch ConfigMap", "configmap", dashboard.Name+"-homer")
				return ctrl.Result{}, err
			}
			homer.UpdateConfigMapHTTPRoute(&configMap, httproute)
			if err := r.updateConfigMapWithRetry(ctx, &configMap, dashboard.Name); err != nil {
				log.Error(err, "unable to update ConfigMap", "configmap", dashboard.Name)
				return ctrl.Result{}, err
			}
			log.Info("Updated ConfigMap", "configmap", dashboard.Name)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.HTTPRoute{}).
		Complete(r)
}

// updateConfigMapWithRetry updates a ConfigMap with exponential backoff retry on conflicts
func (r *HTTPRouteReconciler) updateConfigMapWithRetry(ctx context.Context, configMap *corev1.ConfigMap, dashboardName string) error {
	log := log.FromContext(ctx)

	backoff := wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		Cap:      5 * time.Second,
	}

	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		// Get the latest version of the ConfigMap before updating
		latestConfigMap := &corev1.ConfigMap{}
		key := client.ObjectKeyFromObject(configMap)
		if err := r.Get(ctx, key, latestConfigMap); err != nil {
			if apierrors.IsNotFound(err) {
				// ConfigMap was deleted, no need to retry
				return false, err
			}
			log.V(1).Info("Failed to get latest ConfigMap, retrying", "error", err)
			return false, nil // Retry
		}

		// Copy our changes to the latest version
		latestConfigMap.Data = configMap.Data
		latestConfigMap.BinaryData = configMap.BinaryData

		// Attempt to update
		if err := r.Update(ctx, latestConfigMap); err != nil {
			if apierrors.IsConflict(err) {
				log.V(1).Info("ConfigMap update conflict, retrying", "configmap", dashboardName)
				return false, nil // Retry
			}
			// Non-conflict error, don't retry
			return false, err
		}

		// Success
		return true, nil
	})
}
