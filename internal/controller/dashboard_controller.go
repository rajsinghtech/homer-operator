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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"path"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator/pkg/homer"
	"github.com/rajsinghtech/homer-operator/pkg/utils"
	yaml "gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// DashboardReconciler reconciles a Dashboard object
type DashboardReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	EnableGatewayAPI bool
	HomerImage       string
	ConfigSyncImage  string
	// ClusterManager is an optional injected manager used by direct callers and
	// tests. Production reconciliation keeps managers isolated per Dashboard in
	// clusterManagers so same-named remote clusters cannot share state.
	ClusterManager   *ClusterManager
	clusterManagers  map[client.ObjectKey]*ClusterManager
	clusterManagerMu sync.Mutex
}

const dashboardRelativePath = "../"

// discoveredClusterResources contains the results from the discovery pass used
// to build the dashboard resources. Status must use these results instead of
// issuing a second set of remote-cluster list requests.
type discoveredClusterResources struct {
	ingresses  map[string][]networkingv1.Ingress
	httpRoutes map[string][]gatewayv1.HTTPRoute
	services   map[string][]corev1.Service
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
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
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

	clusterManager := r.clusterManagerForDashboard(&dashboard)

	// Load and validate the effective configuration before doing any remote
	// discovery. In external-config mode the inline HomerConfig is only a
	// stale API default and must not control validation or PWA defaults.
	effectiveHomerConfig, err := r.buildHomerConfig(ctx, &dashboard)
	if err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}
	if err := r.validateDashboardConfig(effectiveHomerConfig); err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}

	// Update cluster connections based on dashboard configuration
	if err := clusterManager.UpdateClusters(ctx, &dashboard); err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}

	// Discover resources from all clusters
	filteredIngressList, discoveredIngresses, err := r.getMultiClusterFilteredIngresses(ctx, &dashboard)
	if err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}

	filteredServices, discoveredServices, err := r.getMultiClusterFilteredServices(ctx, &dashboard)
	if err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}

	resources, _, discoveredHTTPRoutes, err := r.prepareResources(ctx, &dashboard, filteredIngressList, filteredServices, effectiveHomerConfig)
	if err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}
	if err := r.cleanupStaleAssetMirrors(ctx, &dashboard); err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}

	// Check if resources need updating to avoid unnecessary API calls
	if !r.resourcesNeedUpdate(ctx, resources) {
		log := log.FromContext(ctx)
		log.V(1).Info("Resources are up to date, skipping update")
		// Still update status even when resources don't need updating
		if err := r.updateStatus(ctx, &dashboard, &discoveredClusterResources{
			ingresses: discoveredIngresses, httpRoutes: discoveredHTTPRoutes, services: discoveredServices,
		}); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	if err := r.createOrUpdateResources(ctx, resources, dashboard.Name); err != nil {
		return r.reconcileFailure(ctx, &dashboard, err)
	}

	if !dashboard.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if err := r.updateStatus(ctx, &dashboard, &discoveredClusterResources{
		ingresses: discoveredIngresses, httpRoutes: discoveredHTTPRoutes, services: discoveredServices,
	}); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// reconcileFailure records a negative Ready condition before returning the
// original error. Without this update, a Dashboard that was previously ready
// can continue to advertise Ready=true while its new configuration is
// invalid, unreadable, or otherwise failed to reconcile.
func (r *DashboardReconciler) reconcileFailure(ctx context.Context, dashboard *homerv1alpha1.Dashboard, reconcileErr error) (ctrl.Result, error) {
	if dashboard.DeletionTimestamp.IsZero() {
		if err := r.markReconcileFailure(ctx, dashboard, reconcileErr); err != nil {
			log.FromContext(ctx).Error(err, "Failed to record Dashboard reconcile failure", "dashboard", dashboard.Name)
		}
	}
	return ctrl.Result{}, reconcileErr
}

func (r *DashboardReconciler) markReconcileFailure(ctx context.Context, dashboard *homerv1alpha1.Dashboard, reconcileErr error) error {
	previousStatus := dashboard.Status.DeepCopy()
	dashboard.Status.Ready = false
	dashboard.Status.ObservedGeneration = dashboard.Generation
	apiMeta.SetStatusCondition(&dashboard.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: dashboard.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ReconcileError",
		Message:            truncateConditionMessage(reconcileErr),
	})
	if equality.Semantic.DeepEqual(*previousStatus, dashboard.Status) {
		return nil
	}
	if err := r.Status().Update(ctx, dashboard); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func truncateConditionMessage(err error) string {
	if err == nil {
		return ""
	}
	const maxConditionMessageLength = 1024
	message := err.Error()
	if len(message) > maxConditionMessageLength {
		return message[:maxConditionMessageLength-3] + "..."
	}
	return message
}

// resourcesNeedUpdate checks if resources actually need updating to avoid unnecessary API calls
func (r *DashboardReconciler) resourcesNeedUpdate(ctx context.Context, resources []client.Object) bool {
	log := log.FromContext(ctx)

	for _, resource := range resources {
		existing := reflect.New(reflect.TypeOf(resource).Elem()).Interface().(client.Object)
		err := r.Get(ctx, client.ObjectKey{
			Namespace: resource.GetNamespace(),
			Name:      resource.GetName(),
		}, existing)

		if apierrors.IsNotFound(err) {
			log.V(1).Info("Resource not found, needs creation", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
			return true
		}

		if err != nil {
			log.V(1).Info("Error getting resource, assuming update needed", "error", err)
			return true
		}

		if r.resourceDiffers(ctx, resource, existing) {
			log.V(1).Info("Resource differs, needs update", "type", resource.GetObjectKind().GroupVersionKind().Kind, "name", resource.GetName())
			return true
		}
	}

	return false
}

// resourceDiffers compares only fields owned by this controller. Server
// metadata and allocated fields must not make an otherwise equal resource
// appear out of date.
func (r *DashboardReconciler) resourceDiffers(ctx context.Context, desired, existing client.Object) bool {
	switch desired := desired.(type) {
	case *corev1.ConfigMap:
		existingConfigMap, ok := existing.(*corev1.ConfigMap)
		return !ok || !reflect.DeepEqual(desired.Data, existingConfigMap.Data) ||
			!reflect.DeepEqual(desired.BinaryData, existingConfigMap.BinaryData)
	case *appsv1.Deployment:
		existingDeployment, ok := existing.(*appsv1.Deployment)
		return !ok || r.deploymentSpecsDiffer(ctx, desired, existingDeployment)
	case *corev1.Service:
		existingService, ok := existing.(*corev1.Service)
		return !ok || serviceSpecsDiffer(desired, existingService)
	default:
		// A new managed resource type must be explicitly added here so it
		// cannot silently skip reconciliation.
		return true
	}
}

// serviceSpecsDiffer compares the Service fields owned by the operator. The
// API server may default Protocol and allocate NodePort, neither of which
// should cause perpetual updates when omitted from the desired Service.
func serviceSpecsDiffer(desired, existing *corev1.Service) bool {
	if !reflect.DeepEqual(desired.Spec.Selector, existing.Spec.Selector) {
		return true
	}

	if len(desired.Spec.Ports) != len(existing.Spec.Ports) {
		return true
	}

	matched := make([]bool, len(existing.Spec.Ports))
	for _, desiredPort := range desired.Spec.Ports {
		found := false
		for i, existingPort := range existing.Spec.Ports {
			if !matched[i] && servicePortsMatch(desiredPort, existingPort) {
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}

	return false
}

// deploymentSpecsDiffer compares deployment specs semantically, ignoring metadata changes
func (r *DashboardReconciler) deploymentSpecsDiffer(ctx context.Context, desired, existing *appsv1.Deployment) bool {
	log := log.FromContext(ctx)
	// Compare only meaningful fields that should trigger updates

	// Check replicas
	desiredReplicas := desired.Spec.Replicas
	existingReplicas := existing.Spec.Replicas
	if (desiredReplicas == nil) != (existingReplicas == nil) {
		return true
	}
	if desiredReplicas != nil && *desiredReplicas != *existingReplicas {
		log.V(1).Info("Replicas differ", "desired", *desiredReplicas, "existing", *existingReplicas)
		return true
	}
	if !desiredLabelsMatch(desired.Spec.Selector.MatchLabels, existing.Spec.Selector.MatchLabels) ||
		!desiredLabelsMatch(desired.Spec.Template.Labels, existing.Spec.Template.Labels) {
		log.V(1).Info("Deployment selector or pod template labels differ")
		return true
	}

	desiredPodSpec := normalizeManagedPodSpec(desired.Spec.Template.Spec)
	existingPodSpec := normalizeManagedPodSpec(existing.Spec.Template.Spec)
	if !reflect.DeepEqual(desiredPodSpec, existingPodSpec) {
		log.V(1).Info("Managed pod or container fields differ")
		return true
	}

	log.V(1).Info("No differences found, deployment specs are equal")
	return false
}

func desiredLabelsMatch(desired, existing map[string]string) bool {
	for key, value := range desired {
		if existing[key] != value {
			return false
		}
	}
	return true
}

// normalizeManagedPodSpec projects the Pod fields generated by this
// controller and normalizes API-server defaults before comparison.
func normalizeManagedPodSpec(spec corev1.PodSpec) corev1.PodSpec {
	managed := corev1.PodSpec{
		InitContainers:  normalizeContainers(spec.InitContainers),
		Containers:      normalizeContainers(spec.Containers),
		Volumes:         normalizeVolumes(spec.Volumes),
		DNSPolicy:       spec.DNSPolicy,
		DNSConfig:       spec.DNSConfig,
		SecurityContext: spec.SecurityContext,
	}
	if managed.DNSPolicy == "" {
		managed.DNSPolicy = corev1.DNSClusterFirst
	}
	if len(managed.InitContainers) == 0 {
		managed.InitContainers = nil
	}
	if len(managed.Volumes) == 0 {
		managed.Volumes = nil
	}
	return managed
}

func normalizeContainers(containers []corev1.Container) []corev1.Container {
	if len(containers) == 0 {
		return nil
	}
	normalized := make([]corev1.Container, len(containers))
	for i := range containers {
		normalized[i] = *containers[i].DeepCopy()
		container := &normalized[i]
		slices.SortFunc(container.Env, func(a, b corev1.EnvVar) int {
			return strings.Compare(a.Name, b.Name)
		})
		if container.ImagePullPolicy == "" {
			container.ImagePullPolicy = defaultImagePullPolicy(container.Image)
		}
		if container.TerminationMessagePath == "" {
			container.TerminationMessagePath = corev1.TerminationMessagePathDefault
		}
		if container.TerminationMessagePolicy == "" {
			container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
		}
		for j := range container.Ports {
			if container.Ports[j].Protocol == "" {
				container.Ports[j].Protocol = corev1.ProtocolTCP
			}
		}
		normalizeProbe(container.LivenessProbe)
		normalizeProbe(container.ReadinessProbe)
		normalizeProbe(container.StartupProbe)
	}
	return normalized
}

func normalizeVolumes(volumes []corev1.Volume) []corev1.Volume {
	if len(volumes) == 0 {
		return nil
	}
	normalized := make([]corev1.Volume, len(volumes))
	for i := range volumes {
		normalized[i] = *volumes[i].DeepCopy()
		if source := normalized[i].ConfigMap; source != nil && source.DefaultMode == nil {
			mode := corev1.ConfigMapVolumeSourceDefaultMode
			source.DefaultMode = &mode
		}
		if source := normalized[i].Secret; source != nil && source.DefaultMode == nil {
			mode := corev1.SecretVolumeSourceDefaultMode
			source.DefaultMode = &mode
		}
	}
	return normalized
}

func normalizeProbe(probe *corev1.Probe) {
	if probe == nil {
		return
	}
	if probe.TimeoutSeconds == 0 {
		probe.TimeoutSeconds = 1
	}
	if probe.PeriodSeconds == 0 {
		probe.PeriodSeconds = 10
	}
	if probe.SuccessThreshold == 0 {
		probe.SuccessThreshold = 1
	}
	if probe.FailureThreshold == 0 {
		probe.FailureThreshold = 3
	}
}

func defaultImagePullPolicy(image string) corev1.PullPolicy {
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon <= lastSlash || strings.HasSuffix(image, ":latest") {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}

// envVarsEqual compares environment variables ignoring order
func (r *DashboardReconciler) envVarsEqual(desired, existing []corev1.EnvVar) bool {
	if len(desired) != len(existing) {
		return false
	}

	desiredMap := make(map[string]corev1.EnvVar)
	for _, env := range desired {
		desiredMap[env.Name] = env
	}

	for _, env := range existing {
		if desiredEnv, exists := desiredMap[env.Name]; !exists || !reflect.DeepEqual(desiredEnv, env) {
			return false
		}
	}

	return true
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
			r.forgetClusterManager(dashboard)
			controllerutil.RemoveFinalizer(dashboard, dashboardFinalizer)
			return true, r.Update(ctx, dashboard)
		}
		return true, nil
	}
	return false, nil
}

// clusterManagerForDashboard returns the manager associated with one
// Dashboard. A manually injected ClusterManager retains its historical
// behavior; managers created by the controller are keyed by namespaced
// Dashboard identity, not by remote cluster name alone.
func (r *DashboardReconciler) clusterManagerForDashboard(dashboard *homerv1alpha1.Dashboard) *ClusterManager {
	if r.ClusterManager != nil {
		return r.ClusterManager
	}

	key := client.ObjectKeyFromObject(dashboard)
	r.clusterManagerMu.Lock()
	defer r.clusterManagerMu.Unlock()
	if r.clusterManagers == nil {
		r.clusterManagers = make(map[client.ObjectKey]*ClusterManager)
	}
	if manager, ok := r.clusterManagers[key]; ok {
		return manager
	}
	manager := NewClusterManager(r.Client, r.Scheme)
	r.clusterManagers[key] = manager
	return manager
}

func (r *DashboardReconciler) forgetClusterManager(dashboard *homerv1alpha1.Dashboard) {
	if r.ClusterManager != nil {
		return
	}
	r.clusterManagerMu.Lock()
	defer r.clusterManagerMu.Unlock()
	delete(r.clusterManagers, client.ObjectKeyFromObject(dashboard))
}

func (r *DashboardReconciler) getFilteredIngresses(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (networkingv1.IngressList, error) {
	clusterIngresses := &networkingv1.IngressList{}
	if err := r.List(ctx, clusterIngresses); err != nil {
		return networkingv1.IngressList{}, err
	}

	filteredIngresses := []networkingv1.Ingress{}
	for i := range clusterIngresses.Items {
		shouldInclude, err := r.shouldIncludeIngress(ctx, &clusterIngresses.Items[i], dashboard)
		if err != nil {
			return networkingv1.IngressList{}, err
		}
		if shouldInclude {
			filteredIngresses = append(filteredIngresses, clusterIngresses.Items[i])
		}
	}

	return networkingv1.IngressList{Items: filteredIngresses}, nil
}

// getMultiClusterFilteredIngresses discovers and filters Ingresses from all configured clusters
func (r *DashboardReconciler) getMultiClusterFilteredIngresses(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (networkingv1.IngressList, map[string][]networkingv1.Ingress, error) {
	allIngresses := []networkingv1.Ingress{}
	var discoveredIngresses map[string][]networkingv1.Ingress

	// Use the Dashboard-scoped ClusterManager when remote clusters are configured.
	if manager := r.clusterManagerForDashboard(dashboard); manager != nil && len(dashboard.Spec.RemoteClusters) > 0 {
		var err error
		discoveredIngresses, err = manager.DiscoverIngresses(ctx, dashboard)
		if err != nil {
			log := log.FromContext(ctx)
			log.Error(err, "Failed to discover ingresses from clusters")
			// Continue with partial results
		}

		// ClusterManager has already applied each cluster's own selectors and
		// domain filters. Dashboard-level filters are local-cluster filters and
		// must not be applied again to remote resources here.
		for _, clusterName := range sortedClusterNames(discoveredIngresses) {
			ingresses := discoveredIngresses[clusterName]
			log := log.FromContext(ctx)
			log.V(1).Info("Discovered ingresses from cluster", "cluster", clusterName, "count", len(ingresses))

			allIngresses = append(allIngresses, ingresses...)
		}
	} else {
		// Fall back to single-cluster discovery
		filtered, err := r.getFilteredIngresses(ctx, dashboard)
		return filtered, nil, err
	}

	return networkingv1.IngressList{Items: allIngresses}, discoveredIngresses, nil
}

func (r *DashboardReconciler) getFilteredServices(ctx context.Context, dashboard *homerv1alpha1.Dashboard) ([]corev1.Service, error) {
	if dashboard.Spec.ServiceSelector == nil {
		return nil, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.ServiceSelector)
	if err != nil {
		return nil, err
	}

	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
		return nil, err
	}

	return serviceList.Items, nil
}

func (r *DashboardReconciler) getMultiClusterFilteredServices(ctx context.Context, dashboard *homerv1alpha1.Dashboard) ([]corev1.Service, map[string][]corev1.Service, error) {
	if manager := r.clusterManagerForDashboard(dashboard); manager != nil && len(dashboard.Spec.RemoteClusters) > 0 {
		clusterServices, err := manager.DiscoverServices(ctx, dashboard)
		if err != nil {
			log := log.FromContext(ctx)
			log.Error(err, "Failed to discover Services from clusters")
		}

		var allServices []corev1.Service
		for _, clusterName := range sortedClusterNames(clusterServices) {
			allServices = append(allServices, clusterServices[clusterName]...)
		}
		return allServices, clusterServices, nil
	}

	filtered, err := r.getFilteredServices(ctx, dashboard)
	return filtered, nil, err
}

func (r *DashboardReconciler) validateDashboardConfig(config *homer.HomerConfig) error {
	if err := homer.ValidateTheme(config.Theme); err != nil {
		return err
	}
	return homer.ValidateHomerConfig(config)
}

func (r *DashboardReconciler) prepareResources(ctx context.Context, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList, filteredServices []corev1.Service, homerConfig *homer.HomerConfig) ([]client.Object, *homer.HomerConfig, map[string][]gatewayv1.HTTPRoute, error) {
	deploymentConfig := r.buildDeploymentConfig(dashboard, homerConfig)
	deployment := homer.CreateDeployment(dashboard.Name, dashboard.Namespace, dashboard.Spec.Replicas, dashboard, deploymentConfig)
	service := homer.CreateService(dashboard.Name, dashboard.Namespace, dashboard)

	configMap, discoveredHTTPRoutes, err := r.createConfigMap(ctx, homerConfig, dashboard, filteredIngressList, filteredServices)
	if err != nil {
		return nil, nil, nil, err
	}

	resources := []client.Object{&deployment, &service, &configMap}
	if assetMirror, err := r.buildAssetMirror(ctx, dashboard); err != nil {
		return nil, nil, nil, err
	} else if assetMirror != nil {
		resources = append(resources, assetMirror)
	}

	return resources, homerConfig, discoveredHTTPRoutes, nil
}

func (r *DashboardReconciler) buildDeploymentConfig(dashboard *homerv1alpha1.Dashboard, effectiveConfig *homer.HomerConfig) *homer.DeploymentConfig {
	pwaManifest := r.generatePWAManifest(dashboard, effectiveConfig)
	deploymentConfig := &homer.DeploymentConfig{
		PWAManifest:     pwaManifest,
		HomerImage:      r.HomerImage,
		ConfigSyncImage: r.ConfigSyncImage,
	}

	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.ConfigMapRef != nil && dashboard.Spec.Assets.ConfigMapRef.Name != "" {
		ref := dashboard.Spec.Assets.ConfigMapRef
		deploymentConfig.AssetsConfigMapName = ref.Name
		if ref.Namespace != "" && ref.Namespace != dashboard.Namespace {
			deploymentConfig.AssetsConfigMapName = assetMirrorName(ref.Name, ref.Namespace, dashboard.Namespace, dashboard.Name)
		}
	}
	deploymentConfig.PageConfigKeys = make([]string, 0, len(dashboard.Spec.Pages))
	for pageName := range dashboard.Spec.Pages {
		deploymentConfig.PageConfigKeys = append(deploymentConfig.PageConfigKeys, pageName+".yml")
	}
	slices.Sort(deploymentConfig.PageConfigKeys)
	deploymentConfig.IconAliases = iconAliases(dashboard)

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
	if err := validateSmartCardSecretReferences(dashboard); err != nil {
		return nil, err
	}

	homerConfig := dashboard.Spec.HomerConfig.DeepCopy()
	if dashboard.Spec.ConfigMap.Name != "" {
		externalHomerConfig, err := r.getExternalConfig(ctx, dashboard)
		if err != nil {
			return nil, err
		}
		homerConfig = externalHomerConfig
	}

	if dashboard.Spec.Secrets != nil {
		// Handle APIKey secrets
		if dashboard.Spec.Secrets.APIKey != nil {
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

		// Handle Token secrets (Bearer tokens)
		if dashboard.Spec.Secrets.Token != nil {
			secretRef := &homer.SecretKeyRef{
				Name:      dashboard.Spec.Secrets.Token.Name,
				Key:       dashboard.Spec.Secrets.Token.Key,
				Namespace: dashboard.Spec.Secrets.Token.Namespace,
			}

			for serviceIdx := range homerConfig.Services {
				for itemIdx := range homerConfig.Services[serviceIdx].Items {
					item := &homerConfig.Services[serviceIdx].Items[itemIdx]
					if err := homer.ResolveTokenFromSecret(ctx, r.Client, item, secretRef, dashboard.Namespace); err != nil {
						return nil, err
					}
				}
			}
		}

		// Handle Username secrets
		if dashboard.Spec.Secrets.Username != nil {
			secretRef := &homer.SecretKeyRef{
				Name:      dashboard.Spec.Secrets.Username.Name,
				Key:       dashboard.Spec.Secrets.Username.Key,
				Namespace: dashboard.Spec.Secrets.Username.Namespace,
			}

			for serviceIdx := range homerConfig.Services {
				for itemIdx := range homerConfig.Services[serviceIdx].Items {
					item := &homerConfig.Services[serviceIdx].Items[itemIdx]
					if err := homer.ResolveUsernameFromSecret(ctx, r.Client, item, secretRef, dashboard.Namespace); err != nil {
						return nil, err
					}
				}
			}
		}

		// Handle Password secrets
		if dashboard.Spec.Secrets.Password != nil {
			secretRef := &homer.SecretKeyRef{
				Name:      dashboard.Spec.Secrets.Password.Name,
				Key:       dashboard.Spec.Secrets.Password.Key,
				Namespace: dashboard.Spec.Secrets.Password.Namespace,
			}

			for serviceIdx := range homerConfig.Services {
				for itemIdx := range homerConfig.Services[serviceIdx].Items {
					item := &homerConfig.Services[serviceIdx].Items[itemIdx]
					if err := homer.ResolvePasswordFromSecret(ctx, r.Client, item, secretRef, dashboard.Namespace); err != nil {
						return nil, err
					}
				}
			}
		}

		// Handle custom Headers secrets
		if dashboard.Spec.Secrets.Headers != nil {
			for headerName, secretRef := range dashboard.Spec.Secrets.Headers {
				ref := &homer.SecretKeyRef{
					Name:      secretRef.Name,
					Key:       secretRef.Key,
					Namespace: secretRef.Namespace,
				}

				for serviceIdx := range homerConfig.Services {
					for itemIdx := range homerConfig.Services[serviceIdx].Items {
						item := &homerConfig.Services[serviceIdx].Items[itemIdx]
						if err := homer.ResolveHeaderFromSecret(ctx, r.Client, item, headerName, ref, dashboard.Namespace); err != nil {
							return nil, err
						}
					}
				}
			}
		}
	}

	return homerConfig, nil
}

// validateSmartCardSecretReferences prevents Dashboard authors from causing the
// operator to copy another namespace's Secret into the generated Homer ConfigMap.
// Kubeconfig SecretRefs are deliberately not validated here: RemoteCluster uses
// its own KubeconfigSecretRef type and retains its documented cross-namespace
// support.
func validateSmartCardSecretReferences(dashboard *homerv1alpha1.Dashboard) error {
	if dashboard.Spec.Secrets == nil {
		return nil
	}

	refs := []*homerv1alpha1.SecretKeyRef{
		dashboard.Spec.Secrets.APIKey,
		dashboard.Spec.Secrets.Token,
		dashboard.Spec.Secrets.Username,
		dashboard.Spec.Secrets.Password,
	}
	for _, ref := range dashboard.Spec.Secrets.Headers {
		refs = append(refs, ref)
	}

	for _, ref := range refs {
		if ref != nil && ref.Namespace != "" && ref.Namespace != dashboard.Namespace {
			return fmt.Errorf("smart-card Secret reference %q in namespace %q is not allowed for Dashboard %s/%s: smart-card Secrets must be in the Dashboard namespace %q", ref.Name, ref.Namespace, dashboard.Namespace, dashboard.Name, dashboard.Namespace)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Dashboard{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		})

	// Watch ingresses - they trigger reconciliation of all dashboards
	builder = builder.Watches(&networkingv1.Ingress{},
		handler.EnqueueRequestsFromMapFunc(r.findDashboardsForIngress))

	// Add HTTPRoute watching if Gateway API is enabled
	if r.EnableGatewayAPI {
		builder = builder.Watches(&gatewayv1.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findDashboardsForHTTPRoute)).
			Watches(&gatewayv1.Gateway{},
				handler.EnqueueRequestsFromMapFunc(r.findDashboardsForGateway))
	}

	// Watch secrets for multi-cluster kubeconfig changes
	builder = builder.Watches(&corev1.Secret{},
		handler.EnqueueRequestsFromMapFunc(r.findDashboardsForSecret))

	// Watch Services for discovery
	builder = builder.Watches(&corev1.Service{},
		handler.EnqueueRequestsFromMapFunc(r.findDashboardsForService))

	// Watch namespaces for annotation changes
	builder = builder.Watches(&corev1.Namespace{},
		handler.EnqueueRequestsFromMapFunc(r.findDashboardsForNamespace))

	// Watch referenced ConfigMaps, including asset sources in another namespace.
	// This keeps external Homer configuration, owned asset mirrors, and the
	// resulting Dashboard current when users replace or edit their inputs.
	builder = builder.Watches(&corev1.ConfigMap{},
		handler.EnqueueRequestsFromMapFunc(r.findDashboardsForAssetConfigMap))

	return builder.Complete(r)
}

// findDashboardsForIngress finds all dashboards that should be reconciled when an ingress changes
func (r *DashboardReconciler) findDashboardsForIngress(ctx context.Context, obj client.Object) []ctrl.Request {
	if _, ok := obj.(*networkingv1.Ingress); !ok {
		return nil
	}
	return r.allDashboardRequests(ctx)
}

// findDashboardsForService finds all dashboards that should be reconciled when a Service changes
func (r *DashboardReconciler) findDashboardsForService(ctx context.Context, obj client.Object) []ctrl.Request {
	if _, ok := obj.(*corev1.Service); !ok {
		return nil
	}
	return r.allDashboardRequests(ctx)
}

// findDashboardsForHTTPRoute finds all dashboards that should be reconciled when an HTTPRoute changes
func (r *DashboardReconciler) findDashboardsForHTTPRoute(ctx context.Context, obj client.Object) []ctrl.Request {
	if _, ok := obj.(*gatewayv1.HTTPRoute); !ok {
		return nil
	}
	return r.allDashboardRequests(ctx)
}

// findDashboardsForGateway finds all dashboards that should be reconciled when a Gateway changes
func (r *DashboardReconciler) findDashboardsForGateway(ctx context.Context, obj client.Object) []ctrl.Request {
	if _, ok := obj.(*gatewayv1.Gateway); !ok {
		return nil
	}
	return r.allDashboardRequests(ctx)
}

func (r *DashboardReconciler) allDashboardRequests(ctx context.Context) []ctrl.Request {
	dashboards := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboards); err != nil {
		return nil
	}

	requests := make([]ctrl.Request, 0, len(dashboards.Items))
	for _, dashboard := range dashboards.Items {
		requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&dashboard)})
	}
	return requests
}

// findDashboardsForSecret finds all dashboards that should be reconciled when a Secret changes
// This is specifically for kubeconfig secrets used in multi-cluster configurations
func (r *DashboardReconciler) findDashboardsForSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	// List all dashboards to find those using this secret
	dashboards := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboards); err != nil {
		return nil
	}

	var requests []ctrl.Request
	seen := make(map[client.ObjectKey]struct{})
	for _, dashboard := range dashboards.Items {
		requestKey := client.ObjectKeyFromObject(&dashboard)
		enqueue := func() {
			if _, exists := seen[requestKey]; exists {
				return
			}
			seen[requestKey] = struct{}{}
			requests = append(requests, ctrl.Request{NamespacedName: requestKey})
		}
		// Check if this dashboard uses this secret for remote clusters
		for _, remoteCluster := range dashboard.Spec.RemoteClusters {
			secretNamespace := remoteCluster.SecretRef.Namespace
			if secretNamespace == "" {
				secretNamespace = dashboard.Namespace
			}

			// If the secret matches, trigger reconciliation
			if secret.Name == remoteCluster.SecretRef.Name && secret.Namespace == secretNamespace {
				enqueue()
				break // No need to check other clusters once we found a match
			}
		}

		// Smart-card secrets are always resolved in the Dashboard namespace. Remote
		// kubeconfig secrets above intentionally retain their separate cross-namespace
		// support.
		if dashboard.Spec.Secrets != nil {
			secretRefs := []*homerv1alpha1.SecretKeyRef{
				dashboard.Spec.Secrets.APIKey,
				dashboard.Spec.Secrets.Token,
				dashboard.Spec.Secrets.Password,
				dashboard.Spec.Secrets.Username,
			}

			for _, ref := range secretRefs {
				if ref != nil {
					if secret.Name == ref.Name && secret.Namespace == dashboard.Namespace {
						enqueue()
						break
					}
				}
			}

			// Check header secrets
			if dashboard.Spec.Secrets.Headers != nil {
				for _, ref := range dashboard.Spec.Secrets.Headers {
					if ref != nil {
						if secret.Name == ref.Name && secret.Namespace == dashboard.Namespace {
							enqueue()
							break
						}
					}
				}
			}
		}
	}

	return requests
}

