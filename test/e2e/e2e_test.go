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

package e2e

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
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator.git/pkg/homer"
)

var _ = Describe("Homer Operator E2E Tests", func() {
	var (
		k8sClient client.Client
		cfg       *rest.Config
		ctx       context.Context
		testNs    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNs = fmt.Sprintf("homer-e2e-%d", time.Now().Unix())

		// Get the Kubernetes config
		var err error
		cfg, err = config.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		// Create the Kubernetes client
		k8sClient, err = client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		// Add scheme for Homer resources
		err = homerv1alpha1.AddToScheme(k8sClient.Scheme())
		Expect(err).NotTo(HaveOccurred())

		err = networkingv1.AddToScheme(k8sClient.Scheme())
		Expect(err).NotTo(HaveOccurred())

		err = gatewayv1.AddToScheme(k8sClient.Scheme())
		Expect(err).NotTo(HaveOccurred())

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNs,
				Labels: map[string]string{
					"test": "homer-e2e",
				},
			},
		}
		err = k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNs,
			},
		}
		err := k8sClient.Delete(ctx, ns)
		if err != nil {
			GinkgoT().Logf("Warning: failed to delete test namespace %s: %v", testNs, err)
		}
	})

	Context("When deploying Homer Operator", func() {
		It("should be running and healthy", func() {
			Skip("Skipping operator deployment test - requires cluster with operator installed")

			By("Checking that Homer Operator deployment exists")
			deployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "homer-operator-controller-manager",
				Namespace: "homer-operator-system",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			By("Checking that deployment is ready")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "homer-operator-controller-manager",
					Namespace: "homer-operator-system",
				}, deployment)
				if err != nil {
					return false
				}
				return deployment.Status.ReadyReplicas > 0
			}, time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})

	Context("When creating Dashboard resources", func() {
		It("should create a complete Homer dashboard deployment", func() {
			By("Creating a Dashboard resource")
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-dashboard",
					Namespace: testNs,
				},
				Spec: homerv1alpha1.DashboardSpec{
					Replicas: func() *int32 { r := int32(1); return &r }(),
					HomerConfig: homer.HomerConfig{
						Title:    "E2E Test Dashboard",
						Subtitle: "End-to-End Testing",
						Header:   true,
						Footer:   "Powered by Homer Operator E2E Tests",
					},
				},
			}
			err := k8sClient.Create(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for Deployment to be created")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-dashboard-homer",
					Namespace: testNs,
				}, deployment)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for Service to be created")
			service := &corev1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-dashboard-homer",
					Namespace: testNs,
				}, service)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for ConfigMap to be created")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Verifying ConfigMap contains correct configuration")
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: E2E Test Dashboard"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("subtitle: End-to-End Testing"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("footer: Powered by Homer Operator E2E Tests"))

			By("Verifying Deployment has correct configuration")
			Expect(deployment.Spec.Replicas).NotTo(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
			Expect(deployment.Labels["dashboard.homer.rajsingh.info/name"]).To(Equal("e2e-dashboard"))

			By("Verifying Service has correct configuration")
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(service.Spec.Selector["dashboard.homer.rajsingh.info/name"]).To(Equal("e2e-dashboard"))
		})

		It("should handle Dashboard updates", func() {
			By("Creating initial Dashboard")
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-update-dashboard",
					Namespace: testNs,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "Original Title",
						Subtitle: "Original Subtitle",
					},
				},
			}
			err := k8sClient.Create(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for initial ConfigMap")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-update-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			originalConfig := configMap.Data["config.yml"]

			By("Updating Dashboard configuration")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-update-dashboard",
				Namespace: testNs,
			}, dashboard)
			Expect(err).NotTo(HaveOccurred())

			dashboard.Spec.HomerConfig.Title = "Updated Title"
			dashboard.Spec.HomerConfig.Subtitle = "Updated Subtitle"
			dashboard.Spec.HomerConfig.Footer = "Updated Footer"

			err = k8sClient.Update(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for ConfigMap to be updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-update-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				if err != nil {
					return false
				}
				return configMap.Data["config.yml"] != originalConfig
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Verifying updated configuration")
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: Updated Title"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("subtitle: Updated Subtitle"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("footer: Updated Footer"))
		})

		It("should clean up resources when Dashboard is deleted", func() {
			By("Creating Dashboard")
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-cleanup-dashboard",
					Namespace: testNs,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Cleanup Test Dashboard",
					},
				},
			}
			err := k8sClient.Create(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for resources to be created")
			Eventually(func() bool {
				deployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, deployment)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Deleting Dashboard")
			err = k8sClient.Delete(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for Deployment to be deleted")
			Eventually(func() bool {
				deployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, deployment)
				return client.IgnoreNotFound(err) == nil && err != nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for Service to be deleted")
			Eventually(func() bool {
				service := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, service)
				return client.IgnoreNotFound(err) == nil && err != nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for ConfigMap to be deleted")
			Eventually(func() bool {
				configMap := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				return client.IgnoreNotFound(err) == nil && err != nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())
		})
	})

	Context("When working with Ingress integration", func() {
		It("should discover services from Ingress resources", func() {
			By("Creating Dashboard")
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-ingress-dashboard",
					Namespace: testNs,
					Annotations: map[string]string{
						"environment": "e2e-test",
					},
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Ingress Integration Dashboard",
					},
				},
			}
			err := k8sClient.Create(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Creating Ingress with matching annotations")
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-test-ingress",
					Namespace: testNs,
					Annotations: map[string]string{
						"environment":                       "e2e-test",
						"item.homer.rajsingh.info/name":     "E2E Test App",
						"item.homer.rajsingh.info/subtitle": "End-to-End Test Application",
						"service.homer.rajsingh.info/name":  "E2E Services",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "e2e.test.local",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "e2e-test-service",
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
			err = k8sClient.Create(ctx, ingress)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for ConfigMap to include Ingress service")
			Eventually(func() bool {
				configMap := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-ingress-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				if err != nil {
					return false
				}
				configYaml := configMap.Data["config.yml"]
				return configYaml != "" &&
					configYaml != "null" &&
					containsSubstring(configYaml, "e2e.test.local") &&
					containsSubstring(configYaml, "E2E Test App")
			}, time.Minute*3, time.Second*10).Should(BeTrue())

			By("Verifying Ingress service appears in dashboard configuration")
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-ingress-dashboard-homer",
				Namespace: testNs,
			}, configMap)
			Expect(err).NotTo(HaveOccurred())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("e2e.test.local"))
			Expect(configYaml).To(ContainSubstring("E2E Test App"))
			Expect(configYaml).To(ContainSubstring("End-to-End Test Application"))
			Expect(configYaml).To(ContainSubstring("E2E Services"))
		})
	})
})

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(len(substr) == 0 ||
			findSubstring(s, substr) != -1)
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
