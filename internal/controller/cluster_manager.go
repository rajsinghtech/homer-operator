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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	mu           sync.RWMutex
	log          logr.Logger
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

	// Track which clusters should remain
	activeClusters := make(map[string]bool)

	// Add/update remote clusters
	for _, clusterCfg := range dashboard.Spec.RemoteClusters {
		if !clusterCfg.Enabled {
			m.log.V(1).Info("Skipping disabled cluster", "cluster", clusterCfg.Name)
			continue
		}

		activeClusters[clusterCfg.Name] = true

		// Get current secret hash to detect changes
		secretHash, err := m.getSecretHash(ctx, dashboard, &clusterCfg)
		if err != nil {
			m.log.Error(err, "Failed to get secret hash", "cluster", clusterCfg.Name)
			continue
		}

		// Check if we already have this cluster
		if existing, ok := m.clients[clusterCfg.Name]; ok {
			// Check if secret changed
			previousHash, hasHash := m.secretHashes[clusterCfg.Name]
			if hasHash && previousHash != secretHash {
				m.log.Info("Kubeconfig secret changed, reconnecting to cluster", "cluster", clusterCfg.Name)
				// Secret changed, recreate client
				clusterClient, err := m.createClusterClient(ctx, dashboard, &clusterCfg)
				if err != nil {
					m.log.Error(err, "Failed to recreate cluster client after secret change", "cluster", clusterCfg.Name)
					// Keep existing client but mark as potentially stale
					existing.ClusterCfg = &clusterCfg
					m.secretHashes[clusterCfg.Name] = secretHash
					continue
				}
				m.clients[clusterCfg.Name] = clusterClient
				m.secretHashes[clusterCfg.Name] = secretHash
				m.log.Info("Successfully reconnected to cluster with new credentials", "cluster", clusterCfg.Name)
			} else {
				// Update configuration but keep existing client
				existing.ClusterCfg = &clusterCfg
				m.secretHashes[clusterCfg.Name] = secretHash
				m.log.V(1).Info("Cluster configuration updated", "cluster", clusterCfg.Name)
			}
		} else {
			// Create new cluster connection
			clusterClient, err := m.createClusterClient(ctx, dashboard, &clusterCfg)
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
				continue
			}
			m.clients[clusterCfg.Name] = clusterClient
			m.secretHashes[clusterCfg.Name] = secretHash
			m.log.Info("Successfully connected to remote cluster", "cluster", clusterCfg.Name)
		}
	}

	// Remove clusters that are no longer in the configuration
	for name := range m.clients {
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

	// Override current-context to match the cluster name
	config.CurrentContext = clusterCfg.Name

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
	if err := m.testConnection(ctx, remoteClient); err != nil {
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

// testConnection tests if a cluster client can connect
func (m *ClusterManager) testConnection(ctx context.Context, c client.Client) error {
	// Try to list namespaces as a connectivity test
	namespaces := &corev1.NamespaceList{}
	return c.List(ctx, namespaces, client.Limit(1))
}

// GetClusterStatuses returns the status of all managed clusters
func (m *ClusterManager) GetClusterStatuses() []homerv1alpha1.ClusterConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := []homerv1alpha1.ClusterConnectionStatus{}
	for name, cluster := range m.clients {
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string][]networkingv1.Ingress)
	log := log.FromContext(ctx)

	for name, cluster := range m.clients {
		if !cluster.Connected && name != localClusterName {
			log.V(1).Info("Skipping disconnected cluster", "cluster", name)
			continue
		}

		ingresses, err := m.discoverClusterIngresses(ctx, cluster, dashboard)
		if err != nil {
			log.Error(err, "Failed to discover ingresses", "cluster", name)
			// Mark cluster as disconnected on error
			if name != localClusterName {
				cluster.Connected = false
				cluster.LastError = err
				cluster.LastCheck = time.Now()
			}
			continue
		}

		// Update connection status on success
		if name != localClusterName {
			cluster.Connected = true
			cluster.LastError = nil
			cluster.LastCheck = time.Now()
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
	listOpts := []client.ListOption{}
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.NamespaceFilter) > 0 {
		// List ingresses from each specified namespace
		filteredIngresses := []networkingv1.Ingress{}
		for _, ns := range cluster.ClusterCfg.NamespaceFilter {
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
		// List from all namespaces
		if err := cluster.Client.List(ctx, clusterIngresses, listOpts...); err != nil {
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
					if !matchesIngressDomainFilters(&clusterIngresses.Items[i], domainFilters) {
						continue
					}
				}

				// Add cluster labels if specified
				if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
					if clusterIngresses.Items[i].Labels == nil {
						clusterIngresses.Items[i].Labels = make(map[string]string)
					}
					for k, v := range cluster.ClusterCfg.ClusterLabels {
						clusterIngresses.Items[i].Labels[k] = v
					}
				}
				// Add cluster annotation for identification
				if clusterIngresses.Items[i].Annotations == nil {
					clusterIngresses.Items[i].Annotations = make(map[string]string)
				}
				clusterIngresses.Items[i].Annotations["homer.rajsingh.info/cluster"] = cluster.Name

				filtered = append(filtered, clusterIngresses.Items[i])
			}
		}
		return filtered, nil
	}

	// Add cluster metadata to all ingresses and apply domain filtering
	filtered := []networkingv1.Ingress{}
	for i := range clusterIngresses.Items {
		ingress := &clusterIngresses.Items[i]

		// Apply domain filters if specified
		if len(domainFilters) > 0 {
			if !matchesIngressDomainFilters(ingress, domainFilters) {
				continue
			}
		}

		if cluster.ClusterCfg != nil && cluster.ClusterCfg.ClusterLabels != nil {
			if ingress.Labels == nil {
				ingress.Labels = make(map[string]string)
			}
			for k, v := range cluster.ClusterCfg.ClusterLabels {
				ingress.Labels[k] = v
			}
		}
		// Add cluster annotation for identification
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		ingress.Annotations["homer.rajsingh.info/cluster"] = cluster.Name

		filtered = append(filtered, *ingress)
	}

	return filtered, nil
}

