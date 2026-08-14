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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	"github.com/rajsinghtech/homer-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	// Homer annotation prefixes
	serviceAnnotationPrefix = "service.homer.rajsingh.info/"
	itemAnnotationPrefix    = "item.homer.rajsingh.info/"
)

// ClusterClient represents a client connection to a Kubernetes cluster
type ClusterClient struct {
	Name       string
	Client     client.Client
	Config     *rest.Config
	Connected  bool
	LastError  error
	LastCheck  time.Time
	ClusterCfg *homerv1alpha1.RemoteCluster
}

// ClusterManager manages connections to multiple Kubernetes clusters
type ClusterManager struct {
	localClient  client.Client
	scheme       *runtime.Scheme
	clients      map[string]*ClusterClient
	secretHashes map[string]string // Track secret versions for change detection
	// createClusterClientFn is injectable for focused retry tests. Production
	// connections use createClusterClient directly.
	createClusterClientFn func(context.Context, *homerv1alpha1.Dashboard, *homerv1alpha1.RemoteCluster) (*ClusterClient, error)
	mu                    sync.RWMutex
	log                   logr.Logger
}

// NewClusterManager creates a new ClusterManager instance
func NewClusterManager(localClient client.Client, scheme *runtime.Scheme) *ClusterManager {
	return &ClusterManager{
		localClient:  localClient,
		scheme:       scheme,
		clients:      make(map[string]*ClusterClient),
		secretHashes: make(map[string]string),
		log:          ctrl.Log.WithName("cluster-manager"),
	}
}

// UpdateClusters updates the list of managed clusters based on the Dashboard configuration
func (m *ClusterManager) UpdateClusters(ctx context.Context, dashboard *homerv1alpha1.Dashboard) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateRemoteClusters(dashboard.Spec.RemoteClusters); err != nil {
		return err
	}

	// Track which clusters should remain
	activeClusters := make(map[string]bool)

	// Add/update remote clusters
	for _, clusterCfg := range dashboard.Spec.RemoteClusters {
		if !clusterCfg.Enabled {
			m.log.V(1).Info("Skipping disabled cluster", "cluster", clusterCfg.Name)
			activeClusters[clusterCfg.Name] = true
			if existing, ok := m.clients[clusterCfg.Name]; ok {
				existing.ClusterCfg = &clusterCfg
				existing.Connected = false
				existing.LastError = nil
			} else {
				m.clients[clusterCfg.Name] = &ClusterClient{
					Name:       clusterCfg.Name,
					Connected:  false,
					ClusterCfg: &clusterCfg,
				}
			}
			continue
		}

		activeClusters[clusterCfg.Name] = true

		// Get current secret hash to detect changes
		secretHash, err := m.getSecretHash(ctx, dashboard, &clusterCfg)
		if err != nil {
			m.log.Error(err, "Failed to get secret hash", "cluster", clusterCfg.Name)
			if existing, ok := m.clients[clusterCfg.Name]; ok {
				existing.ClusterCfg = &clusterCfg
				existing.Connected = false
				existing.LastError = err
				existing.LastCheck = time.Now()
			} else {
				m.clients[clusterCfg.Name] = &ClusterClient{
					Name:       clusterCfg.Name,
					Connected:  false,
					LastError:  err,
					LastCheck:  time.Now(),
					ClusterCfg: &clusterCfg,
				}
			}
			activeClusters[clusterCfg.Name] = true
			continue
		}

		// Check if we already have this cluster
		if existing, ok := m.clients[clusterCfg.Name]; ok {
			previousHash, hasHash := m.secretHashes[clusterCfg.Name]
			// Retry disconnected connections even when the kubeconfig secret has
			// not changed. A transient API outage during initial connection must
			// not leave the cluster permanently disconnected.
			if !existing.Connected || !hasHash || previousHash != secretHash {
				m.log.Info("Connecting or reconnecting to cluster", "cluster", clusterCfg.Name)
				clusterClient, err := m.connectClusterClient(ctx, dashboard, &clusterCfg)
				if err != nil {
					m.log.Error(err, "Failed to connect to cluster", "cluster", clusterCfg.Name)
					existing.ClusterCfg = &clusterCfg
					existing.Connected = false
					existing.LastError = err
					existing.LastCheck = time.Now()
					m.secretHashes[clusterCfg.Name] = secretHash
					continue
				}
				m.clients[clusterCfg.Name] = clusterClient
				m.secretHashes[clusterCfg.Name] = secretHash
				m.log.Info("Successfully connected to cluster", "cluster", clusterCfg.Name)
			} else {
				// Update configuration but keep existing client
				existing.ClusterCfg = &clusterCfg
				// A successful start-of-reconcile health check begins a new
				// discovery pass. Discovery methods deliberately do not clear this
				// value independently, so an earlier failure cannot be hidden by a
				// later successful resource type in the same pass.
				existing.LastError = nil
				m.secretHashes[clusterCfg.Name] = secretHash
				m.log.V(1).Info("Cluster configuration updated", "cluster", clusterCfg.Name)
			}
		} else {
			// Create new cluster connection
			clusterClient, err := m.connectClusterClient(ctx, dashboard, &clusterCfg)
			if err != nil {
				m.log.Error(err, "Failed to create cluster client", "cluster", clusterCfg.Name)
				// Store failed connection for status reporting
				m.clients[clusterCfg.Name] = &ClusterClient{
					Name:       clusterCfg.Name,
					Connected:  false,
					LastError:  err,
					LastCheck:  time.Now(),
					ClusterCfg: &clusterCfg,
				}
				// Remember the hash even after a failed attempt. The disconnected
				// state above still forces a retry on the next reconcile.
				m.secretHashes[clusterCfg.Name] = secretHash
				continue
			}
			m.clients[clusterCfg.Name] = clusterClient
			m.secretHashes[clusterCfg.Name] = secretHash
			m.log.Info("Successfully connected to remote cluster", "cluster", clusterCfg.Name)
		}
	}

	// Remove clusters that are no longer in the configuration
	for _, name := range sortedClusterNames(m.clients) {
		if name == localClusterName {
			continue // Never remove local cluster
		}
		if !activeClusters[name] {
			m.log.Info("Removing cluster connection", "cluster", name)
			delete(m.clients, name)
		}
	}

	// Ensure local cluster is always present
	if _, ok := m.clients[localClusterName]; !ok {
		m.clients[localClusterName] = &ClusterClient{
			Name:      localClusterName,
			Client:    m.localClient,
			Connected: true,
			LastCheck: time.Now(),
		}
	}

	return nil
}

