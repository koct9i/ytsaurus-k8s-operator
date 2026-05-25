package components

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/ytsaurus/ytsaurus-k8s-operator/api/v1"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/canonize"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/consts"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/ytconfig"
	"go.ytsaurus.tech/yt/go/yson"
	"sigs.k8s.io/yaml"
)

func TestGetTimbertruckConfig(t *testing.T) {
	timbertruckConfig := ytconfig.NewTimbertruckConfig(
		[]v1.StructuredLoggerSpec{
			{
				Category: "Access",
				BaseLoggerSpec: v1.BaseLoggerSpec{
					Name:               "access",
					Format:             v1.LogFormatJson,
					Compression:        v1.LogCompressionNone,
					UseTimestampSuffix: true,
				},
			},
			{
				Category: "Security",
				BaseLoggerSpec: v1.BaseLoggerSpec{
					Name:        "security",
					Format:      v1.LogFormatYson,
					Compression: v1.LogCompressionZstd,
				},
			},
			{
				Category: "Other",
				BaseLoggerSpec: v1.BaseLoggerSpec{
					Name:        "other",
					Format:      v1.LogFormatPlainText,
					Compression: v1.LogCompressionZstd,
				},
			},
		},
		"/yt/master-logs/timbertruck",
		"master",
		"/yt/master-logs",
		"http-proxies-lb.ytsaurus-dev.svc.cluster.local",
		"//sys/admin/logs",
	)

	ysonData, err := timbertruckConfig.ToYSON()
	require.NoError(t, err)

	var config any
	require.NoError(t, yson.Unmarshal(ysonData, &config))
	yamlData, err := yaml.Marshal(config)
	require.NoError(t, err)

	canonize.Assert(t, []byte(strings.TrimSpace(string(yamlData))))
}

func TestBuildTimbertruckVolumeMounts_UsesSpecDerivedMounts(t *testing.T) {
	instanceSpec := &v1.InstanceSpec{
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data-vol", MountPath: "/yt/data"},
			{Name: "logs-vol", MountPath: "/yt/logs"},
		},
		Locations: []v1.LocationSpec{
			{LocationType: v1.LocationTypeLogs, Path: "/yt/logs/master-logs/logs"},
		},
	}

	const configVolumeName = consts.TimbertruckContainerName + "-config"
	mounts, err := buildTimbertruckVolumeMounts(instanceSpec, configVolumeName)
	require.NoError(t, err)

	require.Len(t, mounts, 2)

	assert.Equal(t, "logs-vol", mounts[0].Name)
	assert.Equal(t, "/yt/logs", mounts[0].MountPath)
	assert.False(t, mounts[0].ReadOnly, "timbertruck must be able to write to logs location")

	assert.Equal(t, configVolumeName, mounts[1].Name)
	assert.Equal(t, "/etc/timbertruck", mounts[1].MountPath)
	assert.True(t, mounts[1].ReadOnly, "timbertruck config must be mounted read-only")
}

func TestBuildTimbertruckVolumeMounts_Errors(t *testing.T) {
	const configVolumeName = consts.TimbertruckContainerName + "-config"

	// No logs location defined.
	instanceSpec := &v1.InstanceSpec{
		VolumeMounts: []corev1.VolumeMount{
			{Name: "logs-vol", MountPath: "/yt/logs"},
		},
	}
	_, err := buildTimbertruckVolumeMounts(instanceSpec, configVolumeName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve mounts")

	// Logs location not covered by any volume mount.
	instanceSpec.Locations = []v1.LocationSpec{
		{LocationType: v1.LocationTypeLogs, Path: "/yt/logs-wrong/master-logs/logs"},
	}
	_, err = buildTimbertruckVolumeMounts(instanceSpec, configVolumeName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve mounts")
}
