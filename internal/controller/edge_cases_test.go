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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

const nullString = "null"

var _ = Describe("Edge Cases and Error Handling Tests", func() {
	Context("When creating Dashboard with empty configuration", func() {
		const dashboardName = "test-dashboard-empty"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			// Create Dashboard with minimal configuration
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					// Minimal HomerConfig with empty title (should cause validation error)
					HomerConfig: homer.HomerConfig{},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should handle validation errors gracefully", func() {
			By("Reconciling the Dashboard with empty configuration")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// The controller should now properly return validation errors
			// instead of silently ignoring them
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("homer config validation failed"))
			Expect(err.Error()).To(ContainSubstring("title is required for dashboard"))

			By("Checking that resources are NOT created when validation fails")
			configMap := &corev1.ConfigMap{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err != nil // Should consistently return error (resource not found)
			}, time.Second*3, time.Millisecond*500).Should(BeTrue())
		})
	})

	Context("When creating Dashboard with zero replicas", func() {
		const dashboardName = "test-dashboard-zero-replicas"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			replicas := int32(0)
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					Replicas: &replicas,
					HomerConfig: homer.HomerConfig{
						Title: "Zero Replicas Dashboard",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should create deployment with zero replicas", func() {
			By("Reconciling the Dashboard with zero replicas")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment has zero replicas")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(0)))
		})
	})

	Context("When processing Ingress with malformed annotations", func() {
		const dashboardName = "test-dashboard-malformed"
		const ingressName = "test-ingress-malformed"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var ingress *networkingv1.Ingress

		BeforeEach(func() {
			// Create Dashboard
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"test": "malformed",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Malformed Test Dashboard",
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

			// Create Ingress with malformed annotation values
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"test":                                    "malformed",
						"item.homer.rajsingh.info/name":           "Test App",
						"item.homer.rajsingh.info/url":            "not-a-valid-url", // Invalid URL
						"item.homer.rajsingh.info/target":         "_invalid_target", // Invalid target
						"item.homer.rajsingh.info/usecredentials": "not-a-boolean",   // Invalid boolean
						"service.homer.rajsingh.info/name":        "",                // Empty service name
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "malformed.test.com",
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
			if ingress != nil {
				err := k8sClient.Delete(ctx, ingress)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete ingress: %v", err)
				}
			}
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should handle malformed annotations gracefully", func() {
			By("Reconciling the Ingress with malformed annotations")
			ingressReconciler := &IngressReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := ingressReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ingressName,
					Namespace: namespaceName,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap is updated despite malformed annotations")
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
				return configYaml != "" && configYaml != nullString
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("malformed.test.com"))
			Expect(configYaml).To(ContainSubstring("Test App"))
			// The annotation processor should handle malformed values gracefully
		})
	})

	Context("When processing HTTPRoute with invalid hostname patterns", func() {
		const dashboardName = "test-dashboard-invalid-hostname"
		const httprouteName = "test-httproute-invalid-hostname"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var httproute *gatewayv1.HTTPRoute

		BeforeEach(func() {
			if !isGatewayAPIAvailable() {
				Skip("Gateway API CRDs not available in test environment")
			}
			// Create Dashboard
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"test": "hostname",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Invalid Hostname Test",
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

			// Create HTTPRoute with unusual hostname patterns
			httproute = &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      httprouteName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"test":                              "hostname",
						"item.homer.rajsingh.info/name":     "Invalid Hostname Service",
						"item.homer.rajsingh.info/subtitle": "Testing edge cases",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{
						"*.wildcard.example.com", // Wildcard hostname
						"",                       // Empty hostname
						"very-long-hostname-that-might-cause-issues-with-url-generation.example.com", // Very long hostname
						"192.168.1.1", // IP address
						"[::1]",       // IPv6 address
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

		It("should handle unusual hostname patterns gracefully", func() {
			By("Reconciling the HTTPRoute with unusual hostnames")
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

			By("Checking that ConfigMap handles unusual hostnames")
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
				return configYaml != "" && configYaml != nullString
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			// Should now include ALL hostnames, not just the first one
			Expect(configYaml).To(ContainSubstring("*.wildcard.example.com"))
			Expect(configYaml).To(ContainSubstring("very-long-hostname-that-might-cause-issues-with-url-generation.example.com"))
			Expect(configYaml).To(ContainSubstring("192.168.1.1"))
			Expect(configYaml).To(ContainSubstring("[::1]"))
			Expect(configYaml).To(ContainSubstring("Invalid Hostname Service"))

			// Verify multiple items are created for the same HTTPRoute
			GinkgoT().Logf("ConfigMap YAML content:\n%s", configYaml)
		})
	})

	Context("When Dashboard controller encounters resource conflicts", func() {
		const dashboardName = "test-dashboard-conflict"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var conflictingConfigMap *corev1.ConfigMap

		BeforeEach(func() {
			// Create a ConfigMap with the same name that Dashboard would use
			conflictingConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
					Labels: map[string]string{
						"external": "true", // Not managed by homer-operator
					},
				},
				Data: map[string]string{
					"config.yml": "external: config",
				},
			}
			Expect(k8sClient.Create(ctx, conflictingConfigMap)).To(Succeed())

			// Create Dashboard
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Conflict Test Dashboard",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
			if conflictingConfigMap != nil {
				err := k8sClient.Delete(ctx, conflictingConfigMap)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete conflicting configmap: %v", err)
				}
			}
		})

		It("should handle existing resources by updating them", func() {
			By("Reconciling the Dashboard with existing conflicting resources")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that existing ConfigMap is updated")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				if err != nil {
					return false
				}
				// Check if it was updated by checking the content
				return configMap.Data["config.yml"] != "external: config"
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Should now contain Dashboard configuration
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: Conflict Test Dashboard"))
		})
	})

	Context("When processing very large configuration", func() {
		const dashboardName = "test-dashboard-large-config"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			// Create Dashboard with many services and items
			services := make([]homer.Service, 50)
			for i := 0; i < 50; i++ {
				items := make([]homer.Item, 20)
				for j := 0; j < 20; j++ {
					items[j] = homer.Item{
						Parameters: map[string]string{
							"name":     fmt.Sprintf("Service-%d-Item-%d", i, j),
							"subtitle": fmt.Sprintf("This is item %d in service %d with a very long subtitle to test large configurations", j, i),
							"url":      fmt.Sprintf("https://service-%d-item-%d.example.com/path/to/service", i, j),
							"logo":     fmt.Sprintf("https://example.com/logos/service-%d-item-%d.png", i, j),
							"keywords": fmt.Sprintf("service,item,test,large,config,service%d,item%d", i, j),
							"tag":      fmt.Sprintf("v%d.%d", i, j),
							"type":     "GenericWebhook",
						},
					}
				}
				services[i] = homer.Service{
					Parameters: map[string]string{
						"name": fmt.Sprintf("Service Group %d", i),
						"icon": "fas fa-server",
					},
					Items: items,
				}
			}

			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "Large Configuration Test Dashboard",
						Subtitle: "Testing with 50 services and 1000 items",
						Services: services,
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
		})

		It("should handle large configurations without performance issues", func() {
			By("Reconciling the Dashboard with large configuration")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			start := time.Now()
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			duration := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			// Should complete within reasonable time (adjust threshold as needed)
			Expect(duration).To(BeNumerically("<", time.Second*30))

			By("Checking that large ConfigMap is created successfully")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("Large Configuration Test Dashboard"))
			Expect(configYaml).To(ContainSubstring("Service Group 0"))
			Expect(configYaml).To(ContainSubstring("Service Group 49"))
			Expect(configYaml).To(ContainSubstring("Service-0-Item-0"))
			Expect(configYaml).To(ContainSubstring("Service-49-Item-19"))

			// Check that ConfigMap is not too large (k8s has 1MB limit for ConfigMaps)
			configSize := len(configYaml)
			Expect(configSize).To(BeNumerically("<", 1024*1024)) // Less than 1MB
		})
	})
})