// findDashboardsForNamespace finds all dashboards that should be reconciled when a namespace's annotations change
func (r *DashboardReconciler) findDashboardsForNamespace(ctx context.Context, obj client.Object) []ctrl.Request {
	_, ok := obj.(*corev1.Namespace)
	if !ok {
		return nil
	}

	// Find all dashboards and trigger reconciliation
	// for additions, changes, and removals of inherited Homer annotations.
	dashboards := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboards); err != nil {
		return nil
	}

	requests := make([]ctrl.Request, 0, len(dashboards.Items))
	for _, dashboard := range dashboards.Items {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{
				Namespace: dashboard.Namespace,
				Name:      dashboard.Name,
			},
		})
	}

	return requests
}

func (r *DashboardReconciler) findDashboardsForAssetConfigMap(ctx context.Context, obj client.Object) []ctrl.Request {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	dashboards := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboards); err != nil {
		return nil
	}

	requests := make([]ctrl.Request, 0)
	seen := make(map[client.ObjectKey]struct{})
	for i := range dashboards.Items {
		dashboard := &dashboards.Items[i]
		requestKey := client.ObjectKeyFromObject(dashboard)

		// External Homer configuration is always resolved from the Dashboard's
		// namespace, so only a ConfigMap in that namespace can trigger it.
		externalConfigMatches := dashboard.Spec.ConfigMap.Name != "" &&
			dashboard.Spec.ConfigMap.Name == configMap.Name &&
			dashboard.Namespace == configMap.Namespace

		ref := dashboard.Spec.Assets
		assetMatches := false
		if ref != nil && ref.ConfigMapRef != nil && ref.ConfigMapRef.Name == configMap.Name {
			refNamespace := ref.ConfigMapRef.Namespace
			if refNamespace == "" {
				refNamespace = dashboard.Namespace
			}
			assetMatches = refNamespace == configMap.Namespace
		}

		if !externalConfigMatches && !assetMatches {
			continue
		}
		if _, alreadyQueued := seen[requestKey]; alreadyQueued {
			continue
		}
		seen[requestKey] = struct{}{}
		requests = append(requests, ctrl.Request{NamespacedName: requestKey})
	}
	return requests
}