func (m *ClusterManager) connectClusterClient(ctx context.Context, dashboard *homerv1alpha1.Dashboard, clusterCfg *homerv1alpha1.RemoteCluster) (*ClusterClient, error) {
	if m.createClusterClientFn != nil {
		return m.createClusterClientFn(ctx, dashboard, clusterCfg)
	}
	return m.createClusterClient(ctx, dashboard, clusterCfg)
}

// createClusterClient creates a new client for a remote cluster
func (m *ClusterManager) createClusterClient(ctx context.Context, dashboard *homerv1alpha1.Dashboard, clusterCfg *homerv1alpha1.RemoteCluster) (*ClusterClient, error) {
	// Get the secret containing kubeconfig
	secret := &corev1.Secret{}
	namespace := clusterCfg.SecretRef.Namespace
	if namespace == "" {
		namespace = dashboard.Namespace
	}

	err := m.localClient.Get(ctx, client.ObjectKey{
		Name:      clusterCfg.SecretRef.Name,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig secret: %w", err)
	}

	// Get kubeconfig data from secret
	key := clusterCfg.SecretRef.Key
	if key == "" {
		key = "kubeconfig"
	}

	kubeconfigData, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret %s", key, clusterCfg.SecretRef.Name)
	}

	// Parse kubeconfig and use the context matching the cluster name
	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Prefer a context named after the configured cluster when one exists. Do
	// not overwrite a valid current-context in a kubeconfig that uses a
	// different naming convention; the RemoteCluster name is an operator-local
	// identifier, not necessarily a kubeconfig context name.
	if _, ok := config.Contexts[clusterCfg.Name]; ok {
		config.CurrentContext = clusterCfg.Name
	} else if config.CurrentContext == "" && len(config.Contexts) == 1 {
		for contextName := range config.Contexts {
			config.CurrentContext = contextName
		}
	}

	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config for context %s: %w", clusterCfg.Name, err)
	}

	// Create client for the remote cluster
	remoteClient, err := client.New(restConfig, client.Options{
		Scheme: m.scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Test the connection
	if err := m.testConnection(ctx, restConfig); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster: %w", err)
	}

	return &ClusterClient{
		Name:       clusterCfg.Name,
		Client:     remoteClient,
		Config:     restConfig,
		Connected:  true,
		LastCheck:  time.Now(),
		ClusterCfg: clusterCfg,
	}, nil
}

