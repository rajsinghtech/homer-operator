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

	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator.git/pkg/homer"
)

const nullIngressValue = "null"

var _ = Describe("Ingress Controller", func() {
	Context("When reconciling an Ingress with matching Dashboard annotations", func() {
		const dashboardName = "test-dashboard-ingress"
		const ingressName = "test-ingress"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var ingress *networkingv1.Ingress

		BeforeEach(func() {
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
			// Cleanup Ingress
			if ingress != nil {
				err := k8sClient.Delete(ctx, ingress)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete ingress: %v", err)
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

		It("should update the ConfigMap with Ingress information", func() {
			By("Reconciling the Ingress resource")
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

			By("Checking that ConfigMap was updated with Ingress information")
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
				return configYaml != "" &&
					configYaml != nullIngressValue // Make sure it's not just an empty YAML
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check for Ingress-specific content
			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("app.test.com"))
			Expect(configYaml).To(ContainSubstring("Test Application"))
		})
	})

	Context("When reconciling an Ingress without matching Dashboard annotations", func() {
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
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete ingress: %v", err)
				}
			}
		})

		It("should reconcile successfully without updating any ConfigMaps", func() {
			By("Reconciling the Ingress resource")
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
		})
	})

	Context("When reconciling an Ingress without rules", func() {
		const dashboardName = "test-dashboard-no-rules"
		const ingressName = "test-ingress-no-rules"
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
						"test-annotation": "test-value",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Test Dashboard No Rules",
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

			// Create Ingress without rules (should be handled gracefully)
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"test-annotation": "test-value",
					},
				},
				Spec: networkingv1.IngressSpec{
					DefaultBackend: &networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "default-service",
							Port: networkingv1.ServiceBackendPort{
								Number: 80,
							},
						},
					},
					Rules: []networkingv1.IngressRule{}, // Empty rules
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

		It("should handle Ingress without rules gracefully", func() {
			By("Reconciling the Ingress resource without rules")
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

			By("Verifying ConfigMap still exists and is valid")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: Test Dashboard No Rules"))
		})
	})

	Context("When reconciling an Ingress with TLS configuration", func() {
		const dashboardName = "test-dashboard-tls"
		const ingressName = "test-ingress-tls"
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
						"tls-test": "enabled",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "TLS Test Dashboard",
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

			// Create Ingress with TLS configuration
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"tls-test":                          "enabled",
						"item.homer.rajsingh.info/name":     "Secure App",
						"item.homer.rajsingh.info/subtitle": "HTTPS Application",
					},
				},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"secure.test.com"},
							SecretName: "tls-secret",
						},
					},
					Rules: []networkingv1.IngressRule{
						{
							Host: "secure.test.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "secure-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 443,
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

		It("should generate HTTPS URL for TLS-enabled Ingress", func() {
			By("Reconciling the TLS-enabled Ingress resource")
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
				return configYaml != "" && configYaml != nullIngressValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("https://secure.test.com"))
			Expect(configYaml).To(ContainSubstring("Secure App"))
		})
	})

	Context("When reconciling with complex annotation patterns", func() {
		const dashboardName = "test-dashboard-complex"
		const ingressName = "test-ingress-complex"
		const namespaceName = "default"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var ingress *networkingv1.Ingress

		BeforeEach(func() {
			// Create Dashboard with subset of annotations
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name":    "complex-app",
						"app.kubernetes.io/version": "v1.0.0",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Complex Test Dashboard",
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

			// Create Ingress with superset of annotations (should match)
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespaceName,
					Annotations: map[string]string{
						"app.kubernetes.io/name":            "complex-app",
						"app.kubernetes.io/version":         "v1.0.0",
						"app.kubernetes.io/component":       "web",         // Extra annotation
						"additional-annotation":             "extra-value", // Extra annotation
						"item.homer.rajsingh.info/name":     "Complex Application",
						"item.homer.rajsingh.info/tag":      "production",
						"item.homer.rajsingh.info/keywords": "app,web,service",
						"service.homer.rajsingh.info/name":  "Complex Services",
						"service.homer.rajsingh.info/icon":  "fas fa-cogs",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "complex.test.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/api/v1",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "complex-service",
													Port: networkingv1.ServiceBackendPort{
														Number: 8080,
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

		It("should match Dashboard when Ingress has superset of annotations", func() {
			By("Reconciling the Ingress with complex annotations")
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

			By("Checking that ConfigMap was updated with complex annotation processing")
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
				return configYaml != "" && configYaml != nullIngressValue
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("complex.test.com"))
			Expect(configYaml).To(ContainSubstring("Complex Application"))
			Expect(configYaml).To(ContainSubstring("production"))
			Expect(configYaml).To(ContainSubstring("app,web,service"))
			Expect(configYaml).To(ContainSubstring("Complex Services"))
		})
	})
})
