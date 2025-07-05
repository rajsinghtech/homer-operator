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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch

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
			// Check if HTTPRoute should be included based on all filters
			if shouldIncludeHTTPRoute, err := r.shouldIncludeHTTPRouteForDashboard(ctx, httproute, &dashboard); err != nil {
				log.Error(err, "unable to determine if HTTPRoute should be included", "dashboard", dashboard.Name)
				return ctrl.Result{}, err
			} else if !shouldIncludeHTTPRoute {
				log.V(1).Info("HTTPRoute excluded by selectors or filters", "dashboard", dashboard.Name, "httproute", req.NamespacedName)
				continue
			}

			configMap := corev1.ConfigMap{}
			log.Info("Dashboard annotations are a subset of the HTTPRoute annotations", "dashboard", dashboard.Name)
			if err := r.Get(ctx, client.ObjectKey{Namespace: dashboard.Namespace, Name: dashboard.Name + "-homer"}, &configMap); err != nil {
				if apierrors.IsNotFound(err) {
					log.V(1).Info("ConfigMap not found - likely not created yet", "configmap", dashboard.Name+"-homer")
					continue
				}
				log.Error(err, "unable to fetch ConfigMap", "configmap", dashboard.Name+"-homer")
				return ctrl.Result{}, err
			}
			homer.UpdateConfigMapHTTPRoute(&configMap, httproute, dashboard.Spec.DomainFilters)
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

// shouldIncludeHTTPRouteForDashboard determines if an HTTPRoute should be included
// based on the Dashboard's selectors and filters. If no selectors are specified, all HTTPRoutes are included.
func (r *HTTPRouteReconciler) shouldIncludeHTTPRouteForDashboard(ctx context.Context, httproute *gatewayv1.HTTPRoute, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	log := log.FromContext(ctx)

	// Check HTTPRoute label selector
	if dashboard.Spec.HTTPRouteSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.HTTPRouteSelector)
		if err != nil {
			return false, err
		}
		if !selector.Matches(labels.Set(httproute.Labels)) {
			log.V(1).Info("HTTPRoute excluded by HTTPRoute label selector", "httproute", httproute.Name)
			return false, nil
		}
	}

	// Check domain filters
	if len(dashboard.Spec.DomainFilters) > 0 {
		if !r.matchesDomainFilters(httproute.Spec.Hostnames, dashboard.Spec.DomainFilters) {
			log.Info("HTTPRoute excluded by domain filters", "httproute", httproute.Name, "hostnames", httproute.Spec.Hostnames, "domainFilters", dashboard.Spec.DomainFilters)
			return false, nil
		}
		log.V(1).Info("HTTPRoute included by domain filters", "httproute", httproute.Name, "hostnames", httproute.Spec.Hostnames, "domainFilters", dashboard.Spec.DomainFilters)
	}

	// Check Gateway selector
	if dashboard.Spec.GatewaySelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.GatewaySelector)
		if err != nil {
			return false, err
		}

		gatewayMatched := false
		// Check each parent Gateway reference in the HTTPRoute
		for _, parentRef := range httproute.Spec.ParentRefs {
			// Default to Gateway kind if not specified
			kind := "Gateway"
			if parentRef.Kind != nil {
				kind = string(*parentRef.Kind)
			}

			// Only check Gateway resources
			if kind != "Gateway" {
				continue
			}

			// Default to same namespace as HTTPRoute if not specified
			namespace := httproute.Namespace
			if parentRef.Namespace != nil {
				namespace = string(*parentRef.Namespace)
			}

			// Fetch the Gateway
			gateway := &gatewayv1.Gateway{}
			gatewayKey := client.ObjectKey{
				Name:      string(parentRef.Name),
				Namespace: namespace,
			}

			if err := r.Get(ctx, gatewayKey, gateway); err != nil {
				if apierrors.IsNotFound(err) {
					log.V(1).Info("Gateway not found", "gateway", gatewayKey)
					continue
				}
				return false, err
			}

			// Check if Gateway labels match the selector
			if selector.Matches(labels.Set(gateway.Labels)) {
				gatewayMatched = true
				break
			}
		}

		if !gatewayMatched {
			log.V(1).Info("HTTPRoute excluded by Gateway selector", "httproute", httproute.Name)
			return false, nil
		}
	}

	return true, nil
}

// matchesDomainFilters checks if any hostname matches the domain filters
func (r *HTTPRouteReconciler) matchesDomainFilters(hostnames []gatewayv1.Hostname, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, hostname := range hostnames {
		hostnameStr := string(hostname)
		for _, filter := range domainFilters {
			// Support exact match or subdomain match
			if hostnameStr == filter || strings.HasSuffix(hostnameStr, "."+filter) {
				return true
			}
		}
	}

	return false
}
