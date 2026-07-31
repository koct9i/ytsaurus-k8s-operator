package components

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"go.ytsaurus.tech/yt/go/yson"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/apiproxy"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/consts"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/labeller"
	"github.com/ytsaurus/ytsaurus-k8s-operator/pkg/resources"
)

const (
	JsPrologue string = "module.exports = "
)

type ConfigFormat string

const (
	ConfigFormatText               ConfigFormat = "text"
	ConfigFormatYson               ConfigFormat = "yson"
	ConfigFormatJson               ConfigFormat = "json"
	ConfigFormatJsonWithJsPrologue ConfigFormat = "json_with_js_prologue"
	ConfigFormatToml               ConfigFormat = "toml"
	ConfigFormatYaml               ConfigFormat = "yaml"
)

type TextGeneratorFunc func() ([]string, error)

type ConfigGeneratorFunc func() ([]byte, error)

type ConfigGenerator struct {
	FileName string
	// Format is the desired serialization format for config map.
	// Note that conversion from YSON to Format (if needed) is performed as a very last
	// step of config generation pipeline.
	Format ConfigFormat
	// Generator must generate config in YSON.
	Generator ConfigGeneratorFunc
}

type ConfigMapBuilder struct {
	labeller *labeller.Labeller
	apiProxy apiproxy.APIProxy

	generators []ConfigGenerator

	configOverrides *corev1.LocalObjectReference
	overridesMap    corev1.ConfigMap

	configMap *resources.ConfigMap
}

func NewConfigMapBuilder(
	labeller *labeller.Labeller,
	apiProxy apiproxy.APIProxy,
	name string,
	configOverrides *corev1.LocalObjectReference,
	generators ...ConfigGenerator,
) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		labeller:        labeller,
		apiProxy:        apiProxy,
		configOverrides: configOverrides,
		configMap:       resources.NewConfigMap(name, labeller, apiProxy),
		generators:      generators,
	}
}

func (h *ConfigMapBuilder) AddGenerator(fileName string, format ConfigFormat, generator ConfigGeneratorFunc) {
	h.generators = append(h.generators, ConfigGenerator{
		FileName:  fileName,
		Format:    format,
		Generator: generator,
	})
}

type orderedYSONMapEntry struct {
	key   string
	value any
}

// orderedYSONMap represents a YSON map without losing the order in which its
// fields were decoded. In particular, this keeps generated configuration
// stable when overrides are applied.
type orderedYSONMap []orderedYSONMapEntry

type orderedYSONValueWithAttrs struct {
	attrs orderedYSONMap
	value any
}

func (m *orderedYSONMap) UnmarshalYSON(data []byte) error {
	r := yson.NewReaderFromBytes(data)
	value, err := decodeOrderedYSONValue(r)
	if err != nil {
		return err
	}
	decoded, ok := value.(*orderedYSONMap)
	if !ok {
		return fmt.Errorf("expected YSON map, got %T", value)
	}
	*m = *decoded
	return nil
}