// testConnection probes the Kubernetes API without requiring permission to
// list a cluster-scoped resource. The /version endpoint is available to
// authenticated Kubernetes clients independently of NamespaceFilter and does
// not turn a least-privilege discovery identity into a cluster-wide reader.
func (m *ClusterManager) testConnection(ctx context.Context, restConfig *rest.Config) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create API discovery client: %w", err)
	}

	if err := discoveryClient.RESTClient().Get().AbsPath("/version").Do(ctx).Error(); err != nil {
		return fmt.Errorf("API health probe: %w", err)
	}
	return nil
}

// GetClusterStatuses returns the status of all managed clusters
func (m *ClusterManager) GetClusterStatuses() []homerv1alpha1.ClusterConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := []homerv1alpha1.ClusterConnectionStatus{}
	for _, name := range sortedClusterNames(m.clients) {
		cluster := m.clients[name]
		if name == localClusterName {
			continue // Don't report local cluster status
		}

		status := homerv1alpha1.ClusterConnectionStatus{
			Name:      name,
			Connected: cluster.Connected,
		}

		if cluster.Connected {
			lastTime := metav1.NewTime(cluster.LastCheck)
			status.LastConnectionTime = &lastTime
		}

		if cluster.LastError != nil {
			status.LastError = cluster.LastError.Error()
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// DiscoverIngresses discovers Ingress resources from all connected clusters
func (m *ClusterManager) DiscoverIngresses(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (map[string][]networkingv1.Ingress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := make(map[string][]networkingv1.Ingress)
	log := log.FromContext(ctx)

	for _, name := range sortedClusterNames(m.clients) {
		cluster := m.clients[name]
		if !cluster.Connected && name != localClusterName {
			log.V(1).Info("Skipping disconnected cluster", "cluster", name)
			continue
		}

		ingresses, err := m.discoverClusterIngresses(ctx, cluster, dashboard)
		if err != nil {
			log.Error(err, "Failed to discover ingresses", "cluster", name)
			// A resource-level error (most notably Forbidden when the remote
			// identity is intentionally scoped) must not make the whole API
			// connection unusable. Other resource kinds may still be readable.
			if name != localClusterName {
				m.recordDiscoveryError(cluster, err)
			}
			continue
		}

		// Update connection status on success. LastCheck records the most
		// recent successful connection, not every discovery pass; exposing a
		// fresh timestamp on every reconcile would cause a status-update loop.
		if name != localClusterName {
			cluster.Connected = true
		}

		results[name] = ingresses
		log.V(1).Info("Discovered ingresses", "cluster", name, "count", len(ingresses))
	}

	return results, nil
}

// discoverClusterIngresses discovers Ingresses from a specific cluster
func (m *ClusterManager) discoverClusterIngresses(ctx context.Context, cluster *ClusterClient, dashboard *homerv1alpha1.Dashboard) ([]networkingv1.Ingress, error) {
	clusterIngresses := &networkingv1.IngressList{}

	// Apply namespace filter if specified for remote clusters
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.NamespaceFilter) > 0 {
		filteredIngresses := []networkingv1.Ingress{}
		for _, ns := range uniqueSortedNamespaces(cluster.ClusterCfg.NamespaceFilter) {
			nsIngresses := &networkingv1.IngressList{}
			if err := cluster.Client.List(ctx, nsIngresses, client.InNamespace(ns)); err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, err
				}
				continue
			}
			filteredIngresses = append(filteredIngresses, nsIngresses.Items...)
		}
		clusterIngresses.Items = filteredIngresses
	} else {
		if err := cluster.Client.List(ctx, clusterIngresses); err != nil {
			return nil, err
		}
	}

	// Apply selectors
	var selector *metav1.LabelSelector
	if cluster.ClusterCfg != nil && cluster.ClusterCfg.IngressSelector != nil {
		selector = cluster.ClusterCfg.IngressSelector
	} else if cluster.Name == localClusterName && dashboard.Spec.IngressSelector != nil {
		selector = dashboard.Spec.IngressSelector
	}

	// Determine which domain filters to use for this cluster
	domainFilters := m.getDomainFiltersForCluster(cluster, dashboard)

	if selector != nil {
		labelSelector, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return nil, err
		}

		filtered := []networkingv1.Ingress{}
		for i := range clusterIngresses.Items {
			if labelSelector.Matches(labels.Set(clusterIngresses.Items[i].Labels)) {
				// Apply domain filters if specified
				if len(domainFilters) > 0 {
					if !utils.MatchesIngressDomainFilters(&clusterIngresses.Items[i], domainFilters) {
						continue
					}
				}

				// Add cluster labels if specified
				if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
					if clusterIngresses.Items[i].Labels == nil {
						clusterIngresses.Items[i].Labels = make(map[string]string)
					}
					maps.Copy(clusterIngresses.Items[i].Labels, cluster.ClusterCfg.ClusterLabels)
				}
				// Add cluster annotation for identification
				if clusterIngresses.Items[i].Annotations == nil {
					clusterIngresses.Items[i].Annotations = make(map[string]string)
				}
				clusterIngresses.Items[i].Annotations["homer.rajsingh.info/cluster"] = cluster.Name

				// Merge namespace annotations from the source cluster
				m.mergeHomerAnnotationsFromNamespace(ctx, cluster.Client, clusterIngresses.Items[i].Namespace, clusterIngresses.Items[i].Annotations)

				filtered = append(filtered, clusterIngresses.Items[i])
			}
		}
		return sortAndDedupeResources(filtered, func(ingress networkingv1.Ingress) (string, string) {
			return ingress.Namespace, ingress.Name
		}), nil
	}

	// Add cluster metadata to all ingresses and apply domain filtering
	filtered := []networkingv1.Ingress{}
	for i := range clusterIngresses.Items {
		ingress := &clusterIngresses.Items[i]

		// Apply domain filters if specified
		if len(domainFilters) > 0 {
			if !utils.MatchesIngressDomainFilters(ingress, domainFilters) {
				continue
			}
		}

		if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
			if ingress.Labels == nil {
				ingress.Labels = make(map[string]string)
			}
			maps.Copy(ingress.Labels, cluster.ClusterCfg.ClusterLabels)
		}
		// Add cluster annotation for identification
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		ingress.Annotations["homer.rajsingh.info/cluster"] = cluster.Name

		// Merge namespace annotations from the source cluster
		m.mergeHomerAnnotationsFromNamespace(ctx, cluster.Client, ingress.Namespace, ingress.Annotations)

		filtered = append(filtered, *ingress)
	}

	return sortAndDedupeResources(filtered, func(ingress networkingv1.Ingress) (string, string) {
		return ingress.Namespace, ingress.Name
	}), nil
}