// mergeNamespaceAnnotations merges namespace annotations with resource annotations
// Namespace annotations serve as defaults, resource annotations override
func (r *DashboardReconciler) mergeNamespaceAnnotations(ctx context.Context, resourceNamespace string, resourceAnnotations map[string]string) map[string]string {
	// Fetch the namespace
	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, client.ObjectKey{Name: resourceNamespace}, namespace); err != nil {
		// If we can't get the namespace, just return the resource annotations
		return resourceAnnotations
	}

	// Start with namespace annotations as the base
	merged := make(map[string]string)

	// First, copy relevant namespace annotations (service.homer.* and item.homer.*)
	for key, value := range namespace.Annotations {
		if strings.HasPrefix(key, serviceAnnotationPrefix) || strings.HasPrefix(key, itemAnnotationPrefix) {
			merged[key] = value
		}
	}

	// Then overlay resource annotations (these override namespace defaults)
	maps.Copy(merged, resourceAnnotations)

	return merged
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
			resource.SetResourceVersion(newResource.GetResourceVersion())
			if desiredService, ok := resource.(*corev1.Service); ok {
				preserveServiceFields(desiredService, newResource.(*corev1.Service))
			}
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

// preserveServiceFields keeps fields allocated or owned by the API server (or
// by another service controller) when applying the fields generated by Homer.
// In particular, sending an empty ClusterIP or NodePort during an update can
// make an otherwise valid Service update fail validation.
func preserveServiceFields(desired, existing *corev1.Service) {
	desiredSelector := desired.Spec.Selector
	desiredPorts := append([]corev1.ServicePort(nil), desired.Spec.Ports...)

	// Homer owns only the selector and ports. Preserve the complete remainder
	// of the existing spec, including Type, ClusterIP(s), IP family fields,
	// and any fields managed by Kubernetes or another controller.
	desired.Spec = existing.Spec
	desired.Spec.Selector = desiredSelector
	desired.Spec.Ports = desiredPorts

	for i := range desired.Spec.Ports {
		desiredPort := &desired.Spec.Ports[i]
		if desiredPort.NodePort != 0 {
			continue
		}

		for _, existingPort := range existing.Spec.Ports {
			if servicePortsMatch(*desiredPort, existingPort) {
				desiredPort.NodePort = existingPort.NodePort
				break
			}
		}
	}
}

