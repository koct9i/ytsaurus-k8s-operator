// Package yordered provides YSON values that preserve map field order.
package yordered

import (
	"fmt"

	"go.ytsaurus.tech/yt/go/yson"
)

// Map is a YSON map that retains the order in which keys are inserted.
type Map struct {
	values map[string]any
	keys   []string
}

// NewMap creates an empty Map.
func NewMap() *Map {
	return &Map{values: map[string]any{}}
}

// Get returns the value associated with key.
func (m *Map) Get(key string) (any, bool) {
	value, ok := m.values[key]
	return value, ok
}

// Set associates value with key. Replacing a value does not change key order.
func (m *Map) Set(key string, value any) {
	if m.values == nil {
		m.values = map[string]any{}
	}
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// Keys returns the keys in insertion order.
func (m *Map) Keys() []string {
	return append([]string(nil), m.keys...)
}

// UnmarshalYSON implements yson.StreamUnmarshaler.
func (m *Map) UnmarshalYSON(r *yson.Reader) error {
	value, err := decodeValue(r)
	if err != nil {
		return err
	}
	decoded, ok := value.(*Map)
	if !ok {
		return fmt.Errorf("expected YSON map, got %T", value)
	}
	m.values = decoded.values
	m.keys = decoded.keys
	return nil
}

// MarshalYSON implements yson.StreamMarshaler.
func (m *Map) MarshalYSON(w *yson.Writer) error {
	w.BeginMap()
	for _, key := range m.keys {
		w.MapKeyString(key)
		w.Any(m.values[key])
	}
	w.EndMap()
	return w.Err()
}

type valueWithAttrs struct {
	attrs *Map
	value any
}

func (v valueWithAttrs) MarshalYSON(w *yson.Writer) error {
	w.BeginAttrs()
	for _, key := range v.attrs.keys {
		w.MapKeyString(key)
		w.Any(v.attrs.values[key])
	}
	w.EndAttrs()
	w.Any(v.value)
	return w.Err()
}

func decodeValue(r *yson.Reader) (any, error) {
	event, err := r.Next(false)
	if err != nil {
		return nil, err
	}

	switch event {
	case yson.EventBeginAttrs:
		attrs, err := decodeMapBody(r, yson.EventEndAttrs)
		if err != nil {
			return nil, err
		}
		value, err := decodeValue(r)
		if err != nil {
			return nil, err
		}
		return valueWithAttrs{attrs: attrs, value: value}, nil
	case yson.EventBeginMap:
		return decodeMapBody(r, yson.EventEndMap)
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
			item, err := decodeValue(r)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		if err := expectEvent(r, yson.EventEndList); err != nil {
			return nil, err
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

func decodeMapBody(r *yson.Reader, end yson.Event) (*Map, error) {
	m := NewMap()
	for {
		hasKey, err := r.NextKey()
		if err != nil {
			return nil, err
		}
		if !hasKey {
			break
		}
		key := r.String()
		value, err := decodeValue(r)
		if err != nil {
			return nil, err
		}
		m.Set(key, value)
	}
	if err := expectEvent(r, end); err != nil {
		return nil, err
	}
	return m, nil
}

func expectEvent(r *yson.Reader, expected yson.Event) error {
	event, err := r.Next(false)
	if err != nil {
		return err
	}
	if event != expected {
		return fmt.Errorf("expected YSON event %v, got %v", expected, event)
	}
	return nil
}

var (
	_ yson.StreamMarshaler   = (*Map)(nil)
	_ yson.StreamUnmarshaler = (*Map)(nil)
)
