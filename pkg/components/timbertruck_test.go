package components

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.ytsaurus.tech/yt/go/yson"
	v1 "github.com/ytsaurus/ytsaurus-k8s-operator/api/v1"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/canonize"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/ytconfig"
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