func servicePortsMatch(desired, existing corev1.ServicePort) bool {
	if desired.Name != "" && desired.Name != existing.Name {
		return false
	}

	desiredProtocol := desired.Protocol
	if desiredProtocol == "" {
		desiredProtocol = corev1.ProtocolTCP
	}
	existingProtocol := existing.Protocol
	if existingProtocol == "" {
		existingProtocol = corev1.ProtocolTCP
	}

	return desired.Port == existing.Port &&
		desired.TargetPort == existing.TargetPort &&
		desiredProtocol == existingProtocol &&
		reflect.DeepEqual(desired.AppProtocol, existing.AppProtocol) &&
		(desired.NodePort == 0 || desired.NodePort == existing.NodePort)
}

func (r *DashboardReconciler) createConfigMap(ctx context.Context, homerConfig *homer.HomerConfig, dashboard *homerv1alpha1.Dashboard, filteredIngressList networkingv1.IngressList, filteredServices []corev1.Service) (corev1.ConfigMap, map[string][]gatewayv1.HTTPRoute, error) {
	// Merge namespace annotations into Ingress annotations
	mergedIngressList := networkingv1.IngressList{
		Items: make([]networkingv1.Ingress, len(filteredIngressList.Items)),
	}
	for i, ingress := range filteredIngressList.Items {
		// Create a copy to avoid mutating the original
		ingressCopy := ingress.DeepCopy()
		ingressCopy.Annotations = r.mergeNamespaceAnnotations(ctx, ingress.Namespace, ingress.Annotations)
		mergedIngressList.Items[i] = *ingressCopy
	}

	// Merge namespace annotations into Service annotations
	mergedServices := make([]corev1.Service, len(filteredServices))
	for i, svc := range filteredServices {
		svcCopy := svc.DeepCopy()
		svcCopy.Annotations = r.mergeNamespaceAnnotations(ctx, svc.Namespace, svc.Annotations)
		mergedServices[i] = *svcCopy
	}

	if r.EnableGatewayAPI {
		filteredHTTPRoutes := []gatewayv1.HTTPRoute{}
		var discoveredHTTPRoutes map[string][]gatewayv1.HTTPRoute

		// Use the Dashboard-scoped ClusterManager for multi-cluster discovery.
		if manager := r.clusterManagerForDashboard(dashboard); manager != nil && len(dashboard.Spec.RemoteClusters) > 0 {
			clusterHTTPRoutes, err := manager.DiscoverHTTPRoutes(ctx, dashboard)
			discoveredHTTPRoutes = clusterHTTPRoutes
			if err != nil {
				log := log.FromContext(ctx)
				log.Error(err, "Failed to discover HTTPRoutes from clusters")
				// Continue with partial results
			}

			// Aggregate all discovered HTTPRoutes (already filtered by ClusterManager)
			for _, clusterName := range sortedClusterNames(clusterHTTPRoutes) {
				httproutes := clusterHTTPRoutes[clusterName]
				log := log.FromContext(ctx)
				log.V(1).Info("Discovered HTTPRoutes from cluster", "cluster", clusterName, "count", len(httproutes))

				// HTTPRoutes are already filtered by ClusterManager with per-cluster domain filters
				filteredHTTPRoutes = append(filteredHTTPRoutes, httproutes...)
			}
		} else {
			// Fall back to single-cluster discovery
			clusterHTTPRoutes := &gatewayv1.HTTPRouteList{}
			if err := r.List(ctx, clusterHTTPRoutes); err != nil {
				return corev1.ConfigMap{}, nil, err
			}

			for i := range clusterHTTPRoutes.Items {
				shouldInclude, err := r.shouldIncludeHTTPRoute(ctx, &clusterHTTPRoutes.Items[i], dashboard)
				if err != nil {
					return corev1.ConfigMap{}, nil, err
				}
				if shouldInclude {
					filteredHTTPRoutes = append(filteredHTTPRoutes, clusterHTTPRoutes.Items[i])
				}
			}
		}

		// Merge namespace annotations into HTTPRoute annotations
		mergedHTTPRoutes := make([]gatewayv1.HTTPRoute, len(filteredHTTPRoutes))
		for i, httproute := range filteredHTTPRoutes {
			// Create a copy to avoid mutating the original
			httprouteCopy := httproute.DeepCopy()
			httprouteCopy.Annotations = r.mergeNamespaceAnnotations(ctx, httproute.Namespace, httproute.Annotations)
			mergedHTTPRoutes[i] = *httprouteCopy
		}

		// In multi-cluster mode each discovered route carries its cluster's
		// filters in an annotation. Passing dashboard filters globally would
		// incorrectly apply local-cluster filters to remote routes without an
		// explicit per-cluster filter.
		domainFilters := dashboard.Spec.DomainFilters
		discoveryConfig := discoveryConfigForDashboard(dashboard)
		discoveryConfig.IngressDomainFilters = authorizedIngressDomainFilters(dashboard, mergedIngressList.Items)
		if len(dashboard.Spec.RemoteClusters) > 0 {
			domainFilters = nil
			discoveryConfig.HTTPRouteDomainFilters = authorizedHTTPRouteDomainFilters(dashboard, discoveredHTTPRoutes)
		}
		configMap, err := homer.CreateConfigMapWithHTTPRoutesAndDiscoveryConfig(
			homerConfig, dashboard.Name, dashboard.Namespace, mergedIngressList,
			mergedHTTPRoutes, mergedServices, dashboard, domainFilters,
			discoveryConfig,
		)
		if err != nil {
			return corev1.ConfigMap{}, nil, err
		}
		if err := homer.AddPageConfigsToConfigMap(&configMap, dashboard.Spec.Pages); err != nil {
			return corev1.ConfigMap{}, nil, err
		}
		return configMap, discoveredHTTPRoutes, nil
	}

	discoveryConfig := discoveryConfigForDashboard(dashboard)
	discoveryConfig.IngressDomainFilters = authorizedIngressDomainFilters(dashboard, mergedIngressList.Items)
	configMap, err := homer.CreateConfigMapWithDiscoveryConfig(
		homerConfig, dashboard.Name, dashboard.Namespace, mergedIngressList,
		mergedServices, dashboard, discoveryConfig,
	)
	if err != nil {
		return corev1.ConfigMap{}, nil, err
	}
	if err := homer.AddPageConfigsToConfigMap(&configMap, dashboard.Spec.Pages); err != nil {
		return corev1.ConfigMap{}, nil, err
	}
	return configMap, nil, nil
}

