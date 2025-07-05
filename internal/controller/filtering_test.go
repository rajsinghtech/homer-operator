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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator.git/pkg/homer"
)

var _ = Describe("Filtering Controller Tests", func() {
	BeforeEach(func() {
		if !isGatewayAPIAvailable() {
			Skip("Gateway API CRDs not available in test environment")
		}
	})

	Context("Gateway Selector Filtering", func() {
		const dashboardName = "test-dashboard-gateway-filter"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var gateway *gatewayv1.Gateway
		var httprouteMatching *gatewayv1.HTTPRoute
		var httprouteNonMatching *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create Gateway with specific labels
			gateway = &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public-gateway",
					Namespace: namespaceName,
					Labels: map[string]string{
						"gateway":     "public",
						"environment": "production",
					},
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "test-gateway-class",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, gateway)).To(Succeed())

			// Create Dashboard with Gateway selector
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Gateway Filtered Dashboard",
					},
					GatewaySelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"gateway": "public",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create HTTPRoute that references the matching Gateway
			httprouteMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-matching",
					Namespace: namespaceName,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"matching.example.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "public-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "test-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteMatching)).To(Succeed())

			// Create HTTPRoute that references a non-matching Gateway
			httprouteNonMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-nonmatching",
					Namespace: namespaceName,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"nonmatching.example.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "private-gateway", // This Gateway doesn't exist or has different labels
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "test-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteNonMatching)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			if httprouteMatching != nil {
				k8sClient.Delete(ctx, httprouteMatching)
			}
			if httprouteNonMatching != nil {
				k8sClient.Delete(ctx, httprouteNonMatching)
			}
			if dashboard != nil {
				k8sClient.Delete(ctx, dashboard)
			}
			if gateway != nil {
				k8sClient.Delete(ctx, gateway)
			}
		})

		It("should only include HTTPRoutes from Gateways matching the selector", func() {
			By("Reconciling the Dashboard with Gateway selector")
			dashboardReconciler := &DashboardReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				EnableGatewayAPI: true,
			}

			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap only contains HTTPRoutes from matching Gateways")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" && configYaml != "null"
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Should contain HTTPRoute from matching Gateway
			Expect(configYaml).To(ContainSubstring("matching.example.com"))
			// Should NOT contain HTTPRoute from non-matching Gateway
			Expect(configYaml).NotTo(ContainSubstring("nonmatching.example.com"))
		})
	})

	Context("HTTPRoute Selector Filtering", func() {
		const dashboardName = "test-dashboard-httproute-filter"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httprouteMatching *gatewayv1.HTTPRoute
		var httprouteNonMatching *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create Dashboard with HTTPRoute selector
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "HTTPRoute Filtered Dashboard",
					},
					HTTPRouteSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"team": "platform",
							"tier": "frontend",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create HTTPRoute with matching labels
			httprouteMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-platform-frontend",
					Namespace: namespaceName,
					Labels: map[string]string{
						"team": "platform",
						"tier": "frontend",
						"app":  "api-gateway",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"platform-api.example.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "platform-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteMatching)).To(Succeed())

			// Create HTTPRoute with non-matching labels
			httprouteNonMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-data-backend",
					Namespace: namespaceName,
					Labels: map[string]string{
						"team": "data",
						"tier": "backend",
						"app":  "data-processor",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"data-api.example.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "data-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteNonMatching)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			if httprouteMatching != nil {
				k8sClient.Delete(ctx, httprouteMatching)
			}
			if httprouteNonMatching != nil {
				k8sClient.Delete(ctx, httprouteNonMatching)
			}
			if dashboard != nil {
				k8sClient.Delete(ctx, dashboard)
			}
		})

		It("should only include HTTPRoutes matching the label selector", func() {
			By("Reconciling the Dashboard with HTTPRoute selector")
			dashboardReconciler := &DashboardReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				EnableGatewayAPI: true,
			}

			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap only contains HTTPRoutes with matching labels")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" && configYaml != "null"
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Should contain HTTPRoute with matching labels
			Expect(configYaml).To(ContainSubstring("platform-api.example.com"))
			// Should NOT contain HTTPRoute with non-matching labels
			Expect(configYaml).NotTo(ContainSubstring("data-api.example.com"))
		})
	})

	Context("Ingress Selector Filtering", func() {
		const dashboardName = "test-dashboard-ingress-filter"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var ingressMatching *networkingv1.Ingress
		var ingressNonMatching *networkingv1.Ingress

		BeforeEach(func() {
			// Create Dashboard with Ingress selector
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Ingress Filtered Dashboard",
					},
					IngressSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"environment": "production",
							"public":      "true",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create Ingress with matching labels
			ingressMatching = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-production-public",
					Namespace: namespaceName,
					Labels: map[string]string{
						"environment": "production",
						"public":      "true",
						"app":         "web-service",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "production-web.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { t := networkingv1.PathTypePrefix; return &t }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "web-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ingressMatching)).To(Succeed())

			// Create Ingress with non-matching labels
			ingressNonMatching = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-staging-private",
					Namespace: namespaceName,
					Labels: map[string]string{
						"environment": "staging",
						"public":      "false",
						"app":         "internal-service",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "staging-internal.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { t := networkingv1.PathTypePrefix; return &t }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "internal-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ingressNonMatching)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			if ingressMatching != nil {
				k8sClient.Delete(ctx, ingressMatching)
			}
			if ingressNonMatching != nil {
				k8sClient.Delete(ctx, ingressNonMatching)
			}
			if dashboard != nil {
				k8sClient.Delete(ctx, dashboard)
			}
		})

		It("should only include Ingresses matching the label selector", func() {
			By("Reconciling the Dashboard with Ingress selector")
			dashboardReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap only contains Ingresses with matching labels")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" && configYaml != "null"
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Should contain Ingress with matching labels
			Expect(configYaml).To(ContainSubstring("production-web.example.com"))
			// Should NOT contain Ingress with non-matching labels
			Expect(configYaml).NotTo(ContainSubstring("staging-internal.example.com"))
		})
	})

	Context("Domain Filters", func() {
		const dashboardName = "test-dashboard-domain-filter"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httprouteMatching *gatewayv1.HTTPRoute
		var httprouteSubdomainMatching *gatewayv1.HTTPRoute
		var httprouteNonMatching *gatewayv1.HTTPRoute
		var ingressMatching *networkingv1.Ingress
		var ingressNonMatching *networkingv1.Ingress

		BeforeEach(func() {
			// Create Dashboard with domain filters
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Domain Filtered Dashboard",
					},
					DomainFilters: []string{
						"mycompany.com",
						"internal.local",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create HTTPRoute with exact domain match
			httprouteMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-exact-match",
					Namespace: namespaceName,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"mycompany.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "company-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteMatching)).To(Succeed())

			// Create HTTPRoute with subdomain match
			httprouteSubdomainMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-subdomain-match",
					Namespace: namespaceName,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"api.internal.local"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "internal-api-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteSubdomainMatching)).To(Succeed())

			// Create HTTPRoute with non-matching domain
			httprouteNonMatching = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-no-match",
					Namespace: namespaceName,
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"external.example.org"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "external-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteNonMatching)).To(Succeed())

			// Create Ingress with matching domain
			ingressMatching = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-matching-domain",
					Namespace: namespaceName,
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "web.mycompany.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { t := networkingv1.PathTypePrefix; return &t }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "web-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ingressMatching)).To(Succeed())

			// Create Ingress with non-matching domain
			ingressNonMatching = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-nonmatching-domain",
					Namespace: namespaceName,
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "service.external.org",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { t := networkingv1.PathTypePrefix; return &t }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "external-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ingressNonMatching)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			if httprouteMatching != nil {
				k8sClient.Delete(ctx, httprouteMatching)
			}
			if httprouteSubdomainMatching != nil {
				k8sClient.Delete(ctx, httprouteSubdomainMatching)
			}
			if httprouteNonMatching != nil {
				k8sClient.Delete(ctx, httprouteNonMatching)
			}
			if ingressMatching != nil {
				k8sClient.Delete(ctx, ingressMatching)
			}
			if ingressNonMatching != nil {
				k8sClient.Delete(ctx, ingressNonMatching)
			}
			if dashboard != nil {
				k8sClient.Delete(ctx, dashboard)
			}
		})

		It("should only include resources with hostnames matching domain filters", func() {
			By("Reconciling the Dashboard with domain filters")
			dashboardReconciler := &DashboardReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				EnableGatewayAPI: true,
			}

			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap only contains resources with matching domains")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" && configYaml != "null"
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Should contain exact domain match
			Expect(configYaml).To(ContainSubstring("mycompany.com"))
			// Should contain subdomain match
			Expect(configYaml).To(ContainSubstring("api.internal.local"))
			// Should contain Ingress subdomain match
			Expect(configYaml).To(ContainSubstring("web.mycompany.com"))
			// Should NOT contain non-matching domains
			Expect(configYaml).NotTo(ContainSubstring("external.example.org"))
			Expect(configYaml).NotTo(ContainSubstring("service.external.org"))
		})
	})

	Context("Combined Filtering", func() {
		const dashboardName = "test-dashboard-combined-filter"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var gateway *gatewayv1.Gateway
		var httprouteFullMatch *gatewayv1.HTTPRoute
		var httproutePartialMatch *gatewayv1.HTTPRoute
		var httprouteNoMatch *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create Gateway with specific labels
			gateway = &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "platform-gateway",
					Namespace: namespaceName,
					Labels: map[string]string{
						"gateway":     "public",
						"environment": "production",
					},
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "test-gateway-class",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Port:     80,
							Protocol: gatewayv1.HTTPProtocolType,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, gateway)).To(Succeed())

			// Create Dashboard with multiple filters
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Combined Filters Dashboard",
					},
					GatewaySelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"gateway": "public",
						},
					},
					HTTPRouteSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"team": "platform",
						},
					},
					DomainFilters: []string{
						"mycompany.com",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create HTTPRoute that matches ALL filters
			httprouteFullMatch = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-full-match",
					Namespace: namespaceName,
					Labels: map[string]string{
						"team": "platform",
						"app":  "web-service",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"api.mycompany.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "platform-gateway", // Matches Gateway selector
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "api-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteFullMatch)).To(Succeed())

			// Create HTTPRoute that matches Gateway and domain but NOT HTTPRoute labels
			httproutePartialMatch = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-partial-match",
					Namespace: namespaceName,
					Labels: map[string]string{
						"team": "data", // Doesn't match HTTPRoute selector
						"app":  "data-service",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"data.mycompany.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "platform-gateway", // Matches Gateway selector
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "data-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproutePartialMatch)).To(Succeed())

			// Create HTTPRoute that matches nothing
			httprouteNoMatch = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute-no-match",
					Namespace: namespaceName,
					Labels: map[string]string{
						"team": "external",
						"app":  "external-service",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"external.example.org"}, // Doesn't match domain filter
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "external-gateway", // Doesn't match Gateway selector
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "external-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(80); return &p }(),
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httprouteNoMatch)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup resources
			if httprouteFullMatch != nil {
				k8sClient.Delete(ctx, httprouteFullMatch)
			}
			if httproutePartialMatch != nil {
				k8sClient.Delete(ctx, httproutePartialMatch)
			}
			if httprouteNoMatch != nil {
				k8sClient.Delete(ctx, httprouteNoMatch)
			}
			if dashboard != nil {
				k8sClient.Delete(ctx, dashboard)
			}
			if gateway != nil {
				k8sClient.Delete(ctx, gateway)
			}
		})

		It("should only include HTTPRoutes matching ALL filters", func() {
			By("Reconciling the Dashboard with combined filters")
			dashboardReconciler := &DashboardReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				EnableGatewayAPI: true,
			}

			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap only contains HTTPRoutes matching all filters")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" && configYaml != "null"
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Should contain HTTPRoute that matches ALL filters
			Expect(configYaml).To(ContainSubstring("api.mycompany.com"))
			// Should NOT contain HTTPRoute that only partially matches
			Expect(configYaml).NotTo(ContainSubstring("data.mycompany.com"))
			// Should NOT contain HTTPRoute that doesn't match any filters
			Expect(configYaml).NotTo(ContainSubstring("external.example.org"))
		})
	})
})
