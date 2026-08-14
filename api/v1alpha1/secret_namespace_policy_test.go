package v1alpha1

import (
	"os"
	"path/filepath"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestSmartCardSecretNamespacePolicyIsPresentInCRD(t *testing.T) {
	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "homer.rajsingh.info_dashboards.yaml")
	contents, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("read generated CRD: %v", err)
	}

	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(contents, &crd); err != nil {
		t.Fatalf("unmarshal generated CRD: %v", err)
	}
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("expected one CRD version, got %d", len(crd.Spec.Versions))
	}

	secrets := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["secrets"]
	for _, field := range []string{"apiKey", "token", "password", "username"} {
		namespace := secrets.Properties[field].Properties["namespace"]
		if namespace.MaxLength == nil || *namespace.MaxLength != 0 {
			t.Errorf("smart-card %s namespace must be forbidden, got maxLength %v", field, namespace.MaxLength)
		}
	}
	headerNamespace := secrets.Properties["headers"].AdditionalProperties.Schema.Properties["namespace"]
	if headerNamespace.MaxLength == nil || *headerNamespace.MaxLength != 0 {
		t.Errorf("smart-card header namespace must be forbidden, got maxLength %v", headerNamespace.MaxLength)
	}
}

func TestKubeconfigSecretNamespaceRemainsUnrestricted(t *testing.T) {
	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "homer.rajsingh.info_dashboards.yaml")
	contents, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("read generated CRD: %v", err)
	}

	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(contents, &crd); err != nil {
		t.Fatalf("unmarshal generated CRD: %v", err)
	}
	schema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	remoteClusters := schema.Properties["spec"].Properties["remoteClusters"].Items.Schema
	secretRef := remoteClusters.Properties["secretRef"]
	if len(secretRef.XValidations) != 0 {
		t.Fatalf("remote kubeconfig SecretRef must remain cross-namespace capable, found validations: %#v", secretRef.XValidations)
	}
	if got := secretRef.Properties["namespace"].Description; got != "Namespace of the Secret. If not specified, defaults to the Dashboard's namespace." {
		t.Errorf("unexpected kubeconfig Secret namespace description: %q", got)
	}
}