func discoveryConfigForDashboard(dashboard *homerv1alpha1.Dashboard) *homer.DiscoveryConfig {
	config := &homer.DiscoveryConfig{ValidationLevel: homer.ValidationLevel(dashboard.Spec.ValidationLevel)}
	if grouping := dashboard.Spec.ServiceGrouping; grouping != nil {
		config.ServiceGrouping = &homer.ServiceGroupingConfig{
			Strategy:    homer.ServiceGroupingStrategy(grouping.Strategy),
			LabelKey:    grouping.LabelKey,
			CustomRules: make([]homer.GroupingRule, len(grouping.CustomRules)),
		}
		for i, rule := range grouping.CustomRules {
			config.ServiceGrouping.CustomRules[i] = homer.GroupingRule{
				Name: rule.Name, Condition: maps.Clone(rule.Condition), Priority: rule.Priority,
			}
		}
	}
	if health := dashboard.Spec.HealthCheck; health != nil {
		config.HealthCheck = &homer.ServiceHealthConfig{
			Enabled: health.Enabled, Interval: health.Interval, Timeout: health.Timeout,
			HealthPath: health.HealthPath, ExpectedCode: health.ExpectedCode,
			Headers: maps.Clone(health.Headers),
		}
	}
	return config
}

func authorizedHTTPRouteDomainFilters(
	dashboard *homerv1alpha1.Dashboard,
	discovered map[string][]gatewayv1.HTTPRoute,
) map[string][]string {
	authorized := make(map[string][]string)
	for clusterName, routes := range discovered {
		for i := range routes {
			resourceCluster := routes[i].Annotations["homer.rajsingh.info/cluster"]
			if resourceCluster == "" {
				resourceCluster = clusterName
			}
			filters, configuredCluster := domainFiltersForCluster(dashboard, resourceCluster)
			if !configuredCluster {
				continue
			}
			authorized[homer.HTTPRouteDomainFilterKey(&routes[i])] = append([]string(nil), filters...)
		}
	}
	return authorized
}

