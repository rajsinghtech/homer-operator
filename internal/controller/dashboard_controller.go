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
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator/pkg/homer"
	"github.com/rajsinghtech/homer-operator/pkg/utils"
	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	dashboardFinalizer = "homer.rajsingh.info/finalizer"
	gatewayKind        = "Gateway"
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
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *DashboardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dashboard homerv1alpha1.Dashboard
	if err := r.Get(ctx, req.NamespacedName, &dashboard); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if shouldStop, err := r.handleFinalization(ctx, &dashboard); shouldStop {
		return ctrl.Result{}, err
	}

	filteredIngressList, err := r.getFilteredIngresses(ctx, &dashboard)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.validateDashboardConfig(&dashboard); err != nil {
		return ctrl.Result{}, err
	}

	resources, _, err := r.prepareResources(ctx, &dashboard, filteredIngressList)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createOrUpdateResources(ctx, resources, dashboard.Name); err != nil {
		return ctrl.Result{}, err
	}

	if !dashboard.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if err := r.updateStatus(ctx, &dashboard); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *DashboardReconciler) handleFinalization(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	if dashboard.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(dashboard, dashboardFinalizer) {
			controllerutil.AddFinalizer(dashboard, dashboardFinalizer)
			return true, r.Update(ctx, dashboard)
		}
	} else {
		if controllerutil.ContainsFinalizer(dashboard, dashboardFinalizer) {
			if err := r.cleanupResources(ctx, dashboard); err != nil {
				return true, err
			}
			controllerutil.RemoveFinalizer(dashboard, dashboardFinalizer)
			return true, r.Update(ctx, dashboard)
		}
		return true, nil
	}
	return false, nil
}

func (r *DashboardReconciler) getFilteredIngresses(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (networkingv1.IngressList, error) {
	clusterIngresses := &networkingv1.IngressList{}
	if err := r.List(ctx, clusterIngresses); err != nil {
		return networkingv1.IngressList{}, err
	}

	filteredIngresses := []networkingv1.Ingress{}
	for _, ingress := range clusterIngresses.Items {
		shouldInclude, err := r.shouldIncludeIngress(ctx, &ingress, dashboard)
		if err != nil {
			return networkingv1.IngressList{}, err
		}
		if shouldInclude {
			filteredIngresses = append(filteredIngresses, ingress)
		}
	}

	return networkingv1.IngressList{Items: filteredIngresses}, nil
}

func (r *DashboardReconciler) validateDashboardConfig(dashboard *homerv1alpha1.Dashboard) error {
	if err := homer.ValidateTheme(dashboard.Spec.HomerConfig.Theme); err != nil {
		return err
	}
	return homer.ValidateHomerConfig(&dashboard.Spec.HomerConfig)
}

func (r *DashboardReconciler) prepareResources(ctx context.Context, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList) ([]client.Object, *homer.HomerConfig, error) {
	deploymentConfig := r.buildDeploymentConfig(dashboard)
	deployment := homer.CreateDeployment(dashboard.Name, dashboard.Namespace, dashboard.Spec.Replicas, dashboard, deploymentConfig)
	service := homer.CreateService(dashboard.Name, dashboard.Namespace, dashboard)

	homerConfig, err := r.buildHomerConfig(ctx, dashboard)
	if err != nil {
		return nil, nil, err
	}

	configMap, err := r.createConfigMap(ctx, homerConfig, dashboard, filteredIngressList)
	if err != nil {
		return nil, nil, err
	}

	return []client.Object{&deployment, &service, &configMap}, homerConfig, nil
}

func (r *DashboardReconciler) buildDeploymentConfig(dashboard *homerv1alpha1.Dashboard) *homer.DeploymentConfig {
	pwaManifest := r.generatePWAManifest(dashboard)
	deploymentConfig := &homer.DeploymentConfig{
		PWAManifest: pwaManifest,
	}

	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.ConfigMapRef != nil {
		deploymentConfig.AssetsConfigMapName = dashboard.Spec.Assets.ConfigMapRef.Name
	}

	if dashboard.Spec.DNSPolicy != "" {
		deploymentConfig.DNSPolicy = dashboard.Spec.DNSPolicy
	}
	if dashboard.Spec.DNSConfig != "" {
		deploymentConfig.DNSConfig = dashboard.Spec.DNSConfig
	}

	if dashboard.Spec.Resources != nil {
		k8sResources := &corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{},
			Requests: corev1.ResourceList{},
		}

		if dashboard.Spec.Resources.Limits != nil {
			for name, quantity := range dashboard.Spec.Resources.Limits {
				k8sResources.Limits[corev1.ResourceName(name)] = quantity
			}
		}

		if dashboard.Spec.Resources.Requests != nil {
			for name, quantity := range dashboard.Spec.Resources.Requests {
				k8sResources.Requests[corev1.ResourceName(name)] = quantity
			}
		}

		deploymentConfig.Resources = k8sResources
	}

	return deploymentConfig
}

