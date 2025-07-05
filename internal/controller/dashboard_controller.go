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
	"strings"
	"time"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator.git/pkg/homer"
	appsv1 "k8s.io/api/apps/v1"
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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// DashboardReconciler reconciles a Dashboard object
type DashboardReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	EnableGatewayAPI bool
}

//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=dashboards,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=dashboards/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=dashboards/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Reconcile manages Dashboard resources by:
// 1. Creating/updating ConfigMaps for Homer configuration
// 2. Managing Deployment resources for Homer instances
// 3. Creating Services for Homer deployments
// 4. Watching for changes and updating resources accordingly
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *DashboardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var dashboard homerv1alpha1.Dashboard
	if err := r.Get(ctx, req.NamespacedName, &dashboard); err != nil {
		if apierrors.IsNotFound(err) {
			// Dashboard was deleted, this is normal during deletion
			log.V(1).Info("Dashboard not found - likely deleted", "dashboard", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Dashboard", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// List ingresses with performance optimization - only get ones with rules
	ingresses := &networkingv1.IngressList{}
	if err := r.List(ctx, ingresses, client.HasLabels([]string{})); err != nil {
		log.Error(err, "unable to list Ingresses", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Filter Ingresses based on dashboard selectors
	filteredIngresses := []networkingv1.Ingress{}
	for _, ingress := range ingresses.Items {
		shouldInclude, err := r.shouldIncludeIngressForDashboard(ctx, &ingress, &dashboard)
		if err != nil {
			log.Error(err, "unable to determine if Ingress should be included", "dashboard", dashboard.Name, "ingress", ingress.Name)
			return ctrl.Result{}, err
		}
		if shouldInclude {
			filteredIngresses = append(filteredIngresses, ingress)
		}
	}

	// Create filtered Ingress list
	filteredIngressList := networkingv1.IngressList{
		Items: filteredIngresses,
	}

	// Validate theme configuration
	if err := homer.ValidateTheme(dashboard.Spec.HomerConfig.Theme); err != nil {
		log.Error(err, "invalid theme configuration", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Resource Created - Create all resources
	var deployment appsv1.Deployment
	var assetsConfigMapName string
	var pwaManifest string

	// Generate PWA manifest if enabled
	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.PWA != nil && dashboard.Spec.Assets.PWA.Enabled {
		pwa := dashboard.Spec.Assets.PWA

		// Set defaults if not provided
		name := pwa.Name
		if name == "" {
			name = dashboard.Spec.HomerConfig.Title
		}
		if name == "" {
			name = dashboard.Name
		}

		shortName := pwa.ShortName
		if shortName == "" {
			shortName = name
		}

		description := pwa.Description
		if description == "" {
			description = dashboard.Spec.HomerConfig.Subtitle
		}
		if description == "" {
			description = "Personal Dashboard"
		}

		themeColor := pwa.ThemeColor
		if themeColor == "" {
			themeColor = "#3367d6"
		}

		backgroundColor := pwa.BackgroundColor
		if backgroundColor == "" {
			backgroundColor = "#ffffff"
		}

		display := pwa.Display
		if display == "" {
			display = "standalone"
		}

		startURL := pwa.StartURL
		if startURL == "" {
			startURL = "/"
		}

		// Build icons map
		icons := make(map[string]string)
		if dashboard.Spec.Assets.Icons != nil {
			if dashboard.Spec.Assets.Icons.PWAIcon192 != "" {
				icons["192"] = dashboard.Spec.Assets.Icons.PWAIcon192
			}
			if dashboard.Spec.Assets.Icons.PWAIcon512 != "" {
				icons["512"] = dashboard.Spec.Assets.Icons.PWAIcon512
			}
		}

		// Generate PWA manifest
		pwaManifest = homer.GeneratePWAManifest(name, description, themeColor, backgroundColor, display, startURL, icons)
	}

	// Check if custom assets are configured
	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.ConfigMapRef != nil {
		assetsConfigMapName = dashboard.Spec.Assets.ConfigMapRef.Name
		deployment = homer.CreateDeploymentWithAssets(dashboard.Name, dashboard.Namespace, dashboard.Spec.Replicas, &dashboard, assetsConfigMapName, pwaManifest)
	} else if pwaManifest != "" {
		// PWA enabled but no custom assets ConfigMap - still need to use CreateDeploymentWithAssets for PWA
		deployment = homer.CreateDeploymentWithAssets(dashboard.Name, dashboard.Namespace, dashboard.Spec.Replicas, &dashboard, "", pwaManifest)
	} else {
		deployment = homer.CreateDeployment(dashboard.Name, dashboard.Namespace, dashboard.Spec.Replicas, &dashboard)
	}

	service := homer.CreateService(dashboard.Name, dashboard.Namespace, &dashboard)

	// Create a copy of the HomerConfig to avoid modifying the original
	homerConfig := dashboard.Spec.HomerConfig.DeepCopy()

	// Resolve secrets for smart card items if configured
	if dashboard.Spec.Secrets != nil && dashboard.Spec.Secrets.APIKey != nil {
		// Convert API type to local type to avoid circular imports
		secretRef := &homer.SecretKeyRef{
			Name:      dashboard.Spec.Secrets.APIKey.Name,
			Key:       dashboard.Spec.Secrets.APIKey.Key,
			Namespace: dashboard.Spec.Secrets.APIKey.Namespace,
		}

		for serviceIdx := range homerConfig.Services {
			for itemIdx := range homerConfig.Services[serviceIdx].Items {
				item := &homerConfig.Services[serviceIdx].Items[itemIdx]
				if err := homer.ResolveAPIKeyFromSecret(ctx, r.Client, item, secretRef, dashboard.Namespace); err != nil {
					log.Error(err, "failed to resolve API key from secret", "item", item.Name)
					return ctrl.Result{}, err
				}
			}
		}
	}

	var configMap corev1.ConfigMap
	if r.EnableGatewayAPI {
		httproutes := &gatewayv1.HTTPRouteList{}
		if err := r.List(ctx, httproutes, client.HasLabels([]string{})); err != nil {
			log.Error(err, "unable to list HTTPRoutes", "dashboard", req.NamespacedName)
			return ctrl.Result{}, err
		}

		// Filter HTTPRoutes based on dashboard selectors
		filteredHTTPRoutes := []gatewayv1.HTTPRoute{}
		for _, httproute := range httproutes.Items {
			shouldInclude, err := r.shouldIncludeHTTPRouteForDashboard(ctx, &httproute, &dashboard)
			if err != nil {
				log.Error(err, "unable to determine if HTTPRoute should be included", "dashboard", dashboard.Name, "httproute", httproute.Name)
				return ctrl.Result{}, err
			}
			if shouldInclude {
				filteredHTTPRoutes = append(filteredHTTPRoutes, httproute)
			}
		}

		configMap = homer.CreateConfigMapWithHTTPRoutes(homerConfig, dashboard.Name, dashboard.Namespace, filteredIngressList, filteredHTTPRoutes, &dashboard, dashboard.Spec.DomainFilters)
	} else {
		configMap = homer.CreateConfigMap(homerConfig, dashboard.Name, dashboard.Namespace, filteredIngressList, &dashboard)
	}

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
			if configMap, ok := resource.(*corev1.ConfigMap); ok {
				err = r.updateConfigMapWithRetry(ctx, configMap, dashboard.Name)
			} else {
				err = r.Update(ctx, resource)
			}
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

// updateConfigMapWithRetry updates a ConfigMap with exponential backoff retry on conflicts
func (r *DashboardReconciler) updateConfigMapWithRetry(ctx context.Context, configMap *corev1.ConfigMap, dashboardName string) error {
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
func (r *DashboardReconciler) shouldIncludeHTTPRouteForDashboard(ctx context.Context, httproute *gatewayv1.HTTPRoute, dashboard *homerv1alpha1.Dashboard) (bool, error) {
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
func (r *DashboardReconciler) matchesDomainFilters(hostnames []gatewayv1.Hostname, domainFilters []string) bool {
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

// shouldIncludeIngressForDashboard determines if an Ingress should be included
// based on the Dashboard's selectors and filters. If no selectors are specified, all Ingresses are included.
func (r *DashboardReconciler) shouldIncludeIngressForDashboard(ctx context.Context, ingress *networkingv1.Ingress, dashboard *homerv1alpha1.Dashboard) (bool, error) {
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
func (r *DashboardReconciler) matchesIngressDomainFilters(ingress *networkingv1.Ingress, domainFilters []string) bool {
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