func authorizedIngressDomainFilters(
	dashboard *homerv1alpha1.Dashboard,
	ingresses []networkingv1.Ingress,
) map[string][]string {
	authorized := make(map[string][]string, len(ingresses))
	for i := range ingresses {
		resourceCluster := ingresses[i].Annotations["homer.rajsingh.info/cluster"]
		filters, configuredCluster := domainFiltersForCluster(dashboard, resourceCluster)
		if !configuredCluster {
			continue
		}
		authorized[homer.IngressDomainFilterKey(&ingresses[i])] = append([]string(nil), filters...)
	}
	return authorized
}

func domainFiltersForCluster(dashboard *homerv1alpha1.Dashboard, clusterName string) ([]string, bool) {
	if clusterName == "" || clusterName == homer.LocalCluster {
		return dashboard.Spec.DomainFilters, true
	}
	for i := range dashboard.Spec.RemoteClusters {
		cluster := &dashboard.Spec.RemoteClusters[i]
		if cluster.Name == clusterName && cluster.Enabled {
			return cluster.DomainFilters, true
		}
	}
	return nil, false
}

func iconAliases(dashboard *homerv1alpha1.Dashboard) map[string][]string {
	if dashboard.Spec.Assets == nil || dashboard.Spec.Assets.Icons == nil || dashboard.Spec.Assets.ConfigMapRef == nil || dashboard.Spec.Assets.ConfigMapRef.Name == "" {
		return nil
	}
	icons := dashboard.Spec.Assets.Icons
	aliases := make(map[string][]string)
	if icons.Favicon != "" {
		aliases[icons.Favicon] = append(aliases[icons.Favicon], "icons/favicon.ico")
	}
	if icons.AppleTouchIcon != "" {
		aliases[icons.AppleTouchIcon] = append(aliases[icons.AppleTouchIcon], "icons/apple-touch-icon.png")
	}
	if icons.PWAIcon192 != "" {
		aliases[icons.PWAIcon192] = append(aliases[icons.PWAIcon192], "icons/pwa-192x192.png")
	}
	if icons.PWAIcon512 != "" {
		aliases[icons.PWAIcon512] = append(aliases[icons.PWAIcon512], "icons/pwa-512x512.png")
	}
	return aliases
}

