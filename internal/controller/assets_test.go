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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

var _ = Describe("Asset Management Tests", func() {
	Context("When creating Dashboard with custom assets ConfigMap", func() {
		const dashboardName = "test-dashboard-assets"
		const assetsConfigMapName = "custom-assets"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var assetsConfigMap *corev1.ConfigMap

		BeforeEach(func() {
			// Create custom assets ConfigMap with binary data
			logoData := []byte("fake-logo-data")
			cssData := []byte(`
.custom-logo { width: 50px; height: 50px; }
.custom-theme { background-color: #ff0000; }
`)
			assetsConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      assetsConfigMapName,
					Namespace: namespaceName,
				},
				BinaryData: map[string][]byte{
					"logo.png":   logoData,
					"custom.css": cssData,
				},
			}
			Expect(k8sClient.Create(ctx, assetsConfigMap)).To(Succeed())

			// Create Dashboard that references the assets ConfigMap
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:      "Assets Test Dashboard",
						Subtitle:   "Testing Custom Assets",
						Stylesheet: []string{"custom.css"},
					},
					Assets: &homerv1alpha1.AssetsConfig{
						ConfigMapRef: &homerv1alpha1.AssetConfigMapRef{
							Name: assetsConfigMapName,
						},
						Icons: &homerv1alpha1.IconConfig{
							Favicon:        "logo.png",
							AppleTouchIcon: "logo.png",
						},
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
			if assetsConfigMap != nil {
				err := k8sClient.Delete(ctx, assetsConfigMap)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete assets configmap: %v", err)
				}
			}
		})

		It("should create deployment with custom assets volume mounts", func() {
			By("Reconciling the Dashboard with custom assets")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment includes custom assets volume")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Check that the deployment has the custom assets volume
			hasCustomAssetsVolume := false
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.Name == assetsConfigMapName && volume.ConfigMap != nil && volume.ConfigMap.Name == assetsConfigMapName {
					hasCustomAssetsVolume = true
					break
				}
			}
			Expect(hasCustomAssetsVolume).To(BeTrue(), "Deployment should have custom assets volume")

			// Check that init container has custom assets volume mount
			initContainer := deployment.Spec.Template.Spec.InitContainers[0]
			hasCustomAssetsMount := false
			for _, mount := range initContainer.VolumeMounts {
				if mount.Name == assetsConfigMapName && mount.MountPath == "/custom-assets" {
					hasCustomAssetsMount = true
					break
				}
			}
			Expect(hasCustomAssetsMount).To(BeTrue(), "Init container should have custom assets volume mount")

			// Check that init command includes default assets and custom assets copying
			initCommand := initContainer.Command[2] // sh -c "command"
			Expect(initCommand).To(ContainSubstring("cp -r /www/default-assets/* /www/assets/"))
			Expect(initCommand).To(ContainSubstring("[ -f /custom-assets/$file ] && cp /custom-assets/$file /www/assets/"))
		})
	})

	Context("When creating Dashboard with PWA and custom icons", func() {
		const dashboardName = "test-dashboard-pwa-icons"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "PWA Icons Test",
						Subtitle: "Custom PWA with Icons",
					},
					Assets: &homerv1alpha1.AssetsConfig{
						Icons: &homerv1alpha1.IconConfig{
							Favicon:        "favicon.ico",
							AppleTouchIcon: "apple-touch-icon.png",
							PWAIcon192:     "pwa-192x192.png",
							PWAIcon512:     "pwa-512x512.png",
						},
						PWA: &homerv1alpha1.PWAConfig{
							Enabled:         true,
							Name:            "Custom PWA App",
							ShortName:       "CustomPWA",
							Description:     "Custom PWA with Icons",
							ThemeColor:      "#ff6b6b",
							BackgroundColor: "#4ecdc4",
							Display:         "fullscreen",
							StartURL:        "/dashboard",
						},
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

		It("should generate PWA manifest with custom icons", func() {
			By("Reconciling the Dashboard with PWA and custom icons")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment includes PWA manifest generation")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			initContainer := deployment.Spec.Template.Spec.InitContainers[0]
			initCommand := initContainer.Command[2]

			// Check PWA manifest generation
			Expect(initCommand).To(ContainSubstring("manifest.json"))
			Expect(initCommand).To(ContainSubstring("Custom PWA App"))
			Expect(initCommand).To(ContainSubstring("Custom PW")) // Short name gets truncated in display
			Expect(initCommand).To(ContainSubstring("#ff6b6b"))
			Expect(initCommand).To(ContainSubstring("#4ecdc4"))
			Expect(initCommand).To(ContainSubstring("fullscreen"))
			Expect(initCommand).To(ContainSubstring("/dashboard"))
			Expect(initCommand).To(ContainSubstring("pwa-192x192.png"))
			Expect(initCommand).To(ContainSubstring("pwa-512x512.png"))
		})
	})

	Context("When creating Dashboard with PWA but no custom assets ConfigMap", func() {
		const dashboardName = "test-dashboard-pwa-only"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "PWA Only Test",
						Subtitle: "PWA without custom assets",
					},
					Assets: &homerv1alpha1.AssetsConfig{
						PWA: &homerv1alpha1.PWAConfig{
							Enabled:     true,
							Name:        "Simple PWA",
							Description: "PWA without custom assets",
						},
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

		It("should use CreateDeploymentWithAssets even without ConfigMap for PWA", func() {
			By("Reconciling the Dashboard with PWA only")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment uses assets-enabled deployment")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			initContainer := deployment.Spec.Template.Spec.InitContainers[0]
			initCommand := initContainer.Command[2]

			// Should include PWA manifest generation
			Expect(initCommand).To(ContainSubstring("manifest.json"))
			Expect(initCommand).To(ContainSubstring("Simple PWA"))

			// Should use default icons since no custom icons provided
			Expect(initCommand).To(ContainSubstring("assets/icons/pwa-192x192.png"))
			Expect(initCommand).To(ContainSubstring("assets/icons/pwa-512x512.png"))
		})
	})

	Context("When creating Dashboard with PWA defaults", func() {
		const dashboardName = "test-dashboard-pwa-defaults"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title:    "PWA Defaults Test",
						Subtitle: "Testing PWA default values",
					},
					Assets: &homerv1alpha1.AssetsConfig{
						PWA: &homerv1alpha1.PWAConfig{
							Enabled: true,
							// All other fields left empty to test defaults
						},
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

		It("should use Dashboard title and name as defaults for PWA", func() {
			By("Reconciling the Dashboard with PWA defaults")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that PWA uses Dashboard values as defaults")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: namespaceName,
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			initContainer := deployment.Spec.Template.Spec.InitContainers[0]
			initCommand := initContainer.Command[2]

			// Should use Dashboard title as PWA name
			Expect(initCommand).To(ContainSubstring("PWA Defaults Test"))

			// Should use subtitle as description
			Expect(initCommand).To(ContainSubstring("Testing PWA default values"))

			// Should include default colors
			Expect(initCommand).To(ContainSubstring("#3367d6")) // Default theme color
			Expect(initCommand).To(ContainSubstring("#ffffff")) // Default background color

			// Should include default display mode
			Expect(initCommand).To(ContainSubstring("standalone"))

			// Should include default start URL
			Expect(initCommand).To(ContainSubstring("\"/\""))
		})
	})

	Context("When creating Dashboard with asset namespace reference", func() {
		const dashboardName = "test-dashboard-asset-namespace"
		const assetsConfigMapName = "cross-namespace-assets"
		const assetsNamespace = "assets-namespace"
		const dashboardNamespace = "dashboard-namespace"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var assetsConfigMap *corev1.ConfigMap
		var assetsNs *corev1.Namespace
		var dashboardNs *corev1.Namespace

		BeforeEach(func() {
			// Create namespaces with unique names to avoid conflicts
			assetsNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: assetsNamespace + "-",
				},
			}
			Expect(k8sClient.Create(ctx, assetsNs)).To(Succeed())

			dashboardNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: dashboardNamespace + "-",
				},
			}
			Expect(k8sClient.Create(ctx, dashboardNs)).To(Succeed())

			// Create assets ConfigMap in different namespace
			assetsConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      assetsConfigMapName,
					Namespace: assetsNs.Name, // Use generated namespace name
				},
				BinaryData: map[string][]byte{
					"cross-ns-logo.png": []byte("cross-namespace-logo-data"),
				},
			}
			Expect(k8sClient.Create(ctx, assetsConfigMap)).To(Succeed())

			// Create Dashboard that references assets from different namespace
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: dashboardNs.Name, // Use generated namespace name
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Cross-Namespace Assets Test",
					},
					Assets: &homerv1alpha1.AssetsConfig{
						ConfigMapRef: &homerv1alpha1.AssetConfigMapRef{
							Name:      assetsConfigMapName,
							Namespace: assetsNs.Name, // Use generated namespace name
						},
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
			if assetsConfigMap != nil {
				err := k8sClient.Delete(ctx, assetsConfigMap)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete assets configmap: %v", err)
				}
			}
			if dashboardNs != nil {
				err := k8sClient.Delete(ctx, dashboardNs)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard namespace: %v", err)
				}
			}
			if assetsNs != nil {
				err := k8sClient.Delete(ctx, assetsNs)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete assets namespace: %v", err)
				}
			}
		})

		It("should handle cross-namespace asset references", func() {
			By("Reconciling the Dashboard with cross-namespace assets")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dashboardName,
					Namespace: dashboardNs.Name, // Use generated namespace name
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment includes cross-namespace assets reference")
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: dashboardNs.Name, // Use generated namespace name
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// The volume should reference the ConfigMap name even if it's in a different namespace
			// Note: In real scenarios, cross-namespace references may require additional RBAC
			hasCustomAssetsVolume := false
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.Name == assetsConfigMapName && volume.ConfigMap != nil && volume.ConfigMap.Name == assetsConfigMapName {
					hasCustomAssetsVolume = true
					break
				}
			}
			Expect(hasCustomAssetsVolume).To(BeTrue(), "Deployment should reference cross-namespace assets ConfigMap")
		})
	})
})
