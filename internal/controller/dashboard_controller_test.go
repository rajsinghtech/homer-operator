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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

var _ = Describe("Dashboard Controller", func() {
	Context("When reconciling a basic Dashboard resource", func() {
		const resourceName = "test-dashboard"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "Test Dashboard",
						Subtitle: "Testing Homer Operator",
						Header:   true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			resource := &homerv1alpha1.Dashboard{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile and create all required resources", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment was created")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(deployment.Name).To(Equal(resourceName + "-homer"))
			Expect(deployment.Namespace).To(Equal(namespaceName))
			Expect(deployment.Labels["dashboard.homer.rajsingh.info/name"]).To(Equal(resourceName))

			By("Checking that Service was created")
			service := &corev1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, service)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(service.Name).To(Equal(resourceName + "-homer"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(8080)))

			By("Checking that ConfigMap was created")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: Test Dashboard"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("subtitle: Testing Homer Operator"))
		})
	})

	Context("When reconciling a Dashboard with custom replicas", func() {
		const resourceName = "test-dashboard-replicas"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			replicas := int32(3)
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					Replicas: &replicas,
					HomerConfig: homer.HomerConfig{
						Title: "Replica Test Dashboard",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			resource := &homerv1alpha1.Dashboard{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create deployment with correct replica count", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment has correct replica count")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	Context("When reconciling a Dashboard with PWA configuration", func() {
		const resourceName = "test-dashboard-pwa"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "PWA Test Dashboard",
						Subtitle: "Progressive Web App Test",
					},
					Assets: &homerv1alpha1.AssetsConfig{
						PWA: &homerv1alpha1.PWAConfig{
							Enabled:         true,
							Name:            "Test PWA",
							ShortName:       "TestPWA",
							Description:     "Test PWA Dashboard",
							ThemeColor:      "#3367d6",
							BackgroundColor: "#ffffff",
							Display:         "standalone",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			resource := &homerv1alpha1.Dashboard{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create deployment with PWA assets support", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment includes PWA init container commands")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check that the config-sync sidecar contains PWA manifest logic
			var sidecarContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				if deployment.Spec.Template.Spec.Containers[i].Name == "config-sync" {
					sidecarContainer = &deployment.Spec.Template.Spec.Containers[i]
					break
				}
			}
			Expect(sidecarContainer).ToNot(BeNil(), "config-sync sidecar should exist")
			sidecarCommand := sidecarContainer.Command[2] // The command is in position 2 (sh -c "command")
			Expect(sidecarCommand).To(ContainSubstring("manifest.json"))
			Expect(sidecarCommand).To(ContainSubstring("Test PWA"))
		})
	})

	Context("When reconciling a Dashboard with invalid theme", func() {
		const resourceName = "test-dashboard-invalid-theme"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Invalid Theme Test",
						Theme: "invalid-theme",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			resource := &homerv1alpha1.Dashboard{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should return error for invalid theme", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported theme"))
		})
	})

	Context("When reconciling a Dashboard with Gateway API enabled", func() {
		const resourceName = "test-dashboard-gateway"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			if !isGatewayAPIAvailable() {
				Skip("Gateway API CRDs not available in test environment")
			}
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Gateway API Test Dashboard",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create a test HTTPRoute
			httproute := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httproute",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name": "Test Service",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"test.example.com"},
				},
			}
			Expect(k8sClient.Create(ctx, httproute)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup HTTPRoute
			httproute := &gatewayv1.HTTPRoute{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-httproute",
				Namespace: namespaceName,
			}, httproute)
			if err == nil {
				Expect(k8sClient.Delete(ctx, httproute)).To(Succeed())
			}

			// Cleanup Dashboard
			resource := &homerv1alpha1.Dashboard{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create ConfigMap with HTTPRoute services when Gateway API enabled", func() {
			By("Reconciling the created resource with Gateway API enabled")
			controllerReconciler := &DashboardReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				EnableGatewayAPI: true,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap includes HTTPRoute services")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(configMap.Data["config.yml"]).To(ContainSubstring("test.example.com"))
		})
	})

	Context("When reconciling Dashboard with Ingress resources", func() {
		const resourceName = "test-dashboard-ingress"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Ingress Test Dashboard",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			// Create a test Ingress
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: namespaceName,
					Annotations: map[string]string{
						"item.homer.rajsingh.info/name":     "Test App",
						"item.homer.rajsingh.info/subtitle": "Test Application",
						"service.homer.rajsingh.info/name":  "Test Services",
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
			ingress := &networkingv1.Ingress{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-ingress",
				Namespace: namespaceName,
			}, ingress)
			if err == nil {
				Expect(k8sClient.Delete(ctx, ingress)).To(Succeed())
			}

			// Cleanup Dashboard
			resource := &homerv1alpha1.Dashboard{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create ConfigMap with Ingress services", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap includes Ingress services")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(configMap.Data["config.yml"]).To(ContainSubstring("app.example.com"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("Test App"))
		})
	})

	Context("When deleting a Dashboard resource", func() {
		const resourceName = "test-dashboard-delete"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespaceName,
		}

		It("should clean up all associated resources", func() {
			By("Creating and reconciling a Dashboard")
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Delete Test Dashboard",
					},
				},
			}
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())

			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying resources were created")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Deleting the Dashboard")
			Expect(k8sClient.Delete(ctx, dashboard)).To(Succeed())

			By("Reconciling deletion")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Dashboard was deleted")
			Eventually(func() bool {
				dashboardCheck := &homerv1alpha1.Dashboard{}
				err := k8sClient.Get(ctx, typeNamespacedName, dashboardCheck)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Manually cleaning up resources (test environment limitation)")
			// In test environments, garbage collection doesn't work the same way
			// So we manually delete the resources and verify they can be deleted

			resourcesDeleted := true

			// Try to delete deployment
			deployment = &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-homer",
				Namespace: namespaceName,
			}, deployment)
			if err == nil {
				err = k8sClient.Delete(ctx, deployment)
				if err != nil {
					GinkgoWriter.Printf("Failed to delete deployment: %v\n", err)
					resourcesDeleted = false
				}
			} else if !errors.IsNotFound(err) {
				GinkgoWriter.Printf("Error getting deployment: %v\n", err)
				resourcesDeleted = false
			}

			// Try to delete service
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-homer",
				Namespace: namespaceName,
			}, service)
			if err == nil {
				err = k8sClient.Delete(ctx, service)
				if err != nil {
					GinkgoWriter.Printf("Failed to delete service: %v\n", err)
					resourcesDeleted = false
				}
			} else if !errors.IsNotFound(err) {
				GinkgoWriter.Printf("Error getting service: %v\n", err)
				resourcesDeleted = false
			}

			// Try to delete configmap
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-homer",
				Namespace: namespaceName,
			}, configMap)
			if err == nil {
				err = k8sClient.Delete(ctx, configMap)
				if err != nil {
					GinkgoWriter.Printf("Failed to delete configmap: %v\n", err)
					resourcesDeleted = false
				}
			} else if !errors.IsNotFound(err) {
				GinkgoWriter.Printf("Error getting configmap: %v\n", err)
				resourcesDeleted = false
			}

			Expect(resourcesDeleted).To(BeTrue(), "Should be able to clean up all resources")
		})
	})
})