// DiscoverHTTPRoutes discovers HTTPRoute resources from all connected clusters
func (m *ClusterManager) DiscoverHTTPRoutes(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (map[string][]gatewayv1.HTTPRoute, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := make(map[string][]gatewayv1.HTTPRoute)
	log := log.FromContext(ctx)

	for _, name := range sortedClusterNames(m.clients) {
		cluster := m.clients[name]
		if !cluster.Connected && name != localClusterName {
			log.V(1).Info("Skipping disconnected cluster", "cluster", name)
			continue
		}

		httproutes, err := m.discoverClusterHTTPRoutes(ctx, cluster, dashboard)
		if err != nil {
			log.Error(err, "Failed to discover HTTPRoutes", "cluster", name)
			if name != localClusterName {
				m.recordDiscoveryError(cluster, err)
			}
			continue
		}

		// LastCheck is updated when the connection is established or restored,
		// not on every successful discovery pass.
		if name != localClusterName {
			cluster.Connected = true
		}

		results[name] = httproutes
		log.V(1).Info("Discovered HTTPRoutes", "cluster", name, "count", len(httproutes))
	}

	return results, nil
}

// discoverClusterHTTPRoutes discovers HTTPRoutes from a specific cluster
func (m *ClusterManager) discoverClusterHTTPRoutes(ctx context.Context, cluster *ClusterClient, dashboard *homerv1alpha1.Dashboard) ([]gatewayv1.HTTPRoute, error) {
	clusterHTTPRoutes := &gatewayv1.HTTPRouteList{}

	// Apply namespace filter if specified for remote clusters
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.NamespaceFilter) > 0 {
		filteredHTTPRoutes := []gatewayv1.HTTPRoute{}
		for _, ns := range uniqueSortedNamespaces(cluster.ClusterCfg.NamespaceFilter) {
			nsHTTPRoutes := &gatewayv1.HTTPRouteList{}
			if err := cluster.Client.List(ctx, nsHTTPRoutes, client.InNamespace(ns)); err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, err
				}
				continue
			}
			filteredHTTPRoutes = append(filteredHTTPRoutes, nsHTTPRoutes.Items...)
		}
		clusterHTTPRoutes.Items = filteredHTTPRoutes
	} else {
		if err := cluster.Client.List(ctx, clusterHTTPRoutes); err != nil {
			return nil, err
		}
	}

	// Determine which domain filters to use for this cluster
	domainFilters := m.getDomainFiltersForCluster(cluster, dashboard)

	m.log.V(1).Info("Starting HTTPRoute filtering", "cluster", cluster.Name, "total", len(clusterHTTPRoutes.Items), "domainFilters", domainFilters)

	// Filter HTTPRoutes based on selectors
	filtered := []gatewayv1.HTTPRoute{}
	selectorPassed := 0
	domainFilterPassed := 0
	for i := range clusterHTTPRoutes.Items {
		shouldInclude, err := m.shouldIncludeHTTPRoute(ctx, cluster, &clusterHTTPRoutes.Items[i], dashboard)
		if err != nil {
			m.log.V(1).Error(err, "Error checking HTTPRoute inclusion", "httproute", clusterHTTPRoutes.Items[i].Name, "cluster", cluster.Name)
			continue
		}
		if shouldInclude {
			selectorPassed++
			// Apply domain filters if specified
			if len(domainFilters) > 0 {
				if !utils.MatchesHTTPRouteDomainFilters(clusterHTTPRoutes.Items[i].Spec.Hostnames, domainFilters) {
					m.log.V(1).Info("HTTPRoute filtered out by domain", "cluster", cluster.Name, "httproute", clusterHTTPRoutes.Items[i].Name, "hostnames", clusterHTTPRoutes.Items[i].Spec.Hostnames)
					continue
				}
			}
			domainFilterPassed++

			// Add cluster labels if specified
			if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
				if clusterHTTPRoutes.Items[i].Labels == nil {
					clusterHTTPRoutes.Items[i].Labels = make(map[string]string)
				}
				maps.Copy(clusterHTTPRoutes.Items[i].Labels, cluster.ClusterCfg.ClusterLabels)
			}
			// Add cluster annotation for identification
			if clusterHTTPRoutes.Items[i].Annotations == nil {
				clusterHTTPRoutes.Items[i].Annotations = make(map[string]string)
			}
			clusterHTTPRoutes.Items[i].Annotations["homer.rajsingh.info/cluster"] = cluster.Name

			// Store domain filters as annotation so Homer config generator knows which hostnames to show
			if len(domainFilters) > 0 {
				clusterHTTPRoutes.Items[i].Annotations["homer.rajsingh.info/domain-filters"] = strings.Join(domainFilters, ",")
			}

			// Merge namespace annotations from the source cluster
			m.mergeHomerAnnotationsFromNamespace(ctx, cluster.Client, clusterHTTPRoutes.Items[i].Namespace, clusterHTTPRoutes.Items[i].Annotations)

			filtered = append(filtered, clusterHTTPRoutes.Items[i])
		}
	}

	m.log.V(1).Info("HTTPRoute filtering complete", "cluster", cluster.Name, "total", len(clusterHTTPRoutes.Items), "selectorPassed", selectorPassed, "domainFilterPassed", domainFilterPassed, "final", len(filtered))

	return sortAndDedupeResources(filtered, func(route gatewayv1.HTTPRoute) (string, string) {
		return route.Namespace, route.Name
	}), nil
}