// DiscoverHTTPRoutes discovers HTTPRoute resources from all connected clusters
func (m *ClusterManager) DiscoverHTTPRoutes(ctx context.Context, dashboard *homerv1alpha1.Dashboard) (map[string][]gatewayv1.HTTPRoute, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string][]gatewayv1.HTTPRoute)
	log := log.FromContext(ctx)

	for name, cluster := range m.clients {
		if !cluster.Connected && name != localClusterName {
			log.V(1).Info("Skipping disconnected cluster", "cluster", name)
			continue
		}

		httproutes, err := m.discoverClusterHTTPRoutes(ctx, cluster, dashboard)
		if err != nil {
			log.Error(err, "Failed to discover HTTPRoutes", "cluster", name)
			// Mark cluster as disconnected on error
			if name != localClusterName {
				cluster.Connected = false
				cluster.LastError = err
				cluster.LastCheck = time.Now()
			}
			continue
		}

		// Update connection status on success
		if name != localClusterName {
			cluster.Connected = true
			cluster.LastError = nil
			cluster.LastCheck = time.Now()
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
	listOpts := []client.ListOption{}
	if cluster.ClusterCfg != nil && len(cluster.ClusterCfg.NamespaceFilter) > 0 {
		// List HTTPRoutes from each specified namespace
		filteredHTTPRoutes := []gatewayv1.HTTPRoute{}
		for _, ns := range cluster.ClusterCfg.NamespaceFilter {
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
		// List from all namespaces
		if err := cluster.Client.List(ctx, clusterHTTPRoutes, listOpts...); err != nil {
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
				if !matchesHTTPRouteDomainFilters(&clusterHTTPRoutes.Items[i], domainFilters) {
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
				for k, v := range cluster.ClusterCfg.ClusterLabels {
					clusterHTTPRoutes.Items[i].Labels[k] = v
				}
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

			filtered = append(filtered, clusterHTTPRoutes.Items[i])
		}
	}

	m.log.V(1).Info("HTTPRoute filtering complete", "cluster", cluster.Name, "total", len(clusterHTTPRoutes.Items), "selectorPassed", selectorPassed, "domainFilterPassed", domainFilterPassed, "final", len(filtered))

	return filtered, nil
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

		matchedGateway := false
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
				matchedGateway = true
				m.log.V(1).Info("HTTPRoute matched gateway selector", "cluster", cluster.Name, "httproute", httproute.Name, "gateway", parentRef.Name, "namespace", namespace, "labels", gateway.Labels)
				return true, nil
			} else {
				m.log.V(1).Info("Gateway labels did not match selector", "cluster", cluster.Name, "httproute", httproute.Name, "gateway", parentRef.Name, "namespace", namespace, "gatewayLabels", gateway.Labels, "selector", gatewaySelector)
			}
		}
		if !matchedGateway {
			m.log.V(1).Info("HTTPRoute did not match any gateway", "cluster", cluster.Name, "httproute", httproute.Name, "parentRefs", len(httproute.Spec.ParentRefs))
		}
		return false, nil
	}

	return true, nil
}

// UpdateClusterStatuses updates the cluster connection counts in the status
func (m *ClusterManager) UpdateClusterStatuses(statuses []homerv1alpha1.ClusterConnectionStatus, clusterIngresses map[string][]networkingv1.Ingress, clusterHTTPRoutes map[string][]gatewayv1.HTTPRoute) []homerv1alpha1.ClusterConnectionStatus {
	// Create a map for quick lookup
	statusMap := make(map[string]*homerv1alpha1.ClusterConnectionStatus)
	for i := range statuses {
		statusMap[statuses[i].Name] = &statuses[i]
	}

	// Update counts
	for clusterName, ingresses := range clusterIngresses {
		if clusterName == localClusterName {
			continue
		}
		if status, ok := statusMap[clusterName]; ok {
			status.DiscoveredIngresses = len(ingresses)
		}
	}

	for clusterName, httproutes := range clusterHTTPRoutes {
		if clusterName == localClusterName {
			continue
		}
		if status, ok := statusMap[clusterName]; ok {
			status.DiscoveredHTTPRoutes = len(httproutes)
		}
	}

	return statuses
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

// matchesIngressDomainFilters checks if an Ingress matches any of the domain filters
func matchesIngressDomainFilters(ingress *networkingv1.Ingress, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}
		for _, filter := range domainFilters {
			if matchesDomain(rule.Host, filter) {
				return true
			}
		}
	}
	return false
}

// matchesHTTPRouteDomainFilters checks if an HTTPRoute matches any of the domain filters
func matchesHTTPRouteDomainFilters(httproute *gatewayv1.HTTPRoute, domainFilters []string) bool {
	if len(domainFilters) == 0 {
		return true
	}

	for _, hostname := range httproute.Spec.Hostnames {
		host := string(hostname)
		for _, filter := range domainFilters {
			if matchesDomain(host, filter) {
				return true
			}
		}
	}
	return false
}

// matchesDomain checks if a host matches a domain filter
// Supports exact matches and subdomain wildcards
func matchesDomain(host, filter string) bool {
	if host == filter {
		return true
	}
	// Check if filter matches as subdomain (e.g., "example.com" matches "*.example.com")
	if len(host) > len(filter) && host[len(host)-len(filter)-1:] == "."+filter {
		return true
	}
	return false
}
