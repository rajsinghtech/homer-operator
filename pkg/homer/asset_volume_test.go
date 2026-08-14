package homer

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestAssetVolumeNameIsStableDNS1123Label(t *testing.T) {
	configMapNames := []string{
		"custom-assets",
		"assets.with.dots",
		strings.Repeat("a", 253),
	}
	fixedVolumeNames := map[string]struct{}{
		"config-volume":         {},
		"assets-volume":         {},
		"operator-state-volume": {},
	}
	seen := make(map[string]string, len(configMapNames))

	for _, configMapName := range configMapNames {
		got := AssetVolumeName(configMapName)
		if errs := validation.IsDNS1123Label(got); len(errs) != 0 {
			t.Errorf("AssetVolumeName(%q) = %q is not a DNS-1123 label: %v", configMapName, got, errs)
		}
		if len(got) > 63 {
			t.Errorf("AssetVolumeName(%q) length = %d, want <= 63", configMapName, len(got))
		}
		if _, fixed := fixedVolumeNames[got]; fixed {
			t.Errorf("AssetVolumeName(%q) collides with fixed volume %q", configMapName, got)
		}
		if previous, exists := seen[got]; exists {
			t.Errorf("AssetVolumeName(%q) collides with AssetVolumeName(%q): %q", configMapName, previous, got)
		}
		seen[got] = configMapName
		if repeat := AssetVolumeName(configMapName); repeat != got {
			t.Errorf("AssetVolumeName(%q) is not deterministic: first %q, second %q", configMapName, got, repeat)
		}

		deployment := CreateDeployment("dashboard", "default", nil, nil, &DeploymentConfig{
			AssetsConfigMapName: configMapName,
		})
		podSpec := deployment.Spec.Template.Spec
		var assetVolume *corev1.Volume
		for i := range podSpec.Volumes {
			if podSpec.Volumes[i].ConfigMap != nil && podSpec.Volumes[i].ConfigMap.Name == configMapName {
				assetVolume = &podSpec.Volumes[i]
				break
			}
		}
		if assetVolume == nil || assetVolume.Name != got {
			t.Errorf("deployment asset volume for %q = %#v, want name %q", configMapName, assetVolume, got)
		}
		var assetMount *corev1.VolumeMount
		for i := range podSpec.Containers[0].VolumeMounts {
			if podSpec.Containers[0].VolumeMounts[i].MountPath == "/custom-assets" {
				assetMount = &podSpec.Containers[0].VolumeMounts[i]
				break
			}
		}
		if assetMount == nil || assetMount.Name != got {
			t.Errorf("deployment asset mount for %q = %#v, want name %q", configMapName, assetMount, got)
		}
	}
}