// shouldIncludeHTTPRoute checks if an HTTPRoute should be included based on selectors
func (m *ClusterManager) shouldIncludeHTTPRoute(ctx context.Context, cluster *ClusterClient, httproute *gatewayv1.HTTPRoute, dashboard *homerv1alpha1.Dashboard) (bool, error) {
	// Check HTTPRoute selector
	var httpRouteSelector *metav1.LabelSelector
	if cluster.ClusterCfg != nil && cluster.ClusterCfg.HTTPRouteSelector != nil {
		httpRouteSelector = cluster.ClusterCfg.HTTPRouteSelector
	} else if cluster.Name == localClusterName && dashboard.Spec.HTTPRouteSelector != nil {
		httpRouteSelector = dashboard.Spec.HTTPRouteSelector
	}

	if httpRouteSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(httpRouteSelector)
		if err != nil {
			return false, err
		}
		if !selector.Matches(labels.Set(httproute.Labels)) {
			return false, nil
		}
	}

	// Check Gateway selector
	var gatewaySelector *metav1.LabelSelector
	if cluster.ClusterCfg != nil && cluster.ClusterCfg.GatewaySelector != nil {
		gatewaySelector = cluster.ClusterCfg.GatewaySelector
	} else if cluster.Name == localClusterName && dashboard.Spec.GatewaySelector != nil {
		gatewaySelector = dashboard.Spec.GatewaySelector
	}

	if gatewaySelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(gatewaySelector)
		if err != nil {
			return false, err
		}

		for _, parentRef := range httproute.Spec.ParentRefs {
			if parentRef.Kind != nil && string(*parentRef.Kind) != gatewayKind {
				continue
			}

			namespace := httproute.Namespace
			if parentRef.Namespace != nil {
				namespace = string(*parentRef.Namespace)
			}

			gateway := &gatewayv1.Gateway{}
			if err := cluster.Client.Get(ctx, client.ObjectKey{Name: string(parentRef.Name), Namespace: namespace}, gateway); err != nil {
				if apierrors.IsNotFound(err) {
					m.log.V(1).Info("Gateway not found for HTTPRoute", "cluster", cluster.Name, "httproute", httproute.Name, "gateway", parentRef.Name, "namespace", namespace)
					continue
				}
				return false, err
			}

			if selector.Matches(labels.Set(gateway.Labels)) {
				m.log.V(1).Info("HTTPRoute matched gateway selector", "cluster", cluster.Name, "httproute", httproute.Name, "gateway", parentRef.Name, "namespace", namespace, "labels", gateway.Labels)
				return true, nil
			}
			m.log.V(1).Info("Gateway labels did not match selector", "cluster", cluster.Name, "httproute", httproute.Name, "gateway", parentRef.Name, "namespace", namespace, "gatewayLabels", gateway.Labels, "selector", gatewaySelector)
		}
		m.log.V(1).Info("HTTPRoute did not match any gateway", "cluster", cluster.Name, "httproute", httproute.Name, "parentRefs", len(httproute.Spec.ParentRefs))
		return false, nil
	}

	return true, nil
}

