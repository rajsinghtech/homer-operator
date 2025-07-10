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

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	homerv1alpha1 "github.com/rajsinghtech/homer-operator/api/v1alpha1"
)

type TestResourceManager struct {
	ctx      context.Context
	client   client.Client
	timeout  time.Duration
	interval time.Duration
}

func NewTestResourceManager(ctx context.Context, client client.Client) *TestResourceManager {
	return &TestResourceManager{
		ctx:      ctx,
		client:   client,
		timeout:  time.Second * 10,
		interval: time.Millisecond * 100,
	}
}

func (m *TestResourceManager) Cleanup(obj client.Object) {
	if obj == nil {
		return
	}

	namespacedName := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Special handling for Dashboard finalizers
	if _, ok := obj.(*homerv1alpha1.Dashboard); ok {
		current := &homerv1alpha1.Dashboard{}
		if err := m.client.Get(m.ctx, namespacedName, current); err == nil {
			if len(current.Finalizers) > 0 {
				current.Finalizers = []string{}
				_ = m.client.Update(m.ctx, current)
			}
		}
	}

	err := m.client.Delete(m.ctx, obj)
	if err != nil && !apierrors.IsNotFound(err) {
		return
	}

	// Wait for deletion
	Eventually(func() bool {
		err := m.client.Get(m.ctx, namespacedName, obj)
		return apierrors.IsNotFound(err)
	}, m.timeout, m.interval).Should(BeTrue())
}

func (m *TestResourceManager) CleanupAll(resources ...client.Object) {
	for _, resource := range resources {
		m.Cleanup(resource)
	}
}

func (m *TestResourceManager) WaitForResource(namespacedName types.NamespacedName, resource client.Object) {
	Eventually(func() bool {
		err := m.client.Get(m.ctx, namespacedName, resource)
		return err == nil
	}, m.timeout, m.interval).Should(BeTrue())
}

func (m *TestResourceManager) WaitForConfigMapData(namespacedName types.NamespacedName, key string) {
	Eventually(func() bool {
		configMap := &corev1.ConfigMap{}
		err := m.client.Get(m.ctx, namespacedName, configMap)
		if err != nil {
			return false
		}
		data, exists := configMap.Data[key]
		return exists && data != "" && data != "null"
	}, m.timeout, m.interval).Should(BeTrue())
}

func (m *TestResourceManager) GetHomerResources(dashboardName, namespace string) (*corev1.ConfigMap, *appsv1.Deployment, *corev1.Service) {
	homerName := dashboardName + "-homer"

	configMap := &corev1.ConfigMap{}
	_ = m.client.Get(m.ctx, types.NamespacedName{Name: homerName, Namespace: namespace}, configMap)

	deployment := &appsv1.Deployment{}
	_ = m.client.Get(m.ctx, types.NamespacedName{Name: homerName, Namespace: namespace}, deployment)

	service := &corev1.Service{}
	_ = m.client.Get(m.ctx, types.NamespacedName{Name: homerName, Namespace: namespace}, service)

	return configMap, deployment, service
}
