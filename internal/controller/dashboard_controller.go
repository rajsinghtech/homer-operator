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
	// List ingresses with performance optimization - only get ones with rules
	ingresses := &networkingv1.IngressList{}
	if err := r.List(ctx, ingresses, client.HasLabels([]string{})); err != nil {
		log.Error(err, "unable to list Ingresses", "dashboard", req.NamespacedName)
		return ctrl.Result{}, err
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
		configMap = homer.CreateConfigMapWithHTTPRoutes(homerConfig, dashboard.Name, dashboard.Namespace, *ingresses, httproutes.Items, &dashboard)
	} else {
		configMap = homer.CreateConfigMap(homerConfig, dashboard.Name, dashboard.Namespace, *ingresses, &dashboard)
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