// DiscoverServices discovers Service resources from all connected clusters
func (m *ClusterManager) DiscoverServices(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (map[string][]corev1.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := make(map[string][]corev1.Service)
	log := log.FromContext(ctx)

	// If no ServiceSelector anywhere, skip entirely
	if dashboard.Spec.ServiceSelector == nil {
		hasRemoteSelector := false
		for _, rc := range dashboard.Spec.RemoteClusters {
			if rc.ServiceSelector != nil {
				hasRemoteSelector = true
				break
			}
		}
		if !hasRemoteSelector {
			return results, nil
		}
	}

	for _, name := range sortedClusterNames(m.clients) {
		cluster := m.clients[name]
		if !cluster.Connected && name != localClusterName {
			log.V(1).Info("Skipping disconnected cluster for Service discovery", "cluster", name)
			continue
		}

		services, err := m.discoverClusterServices(ctx, cluster, dashboard)
		if err != nil {
			log.Error(err, "Failed to discover Services", "cluster", name)
			if name != localClusterName {
				m.recordDiscoveryError(cluster, err)
			}
			continue
		}

		if name != localClusterName {
			cluster.Connected = true
		}

		results[name] = services
		log.V(1).Info("Discovered Services", "cluster", name, "count", len(services))
	}

	return results, nil
}

// recordDiscoveryError records the failed resource query while preserving an
// otherwise healthy API connection. A remote identity can be authorized to
// read Services but not Ingresses (or vice versa); marking the whole cluster
// disconnected would suppress all of the permitted resource types on the
// next discovery pass. UpdateClusters owns connection establishment and
// retries failed initial connections, while discovery errors remain visible in
// status through LastError.
func (m *ClusterManager) recordDiscoveryError(cluster *ClusterClient, err error) {
	cluster.LastError = err
	if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
		return
	}

	// A client created successfully by createClusterClient has already passed
	// the /version health probe. Keep it usable after a later transient list
	// failure; the next discovery pass can recover without throwing away the
	// client. This also keeps selector/configuration errors from masquerading as
	// connection failures.
	cluster.Connected = true
}