func assetMirrorName(name, namespace, dashboardNamespace, dashboardName string) string {
	digest := sha256.Sum256([]byte(namespace + "/" + name + "@" + dashboardNamespace + "/" + dashboardName))
	suffix := "-homer-" + hex.EncodeToString(digest[:])[:8]
	baseLength := 63 - len(suffix)
	base := name
	if len(base) > baseLength {
		base = base[:baseLength]
	}
	base = strings.TrimRight(base, "-")
	return base + suffix
}

// buildAssetMirror copies a ConfigMap from another namespace into the
// Dashboard namespace. Kubernetes ConfigMap volumes are namespace-scoped, so
// a direct cross-namespace volume reference cannot work.
func (r *DashboardReconciler) buildAssetMirror(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (*corev1.ConfigMap, error) {
	ref := dashboard.Spec.Assets
	if ref == nil || ref.ConfigMapRef == nil || ref.ConfigMapRef.Namespace == "" || ref.ConfigMapRef.Namespace == dashboard.Namespace {
		return nil, nil
	}

	source := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: ref.ConfigMapRef.Name, Namespace: ref.ConfigMapRef.Namespace}, source); err != nil {
		return nil, fmt.Errorf("failed to retrieve assets ConfigMap %s/%s: %w", ref.ConfigMapRef.Namespace, ref.ConfigMapRef.Name, err)
	}

	mirror := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      assetMirrorName(ref.ConfigMapRef.Name, ref.ConfigMapRef.Namespace, dashboard.Namespace, dashboard.Name),
			Namespace: dashboard.Namespace,
			Labels: map[string]string{
				"managed-by":                         "homer-operator",
				"dashboard.homer.rajsingh.info/name": dashboard.Name,
				"homer.rajsingh.info/type":           "asset-mirror",
			},
		},
		Data:       maps.Clone(source.Data),
		BinaryData: cloneBinaryData(source.BinaryData),
	}
	if err := controllerutil.SetControllerReference(dashboard, mirror, r.Scheme); err != nil {
		return nil, fmt.Errorf("set owner reference on asset mirror: %w", err)
	}
	return mirror, nil
}

// cleanupStaleAssetMirrors removes mirrors that belonged to an earlier asset
// reference. A Dashboard can change from one source ConfigMap to another (or
// back to a same-namespace ConfigMap), while owner references only clean up
// when the Dashboard itself is deleted.
func (r *DashboardReconciler) cleanupStaleAssetMirrors(ctx context.Context, dashboard *homerv1alpha1.Dashboard) error {
	mirrors := &corev1.ConfigMapList{}
	if err := r.List(ctx, mirrors, client.InNamespace(dashboard.Namespace), client.MatchingLabels{
		"managed-by":                         "homer-operator",
		"dashboard.homer.rajsingh.info/name": dashboard.Name,
		"homer.rajsingh.info/type":           "asset-mirror",
	}); err != nil {
		return fmt.Errorf("list asset mirrors for Dashboard %s/%s: %w", dashboard.Namespace, dashboard.Name, err)
	}

	desiredName := ""
	if assets := dashboard.Spec.Assets; assets != nil && assets.ConfigMapRef != nil &&
		assets.ConfigMapRef.Namespace != "" && assets.ConfigMapRef.Namespace != dashboard.Namespace {
		desiredName = assetMirrorName(assets.ConfigMapRef.Name, assets.ConfigMapRef.Namespace, dashboard.Namespace, dashboard.Name)
	}

	for i := range mirrors.Items {
		mirror := &mirrors.Items[i]
		if mirror.Name == desiredName || !isOwnedByDashboard(mirror, dashboard) {
			continue
		}
		if err := r.Delete(ctx, mirror); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete stale asset mirror %s/%s: %w", mirror.Namespace, mirror.Name, err)
		}
	}
	return nil
}

func cloneBinaryData(data map[string][]byte) map[string][]byte {
	if data == nil {
		return nil
	}
	clone := make(map[string][]byte, len(data))
	for key, value := range data {
		clone[key] = append([]byte(nil), value...)
	}
	return clone
}

