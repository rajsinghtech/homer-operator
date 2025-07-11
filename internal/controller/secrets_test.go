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

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/homer"
)

// Helper functions for secrets tests
func createDashboardWithSecrets(name, namespace, secretName string) *homerv1alpha1.Dashboard {
	return &homerv1alpha1.Dashboard{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: homerv1alpha1.DashboardSpec{
			HomerConfig: homer.HomerConfig{
				Title:    "Smart Card Dashboard",
				Subtitle: "Dashboard with Secret Integration",
				Services: []homer.Service{
					{
						Parameters: map[string]string{
							"name": "Media Services",
							"icon": "fas fa-film",
						},
						Items: []homer.Item{
							{
								Parameters: map[string]string{
									"name":     "Plex Server",
									"subtitle": "Media Streaming",
									"type":     "Emby",
									"url":      "https://plex.example.com",
									"endpoint": "/api/v1",
								},
							},
							{
								Parameters: map[string]string{
									"name":     "Sonarr",
									"subtitle": "TV Show Management",
									"type":     "Sonarr",
									"url":      "https://sonarr.example.com",
								},
							},
						},
					},
					{
						Parameters: map[string]string{
							"name": "Monitoring",
							"icon": "fas fa-chart-line",
						},
						Items: []homer.Item{
							{
								Parameters: map[string]string{
									"name":     "Prometheus",
									"subtitle": "Metrics Collection",
									"type":     "Prometheus",
									"url":      "https://prometheus.example.com",
									"endpoint": "/metrics",
								},
							},
							{
								Parameters: map[string]string{
									"name":     "Grafana",
									"subtitle": "Dashboards",
									"type":     "Grafana",
									"url":      "https://grafana.example.com",
								},
							},
						},
					},
				},
			},
			Secrets: &homerv1alpha1.SmartCardSecrets{
				APIKey: &homerv1alpha1.SecretKeyRef{
					Name: secretName,
					Key:  "plex-api-key",
				},
				Token: &homerv1alpha1.SecretKeyRef{
					Name: secretName,
					Key:  "prometheus-token",
				},
				Username: &homerv1alpha1.SecretKeyRef{
					Name: secretName,
					Key:  "grafana-username",
				},
				Password: &homerv1alpha1.SecretKeyRef{
					Name: secretName,
					Key:  "grafana-password",
				},
				Headers: map[string]*homerv1alpha1.SecretKeyRef{
					"Authorization": {
						Name: secretName,
						Key:  "custom-auth-header",
					},
				},
			},
		},
	}
}

func createTestSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"plex-api-key":       []byte("plex-secret-api-key-12345"),
			"sonarr-api-key":     []byte("sonarr-secret-api-key-67890"),
			"prometheus-token":   []byte("prometheus-bearer-token-abcdef"),
			"grafana-username":   []byte("admin"),
			"grafana-password":   []byte("supersecret"),
			"custom-auth-header": []byte("Bearer custom-token-xyz"),
		},
	}
}

func reconcileDashboardTwice(ctx context.Context, reconciler *DashboardReconciler, namespacedName types.NamespacedName) {
	// First reconcile: adds finalizer
	_, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: namespacedName,
	})
	Expect(err).NotTo(HaveOccurred())

	// Second reconcile: processes the Dashboard
	_, err = reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: namespacedName,
	})
	Expect(err).NotTo(HaveOccurred())
}

func waitForConfigMapCreation(ctx context.Context, name string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name + "-homer",
			Namespace: "default",
		}, configMap)
		return err == nil
	}, time.Second*10, time.Millisecond*250).Should(BeTrue())
	return configMap
}

