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
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

// E2E test helper functions
func setupE2ETest() (client.Client, context.Context, string) {
	ctx := context.Background()
	// Use nanoseconds because all specs create and delete namespaces in quick
	// succession. A seconds-only name can collide while the previous namespace
	// is still terminating, causing an unrelated BeforeEach failure.
	testNs := fmt.Sprintf("homer-e2e-%d", time.Now().UnixNano())

	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	err = homerv1alpha1.AddToScheme(k8sClient.Scheme())
	Expect(err).NotTo(HaveOccurred())
	err = networkingv1.AddToScheme(k8sClient.Scheme())
	Expect(err).NotTo(HaveOccurred())
	err = gatewayv1.Install(k8sClient.Scheme())
	Expect(err).NotTo(HaveOccurred())

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNs,
			Labels: map[string]string{"test": "homer-e2e"},
		},
	}
	err = k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	return k8sClient, ctx, testNs
}

var (
	e2eCleanupTimeout      = 2 * time.Minute
	e2eCleanupPollInterval = time.Second
)

func cleanupE2ETest(k8sClient client.Client, ctx context.Context, testNs string) error {
	dashboards := &homerv1alpha1.DashboardList{}
	if err := k8sClient.List(ctx, dashboards, client.InNamespace(testNs)); err != nil {
		return cleanupFailure(k8sClient, ctx, testNs, "list Dashboards before cleanup", err)
	}

	for i := range dashboards.Items {
		dashboard := &dashboards.Items[i]
		if err := k8sClient.Delete(ctx, dashboard); err != nil && !apierrors.IsNotFound(err) {
			return cleanupFailure(k8sClient, ctx, testNs, fmt.Sprintf("delete Dashboard %s", dashboard.Name), err)
		}
	}

	if err := waitForDashboardsDeleted(k8sClient, ctx, testNs); err != nil {
		return cleanupFailure(k8sClient, ctx, testNs, "wait for Dashboard finalizers and owned resources", err)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNs}}
	err := k8sClient.Delete(ctx, ns)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return cleanupFailure(k8sClient, ctx, testNs, "delete test namespace", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, e2eCleanupPollInterval, e2eCleanupTimeout, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs}, ns)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}); err != nil {
		return cleanupFailure(k8sClient, ctx, testNs, "wait for test namespace deletion", err)
	}

	return nil
}

func waitForDashboardsDeleted(k8sClient client.Client, ctx context.Context, testNs string) error {
	return wait.PollUntilContextTimeout(ctx, e2eCleanupPollInterval, e2eCleanupTimeout, true, func(ctx context.Context) (bool, error) {
		dashboards := &homerv1alpha1.DashboardList{}
		if err := k8sClient.List(ctx, dashboards, client.InNamespace(testNs)); err != nil {
			return false, err
		}
		return len(dashboards.Items) == 0, nil
	})
}

func cleanupFailure(k8sClient client.Client, ctx context.Context, testNs, action string, err error) error {
	return fmt.Errorf("%s for e2e namespace %q: %w\nremaining resources:\n%s", action, testNs, err, cleanupState(k8sClient, ctx, testNs))
}

func cleanupState(k8sClient client.Client, ctx context.Context, testNs string) string {
	var state []string
	appendState := func(kind string, objects []metav1.Object) {
		if len(objects) == 0 {
			return
		}

		for _, object := range objects {
			deleting := object.GetDeletionTimestamp() != nil
			state = append(state, fmt.Sprintf("- %s %s/%s finalizers=%v deleting=%t", kind, object.GetNamespace(), object.GetName(), object.GetFinalizers(), deleting))
		}
	}

	dashboards := &homerv1alpha1.DashboardList{}
	if err := k8sClient.List(ctx, dashboards, client.InNamespace(testNs)); err != nil {
		state = append(state, fmt.Sprintf("- unable to list Dashboards: %v", err))
	} else {
		objects := make([]metav1.Object, 0, len(dashboards.Items))
		for i := range dashboards.Items {
			objects = append(objects, &dashboards.Items[i])
		}
		appendState("Dashboard", objects)
	}

	deployments := &appsv1.DeploymentList{}
	if err := k8sClient.List(ctx, deployments, client.InNamespace(testNs)); err != nil {
		state = append(state, fmt.Sprintf("- unable to list Deployments: %v", err))
	} else {
		objects := make([]metav1.Object, 0, len(deployments.Items))
		for i := range deployments.Items {
			objects = append(objects, &deployments.Items[i])
		}
		appendState("Deployment", objects)
	}

	services := &corev1.ServiceList{}
	if err := k8sClient.List(ctx, services, client.InNamespace(testNs)); err != nil {
		state = append(state, fmt.Sprintf("- unable to list Services: %v", err))
	} else {
		objects := make([]metav1.Object, 0, len(services.Items))
		for i := range services.Items {
			objects = append(objects, &services.Items[i])
		}
		appendState("Service", objects)
	}

	configMaps := &corev1.ConfigMapList{}
	if err := k8sClient.List(ctx, configMaps, client.InNamespace(testNs)); err != nil {
		state = append(state, fmt.Sprintf("- unable to list ConfigMaps: %v", err))
	} else {
		objects := make([]metav1.Object, 0, len(configMaps.Items))
		for i := range configMaps.Items {
			objects = append(objects, &configMaps.Items[i])
		}
		appendState("ConfigMap", objects)
	}

	namespace := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: testNs}, namespace); err != nil && !apierrors.IsNotFound(err) {
		state = append(state, fmt.Sprintf("- unable to get Namespace %s: %v", testNs, err))
	} else if err == nil {
		deleting := namespace.GetDeletionTimestamp() != nil
		state = append(state, fmt.Sprintf("- Namespace %s finalizers=%v deleting=%t", namespace.Name, namespace.Finalizers, deleting))
	}

	if len(state) == 0 {
		return "- none"
	}
	return strings.Join(state, "\n")
}

