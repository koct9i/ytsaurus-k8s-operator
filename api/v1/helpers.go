package v1

import (
	"fmt"
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

func ResolveMountsForLocations(
	locations []LocationSpec,
	volumeMounts []corev1.VolumeMount,
	requiredLocations []LocationType,
) ([]corev1.VolumeMount, error) {
	seen := make(map[string]struct{})
	resolvedMounts := make([]corev1.VolumeMount, 0, len(requiredLocations))
	for _, requiredLocation := range requiredLocations {
		location := FindFirstLocation(locations, requiredLocation)
		if location == nil {
			return nil, fmt.Errorf("no location of type %q found", requiredLocation)
		}
		volumeMount := FindVolumeMountForPath(volumeMounts, location.Path)
		if volumeMount == nil {
			return nil, fmt.Errorf("no volume mount covers location %q (path %q)", requiredLocation, location.Path)
		}
		if _, ok := seen[volumeMount.MountPath]; ok {
			continue
		}
		seen[volumeMount.MountPath] = struct{}{}
		resolvedMounts = append(resolvedMounts, *volumeMount)
	}
	return resolvedMounts, nil
}
