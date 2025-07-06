package utils

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// UpdateConfigMapWithRetry updates a ConfigMap with exponential backoff retry on conflicts.
// This function handles optimistic concurrency by retrying on update conflicts.
func UpdateConfigMapWithRetry(
	ctx context.Context,
	k8sClient client.Client,
	configMap *corev1.ConfigMap,
	resourceName string,
) error {
	log := log.FromContext(ctx)

	backoff := wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		Cap:      5 * time.Second,
	}

	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		// Get the latest version of the ConfigMap before updating
		latestConfigMap := &corev1.ConfigMap{}
		key := client.ObjectKeyFromObject(configMap)
		if err := k8sClient.Get(ctx, key, latestConfigMap); err != nil {
			if apierrors.IsNotFound(err) {
				// ConfigMap was deleted, no need to retry
				return false, err
			}
			log.V(1).Info("Failed to get latest ConfigMap, retrying", "error", err)
			return false, nil // Retry
		}

		// Copy our changes to the latest version
		latestConfigMap.Data = configMap.Data
		latestConfigMap.BinaryData = configMap.BinaryData

		// Attempt to update
		if err := k8sClient.Update(ctx, latestConfigMap); err != nil {
			if apierrors.IsConflict(err) {
				log.V(1).Info("ConfigMap update conflict, retrying", "configmap", resourceName)
				return false, nil // Retry
			}
			// Non-conflict error, don't retry
			return false, err
		}

		// Success
		return true, nil
	})
}