// discoverClusterServices discovers Services from a specific cluster
func (m *ClusterManager) discoverClusterServices(ctx context.Context, cluster *ClusterClient, dashboard *homerv1alpha1.Dashboard) ([]corev1.Service, error) {
	// Service discovery intentionally treats the dashboard selector as a
	// default for every cluster. A remote selector overrides that default for
	// its own cluster; this is distinct from Ingress and HTTPRoute selectors,
	// which are local-only unless explicitly configured per remote cluster.
	var selector *metav1.LabelSelector
	if cluster.ClusterCfg != nil && cluster.ClusterCfg.ServiceSelector != nil {
		selector = cluster.ClusterCfg.ServiceSelector
	} else if dashboard.Spec.ServiceSelector != nil {
		selector = dashboard.Spec.ServiceSelector
	}

	// No selector means no Service discovery for this cluster
	if selector == nil {
		return nil, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}

	clusterServices := &corev1.ServiceList{}

	// Apply namespace filter
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.NamespaceFilter) > 0 {
		filteredServices := []corev1.Service{}
		for _, ns := range uniqueSortedNamespaces(cluster.ClusterCfg.NamespaceFilter) {
			nsServices := &corev1.ServiceList{}
			if err := cluster.Client.List(ctx, nsServices, client.InNamespace(ns)); err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, err
				}
				continue
			}
			filteredServices = append(filteredServices, nsServices.Items...)
		}
		clusterServices.Items = filteredServices
	} else {
		if err := cluster.Client.List(ctx, clusterServices); err != nil {
			return nil, err
		}
	}

	// Filter by label selector and add metadata
	filtered := []corev1.Service{}
	for i := range clusterServices.Items {
		svc := &clusterServices.Items[i]

		if !labelSelector.Matches(labels.Set(svc.Labels)) {
			continue
		}

		// Add cluster labels
		if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
			if svc.Labels == nil {
				svc.Labels = make(map[string]string)
			}
			maps.Copy(svc.Labels, cluster.ClusterCfg.ClusterLabels)
		}

		// Add cluster annotation
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations["homer.rajsingh.info/cluster"] = cluster.Name

		// Merge namespace annotations
		m.mergeHomerAnnotationsFromNamespace(ctx, cluster.Client, svc.Namespace, svc.Annotations)

		filtered = append(filtered, *svc)
	}

	return sortAndDedupeResources(filtered, func(service corev1.Service) (string, string) {
		return service.Namespace, service.Name
	}), nil
}

// mergeHomerAnnotationsFromNamespace fetches a namespace and merges its homer annotations
// into the target annotations map as defaults (existing annotations take precedence).
func (m *ClusterManager) mergeHomerAnnotationsFromNamespace(ctx context.Context, clusterClient client.Client, namespaceName string, target map[string]string) {
	ns := &corev1.Namespace{}
	if err := clusterClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err != nil {
		m.log.V(2).Info("Could not fetch namespace for annotation merge", "namespace", namespaceName, "error", err)
		return
	}

	for k, v := range ns.Annotations {
		if strings.HasPrefix(k, serviceAnnotationPrefix) || strings.HasPrefix(k, itemAnnotationPrefix) {
			if _, exists := target[k]; !exists {
				target[k] = v
			}
		}
	}
}

