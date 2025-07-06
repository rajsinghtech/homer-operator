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

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator/pkg/homer"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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
// Reconcile watches Ingress resources and automatically updates Dashboard
// configurations by:
// 1. Extracting service information from Ingress rules
// 2. Finding associated Dashboard resources
// 3. Updating Dashboard specs with discovered services
// 4. Triggering Homer configuration regeneration
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
	dashboardList, err := getAllDashboard(ctx, r)
	if err != nil {
		log.Error(err, "unable to fetch DashboardList")
		return ctrl.Result{}, err
	}
	for _, dashboard := range dashboardList.Items {
		// Check if dashboard annotations are a subset of the ingress annotations
		delete(dashboard.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if isSubset(ingress.Annotations, dashboard.Annotations) {
			// Check if Ingress should be included based on all filters
			if shouldIncludeIngress, err := r.shouldIncludeIngressForDashboard(ctx, &ingress, &dashboard); err != nil {
				log.Error(err, "unable to determine if Ingress should be included", "dashboard", dashboard.Name)
				return ctrl.Result{}, err
			} else if !shouldIncludeIngress {
				log.V(1).Info("Ingress excluded by selectors or filters", "dashboard", dashboard.Name, "ingress", req.NamespacedName)
				continue
			}

			configMap := corev1.ConfigMap{}
			log.Info("Dashboard annotations are a subset of the ingress annotations", "dashboard", dashboard.Name)
			if error := r.Get(ctx, client.ObjectKey{Namespace: dashboard.Namespace, Name: dashboard.Name + "-homer"}, &configMap); error != nil {
				if apierrors.IsNotFound(error) {
					log.V(1).Info("ConfigMap not found - likely not created yet", "configmap", dashboard.Name+"-homer")
					continue
				}
				log.Error(error, "unable to fetch ConfigMap", "configmap", dashboard.Name+"-homer")
				return ctrl.Result{}, error
			}
			homer.UpdateConfigMapIngress(&configMap, ingress, dashboard.Spec.DomainFilters)
			if err := r.updateConfigMapWithRetry(ctx, &configMap, dashboard.Name); err != nil {
				log.Error(err, "unable to update ConfigMap", "configmap", dashboard.Name)
				return ctrl.Result{}, err
			}
			log.Info("Updated ConfigMap", "configmap", dashboard.Name)
		}
	}

	return ctrl.Result{}, nil
}

// shouldIncludeIngressForDashboard determines if an Ingress should be included
// based on the Dashboard's selectors and filters. If no selectors are specified, all Ingresses are included.
func (r *IngressReconciler) shouldIncludeIngressForDashboard(ctx context.Context, ingress *networkingv1.Ingress, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	log := log.FromContext(ctx)

	// Check Ingress label selector
	if dashboard.Spec.IngressSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.IngressSelector)
		if err != nil {
			return false, err
		}
		if !selector.Matches(labels.Set(ingress.Labels)) {
			log.V(1).Info("Ingress excluded by Ingress label selector", "ingress", ingress.Name)
			return false, nil
		}
	}

	// Check domain filters
	if len(dashboard.Spec.DomainFilters) > 0 {
		if !r.matchesIngressDomainFilters(ingress, dashboard.Spec.DomainFilters) {
			log.V(1).Info("Ingress excluded by domain filters", "ingress", ingress.Name)
			return false, nil
		}
	}

	return true, nil
}

// matchesIngressDomainFilters checks if any Ingress rule host matches the domain filters
func (r *IngressReconciler) matchesIngressDomainFilters(ingress *networkingv1.Ingress, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}

		for _, filter := range domainFilters {
			// Support exact match or subdomain match
			if rule.Host == filter || strings.HasSuffix(rule.Host, "."+filter) {
				return true
			}
		}
	}

	return false
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
	var clusterDashboards homerv1alpha1.DashboardList
	if err := r.List(ctx, &clusterDashboards); err != nil {
		return nil, err
	}
	return &clusterDashboards, nil
}

// updateConfigMapWithRetry updates a ConfigMap with exponential backoff retry on conflicts
func (r *IngressReconciler) updateConfigMapWithRetry(ctx context.Context, configMap *corev1.ConfigMap, dashboardName string) error {
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