func (r *DashboardReconciler) buildHomerConfig(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (*homer.HomerConfig, error) {
	homerConfig := dashboard.Spec.HomerConfig.DeepCopy()

	if dashboard.Spec.Secrets != nil && dashboard.Spec.Secrets.APIKey != nil {
		secretRef := &homer.SecretKeyRef{
			Name:      dashboard.Spec.Secrets.APIKey.Name,
			Key:       dashboard.Spec.Secrets.APIKey.Key,
			Namespace: dashboard.Spec.Secrets.APIKey.Namespace,
		}

		for serviceIdx := range homerConfig.Services {
			for itemIdx := range homerConfig.Services[serviceIdx].Items {
				item := &homerConfig.Services[serviceIdx].Items[itemIdx]
				if err := homer.ResolveAPIKeyFromSecret(ctx, r.Client, item, secretRef, dashboard.Namespace); err != nil {
					return nil, err
				}
			}
		}
	}

	if dashboard.Spec.ConfigMap.Name != "" {
		externalHomerConfig, err := r.getExternalConfig(ctx, dashboard)
		if err != nil {
			return nil, err
		}
		homerConfig = externalHomerConfig
	}

	return homerConfig, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Dashboard{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *DashboardReconciler) shouldIncludeIngress(ctx context.Context, ingress *networkingv1.Ingress, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	log := log.FromContext(ctx)

	if match, err := validateLabelSelector(dashboard.Spec.IngressSelector, ingress.Labels, ingress.Name, "ingress", log); err != nil {
		return false, err
	} else if !match {
		return false, nil
	}

	if !validateIngressDomainFilters(ingress, dashboard.Spec.DomainFilters, log) {
		return false, nil
	}

	return true, nil
}

func (r *DashboardReconciler) shouldIncludeHTTPRoute(ctx context.Context, httproute *gatewayv1.HTTPRoute, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	log := log.FromContext(ctx)

	if match, err := validateLabelSelector(dashboard.Spec.HTTPRouteSelector, httproute.Labels, httproute.Name, "httproute", log); err != nil {
		return false, err
	} else if !match {
		return false, nil
	}

	if !validateHTTPRouteDomainFilters(httproute, dashboard.Spec.DomainFilters, log) {
		return false, nil
	}

	if dashboard.Spec.GatewaySelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.GatewaySelector)
		if err != nil {
			return false, err
		}

		for _, parentRef := range httproute.Spec.ParentRefs {
			if parentRef.Kind != nil && string(*parentRef.Kind) != gatewayKind {
				continue
			}

			namespace := httproute.Namespace
			if parentRef.Namespace != nil {
				namespace = string(*parentRef.Namespace)
			}

			gateway := &gatewayv1.Gateway{}
			if err := r.Get(ctx, client.ObjectKey{Name: string(parentRef.Name), Namespace: namespace}, gateway); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return false, err
			}

			if selector.Matches(labels.Set(gateway.Labels)) {
				return true, nil
			}
		}
		return false, nil
	}

	return true, nil
}

