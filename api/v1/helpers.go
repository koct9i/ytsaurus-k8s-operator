package v1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func FindFirstLocation(locations []LocationSpec, locationType LocationType) *LocationSpec {
	for _, location := range locations {
		if location.LocationType == locationType {
			return &location
		}
	}
	return nil
}

func FindAllLocations(locations []LocationSpec, locationType LocationType) []LocationSpec {
	result := make([]LocationSpec, 0)
	for _, location := range locations {
		if location.LocationType == locationType {
			result = append(result, location)
		}
	}
	return result
}

func FindVolumeMountForPath(volumeMounts []corev1.VolumeMount, path string) *corev1.VolumeMount {
	var actual *corev1.VolumeMount
	for i := range volumeMounts {
		volumeMount := &volumeMounts[i]
		mountPath := volumeMount.MountPath
		normalizedMount := strings.TrimRight(mountPath, "/") + "/"
		normalizedPath := strings.TrimRight(path, "/") + "/"
		if strings.HasPrefix(normalizedPath, normalizedMount) {
			if actual == nil || len(volumeMount.MountPath) > len(actual.MountPath) {
				actual = volumeMount
			}
		}
	}
	return actual
}