// UpdateClusterStatuses updates the cluster connection counts in the status
func (m *ClusterManager) UpdateClusterStatuses(statuses []homerv1alpha1.ClusterConnectionStatus, clusterIngresses map[string][]networkingv1.Ingress, clusterHTTPRoutes map[string][]gatewayv1.HTTPRoute, clusterServices map[string][]corev1.Service) []homerv1alpha1.ClusterConnectionStatus {
	// Create a map for quick lookup
	statusMap := make(map[string]*homerv1alpha1.ClusterConnectionStatus)
	for i := range statuses {
		statusMap[statuses[i].Name] = &statuses[i]
	}

	// Update counts
	for _, clusterName := range sortedClusterNames(clusterIngresses) {
		ingresses := clusterIngresses[clusterName]
		if clusterName == localClusterName {
			continue
		}
		if status, ok := statusMap[clusterName]; ok {
			status.DiscoveredIngresses = len(ingresses)
		}
	}

	for _, clusterName := range sortedClusterNames(clusterHTTPRoutes) {
		httproutes := clusterHTTPRoutes[clusterName]
		if clusterName == localClusterName {
			continue
		}
		if status, ok := statusMap[clusterName]; ok {
			status.DiscoveredHTTPRoutes = len(httproutes)
		}
	}

	for _, clusterName := range sortedClusterNames(clusterServices) {
		svcs := clusterServices[clusterName]
		if clusterName == localClusterName {
			continue
		}
		if status, ok := statusMap[clusterName]; ok {
			status.DiscoveredServices = len(svcs)
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	return statuses
}

// sortedClusterNames returns map keys in a stable order. Discovery results are
// maps because each cluster is queried independently, but their aggregation
// must not depend on Go's randomized map iteration order.
func sortedClusterNames[T any](clusters map[string]T) []string {
	names := make([]string, 0, len(clusters))
	for name := range clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateRemoteClusters(clusters []homerv1alpha1.RemoteCluster) error {
	seen := make(map[string]struct{}, len(clusters))
	for _, cluster := range clusters {
		if cluster.Name == localClusterName {
			return fmt.Errorf("remote cluster name %q is reserved for the local cluster", localClusterName)
		}
		if cluster.Name == "" {
			return fmt.Errorf("remote cluster name must not be empty")
		}
		if _, ok := seen[cluster.Name]; ok {
			return fmt.Errorf("remote cluster name %q is duplicated", cluster.Name)
		}
		seen[cluster.Name] = struct{}{}
	}
	return nil
}

func uniqueSortedNamespaces(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func sortAndDedupeResources[T any](items []T, key func(T) (string, string)) []T {
	sort.SliceStable(items, func(i, j int) bool {
		iNamespace, iName := key(items[i])
		jNamespace, jName := key(items[j])
		if iNamespace != jNamespace {
			return iNamespace < jNamespace
		}
		return iName < jName
	})

	result := items[:0]
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		namespace, name := key(item)
		identity := namespace + "\x00" + name
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		result = append(result, item)
	}
	return result
}

// getSecretHash computes a hash of the kubeconfig secret to detect changes
func (m *ClusterManager) getSecretHash(ctx context.Context, dashboard *homerv1alpha1.Dashboard, clusterCfg *homerv1alpha1.RemoteCluster) (string, error) {
	// Get the secret
	namespace := clusterCfg.SecretRef.Namespace
	if namespace == "" {
		namespace = dashboard.Namespace
	}

	secret := &corev1.Secret{}
	err := m.localClient.Get(ctx, client.ObjectKey{
		Name:      clusterCfg.SecretRef.Name,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	// Get kubeconfig data
	key := clusterCfg.SecretRef.Key
	if key == "" {
		key = "kubeconfig"
	}

	kubeconfigData, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret", key)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(kubeconfigData)
	return hex.EncodeToString(hash[:]), nil
}

// getDomainFiltersForCluster returns the domain filters to use for a cluster
// Local cluster uses dashboard-level filters, remote clusters use their own explicit filters
func (m *ClusterManager) getDomainFiltersForCluster(cluster *ClusterClient, dashboard *homerv1alpha1.Dashboard) []string {
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.DomainFilters) > 0 {
		return cluster.ClusterCfg.DomainFilters
	}
	if cluster.Name == localClusterName {
		return dashboard.Spec.DomainFilters
	}
	// Remote clusters with no explicit domain filters: no filtering (return empty)
	return nil
}
