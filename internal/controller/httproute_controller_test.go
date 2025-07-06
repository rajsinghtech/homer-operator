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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

const nullHTTPRouteValue = "null"

var _ = Describe("HTTPRoute Controller", func() {
	BeforeEach(func() {
		if !isGatewayAPIAvailable() {
			Skip("Gateway API CRDs not available in test environment")
		}
	})

	Context("When reconciling an HTTPRoute with matching Dashboard annotations", func() {
		const dashboardName = "test-dashboard-httproute"
		const httprouteName = "test-httproute"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// First, reconcile the Dashboard to create its ConfigMap
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

			// Wait for ConfigMap to be created
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
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
						"item.homer.rajsingh.info/subtitle": "API Gateway",
						"item.homer.rajsingh.info/logo":     "https://example.com/gateway-logo.png",
						"item.homer.rajsingh.info/type":     "GenericWebhook",
						"service.homer.rajsingh.info/name":  "Gateway Services",
						"service.homer.rajsingh.info/icon":  "fas fa-route",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"api.gateway.com"},
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
										Type:  func() *gatewayv1.PathMatchType { t := gatewayv1.PathMatchPathPrefix; return &t }(),
										Value: func() *string { s := "/api"; return &s }(),
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "api-service",
											Port: func() *gatewayv1.PortNumber { p := gatewayv1.PortNumber(8080); return &p }(),
										},
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
			// Cleanup HTTPRoute
			if httproute != nil {
				err := k8sClient.Delete(ctx, httproute)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}
			}

			// Cleanup Dashboard
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should update the ConfigMap with HTTPRoute information", func() {
			By("Reconciling the HTTPRoute resource")
			httprouteReconciler := &HTTPRouteReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
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
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" && configYaml != nullHTTPRouteValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check for HTTPRoute-specific content
			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("api.gateway.com"))
			Expect(configYaml).To(ContainSubstring("Gateway Service"))
			Expect(configYaml).To(ContainSubstring("API Gateway"))
		})
	})

	Context("When reconciling an HTTPRoute without annotations", func() {
		const httprouteName = "test-httproute-no-annotations"
		const namespaceName = "default"

		ctx := context.Background()

		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create HTTPRoute without annotations
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					// No annotations
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"no-annotations.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			if httproute != nil {
				err := k8sClient.Delete(ctx, httproute)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}
			}
		})

		It("should skip processing when HTTPRoute has no annotations", func() {
			By("Reconciling the HTTPRoute resource without annotations")
			httprouteReconciler := &HTTPRouteReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      httprouteName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling an HTTPRoute without matching Dashboard annotations", func() {
		const httprouteName = "test-httproute-no-match"
		const namespaceName = "default"

		ctx := context.Background()

		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create HTTPRoute without matching Dashboard annotations
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name": "unmatched-app",
						"environment":            "staging",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"unmatched.gateway.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			if httproute != nil {
				err := k8sClient.Delete(ctx, httproute)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}
			}
		})

		It("should reconcile successfully without updating any ConfigMaps", func() {
			By("Reconciling the HTTPRoute resource")
			httprouteReconciler := &HTTPRouteReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      httprouteName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling an HTTPRoute with HTTPS-indicating patterns", func() {
		const dashboardName = "test-dashboard-https"
		const httprouteName = "test-httproute-https"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create Dashboard
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"secure": "true",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Secure Gateway Dashboard",
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

			// Create HTTPRoute with patterns that suggest HTTPS
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"secure":                            "true",
						"item.homer.rajsingh.info/name":     "Secure API",
						"item.homer.rajsingh.info/subtitle": "Production API",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"api.example.com"}, // .com suggests HTTPS
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name:        "https-gateway",
								SectionName: func() *gatewayv1.SectionName { s := gatewayv1.SectionName("https-listener"); return &s }(),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			if httproute != nil {
				err := k8sClient.Delete(ctx, httproute)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}
			}

			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should generate HTTPS URL for HTTPRoute with HTTPS indicators", func() {
			By("Reconciling the HTTPRoute with HTTPS patterns")
			httprouteReconciler := &HTTPRouteReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      httprouteName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap contains HTTPS URL")
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
				return configYaml != "" && configYaml != nullHTTPRouteValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("https://api.example.com"))
			Expect(configYaml).To(ContainSubstring("Secure API"))
		})
	})

	Context("When reconciling an HTTPRoute with local development hostname", func() {
		const dashboardName = "test-dashboard-local"
		const httprouteName = "test-httproute-local"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create Dashboard
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"environment": "development",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Local Development Dashboard",
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

			// Create HTTPRoute with local development hostname
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"environment":                       "development",
						"item.homer.rajsingh.info/name":     "Local API",
						"item.homer.rajsingh.info/subtitle": "Development Environment",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"localhost:8080"}, // Local development pattern
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "local-gateway",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			if httproute != nil {
				err := k8sClient.Delete(ctx, httproute)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}
			}

			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should generate HTTP URL for local development hostname", func() {
			By("Reconciling the HTTPRoute with local hostname")
			httprouteReconciler := &HTTPRouteReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      httprouteName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap contains HTTP URL for localhost")
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
				return configYaml != "" && configYaml != nullHTTPRouteValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("http://localhost:8080"))
			Expect(configYaml).To(ContainSubstring("Local API"))
		})
	})

	Context("When reconciling an HTTPRoute with complex service metadata", func() {
		const dashboardName = "test-dashboard-metadata"
		const httprouteName = "test-httproute-metadata"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
			// Create Dashboard
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"team": "platform",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Platform Services Dashboard",
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

			// Create HTTPRoute with comprehensive metadata annotations
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"team":                                "platform",
						"item.homer.rajsingh.info/name":       "Metadata Service",
						"item.homer.rajsingh.info/subtitle":   "Advanced API with Metadata",
						"item.homer.rajsingh.info/logo":       "https://example.com/metadata-logo.png",
						"item.homer.rajsingh.info/tag":        "v2.0",
						"item.homer.rajsingh.info/tagstyle":   "is-success",
						"item.homer.rajsingh.info/keywords":   "metadata,api,platform",
						"item.homer.rajsingh.info/target":     "_blank",
						"item.homer.rajsingh.info/class":      "highlight",
						"item.homer.rajsingh.info/background": "#f0f0f0",
						"item.homer.rajsingh.info/type":       "Prometheus",
						"item.homer.rajsingh.info/endpoint":   "/metrics",
						"service.homer.rajsingh.info/name":    "Platform APIs",
						"service.homer.rajsingh.info/icon":    "fas fa-database",
						"service.homer.rajsingh.info/logo":    "https://example.com/platform-logo.png",
						"service.homer.rajsingh.info/class":   "platform-services",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"metadata.platform.com"},
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name: "platform-gateway",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			if httproute != nil {
				err := k8sClient.Delete(ctx, httproute)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete httproute: %v", err)
				}
			}

			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should process all metadata annotations correctly", func() {
			By("Reconciling the HTTPRoute with comprehensive metadata")
			httprouteReconciler := &HTTPRouteReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := httprouteReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      httprouteName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap contains all metadata")
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
				return configYaml != "" && configYaml != nullHTTPRouteValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Check hostname and basic service info
			Expect(configYaml).To(ContainSubstring("metadata.platform.com"))
			Expect(configYaml).To(ContainSubstring("Metadata Service"))
			Expect(configYaml).To(ContainSubstring("Advanced API with Metadata"))

			// Check item metadata
			Expect(configYaml).To(ContainSubstring("v2.0"))
			Expect(configYaml).To(ContainSubstring("is-success"))
			Expect(configYaml).To(ContainSubstring("metadata,api,platform"))
			Expect(configYaml).To(ContainSubstring("_blank"))
			Expect(configYaml).To(ContainSubstring("highlight"))
			Expect(configYaml).To(ContainSubstring("/metrics"))

			// Check service metadata
			Expect(configYaml).To(ContainSubstring("Platform APIs"))
			Expect(configYaml).To(ContainSubstring("fas fa-database"))
		})
	})
})