// generatePWAManifest generates PWA manifest if enabled, returns empty string if disabled
func (r *DashboardReconciler) generatePWAManifest(dashboard *homerv1alpha1.Dashboard, effectiveConfig *homer.HomerConfig) string {
	if dashboard.Spec.Assets == nil || dashboard.Spec.Assets.PWA == nil || !dashboard.Spec.Assets.PWA.Enabled {
		return ""
	}

	pwa := dashboard.Spec.Assets.PWA

	// Set defaults if not provided
	name := pwa.Name
	if name == "" {
		if effectiveConfig != nil {
			name = effectiveConfig.Title
		}
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
		if effectiveConfig != nil {
			description = effectiveConfig.Subtitle
		}
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
		startURL = dashboardRelativePath
	}

	// Build icons map
	icons := make(map[string]string)
	if dashboard.Spec.Assets.Icons != nil {
		if dashboard.Spec.Assets.Icons.PWAIcon192 != "" {
			icons["192"] = stagedIconPath(dashboard, dashboard.Spec.Assets.Icons.PWAIcon192, "icons/pwa-192x192.png")
		}
		if dashboard.Spec.Assets.Icons.PWAIcon512 != "" {
			icons["512"] = stagedIconPath(dashboard, dashboard.Spec.Assets.Icons.PWAIcon512, "icons/pwa-512x512.png")
		}
	}

	// Generate PWA manifest
	return homer.GeneratePWAManifest(name, shortName, description, themeColor, backgroundColor, display, startURL, icons)
}

func stagedIconPath(dashboard *homerv1alpha1.Dashboard, source, destination string) string {
	if !isStageableAssetPath(source) {
		return source
	}
	if dashboard.Spec.Assets == nil || dashboard.Spec.Assets.ConfigMapRef == nil || dashboard.Spec.Assets.ConfigMapRef.Name == "" {
		slog.Warn("skipping relative PWA icon without an assets ConfigMap", "source", source)
		return ""
	}
	return destination
}

func isStageableAssetPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "#") || strings.Contains(value, "\\") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || strings.HasPrefix(value, "//") {
		return false
	}
	clean := path.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, dashboardRelativePath)
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
			return fmt.Errorf("failed to get %s %s for cleanup: %w", resource.name, resourceName, err)
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
	if dashboard.Spec.Assets != nil && dashboard.Spec.Assets.ConfigMapRef != nil &&
		(dashboard.Spec.Assets.ConfigMapRef.Namespace == "" || dashboard.Spec.Assets.ConfigMapRef.Namespace == dashboard.Namespace) {
		assetsConfigMapName := dashboard.Spec.Assets.ConfigMapRef.Name
		assetsConfigMap := &corev1.ConfigMap{}
		err := r.Get(ctx, client.ObjectKey{
			Name:      assetsConfigMapName,
			Namespace: namespace,
		}, assetsConfigMap)
		if err == nil {
			if isOwnedByDashboard(assetsConfigMap, dashboard) {
				if err := r.Delete(ctx, assetsConfigMap); err != nil && !apierrors.IsNotFound(err) {
					log.Error(err, "failed to delete assets ConfigMap", "name", assetsConfigMapName)
					return fmt.Errorf("failed to delete assets ConfigMap %s: %w", assetsConfigMapName, err)
				} else {
					log.Info("Successfully deleted assets ConfigMap", "name", assetsConfigMapName)
				}
			}
		} else if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get assets ConfigMap for cleanup", "name", assetsConfigMapName)
			return fmt.Errorf("failed to get assets ConfigMap %s for cleanup: %w", assetsConfigMapName, err)
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
func (r *DashboardReconciler) updateStatus(ctx context.Context, dashboard *homerv1alpha1.Dashboard, discovered *discoveredClusterResources) error {
	log := log.FromContext(ctx)
	previousStatus := dashboard.Status.DeepCopy()

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
	readyReason := "DeploymentNotReady"
	readyMessage := "Homer Deployment is not available"
	availableReplicas := int32(0)
	readyReplicas := int32(0)
	replicas := int32(1) // Default value

	// Get desired replicas from dashboard spec
	if dashboard.Spec.Replicas != nil {
		replicas = *dashboard.Spec.Replicas
	}

	if err == nil {
		// Deployment exists, check if it's available
		availableReplicas = deployment.Status.AvailableReplicas
		readyReplicas = deployment.Status.ReadyReplicas

		// Update replicas from deployment spec if it differs
		if deployment.Spec.Replicas != nil {
			replicas = *deployment.Spec.Replicas
		}

		// Check for Available condition (standard Kubernetes pattern)
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				ready = true
				readyReason = "DeploymentAvailable"
				readyMessage = "Homer Deployment is available"
				break
			}
		}

		log.V(2).Info("Deployment status check",
			"deploymentName", deployment.Name,
			"replicas", replicas,
			"readyReplicas", readyReplicas,
			"availableReplicas", availableReplicas,
			"ready", ready)
	} else if apierrors.IsNotFound(err) {
		// Deployment doesn't exist yet - not ready
		log.V(2).Info("Deployment not found, status not ready")
	} else {
		// Error getting deployment - not ready
		log.V(1).Info("Error getting deployment for status check", "error", err)
		readyReason = "DeploymentStatusUnknown"
		readyMessage = truncateConditionMessage(fmt.Errorf("unable to read Homer Deployment: %w", err))
	}

	// Status.Ready is required by the CRD, so use a status update rather than a
	// merge patch. A merge patch omits an unchanged false boolean and the API
	// server then rejects the status object as missing its required field.
	dashboard.Status.Ready = ready
	dashboard.Status.Replicas = replicas
	dashboard.Status.ReadyReplicas = readyReplicas
	dashboard.Status.AvailableReplicas = availableReplicas
	dashboard.Status.ObservedGeneration = dashboard.Generation
	conditionStatus := metav1.ConditionFalse
	if ready {
		conditionStatus = metav1.ConditionTrue
	}
	apiMeta.SetStatusCondition(&dashboard.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		ObservedGeneration: dashboard.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             readyReason,
		Message:            readyMessage,
	})

	// Update cluster connection statuses from this Dashboard's manager only.
	if manager := r.clusterManagerForDashboard(dashboard); manager != nil {
		clusterStatuses := manager.GetClusterStatuses()

		// Use the resources discovered during reconciliation. Re-discovering here
		// causes duplicate remote API calls and can overwrite a successful pass.
		if discovered != nil && len(dashboard.Spec.RemoteClusters) > 0 {
			clusterStatuses = manager.UpdateClusterStatuses(clusterStatuses, discovered.ingresses, discovered.httpRoutes, discovered.services)
		}

		dashboard.Status.ClusterStatuses = clusterStatuses
	}

	if equality.Semantic.DeepEqual(*previousStatus, dashboard.Status) {
		return nil
	}

	if err := r.Status().Update(ctx, dashboard); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("Dashboard was deleted during status update")
			return nil // Don't return error if Dashboard was deleted
		}
		log.V(1).Info("Failed to update Dashboard status", "error", err)
		return err
	}

	log.V(2).Info("Status updated successfully",
		"ready", dashboard.Status.Ready,
		"replicas", dashboard.Status.Replicas,
		"readyReplicas", dashboard.Status.ReadyReplicas,
		"availableReplicas", dashboard.Status.AvailableReplicas)

	return nil
}
