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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	// corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	homerv1alpha1 "github.com/rajsinghtech/homer-operator.git/api/v1alpha1"
)

// ConfigurationReconciler reconciles a Configuration object
type ConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=configurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=configurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=homer.rajsingh.info,resources=configurations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Configuration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	config := &homerv1alpha1.Configuration{}
	err := r.Get(ctx, req.NamespacedName, config)
	if err != nil {
		l.Error(err, "unable to fetch Configuration")
		return ctrl.Result{}, client.IgnoreAlreadyExists(err)
	}

	l.Info("Configuration", "config", config)
	spec := createHomerDeploymentConfiguration(config.Name)
	l.Info("Generated", "Deployment", spec)

	// // Create a slice to store the namespace names
	// var nsNames []string
	// for _, ns := range nsList.Items {
	// 	nsNames = append(nsNames, ns.Name)
	// }
	// l.Info("NamespaceList", "nsNames", nsNames)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&homerv1alpha1.Configuration{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func createHomerDeploymentConfiguration(resourceName string) *appsv1.Deployment {
	overlay:= &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: "default",
		},
		// TODO(user): Specify other spec details if needed.
	}
	return overlay
}