// createOrUpdateResources creates or updates all Kubernetes resources for the dashboard
func (r *DashboardReconciler) createOrUpdateResources(ctx context.Context, resources []client.Object, dashboardName string) error {
	log := log.FromContext(ctx)

	for _, resource := range resources {
		newResource := reflect.New(reflect.TypeOf(resource).Elem()).Interface().(client.Object)
		err := r.Get(ctx, client.ObjectKey{Namespace: resource.GetNamespace(), Name: resource.GetName()}, newResource)
		switch {
		case apierrors.IsNotFound(err):
			// Resource doesn't exist, create it
			err = r.Create(ctx, resource)
			if err != nil {
				log.Error(err, "unable to create resource", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
				return err
			}
			log.Info("Resource created", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
		case err != nil:
			// Other error occurred while fetching
			log.Error(err, "unable to fetch resource", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
			return err
		default:
			if configMap, ok := resource.(*corev1.ConfigMap); ok {
				err = utils.UpdateConfigMapWithRetry(ctx, r.Client, configMap, dashboardName)
			} else {
				err = r.Update(ctx, resource)
			}
			if err != nil {
				log.Error(err, "unable to update resource", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
				return err
			}
			log.Info("Resource updated", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
		}
	}
	return nil
}

func (r *DashboardReconciler) createConfigMap(ctx context.Context, homerConfig *homer.HomerConfig, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList) (corev1.ConfigMap, error) {
	if r.EnableGatewayAPI {
		clusterHTTPRoutes := &gatewayv1.HTTPRouteList{}

		if err := r.List(ctx, clusterHTTPRoutes); err != nil {
			return corev1.ConfigMap{}, err
		}

		filteredHTTPRoutes := []gatewayv1.HTTPRoute{}
		for _, httproute := range clusterHTTPRoutes.Items {
			shouldInclude, err := r.shouldIncludeHTTPRoute(ctx, &httproute, dashboard)
			if err != nil {
				return corev1.ConfigMap{}, err
			}
			if shouldInclude {
				filteredHTTPRoutes = append(filteredHTTPRoutes, httproute)
			}
		}

		return homer.CreateConfigMapWithHTTPRoutes(homerConfig, dashboard.Name, dashboard.Namespace, filteredIngressList, filteredHTTPRoutes, dashboard, dashboard.Spec.DomainFilters)
	}

	return homer.CreateConfigMap(homerConfig, dashboard.Name, dashboard.Namespace, filteredIngressList, dashboard)
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
func (r *DashboardReconciler) getExternalConfig(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (*homer.HomerConfig, error) {
	log := log.FromContext(ctx)

	if dashboard.Spec.ConfigMap.Name == "" {
		return nil, fmt.Errorf("external ConfigMap name is empty")
	}

	// Default key if not specified
	key := dashboard.Spec.ConfigMap.Key
	if key == "" {
		key = "config.yml"
	}

	// External ConfigMap is always in the same namespace as the Dashboard
	namespace := dashboard.Namespace

	// Retrieve the external ConfigMap
	externalConfigMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      dashboard.Spec.ConfigMap.Name,
		Namespace: namespace,
	}, externalConfigMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("external ConfigMap %s not found in namespace %s", dashboard.Spec.ConfigMap.Name, namespace)
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

// cleanupDashboardResources removes all resources created by this Dashboard
func (r *DashboardReconciler) cleanupResources(ctx context.Context, dashboard *homerv1alpha1.Dashboard) error {
	log := log.FromContext(ctx)
	dashboardName := dashboard.Name
	namespace := dashboard.Namespace

	// List of resource types to clean up
	resourcesToCleanup := []struct {
		name         string
		resourceType client.Object
	}{
		{"ConfigMap", &corev1.ConfigMap{}},
		{"Deployment", &appsv1.Deployment{}},
		{"Service", &corev1.Service{}},
	}

	// Clean up each resource type
	for _, resource := range resourcesToCleanup {
		resourceName := dashboardName + "-homer"
		if err := r.Get(ctx, client.ObjectKey{
			Name:      resourceName,
			Namespace: namespace,
		}, resource.resourceType); err != nil {
			if apierrors.IsNotFound(err) {
				// Resource doesn't exist, which is fine
				log.V(1).Info("Resource already deleted", "type", resource.name, "name", resourceName)
				continue
			}
			log.Error(err, "failed to get resource for cleanup", "type", resource.name, "name", resourceName)
			continue // Continue cleaning up other resources
		}

		// Check if this resource is owned by our Dashboard
		if isOwnedByDashboard(resource.resourceType, dashboard) {
			if err := r.Delete(ctx, resource.resourceType); err != nil {
				if !apierrors.IsNotFound(err) {
					log.Error(err, "failed to delete resource", "type", resource.name, "name", resourceName)
					return fmt.Errorf("failed to delete %s %s: %w", resource.name, resourceName, err)
				}
			} else {
				log.Info("Successfully deleted resource", "type", resource.name, "name", resourceName)
			}
		} else {
			log.V(1).Info("Skipping resource not owned by this Dashboard", "type", resource.name, "name", resourceName)
		}
	}

	// Clean up any custom assets ConfigMap if it exists
	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.ConfigMapRef != nil {
		assetsConfigMapName := dashboard.Spec.Assets.ConfigMapRef.Name
		assetsConfigMap := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{
			Name:      assetsConfigMapName,
			Namespace: namespace,
		}, assetsConfigMap); err == nil {
			if isOwnedByDashboard(assetsConfigMap, dashboard) {
				if err := r.Delete(ctx, assetsConfigMap); err != nil && !apierrors.IsNotFound(err) {
					log.Error(err, "failed to delete assets ConfigMap", "name", assetsConfigMapName)
					return fmt.Errorf("failed to delete assets ConfigMap %s: %w", assetsConfigMapName, err)
				} else {
					log.Info("Successfully deleted assets ConfigMap", "name", assetsConfigMapName)
				}
			}
		}
	}

	log.Info("Successfully cleaned up all Dashboard resources", "dashboard", dashboardName)
	return nil
}

// isOwnedByDashboard checks if a resource is owned by the given Dashboard
func isOwnedByDashboard(resource client.Object, dashboard *homerv1alpha1.Dashboard) bool {
	for _, ownerRef := range resource.GetOwnerReferences() {
		if ownerRef.Kind == "Dashboard" &&
			ownerRef.APIVersion == "homer.rajsingh.info/v1alpha1" &&
			ownerRef.Name == dashboard.Name &&
			ownerRef.UID == dashboard.UID {
			return true
		}
	}
	return false
}

// checkLabelSelector checks if the resource labels match the given label selector
func validateLabelSelector(selector *metav1.LabelSelector, resourceLabels map[string]string, resourceName, resourceType string, log logr.Logger) (bool, error) {
	if selector == nil {
		return true, nil // No selector means include all
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return false, err
	}

	if !labelSelector.Matches(labels.Set(resourceLabels)) {
		log.V(1).Info(fmt.Sprintf("%s excluded by label selector", resourceType), resourceType, resourceName)
		return false, nil
	}

	return true, nil
}

// checkDomainFilters checks if domain filters match for the given resource
func validateIngressDomainFilters(ingress *networkingv1.Ingress, domainFilters []string, log logr.Logger) bool {
	if len(domainFilters) == 0 {
		return true // No filters means include all
	}

	if !utils.MatchesIngressDomainFilters(ingress, domainFilters) {
		log.V(1).Info("Ingress excluded by domain filters", "ingress", ingress.Name)
		return false
	}

	return true
}

func validateHTTPRouteDomainFilters(httproute *gatewayv1.HTTPRoute, domainFilters []string, log logr.Logger) bool {
	if len(domainFilters) == 0 {
		return true // No filters means include all
	}

	if !utils.MatchesHTTPRouteDomainFilters(httproute.Spec.Hostnames, domainFilters) {
		log.V(1).Info("HTTPRoute excluded by domain filters", "httproute", httproute.Name, "hostnames", httproute.Spec.Hostnames, "domainFilters", domainFilters)
		return false
	}
	log.V(2).Info("HTTPRoute included by domain filters", "httproute", httproute.Name, "hostnames", httproute.Spec.Hostnames, "domainFilters", domainFilters)

	return true
}

// updateDashboardStatusComplete updates both deployment and service discovery status in one call
func (r *DashboardReconciler) updateStatus(ctx context.Context, dashboard *homerv1alpha1.Dashboard) error {
	log := log.FromContext(ctx)

	// Check if Dashboard is being deleted
	if !dashboard.DeletionTimestamp.IsZero() {
		log.V(2).Info("Skipping status update for Dashboard being deleted")
		return nil
	}

	// Get the current deployment to check if it's available
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      dashboard.Name + "-homer",
		Namespace: dashboard.Namespace,
	}, deployment)

	// Simplified status logic: Ready = deployment exists and Available condition is true
	ready := false
	availableReplicas := int32(0)

	if err == nil {
		// Deployment exists, check if it's available
		availableReplicas = deployment.Status.AvailableReplicas

		// Check for Available condition (standard Kubernetes pattern)
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}

		log.V(2).Info("Deployment status check",
			"deploymentName", deployment.Name,
			"availableReplicas", availableReplicas,
			"ready", ready)
	} else if apierrors.IsNotFound(err) {
		// Deployment doesn't exist yet - not ready
		log.V(2).Info("Deployment not found, status not ready")
	} else {
		// Error getting deployment - not ready
		log.V(1).Info("Error getting deployment for status check", "error", err)
	}

	// Update status using patch to avoid conflicts
	patch := client.MergeFrom(dashboard.DeepCopy())
	dashboard.Status.Ready = ready
	dashboard.Status.AvailableReplicas = availableReplicas

	if err := r.Status().Patch(ctx, dashboard, patch); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("Dashboard was deleted during status update")
			return nil // Don't return error if Dashboard was deleted
		}
		log.V(1).Info("Failed to update Dashboard status", "error", err)
		return err
	}

	log.V(2).Info("Status updated successfully",
		"ready", dashboard.Status.Ready,
		"availableReplicas", dashboard.Status.AvailableReplicas)

	return nil
}