func environmentValue(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	switch name {
	case "E2E_OPERATOR_DEPLOYMENT":
		return defaultOperatorDeployment
	case "E2E_OPERATOR_NAMESPACE":
		return defaultOperatorNamespace
	default:
		return ""
	}
}

func hasUpdatedDashboardConfig(config string) bool {
	return strings.Contains(config, "title: Updated Title") &&
		strings.Contains(config, "subtitle: Updated Subtitle") &&
		strings.Contains(config, "footer: Updated Footer")
}

const (
	// The defaults match the documented Helm installation. Kustomize installs
	// and CI can target their own deployment through E2E_OPERATOR_* overrides.
	defaultOperatorDeployment = "homer-operator"
	defaultOperatorNamespace  = "homer-operator"
)

var _ = Describe("Homer Operator E2E Tests", func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNs    string
	)

	BeforeEach(func() {
		k8sClient, ctx, testNs = setupE2ETest()
	})

	AfterEach(func() {
		Expect(cleanupE2ETest(k8sClient, ctx, testNs)).To(Succeed())
	})

	Context("When deploying Homer Operator", func() {
		It("should be running and healthy", func() {
			By("Checking that Homer Operator deployment exists")
			deployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      environmentValue("E2E_OPERATOR_DEPLOYMENT"),
				Namespace: environmentValue("E2E_OPERATOR_NAMESPACE"),
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			By("Checking that deployment is ready")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      environmentValue("E2E_OPERATOR_DEPLOYMENT"),
					Namespace: environmentValue("E2E_OPERATOR_NAMESPACE"),
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

			By("Waiting for updated Dashboard configuration")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-update-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				if err != nil {
					return false
				}
				return hasUpdatedDashboardConfig(configMap.Data["config.yml"])
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Verifying updated configuration")
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: Updated Title"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("subtitle: Updated Subtitle"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("footer: Updated Footer"))
		})

		It("should support footer: false to hide footer", func() {
			By("Creating Dashboard with footer: false")
			dashboard := &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-footer-false-dashboard",
					Namespace: testNs,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "Footer False Test",
						Subtitle: "Testing footer opt-out",
						Footer:   homer.FooterHidden,
					},
				},
			}
			err := k8sClient.Create(ctx, dashboard)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for ConfigMap to be created")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-footer-false-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Verifying footer: false is in configuration")
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("title: Footer False Test"))
			Expect(configMap.Data["config.yml"]).To(ContainSubstring("footer: false"))
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

			By("Waiting for Dashboard finalizer removal")
			Eventually(func() bool {
				deletedDashboard := &homerv1alpha1.Dashboard{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard",
					Namespace: testNs,
				}, deletedDashboard)
				return apierrors.IsNotFound(err)
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for Deployment to be deleted")
			Eventually(func() bool {
				deployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, deployment)
				return apierrors.IsNotFound(err)
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for Service to be deleted")
			Eventually(func() bool {
				service := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, service)
				return apierrors.IsNotFound(err)
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for ConfigMap to be deleted")
			Eventually(func() bool {
				configMap := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "e2e-cleanup-dashboard-homer",
					Namespace: testNs,
				}, configMap)
				return apierrors.IsNotFound(err)
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
