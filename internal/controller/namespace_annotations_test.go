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

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

var _ = Describe("Namespace Annotation Inheritance", func() {
	Context("When namespace has Homer annotations", func() {
		const resourceName = "test-dashboard-ns-annotations"
		const namespaceName = "test-namespace-annotations"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		BeforeEach(func() {
			// Create test namespace with Homer annotations
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
					Annotations: map[string]string{
						"service.homer.rajsingh.info/name":        "AI Services",
						"service.homer.rajsingh.info/icon":        "fas fa-robot",
						"service.homer.rajsingh.info/displayName": "AI & ML",
						"item.homer.rajsingh.info/tag":            "AI",
						"item.homer.rajsingh.info/tagStyle":       "is-info",
					},
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

			// Create Dashboard
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Namespace Annotations Test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create an Ingress without any Homer annotations (should inherit from namespace)
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-inherit",
					Namespace: namespaceName,
					// No annotations - should inherit from namespace
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "chatgpt.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "chatgpt-api",
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
			// Cleanup Ingress
			ingress := &networkingv1.Ingress{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-ingress-inherit",
				Namespace: namespaceName,
			}, ingress)
			if err == nil {
				Expect(k8sClient.Delete(ctx, ingress)).To(Succeed())
			}

			// Cleanup Dashboard
			dashboard := &homerv1alpha1.Dashboard{}
			err = k8sClient.Get(ctx, typeNamespacedName, dashboard)
			if err == nil {
				Expect(k8sClient.Delete(ctx, dashboard)).To(Succeed())
			}

			// Cleanup Namespace
			namespace := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
			if err == nil {
				Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
			}
		})

		It("should inherit namespace annotations in ConfigMap", func() {
			By("Reconciling the dashboard")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile: adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap includes namespace annotations")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check that the config contains namespace annotation values
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("AI Services"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("fas fa-robot"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("chatgpt.example.com"))
		})
	})

	Context("When resource annotations override namespace annotations", func() {
		const resourceName = "test-dashboard-override"
		const namespaceName = "test-namespace-override"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		BeforeEach(func() {
			// Create namespace with default annotations
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
					Annotations: map[string]string{
						"service.homer.rajsingh.info/name": "Default Services",
						"service.homer.rajsingh.info/icon": "fas fa-server",
						"item.homer.rajsingh.info/tag":     "default",
					},
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

			// Create Dashboard
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Override Test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create an Ingress with its own annotations (should override namespace)
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-override",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"item.homer.rajsingh.info/icon": "fas fa-custom", // Override namespace default
						"item.homer.rajsingh.info/tag":  "custom",        // Override namespace tag
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "custom.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "custom-service",
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
			// Cleanup
			ingress := &networkingv1.Ingress{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-ingress-override",
				Namespace: namespaceName,
			}, ingress)
			if err == nil {
				Expect(k8sClient.Delete(ctx, ingress)).To(Succeed())
			}

			dashboard := &homerv1alpha1.Dashboard{}
			err = k8sClient.Get(ctx, typeNamespacedName, dashboard)
			if err == nil {
				Expect(k8sClient.Delete(ctx, dashboard)).To(Succeed())
			}

			namespace := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
			if err == nil {
				Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
			}
		})

		It("should use resource annotations over namespace annotations", func() {
			By("Reconciling the dashboard")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap uses resource annotations")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Should contain resource-level overrides
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("fas fa-custom"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("custom"))
			// Should still inherit service name from namespace
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("Default Services"))
		})
	})

	Context("When namespace has no annotations", func() {
		const resourceName = "test-dashboard-no-ns-ann"
		const namespaceName = "test-namespace-no-annotations"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		BeforeEach(func() {
			// Create namespace without annotations
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
					// No annotations
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "No NS Annotations Test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-no-ns",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "My App",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "app.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "app-service",
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
			ingress := &networkingv1.Ingress{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-ingress-no-ns",
				Namespace: namespaceName,
			}, ingress)
			if err == nil {
				Expect(k8sClient.Delete(ctx, ingress)).To(Succeed())
			}

			dashboard := &homerv1alpha1.Dashboard{}
			err = k8sClient.Get(ctx, typeNamespacedName, dashboard)
			if err == nil {
				Expect(k8sClient.Delete(ctx, dashboard)).To(Succeed())
			}

			namespace := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
			if err == nil {
				Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
			}
		})

		It("should work normally with default behavior", func() {
			By("Reconciling the dashboard")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap works with resource annotations only")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(configMap.Data["config.yml"]).To(ContainSubstring("My App"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("app.example.com"))
		})
	})

	Context("When using Gateway API with namespace annotations", func() {
		const resourceName = "test-dashboard-gateway-ns"
		const namespaceName = "test-namespace-gateway"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		BeforeEach(func() {
			if !isGatewayAPIAvailable() {
				Skip("Gateway API CRDs not available in test environment")
			}

			// Create namespace with annotations
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
					Annotations: map[string]string{
						"service.homer.rajsingh.info/name": "Gateway Services",
						"item.homer.rajsingh.info/tag":     "gateway",
					},
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Gateway NS Test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create HTTPRoute without annotations (should inherit from namespace)
			httproute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httproute-ns",
					Namespace: namespaceName,
					// No annotations - inherit from namespace
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"gateway.example.com"},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			httproute := &gatewayv1.HTTPRoute{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-httproute-ns",
				Namespace: namespaceName,
			}, httproute)
			if err == nil {
				Expect(k8sClient.Delete(ctx, httproute)).To(Succeed())
			}

			dashboard := &homerv1alpha1.Dashboard{}
			err = k8sClient.Get(ctx, typeNamespacedName, dashboard)
			if err == nil {
				Expect(k8sClient.Delete(ctx, dashboard)).To(Succeed())
			}

			namespace := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
			if err == nil {
				Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
			}
		})

		It("should inherit namespace annotations for HTTPRoutes", func() {
			By("Reconciling with Gateway API enabled")
			controllerReconciler := &DashboardReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				EnableGatewayAPI: true,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking ConfigMap includes namespace annotations for HTTPRoute")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(configMap.Data["config.yml"]).To(ContainSubstring("Gateway Services"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("gateway"))
		})
	})
})