func decodeOrderedYSONValue(r *yson.Reader) (any, error) {
	event, err := r.Next(false)
	if err != nil {
		return nil, err
	}
	switch event {
	case yson.EventBeginAttrs:
		attrs := orderedYSONMap{}
		for {
			hasKey, err := r.NextKey()
			if err != nil {
				return nil, err
			}
			if !hasKey {
				break
			}
			key := r.String()
			value, err := decodeOrderedYSONValue(r)
			if err != nil {
				return nil, err
			}
			attrs = append(attrs, orderedYSONMapEntry{key: key, value: value})
		}
		if event, err := r.Next(false); err != nil {
			return nil, err
		} else if event != yson.EventEndAttrs {
			return nil, fmt.Errorf("expected end of YSON attributes, got %v", event)
		}
		value, err := decodeOrderedYSONValue(r)
		if err != nil {
			return nil, err
		}
		return orderedYSONValueWithAttrs{attrs: attrs, value: value}, nil
	case yson.EventBeginMap:
		m := orderedYSONMap{}
		for {
			hasKey, err := r.NextKey()
			if err != nil {
				return nil, err
			}
			if !hasKey {
				break
			}

			key := r.String()
			value, err := decodeOrderedYSONValue(r)
			if err != nil {
				return nil, err
			}
			m = append(m, orderedYSONMapEntry{key: key, value: value})
		}
		if event, err := r.Next(false); err != nil {
			return nil, err
		} else if event != yson.EventEndMap {
			return nil, fmt.Errorf("expected end of YSON map, got %v", event)
		}
		return &m, nil
	case yson.EventBeginList:
		var list []any
		for {
			hasItem, err := r.NextListItem()
			if err != nil {
				return nil, err
			}
			if !hasItem {
				break
			}
			item, err := decodeOrderedYSONValue(r)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		if event, err := r.Next(false); err != nil {
			return nil, err
		} else if event != yson.EventEndList {
			return nil, fmt.Errorf("expected end of YSON list, got %v", event)
		}
		return list, nil
	case yson.EventLiteral:
		switch r.Type() {
		case yson.TypeEntity:
			return nil, nil
		case yson.TypeBool:
			return r.Bool(), nil
		case yson.TypeString:
			return r.String(), nil
		case yson.TypeInt64:
			return r.Int64(), nil
		case yson.TypeUint64:
			return r.Uint64(), nil
		case yson.TypeFloat64:
			return r.Float64(), nil
		}
	}
	return nil, fmt.Errorf("unexpected YSON event %v", event)
}

func (m orderedYSONMap) MarshalYSON(w *yson.Writer) error {
	w.BeginMap()
	for _, entry := range m {
		w.MapKeyString(entry.key)
		w.Any(entry.value)
	}
	w.EndMap()
	return w.Err()
}

func (v orderedYSONValueWithAttrs) MarshalYSON(w *yson.Writer) error {
	w.BeginAttrs()
	for _, attr := range v.attrs {
		w.MapKeyString(attr.key)
		w.Any(attr.value)
	}
	w.EndAttrs()
	w.Any(v.value)
	return w.Err()
}

func mergeYSONMaps(dst, src orderedYSONMap) orderedYSONMap {
	indices := make(map[string]int, len(dst))
	for i, entry := range dst {
		indices[entry.key] = i
	}

	for _, srcEntry := range src {
		if i, ok := indices[srcEntry.key]; ok {
			dstMap, dstIsMap := dst[i].value.(*orderedYSONMap)
			srcMap, srcIsMap := srcEntry.value.(*orderedYSONMap)
			if dstIsMap && srcIsMap {
				merged := mergeYSONMaps(*dstMap, *srcMap)
				dst[i].value = &merged
			} else {
				dst[i].value = srcEntry.value
			}
			continue
		}

		indices[srcEntry.key] = len(dst)
		dst = append(dst, srcEntry)
	}
	return dst
}

func overrideYsonConfigs(base []byte, overrides []byte) ([]byte, error) {
	b := orderedYSONMap{}
	err := yson.Unmarshal(base, &b)
	if err != nil {
		return base, err
	}

	o := orderedYSONMap{}
	err = yson.Unmarshal(overrides, &o)
	if err != nil {
		return base, err
	}

	merged := mergeYSONMaps(b, o)
	return yson.MarshalFormat(merged, yson.FormatPretty)
}

func (h *ConfigMapBuilder) GetConfigMapName() string {
	return h.configMap.Name()
}

func (h *ConfigMapBuilder) getConfig(descriptor ConfigGenerator) ([]byte, error) {
	name := h.GetConfigMapName()
	serializedConfig, err := descriptor.Generator()
	if err != nil {
		h.apiProxy.RecordWarning("Failure", fmt.Sprintf("Cannot build configmap/%s %q: %s",
			name, descriptor.FileName, err))
		return nil, fmt.Errorf("failed to generate configmap/%s %q: %w", name, descriptor.FileName, err)
	}

	if h.overridesMap.GetResourceVersion() != "" {
		overrideNames := []string{
			descriptor.FileName,
			fmt.Sprintf("%s--%s", name, descriptor.FileName),
		}
		for _, overrideName := range overrideNames {
			if value, ok := h.overridesMap.Data[overrideName]; ok {
				configWithOverrides, err := overrideYsonConfigs(serializedConfig, []byte(value))
				if err != nil {
					h.apiProxy.RecordWarning("Failure", fmt.Sprintf("Cannot apply configmap/%s %q override %q: %s",
						name, descriptor.FileName, overrideName, err))
					return nil, fmt.Errorf("failed to apply configmap/%s %q override %q: %w",
						name, descriptor.FileName, overrideName, err)
				}
				h.apiProxy.RecordNormal("Override", fmt.Sprintf("Applied configmap/%s %q override %q",
					name, descriptor.FileName, overrideName))
				serializedConfig = configWithOverrides
			}
		}
	}

	switch descriptor.Format {
	case ConfigFormatJson, ConfigFormatJsonWithJsPrologue:
		var config any
		err := yson.Unmarshal(serializedConfig, &config)
		if err != nil {
			return nil, err
		}
		serializedConfig, err = json.Marshal(config)
		if err != nil {
			return nil, err
		}
		if descriptor.Format == ConfigFormatJsonWithJsPrologue {
			serializedConfig = append([]byte(JsPrologue), serializedConfig...)
		}
	case ConfigFormatToml:
		var config any
		err := yson.Unmarshal(serializedConfig, &config)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		err = toml.NewEncoder(&buf).Encode(config)
		if err != nil {
			return nil, err
		}
		serializedConfig = buf.Bytes()
	case ConfigFormatYaml:
		var config any
		if err := yson.Unmarshal(serializedConfig, &config); err != nil {
			return nil, err
		}
		var err error
		serializedConfig, err = yaml.Marshal(config)
		if err != nil {
			return nil, err
		}
	}

	return serializedConfig, nil
}

func (h *ConfigMapBuilder) getCurrentConfigValue(fileName string) []byte {
	if !h.configMap.Exists() {
		return nil
	}

	data, exists := h.configMap.OldObject().Data[fileName]
	if !exists {
		return nil
	}
	return []byte(data)
}

func (h *ConfigMapBuilder) getAnnotation(ann string) string {
	return h.configMap.OldObject().Annotations[ann]
}

func (h *ConfigMapBuilder) setAnnotation(ann, value string) {
	metav1.SetMetaDataAnnotation(&h.configMap.NewObject().ObjectMeta, ann, value)
}

func (h *ConfigMapBuilder) needReload() (ComponentStatus, error) {
	for _, descriptor := range h.generators {
		newConfig, err := h.getConfig(descriptor)
		if err != nil {
			return ComponentStatusBlocked("Config %s generation error: %v", descriptor.FileName, err), err
		}
		curConfig := h.getCurrentConfigValue(descriptor.FileName)
		if !cmp.Equal(curConfig, newConfig) {
			if curConfig == nil {
				h.apiProxy.RecordNormal(
					"Reconciliation",
					fmt.Sprintf("Config %s needs creation", descriptor.FileName))
				return ComponentStatusNeedUpdate("Config %s needs creation", descriptor.FileName), nil
			} else {
				configsDiff := cmp.Diff(string(curConfig), string(newConfig))
				h.apiProxy.RecordNormal(
					"Reconciliation",
					fmt.Sprintf("Config %s needs reload. Diff: %s", descriptor.FileName, configsDiff))
				return ComponentStatusNeedUpdate("Config %s needs reload", descriptor.FileName), nil
			}
		}
	}
	return ComponentStatusReady(), nil
}

func (h *ConfigMapBuilder) Build() (*corev1.ConfigMap, error) {
	cm := h.configMap.Build()

	if version := h.overridesMap.ResourceVersion; version != "" {
		metav1.SetMetaDataAnnotation(&cm.ObjectMeta, consts.ConfigOverridesVersionAnnotationName, version)
	}

	for _, descriptor := range h.generators {
		data, err := h.getConfig(descriptor)
		if err != nil {
			return nil, err
		}
		if data != nil {
			cm.Data[descriptor.FileName] = string(data)
		}
	}

	currentHash, err := resources.Hash(cm.Data)
	if err != nil {
		return nil, err
	}
	metav1.SetMetaDataAnnotation(&cm.ObjectMeta, consts.ConfigHashAnnotationName, currentHash)

	return cm, nil
}

func (h *ConfigMapBuilder) Sync(ctx context.Context) error {
	status, err := h.needReload()
	if err != nil {
		return err
	}
	if h.Exists() && !status.IsNeedUpdate() {
		return nil
	}
	return h.configMap.Sync(ctx)
}

func (h *ConfigMapBuilder) Fetch(ctx context.Context) error {
	if h.configOverrides != nil {
		name := h.configOverrides.Name
		err := h.apiProxy.FetchObject(ctx, name, &h.overridesMap)
		if err != nil {
			return err
		}
	}
	return h.configMap.Fetch(ctx)
}

func (h *ConfigMapBuilder) Exists() bool {
	return h.configMap.Exists()
}
