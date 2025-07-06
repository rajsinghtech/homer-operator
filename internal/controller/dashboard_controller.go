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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator/pkg/homer"
	yaml "gopkg.in/yaml.v2"
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

// Reconcile manages Dashboard resources and their associated Homer deployments.
func (r *DashboardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var dashboard homerv1alpha1.Dashboard
	if err := r.Get(ctx, req.NamespacedName, &dashboard); err != nil {
		if apierrors.IsNotFound(err) {
			// Dashboard was deleted, this is normal during deletion
			log.V(1).Info("Dashboard not found - likely deleted", "dashboard", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to fetch Dashboard", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}
	// List ingresses with performance optimization - only get ones with rules
	clusterIngresses := &networkingv1.IngressList{}
	if err := r.List(ctx, clusterIngresses, client.HasLabels([]string{})); err != nil {
		log.Error(err, "failed to list Ingresses", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Filter Ingresses based on dashboard selectors
	filteredIngresses := []networkingv1.Ingress{}
	for _, ingress := range clusterIngresses.Items {
		shouldInclude, err := r.shouldIncludeIngressForDashboard(ctx, &ingress, &dashboard)
		if err != nil {
			log.Error(err, "failed to filter Ingress", "dashboard", dashboard.Name, "ingress", ingress.Name)
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

	// Generate PWA manifest if enabled
	pwaManifest := r.generatePWAManifest(&dashboard)

	// Create deployment config
	deploymentConfig := &homer.DeploymentConfig{
		PWAManifest: pwaManifest,
	}

	// Check if custom assets are configured
	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.ConfigMapRef != nil {
		deploymentConfig.AssetsConfigMapName = dashboard.Spec.Assets.ConfigMapRef.Name
	}

	// Add DNS configuration if provided
	if dashboard.Spec.DNSPolicy != "" {
		deploymentConfig.DNSPolicy = dashboard.Spec.DNSPolicy
	}

	deployment = homer.CreateDeployment(dashboard.Name, dashboard.Namespace, dashboard.Spec.Replicas, &dashboard, deploymentConfig)

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

	// Check if external ConfigMap is specified
	if dashboard.Spec.ConfigMap.Name != "" {
		// Use external ConfigMap instead of generating one
		externalHomerConfig, err := r.getExternalHomerConfig(ctx, &dashboard)
		if err != nil {
			log.Error(err, "failed to retrieve external ConfigMap", "configMap", dashboard.Spec.ConfigMap.Name)
			return ctrl.Result{}, err
		}
		homerConfig = externalHomerConfig
	}

	configMap, err := r.createConfigMapForDashboard(ctx, req, homerConfig, &dashboard, filteredIngressList)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create or update all resources
	resources := []client.Object{&deployment, &service, &configMap}
	if err := r.createOrUpdateResources(ctx, resources, dashboard.Name); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Dashboard{}).
		Complete(r)
}

// updateConfigMapWithRetry updates a ConfigMap with retry on conflicts
func (r *DashboardReconciler) updateConfigMapWithRetry(ctx context.Context, configMap *corev1.ConfigMap, dashboardName string) error {
	log := log.FromContext(ctx)

	backoff := wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Cap:      5 * time.Second,
	}

	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		latest := &corev1.ConfigMap{}
		key := client.ObjectKeyFromObject(configMap)
		if err := r.Get(ctx, key, latest); err != nil {
			if apierrors.IsNotFound(err) {
				return false, err
			}
			return false, nil
		}

		latest.Data = configMap.Data
		latest.BinaryData = configMap.BinaryData

		if err := r.Update(ctx, latest); err != nil {
			if apierrors.IsConflict(err) {
				log.V(1).Info("ConfigMap update conflict, retrying", "configmap", dashboardName)
				return false, nil
			}
			return false, err
		}

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
			const gatewayKind = "Gateway"
			kind := gatewayKind
			if parentRef.Kind != nil {
				kind = string(*parentRef.Kind)
			}

			// Only check Gateway resources
			if kind != gatewayKind {
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

// createOrUpdateResources creates or updates all Kubernetes resources for the dashboard
func (r *DashboardReconciler) createOrUpdateResources(ctx context.Context, resources []client.Object, dashboardName string) error {
	log := log.FromContext(ctx)

	for _, resource := range resources {
		newResource := reflect.New(reflect.TypeOf(resource).Elem()).Interface().(client.Object)
		err := r.Get(ctx, client.ObjectKey{Namespace: resource.GetNamespace(), Name: resource.GetName()}, newResource)
		switch {
		case err != nil:
			err = r.Create(ctx, resource)
			if err != nil {
				log.Error(err, "unable to create resource", "resource", resource)
				return err
			}
			log.Info("Resource created", "resource", resource)
		case client.IgnoreNotFound(err) != nil:
			log.Error(err, "unable to fetch resource", "resource", resource)
			return err
		default:
			if configMap, ok := resource.(*corev1.ConfigMap); ok {
				err = r.updateConfigMapWithRetry(ctx, configMap, dashboardName)
			} else {
				err = r.Update(ctx, resource)
			}
			if err != nil {
				log.Error(err, "unable to update resource", "resource", resource)
				return err
			}
			log.Info("Resource updated", "resource", resource)
		}
	}
	return nil
}

// createConfigMapForDashboard creates the appropriate ConfigMap based on Gateway API enablement
func (r *DashboardReconciler) createConfigMapForDashboard(ctx context.Context, req ctrl.Request, homerConfig *homer.HomerConfig, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList) (corev1.ConfigMap, error) {
	log := log.FromContext(ctx)

	if r.EnableGatewayAPI {
		clusterHTTPRoutes := &gatewayv1.HTTPRouteList{}
		if err := r.List(ctx, clusterHTTPRoutes, client.HasLabels([]string{})); err != nil {
			log.Error(err, "unable to list HTTPRoutes", "dashboard", req.NamespacedName)
			return corev1.ConfigMap{}, err
		}

		// Filter HTTPRoutes based on dashboard selectors
		filteredHTTPRoutes := []gatewayv1.HTTPRoute{}
		for _, httproute := range clusterHTTPRoutes.Items {
			shouldInclude, err := r.shouldIncludeHTTPRouteForDashboard(ctx, &httproute, dashboard)
			if err != nil {
				log.Error(err, "failed to filter HTTPRoute", "dashboard", dashboard.Name, "httproute", httproute.Name)
				return corev1.ConfigMap{}, err
			}
			if shouldInclude {
				filteredHTTPRoutes = append(filteredHTTPRoutes, httproute)
			}
		}

		return homer.CreateConfigMapWithHTTPRoutes(homerConfig, dashboard.Name, dashboard.Namespace, filteredIngressList, filteredHTTPRoutes, dashboard, dashboard.Spec.DomainFilters), nil
	}

	return homer.CreateConfigMap(homerConfig, dashboard.Name, dashboard.Namespace, filteredIngressList, dashboard), nil
}

// generatePWAManifest generates PWA manifest if enabled, returns empty string if disabled
func (r *DashboardReconciler) generatePWAManifest(dashboard *homerv1alpha1.Dashboard) string {
	if dashboard.Spec.Assets == nil || dashboard.Spec.Assets.PWA == nil || !dashboard.Spec.Assets.PWA.Enabled {
		return ""
	}

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
	return homer.GeneratePWAManifest(name, shortName, description, themeColor, backgroundColor, display, startURL, icons)
}

// getExternalHomerConfig retrieves Homer configuration from an external ConfigMap
func (r *DashboardReconciler) getExternalHomerConfig(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (*homer.HomerConfig, error) {
	log := log.FromContext(ctx)

	if dashboard.Spec.ConfigMap.Name == "" {
		return nil, errors.New("external ConfigMap name is empty")
	}

	// Default key if not specified
	key := dashboard.Spec.ConfigMap.Key
	if key == "" {
		key = "config.yml"
	}

	// Retrieve the external ConfigMap
	externalConfigMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      dashboard.Spec.ConfigMap.Name,
		Namespace: dashboard.Namespace,
	}, externalConfigMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("external ConfigMap %s not found in namespace %s", dashboard.Spec.ConfigMap.Name, dashboard.Namespace)
		}
		return nil, fmt.Errorf("failed to retrieve external ConfigMap %s: %w", dashboard.Spec.ConfigMap.Name, err)
	}

	// Get the configuration data from the specified key
	configData, exists := externalConfigMap.Data[key]
	if !exists {
		return nil, fmt.Errorf("key %s not found in external ConfigMap %s", key, dashboard.Spec.ConfigMap.Name)
	}

	// Parse the YAML configuration
	var homerConfig homer.HomerConfig
	if err := yaml.Unmarshal([]byte(configData), &homerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML configuration from external ConfigMap %s: %w", dashboard.Spec.ConfigMap.Name, err)
	}

	log.Info("Successfully loaded external Homer configuration", "configMap", dashboard.Spec.ConfigMap.Name, "key", key)
	return &homerConfig, nil
}
