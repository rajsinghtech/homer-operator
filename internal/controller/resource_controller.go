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

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	homer "github.com/rajsinghtech/homer-operator/pkg/homer"
	"github.com/rajsinghtech/homer-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ResourceInfo struct {
	Name        string
	Namespace   string
	Annotations map[string]string
	Labels      map[string]string
	Object      client.Object
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch

type GenericResourceReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	IsHTTPRoute bool
}

func (r *GenericResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	resourceInfo, err := r.getResourceInfo(ctx, req)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// List all Dashboard CRs
	dashboardList := &homerv1alpha1.DashboardList{}
	if err := r.List(ctx, dashboardList); err != nil {
		return ctrl.Result{}, err
	}

	for _, dashboard := range dashboardList.Items {
		delete(dashboard.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if utils.IsSubset(resourceInfo.Annotations, dashboard.Annotations) {
			shouldInclude, err := r.shouldIncludeResource(ctx, resourceInfo, &dashboard)
			if err != nil {
				return ctrl.Result{}, err
			}
			if !shouldInclude {
				continue
			}

			configMap := corev1.ConfigMap{}
			configMapName := dashboard.Name + homer.ResourceSuffix
			if err := r.Get(ctx, client.ObjectKey{Namespace: dashboard.Namespace, Name: configMapName}, &configMap); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return ctrl.Result{}, err
			}

			r.updateConfigMap(resourceInfo, &configMap, dashboard.Spec.DomainFilters)

			if err := utils.UpdateConfigMapWithRetry(ctx, r.Client, &configMap, dashboard.Name); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *GenericResourceReconciler) getResourceInfo(ctx context.Context, req ctrl.Request) (*ResourceInfo, error) {
	if r.IsHTTPRoute {
		var httproute gatewayv1.HTTPRoute
		if err := r.Get(ctx, req.NamespacedName, &httproute); err != nil {
			return nil, err
		}
		return &ResourceInfo{
			Name:        httproute.Name,
			Namespace:   httproute.Namespace,
			Annotations: httproute.Annotations,
			Labels:      httproute.Labels,
			Object:      &httproute,
		}, nil
	}

	var ingress networkingv1.Ingress
	if err := r.Get(ctx, req.NamespacedName, &ingress); err != nil {
		return nil, err
	}
	return &ResourceInfo{
		Name:        ingress.Name,
		Namespace:   ingress.Namespace,
		Annotations: ingress.Annotations,
		Labels:      ingress.Labels,
		Object:      &ingress,
	}, nil
}

func (r *GenericResourceReconciler) shouldIncludeResource(ctx context.Context, resourceInfo *ResourceInfo, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	if r.IsHTTPRoute {
		return r.shouldIncludeHTTPRoute(ctx, resourceInfo, dashboard)
	}
	return r.shouldIncludeIngress(resourceInfo, dashboard)
}

func (r *GenericResourceReconciler) shouldIncludeIngress(resourceInfo *ResourceInfo, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	if dashboard.Spec.IngressSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.IngressSelector)
		if err != nil {
			return false, err
		}
		if !selector.Matches(labels.Set(resourceInfo.Labels)) {
			return false, nil
		}
	}

	if len(dashboard.Spec.DomainFilters) > 0 {
		ingress := resourceInfo.Object.(*networkingv1.Ingress)
		if !utils.MatchesIngressDomainFilters(ingress, dashboard.Spec.DomainFilters) {
			return false, nil
		}
	}

	return true, nil
}

func (r *GenericResourceReconciler) shouldIncludeHTTPRoute(ctx context.Context, resourceInfo *ResourceInfo, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	httproute := resourceInfo.Object.(*gatewayv1.HTTPRoute)

	if dashboard.Spec.HTTPRouteSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.HTTPRouteSelector)
		if err != nil {
			return false, err
		}
		if !selector.Matches(labels.Set(resourceInfo.Labels)) {
			return false, nil
		}
	}

	if len(dashboard.Spec.DomainFilters) > 0 {
		if !utils.MatchesHTTPRouteDomainFilters(httproute.Spec.Hostnames, dashboard.Spec.DomainFilters) {
			return false, nil
		}
	}

	if dashboard.Spec.GatewaySelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(dashboard.Spec.GatewaySelector)
		if err != nil {
			return false, err
		}

		for _, parentRef := range httproute.Spec.ParentRefs {
			if parentRef.Kind != nil && string(*parentRef.Kind) != "Gateway" {
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

func (r *GenericResourceReconciler) updateConfigMap(resourceInfo *ResourceInfo, configMap *corev1.ConfigMap, domainFilters []string) {
	if r.IsHTTPRoute {
		httproute := resourceInfo.Object.(*gatewayv1.HTTPRoute)
		homer.UpdateConfigMapHTTPRoute(configMap, httproute, domainFilters)
	} else {
		ingress := resourceInfo.Object.(*networkingv1.Ingress)
		homer.UpdateConfigMapIngress(configMap, *ingress, domainFilters)
	}
}

func (r *GenericResourceReconciler) SetupIngressController(mgr ctrl.Manager) error {
	r.IsHTTPRoute = false
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}

func (r *GenericResourceReconciler) SetupHTTPRouteController(mgr ctrl.Manager) error {
	r.IsHTTPRoute = true
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.HTTPRoute{}).
		Complete(r)
}