var _ = Describe("Secret Integration Tests", func() {
	Context("When creating Dashboard with secret for smart card API key", func() {
		const dashboardName = "test-dashboard-secrets"
		const secretName = "smart-card-secrets"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var secret *corev1.Secret

		BeforeEach(func() {
			// Create Secret with API keys for smart cards
			secret = createTestSecret(secretName, namespaceName)
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create Dashboard with smart card services that use secrets
			dashboard = createDashboardWithSecrets(dashboardName, namespaceName, secretName)
			Expect(k8sClient.Create(ctx, dashboard)).To(Succeed())
		})

		AfterEach(func() {
			if dashboard != nil {
				err := k8sClient.Delete(ctx, dashboard)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard: %v", err)
				}
			}
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret: %v", err)
				}
			}
		})

		It("should resolve secrets and inject API keys into smart card items", func() {
			By("Reconciling the Dashboard with secret references")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			reconcileDashboardTwice(ctx, controllerReconciler, typeNamespacedName)

			By("Checking that ConfigMap contains resolved API keys")
			configMap := waitForConfigMapCreation(ctx, dashboardName)

			configYaml := configMap.Data["config.yml"]

			// For smart card items with Type field, API keys should be injected
			// The secret values should appear in the configuration
			Expect(configYaml).To(ContainSubstring("plex-secret-api-key-12345"))

			// Check that the structure includes the smart card configuration
			Expect(configYaml).To(ContainSubstring("Plex Server"))
			Expect(configYaml).To(ContainSubstring("type: Emby"))
			Expect(configYaml).To(ContainSubstring("endpoint: /api/v1"))

			Expect(configYaml).To(ContainSubstring("Sonarr"))
			Expect(configYaml).To(ContainSubstring("type: Sonarr"))

			Expect(configYaml).To(ContainSubstring("Prometheus"))
			Expect(configYaml).To(ContainSubstring("type: Prometheus"))

			Expect(configYaml).To(ContainSubstring("Grafana"))
			Expect(configYaml).To(ContainSubstring("type: Grafana"))
		})
	})

	Context("When creating Dashboard with missing secret", func() {
		const dashboardName = "test-dashboard-missing-secret"
		const missingSecretName = "non-existent-secret"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard

		BeforeEach(func() {
			// Create Dashboard that references a non-existent secret
			dashboard = createDashboardWithSecrets(dashboardName, namespaceName, missingSecretName)
			dashboard.Spec.HomerConfig.Title = "Missing Secret Dashboard"
			dashboard.Spec.HomerConfig.Services = []homer.Service{
				{
					Parameters: map[string]string{
						"name": "Test Services",
					},
					Items: []homer.Item{
						{
							Parameters: map[string]string{
								"name": "Test Smart Card",
								"type": "Prometheus",
								"url":  "https://test.example.com",
							},
						},
					},
				},
			}
			dashboard.Spec.Secrets = &homerv1alpha1.SmartCardSecrets{
				APIKey: &homerv1alpha1.SecretKeyRef{
					Name: missingSecretName,
					Key:  "api-key",
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

		It("should return error when secret is not found", func() {
			By("Reconciling the Dashboard with missing secret reference")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile: adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: should fail
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret default/non-existent-secret"))
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("When creating Dashboard with secret missing key", func() {
		const dashboardName = "test-dashboard-missing-key"
		const secretName = "incomplete-secret"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var secret *corev1.Secret

		BeforeEach(func() {
			// Create Secret without the required key
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespaceName,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"other-key": []byte("some-value"),
					// Missing "api-key" that Dashboard expects
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create Dashboard that references a key that doesn't exist in the secret
			dashboard = createDashboardWithSecrets(dashboardName, namespaceName, secretName)
			dashboard.Spec.HomerConfig.Title = "Missing Key Dashboard"
			dashboard.Spec.HomerConfig.Services = []homer.Service{
				{
					Parameters: map[string]string{
						"name": "Test Services",
					},
					Items: []homer.Item{
						{
							Parameters: map[string]string{
								"name": "Test Smart Card",
								"type": "Prometheus",
								"url":  "https://test.example.com",
							},
						},
					},
				},
			}
			dashboard.Spec.Secrets = &homerv1alpha1.SmartCardSecrets{
				APIKey: &homerv1alpha1.SecretKeyRef{
					Name: secretName,
					Key:  "api-key", // This key doesn't exist in the secret
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
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret: %v", err)
				}
			}
		})

		It("should return error when secret key is not found", func() {
			By("Reconciling the Dashboard with missing secret key")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile: adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: should fail with missing key
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret default/incomplete-secret"))
			Expect(err.Error()).To(ContainSubstring("key api-key not found"))
		})
	})

	Context("When creating Dashboard with cross-namespace secret reference", func() {
		const dashboardName = "test-dashboard-cross-ns-secret"
		const secretName = "cross-namespace-secret"
		const secretNamespace = "secret-namespace"
		const dashboardNamespace = "dashboard-namespace"

		ctx := context.Background()

		var dashboard *homerv1alpha1.Dashboard
		var secret *corev1.Secret
		var secretNs *corev1.Namespace
		var dashboardNs *corev1.Namespace

		BeforeEach(func() {
			// Create namespaces with unique names to avoid conflicts
			secretNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: secretNamespace + "-",
				},
			}
			Expect(k8sClient.Create(ctx, secretNs)).To(Succeed())

			dashboardNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: dashboardNamespace + "-",
				},
			}
			Expect(k8sClient.Create(ctx, dashboardNs)).To(Succeed())

			// Create Secret in different namespace
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: secretNs.Name, // Use generated namespace name
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"cross-ns-api-key": []byte("cross-namespace-secret-value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create Dashboard that references secret from different namespace
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: dashboardNs.Name, // Use generated namespace name
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Cross-Namespace Secret Dashboard",
						Services: []homer.Service{
							{
								Parameters: map[string]string{
									"name": "Cross-NS Services",
								},
								Items: []homer.Item{
									{
										Parameters: map[string]string{
											"name": "Cross-NS Smart Card",
											"type": "Prometheus",
											"url":  "https://cross-ns.example.com",
										},
									},
								},
							},
						},
					},
					Secrets: &homerv1alpha1.SmartCardSecrets{
						APIKey: &homerv1alpha1.SecretKeyRef{
							Name:      secretName,
							Key:       "cross-ns-api-key",
							Namespace: secretNs.Name, // Use generated namespace name
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
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret: %v", err)
				}
			}
			if dashboardNs != nil {
				err := k8sClient.Delete(ctx, dashboardNs)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete dashboard namespace: %v", err)
				}
			}
			if secretNs != nil {
				err := k8sClient.Delete(ctx, secretNs)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret namespace: %v", err)
				}
			}
		})

		It("should resolve secrets from different namespace", func() {
			By("Reconciling the Dashboard with cross-namespace secret reference")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			namespacedName := types.NamespacedName{
				Name:      dashboardName,
				Namespace: dashboardNs.Name, // Use generated namespace name
			}

			reconcileDashboardTwice(ctx, controllerReconciler, namespacedName)

			By("Checking that ConfigMap contains resolved cross-namespace secret")
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      dashboardName + "-homer",
					Namespace: dashboardNs.Name,
				}, configMap)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("cross-namespace-secret-value"))
			Expect(configYaml).To(ContainSubstring("Cross-NS Smart Card"))
		})
	})

	Context("When creating Dashboard with secret but no smart card items", func() {
		const dashboardName = "test-dashboard-secret-no-smart-cards"
		const secretName = "unused-secret"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var secret *corev1.Secret

		BeforeEach(func() {
			// Create Secret
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespaceName,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"unused-api-key": []byte("unused-secret-value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create Dashboard with secret config but no smart card items (no Type field)
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "No Smart Cards Dashboard",
						Services: []homer.Service{
							{
								Parameters: map[string]string{
									"name": "Regular Services",
								},
								Items: []homer.Item{
									{
										Parameters: map[string]string{
											"name":     "Regular Link",
											"subtitle": "No smart card",
											"url":      "https://regular.example.com",
											// No Type field = not a smart card
										},
									},
								},
							},
						},
					},
					Secrets: &homerv1alpha1.SmartCardSecrets{
						APIKey: &homerv1alpha1.SecretKeyRef{
							Name: secretName,
							Key:  "unused-api-key",
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
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret: %v", err)
				}
			}
		})

		It("should reconcile successfully without processing secrets", func() {
			By("Reconciling the Dashboard with secrets but no smart cards")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			reconcileDashboardTwice(ctx, controllerReconciler, typeNamespacedName)

			By("Checking that ConfigMap is created without secret injection")
			configMap := waitForConfigMapCreation(ctx, dashboardName)

			configYaml := configMap.Data["config.yml"]
			// Should contain the regular service but not inject secret values
			Expect(configYaml).To(ContainSubstring("Regular Link"))
			Expect(configYaml).To(ContainSubstring("https://regular.example.com"))
			// Should NOT contain the secret value since there are no smart cards
			Expect(configYaml).NotTo(ContainSubstring("unused-secret-value"))
		})
	})

	Context("When creating Dashboard with empty secret values", func() {
		const dashboardName = "test-dashboard-empty-secret-values"
		const secretName = "empty-secret-values"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var secret *corev1.Secret

		BeforeEach(func() {
			// Create Secret with empty values
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespaceName,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"empty-api-key":  []byte(""),    // Empty value
					"whitespace-key": []byte("   "), // Whitespace only
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create Dashboard with smart card that references empty secret values
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Empty Secret Values Dashboard",
						Services: []homer.Service{
							{
								Parameters: map[string]string{
									"name": "Test Services",
								},
								Items: []homer.Item{
									{
										Parameters: map[string]string{
											"name": "Empty API Key Service",
											"type": "Prometheus",
											"url":  "https://empty.example.com",
										},
									},
								},
							},
						},
					},
					Secrets: &homerv1alpha1.SmartCardSecrets{
						APIKey: &homerv1alpha1.SecretKeyRef{
							Name: secretName,
							Key:  "empty-api-key",
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
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret: %v", err)
				}
			}
		})

		It("should handle empty secret values gracefully", func() {
			By("Reconciling the Dashboard with empty secret values")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			reconcileDashboardTwice(ctx, controllerReconciler, typeNamespacedName)

			By("Checking that ConfigMap is created with empty API key handling")
			configMap := waitForConfigMapCreation(ctx, dashboardName)

			configYaml := configMap.Data["config.yml"]
			Expect(configYaml).To(ContainSubstring("Empty API Key Service"))
			Expect(configYaml).To(ContainSubstring("type: Prometheus"))
			// Configuration should be valid even with empty secret values
		})
	})

	Context("When creating Dashboard with multiple secret types", func() {
		const dashboardName = "test-dashboard-multiple-secrets"
		const secretName = "multi-type-secrets"
		const namespaceName = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      dashboardName,
			Namespace: namespaceName,
		}

		var dashboard *homerv1alpha1.Dashboard
		var secret *corev1.Secret

		BeforeEach(func() {
			// Create Secret with multiple types of credentials
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespaceName,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"api-key":        []byte("multi-type-api-key-123"),
					"bearer-token":   []byte("Bearer multi-type-token-456"),
					"basic-username": []byte("admin"),
					"basic-password": []byte("password123"),
					"custom-header":  []byte("Custom-Value-789"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create Dashboard with multiple smart card types using different secret fields
			dashboard = &homerv1alpha1.Dashboard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dashboardName,
					Namespace: namespaceName,
				},
				Spec: homerv1alpha1.DashboardSpec{
					HomerConfig: homer.HomerConfig{
						Title: "Multiple Secret Types Dashboard",
						Services: []homer.Service{
							{
								Parameters: map[string]string{
									"name": "Multi-Auth Services",
								},
								Items: []homer.Item{
									{
										Parameters: map[string]string{
											"name": "API Key Service",
											"type": "Sonarr",
											"url":  "https://apikey.example.com",
										},
									},
									{
										Parameters: map[string]string{
											"name": "Token Service",
											"type": "Prometheus",
											"url":  "https://token.example.com",
										},
									},
									{
										Parameters: map[string]string{
											"name": "Basic Auth Service",
											"type": "Grafana",
											"url":  "https://basic.example.com",
										},
									},
									{
										Parameters: map[string]string{
											"name": "Custom Header Service",
											"type": "GenericWebhook",
											"url":  "https://custom.example.com",
										},
									},
								},
							},
						},
					},
					Secrets: &homerv1alpha1.SmartCardSecrets{
						APIKey: &homerv1alpha1.SecretKeyRef{
							Name: secretName,
							Key:  "api-key",
						},
						Token: &homerv1alpha1.SecretKeyRef{
							Name: secretName,
							Key:  "bearer-token",
						},
						Username: &homerv1alpha1.SecretKeyRef{
							Name: secretName,
							Key:  "basic-username",
						},
						Password: &homerv1alpha1.SecretKeyRef{
							Name: secretName,
							Key:  "basic-password",
						},
						Headers: map[string]*homerv1alpha1.SecretKeyRef{
							"X-Custom-Auth": {
								Name: secretName,
								Key:  "custom-header",
							},
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
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				if err != nil {
					GinkgoT().Logf("Warning: failed to delete secret: %v", err)
				}
			}
		})

		It("should resolve all secret types correctly", func() {
			By("Reconciling the Dashboard with multiple secret types")
			controllerReconciler := &DashboardReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			reconcileDashboardTwice(ctx, controllerReconciler, typeNamespacedName)

			By("Checking that ConfigMap contains all resolved secret values")
			configMap := waitForConfigMapCreation(ctx, dashboardName)

			configYaml := configMap.Data["config.yml"]

			// Check that all secret values are properly injected
			Expect(configYaml).To(ContainSubstring("multi-type-api-key-123"))
			Expect(configYaml).To(ContainSubstring("Bearer multi-type-token-456"))
			Expect(configYaml).To(ContainSubstring("admin"))
			Expect(configYaml).To(ContainSubstring("password123"))
			Expect(configYaml).To(ContainSubstring("Custom-Value-789"))

			// Check that all service types are preserved
			Expect(configYaml).To(ContainSubstring("type: Sonarr"))
			Expect(configYaml).To(ContainSubstring("type: Prometheus"))
			Expect(configYaml).To(ContainSubstring("type: Grafana"))
			Expect(configYaml).To(ContainSubstring("type: GenericWebhook"))
		})
	})
})
