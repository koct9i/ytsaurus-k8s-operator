package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	ytv1 "github.com/ytsaurus/ytsaurus-k8s-operator/api/v1"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/consts"
)

func TestAddHydraPersistenceUploaderToPodSpec_UsesSpecDerivedMounts(t *testing.T) {
	type wantDataMount struct {
		Name      string
		MountPath string
	}
	tests := []struct {
		name             string
		specVolumeMounts []corev1.VolumeMount
		locations        []ytv1.LocationSpec
		wantDataMounts   []wantDataMount
	}{
		{
			name: "default spec — master-data at /yt/master-data",
			specVolumeMounts: []corev1.VolumeMount{
				{Name: "master-data", MountPath: "/yt/master-data"},
			},
			locations: []ytv1.LocationSpec{
				{LocationType: ytv1.LocationTypeMasterChangelogs, Path: "/yt/master-data/master-changelogs"},
				{LocationType: ytv1.LocationTypeMasterSnapshots, Path: "/yt/master-data/master-snapshots"},
			},
			wantDataMounts: []wantDataMount{
				{Name: "master-data", MountPath: "/yt/master-data"},
			},
		},
		{
			name: "custom volume name and path",
			specVolumeMounts: []corev1.VolumeMount{
				{Name: "storage", MountPath: "/data/master"},
			},
			locations: []ytv1.LocationSpec{
				{LocationType: ytv1.LocationTypeMasterChangelogs, Path: "/data/master/changelogs"},
				{LocationType: ytv1.LocationTypeMasterSnapshots, Path: "/data/master/snapshots"},
			},
			wantDataMounts: []wantDataMount{
				{Name: "storage", MountPath: "/data/master"},
			},
		},
		{
			name: "snapshots and changelogs on different volumes",
			specVolumeMounts: []corev1.VolumeMount{
				{Name: "snapshots-vol", MountPath: "/yt/snapshots"},
				{Name: "changelogs-vol", MountPath: "/yt/changelogs"},
			},
			locations: []ytv1.LocationSpec{
				{LocationType: ytv1.LocationTypeMasterSnapshots, Path: "/yt/snapshots/data"},
				{LocationType: ytv1.LocationTypeMasterChangelogs, Path: "/yt/changelogs/data"},
			},
			wantDataMounts: []wantDataMount{
				{Name: "snapshots-vol", MountPath: "/yt/snapshots"},
				{Name: "changelogs-vol", MountPath: "/yt/changelogs"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instanceSpec := &ytv1.InstanceSpec{
				VolumeMounts: tc.specVolumeMounts,
				Locations:    tc.locations,
			}

			mainContainerMounts := createVolumeMounts(tc.specVolumeMounts)
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:         "ytserver",
						VolumeMounts: mainContainerMounts,
					},
				},
			}

			err := addHydraPersistenceUploaderToPodSpec(
				"ghcr.io/ytsaurus/sidecars:0.0.1",
				podSpec,
				"http-proxies.ytsaurus-dev.svc",
				"robot-hydra-persistence-uploader-secret",
				instanceSpec,
			)
			require.NoError(t, err)

			var sidecar *corev1.Container
			for i := range podSpec.Containers {
				if podSpec.Containers[i].Name == consts.HydraPersistenceUploaderContainerName {
					sidecar = &podSpec.Containers[i]
					break
				}
			}
			require.NotNil(t, sidecar, "hydra-persistence-uploader sidecar must be added")

			assert.Len(t, sidecar.VolumeMounts, 1+len(tc.wantDataMounts)+1)

			for _, want := range tc.wantDataMounts {
				var got *corev1.VolumeMount
				for i, vm := range sidecar.VolumeMounts {
					if vm.Name == want.Name {
						got = &sidecar.VolumeMounts[i]
						break
					}
				}
				require.NotNil(t, got, "data volume mount %q must be present", want.Name)
				assert.Equal(t, want.MountPath, got.MountPath)
				assert.True(t, got.ReadOnly, "data mount %q must be read-only", want.Name)
			}

			var hasSharedBinariesVolume bool
			for _, v := range podSpec.Volumes {
				if v.Name == "shared-binaries" {
					hasSharedBinariesVolume = true
					require.NotNil(t, v.EmptyDir, "shared-binaries must be backed by emptyDir")
					break
				}
			}
			assert.True(t, hasSharedBinariesVolume)

			var ytserver *corev1.Container
			for i := range podSpec.Containers {
				if podSpec.Containers[i].Name == "ytserver" {
					ytserver = &podSpec.Containers[i]
					break
				}
			}
			require.NotNil(t, ytserver)
			require.NotNil(t, ytserver.Lifecycle)
			require.NotNil(t, ytserver.Lifecycle.PostStart)
			var hasShared bool
			for _, vm := range ytserver.VolumeMounts {
				if vm.Name == "shared-binaries" {
					hasShared = true
					break
				}
			}
			assert.True(t, hasShared)
		})
	}
}

func TestAddHydraPersistenceUploaderToPodSpec_ErrorsOnMissingLocation(t *testing.T) {
	instanceSpec := &ytv1.InstanceSpec{
		VolumeMounts: []corev1.VolumeMount{
			{Name: "master-data", MountPath: "/yt/master-data"},
		},
		Locations: []ytv1.LocationSpec{
			{LocationType: ytv1.LocationTypeMasterChangelogs, Path: "/yt/master-data/master-changelogs"},
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "ytserver"}},
	}

	err := addHydraPersistenceUploaderToPodSpec(
		"img", podSpec, "proxy", "secret", instanceSpec,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MasterSnapshots")

	for _, c := range podSpec.Containers {
		assert.NotEqual(t, consts.HydraPersistenceUploaderContainerName, c.Name)
	}
}
