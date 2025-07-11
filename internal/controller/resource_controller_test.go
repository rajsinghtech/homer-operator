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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

var _ = Describe("Generic Resource Controller", func() {

	Context("When reconciling Ingress resources", func() {
		const ingressName = "test-ingress"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var ingress *networkingv1.Ingress
		var dashboardName string

		BeforeEach(func() {
			// Generate unique dashboard name for each test to avoid conflicts
			dashboardName = fmt.Sprintf("test-dashboard-ingress-%d-%d", GinkgoRandomSeed(), time.Now().UnixNano())

			// Create Dashboard with annotations that will match the Ingress
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "test-app",
						"environment":            "testing",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "Test Dashboard",
						Subtitle: "For Ingress Testing",
						Header:   true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Reconcile the Dashboard to create its ConfigMap
			dashboardReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			namespacedName := types.NamespacedName{
				Name:      dashboardName,
				Namespace: namespaceName,
			}

			// First reconcile: adds finalizer
			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: creates resources
			_, err = dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for ConfigMap to be created
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + homer.ResourceSuffix,
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Create Ingress with matching annotations
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name":            "test-app",
						"environment":                       "testing",
						"item.homer.rajsingh.info/name":     "Test Application",
						"item.homer.rajsingh.info/subtitle": "Production App",
						"item.homer.rajsingh.info/logo":     "https://example.com/logo.png",
						"service.homer.rajsingh.info/name":  "Production Services",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "app.test.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "test-service",
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
			Expect(k8sClient.Create(ctx, ingress)).To(Succeed())
		})

		AfterEach(func() {
			// Force cleanup with removal of finalizers to prevent stuck deletions
			if ingress != nil {
				// First try normal deletion
				err := k8sClient.Delete(ctx, ingress)
				if err != nil && !apierrors.IsNotFound(err) {
					GinkgoT().Logf("Warning: failed to delete ingress: %v", err)
				}

				// Wait for ingress to be fully deleted with longer timeout
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      ingress.Name,
						Namespace: ingress.Namespace,
					}, &networkingv1.Ingress{})
					return apierrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*200).Should(BeTrue())
				ingress = nil
			}

			// Cleanup Dashboard with finalizer handling
			if dashboard != nil {
				// Remove finalizers to force deletion if stuck
				currentDashboard := &homerv1alpha1.Dashboard{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboard.Name,
					Namespace: dashboard.Namespace,
				}, currentDashboard)

				if err == nil {
					// Remove all finalizers
					currentDashboard.SetFinalizers([]string{})
					_ = k8sClient.Update(ctx, currentDashboard)

					// Now delete
					err = k8sClient.Delete(ctx, currentDashboard)
					if err != nil && !apierrors.IsNotFound(err) {
						GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
					}
				}

				// Wait for dashboard to be fully deleted with longer timeout
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      dashboard.Name,
						Namespace: dashboard.Namespace,
					}, &homerv1alpha1.Dashboard{})
					return apierrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*200).Should(BeTrue())
				dashboard = nil
			}

			// Extra pause to ensure complete cleanup before next test
			time.Sleep(time.Millisecond * 100)
		})

		It("should update the ConfigMap with Ingress information", func() {
			By("Reconciling the Ingress resource using GenericResourceReconciler")
			ingressReconciler := &GenericResourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			ingressReconciler.IsHTTPRoute = false

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ingressName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap was updated with Ingress information")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + homer.ResourceSuffix,
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				const nullValue = "null"
				return configYaml != "" && configYaml != nullValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check for Ingress-specific content
			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("app.test.com"))
			Expect(configYaml).To(ContainSubstring("Test Application"))
		})

		It("should handle Ingress with TLS configuration", func() {
			By("Updating Ingress to include TLS configuration")
			// Update the existing ingress to include TLS
			ingress.Spec.TLS = []networkingv1.IngressTLS{
				{
					Hosts:      []string{"app.test.com"},
					SecretName: "tls-secret",
				},
			}
			Expect(k8sClient.Update(ctx, ingress)).To(Succeed())

			By("Reconciling the TLS-enabled Ingress resource")
			ingressReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: false,
			}

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ingressName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap contains HTTPS URL")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + homer.ResourceSuffix,
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				const nullValue = "null"
				return configYaml != "" && configYaml != nullValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("https://app.test.com"))
		})

		It("should handle Ingress without rules gracefully", func() {
			By("Creating an Ingress without rules")
			emptyIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-ingress",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "test-app",
						"environment":            "testing",
					},
				},
				Spec: networkingv1.IngressSpec{
					DefaultBackend: &networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "default-service",
							Port: networkingv1.ServiceBackendPort{Number: 80},
						},
					},
					Rules: []networkingv1.IngressRule{}, // Empty rules
				},
			}
			Expect(k8sClient.Create(ctx, emptyIngress)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, emptyIngress) }()

			By("Reconciling the empty Ingress")
			ingressReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: false,
			}

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "empty-ingress",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling HTTPRoute resources", func() {
		const httprouteName = "test-httproute"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httproute *gatewayv1.HTTPRoute
		var dashboardName string

		BeforeEach(func() {
			if !isGatewayAPIAvailable() {
				Skip("Gateway API CRDs not available in test environment")
			}

			// Generate unique dashboard name for each test to avoid conflicts
			dashboardName = fmt.Sprintf("test-dashboard-httproute-%d-%d", GinkgoRandomSeed(), time.Now().UnixNano())

			// Create Dashboard with annotations that will match the HTTPRoute
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "gateway-app",
						"tier":                   "backend",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "Gateway API Dashboard",
						Subtitle: "For HTTPRoute Testing",
						Header:   true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Reconcile the Dashboard to create its ConfigMap
			dashboardReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			namespacedName := types.NamespacedName{
				Name:      dashboardName,
				Namespace: namespaceName,
			}

			// First reconcile: adds finalizer
			_, err := dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: creates resources
			_, err = dashboardReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for ConfigMap to be created
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + homer.ResourceSuffix,
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Create HTTPRoute with matching annotations
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name":            "gateway-app",
						"tier":                              "backend",
						"item.homer.rajsingh.info/name":     "Gateway Service",
						"item.homer.rajsingh.info/subtitle": "HTTPRoute-based service",
						"service.homer.rajsingh.info/name":  "Gateway Services",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{
						"api.test.com",
					},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  &[]gatewayv1.PathMatchType{gatewayv1.PathMatchPathPrefix}[0],
										Value: &[]string{"/api"}[0],
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			// Force cleanup with removal of finalizers to prevent stuck deletions
			if httproute != nil {
				// First try normal deletion
				err := k8sClient.Delete(ctx, httproute)
				if err != nil && !apierrors.IsNotFound(err) {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}

				// Wait for httproute to be fully deleted with longer timeout
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      httproute.Name,
						Namespace: httproute.Namespace,
					}, &gatewayv1.HTTPRoute{})
					return apierrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*200).Should(BeTrue())
				httproute = nil
			}

			// Cleanup Dashboard with finalizer handling
			if dashboard != nil {
				// Remove finalizers to force deletion if stuck
				currentDashboard := &homerv1alpha1.Dashboard{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboard.Name,
					Namespace: dashboard.Namespace,
				}, currentDashboard)

				if err == nil {
					// Remove all finalizers
					currentDashboard.SetFinalizers([]string{})
					_ = k8sClient.Update(ctx, currentDashboard)

					// Now delete
					err = k8sClient.Delete(ctx, currentDashboard)
					if err != nil && !apierrors.IsNotFound(err) {
						GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
					}
				}

				// Wait for dashboard to be fully deleted with longer timeout
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      dashboard.Name,
						Namespace: dashboard.Namespace,
					}, &homerv1alpha1.Dashboard{})
					return apierrors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*200).Should(BeTrue())
				dashboard = nil
			}

			// Extra pause to ensure complete cleanup before next test
			time.Sleep(time.Millisecond * 100)
		})

		It("should update the ConfigMap with HTTPRoute information", func() {
			By("Reconciling the HTTPRoute resource using GenericResourceReconciler")
			httprouteReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: true,
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      httprouteName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap was updated with HTTPRoute information")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + homer.ResourceSuffix,
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				const nullValue = "null"
				return configYaml != "" && configYaml != nullValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check for HTTPRoute-specific content
			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("api.test.com"))
			Expect(configYaml).To(ContainSubstring("Gateway Service"))
		})

		It("should handle HTTPRoute with empty parent refs", func() {
			By("Creating an HTTPRoute with empty parent refs")
			emptyHTTPRoute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-httproute",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "gateway-app",
						"tier":                   "backend",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{
						"empty.test.com",
					},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{}, // Empty parent refs
					},
				},
			}
			Expect(k8sClient.Create(ctx, emptyHTTPRoute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, emptyHTTPRoute) }()

			By("Reconciling the HTTPRoute with empty parent refs")
			httprouteReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: true,
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "empty-httproute",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle HTTPRoute with invalid Gateway reference", func() {
			By("Creating an HTTPRoute with invalid Gateway reference")
			invalidHTTPRoute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-httproute",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "gateway-app",
						"tier":                   "backend",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{
						"invalid.test.com",
					},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "nonexistent-gateway", // Invalid Gateway reference
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, invalidHTTPRoute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, invalidHTTPRoute) }()

			By("Reconciling the HTTPRoute with invalid Gateway reference")
			httprouteReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: true,
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "invalid-httproute",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle HTTPRoute with multiple hostnames", func() {
			By("Creating an HTTPRoute with multiple hostnames")
			multiHostHTTPRoute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-host-httproute",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name":           "gateway-app",
						"tier":                             "backend",
						"item.homer.rajsingh.info/name":    "Multi-Host Service",
						"service.homer.rajsingh.info/name": "Multi-Host Services",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{
						"api1.test.com",
						"api2.test.com",
						"*.wildcard.test.com",
					},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, multiHostHTTPRoute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, multiHostHTTPRoute) }()

			By("Reconciling the HTTPRoute with multiple hostnames")
			httprouteReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: true,
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "multi-host-httproute",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap contains multiple hostnames")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + homer.ResourceSuffix,
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				const nullValue = "null"
				return configYaml != "" && configYaml != nullValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("api1.test.com"))
			Expect(configYaml).To(ContainSubstring("api2.test.com"))
			Expect(configYaml).To(ContainSubstring("*.wildcard.test.com"))
		})
	})

	Context("When resource doesn't match any Dashboard", func() {
		const ingressName = "test-ingress-no-match"
		const namespaceName = "default"

		ctx := context.Background()

		var ingress *networkingv1.Ingress

		BeforeEach(func() {
			// Create Ingress without matching Dashboard annotations
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "different-app",
						"environment":            "production",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "different.test.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "different-service",
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
			Expect(k8sClient.Create(ctx, ingress)).To(Succeed())
		})

		AfterEach(func() {
			if ingress != nil {
				err := k8sClient.Delete(ctx, ingress)
				if err != nil && !apierrors.IsNotFound(err) {
					GinkgoT().Logf("Warning: failed to delete ingress: %v", err)
				}
				// Wait for ingress to be fully deleted
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      ingress.Name,
						Namespace: ingress.Namespace,
					}, &networkingv1.Ingress{})
					return apierrors.IsNotFound(err)
				}, time.Second*10, time.Millisecond*100).Should(BeTrue())
			}
		})

		It("should reconcile successfully without updating any ConfigMaps", func() {
			By("Reconciling the Ingress resource without matching Dashboard")
			ingressReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: false,
			}

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ingressName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing resource filtering", func() {
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var dashboardName string

		BeforeEach(func() {
			// Generate unique dashboard name for each test to avoid conflicts
			dashboardName = fmt.Sprintf("test-dashboard-filtering-%d-%d", GinkgoRandomSeed(), time.Now().UnixNano())

			// Create Dashboard with selectors
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:  "Filtering Test Dashboard",
						Header: true,
					},
					IngressSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "filtered-app",
						},
					},
					DomainFilters: []string{
						"filtered.test.com",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Reconcile Dashboard first
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
		})

		AfterEach(func() {
			if dashboard != nil {
				// Remove finalizers to force deletion if stuck
				current := &homerv1alpha1.Dashboard{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboard.Name,
					Namespace: dashboard.Namespace,
				}, current); err == nil {
					// Remove all finalizers
					current.Finalizers = []string{}
					_ = k8sClient.Update(ctx, current)
				}

				err := k8sClient.Delete(ctx, dashboard)
				if err != nil && !apierrors.IsNotFound(err) {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
				// Wait for dashboard to be fully deleted
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      dashboard.Name,
						Namespace: dashboard.Namespace,
					}, &homerv1alpha1.Dashboard{})
					return apierrors.IsNotFound(err)
				}, time.Second*10, time.Millisecond*100).Should(BeTrue())
			}
		})

		It("should filter Ingress resources by label selector", func() {
			By("Creating an Ingress that matches the label selector")
			matchingIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matching-ingress",
					Namespace: namespaceName,
					Labels: map[string]string{
						"app": "filtered-app", // Matches selector
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "filtered.test.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "filtered-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
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
			Expect(k8sClient.Create(ctx, matchingIngress)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, matchingIngress) }()

			By("Reconciling the matching Ingress")
			ingressReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: false,
			}

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "matching-ingress",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Creating an Ingress that doesn't match the label selector")
			nonMatchingIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-matching-ingress",
					Namespace: namespaceName,
					Labels: map[string]string{
						"app": "different-app", // Doesn't match selector
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "filtered.test.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "different-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
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
			Expect(k8sClient.Create(ctx, nonMatchingIngress)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, nonMatchingIngress) }()

			By("Reconciling the non-matching Ingress")
			_, err = ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-matching-ingress",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should filter Ingress resources by domain filters", func() {
			By("Creating an Ingress with domain that matches the filter")
			matchingDomainIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matching-domain-ingress",
					Namespace: namespaceName,
					Labels: map[string]string{
						"app": "filtered-app",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "filtered.test.com", // Matches domain filter
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "filtered-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
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
			Expect(k8sClient.Create(ctx, matchingDomainIngress)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, matchingDomainIngress) }()

			By("Reconciling the matching domain Ingress")
			ingressReconciler := &GenericResourceReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				IsHTTPRoute: false,
			}

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "matching-domain-ingress",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Creating an Ingress with domain that doesn't match the filter")
			nonMatchingDomainIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-matching-domain-ingress",
					Namespace: namespaceName,
					Labels: map[string]string{
						"app": "filtered-app",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "different.test.com", // Doesn't match domain filter
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "different-service",
													Port: networkingv1.ServiceBackendPort{Number: 80},
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
			Expect(k8sClient.Create(ctx, nonMatchingDomainIngress)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, nonMatchingDomainIngress) }()

			By("Reconciling the non-matching domain Ingress")
			_, err = ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-matching-domain-ingress",
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
