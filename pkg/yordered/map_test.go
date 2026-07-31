package yordered

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.ytsaurus.tech/yt/go/yson"
)

func TestMapMarshalUnmarshalPreservesOrder(t *testing.T) {
	input := []byte(`{
		first = 1;
		nested = { second = 2; first = 1; };
		list = [{ beta = 2; alpha = 1; }];
		attributed = <second = 2; first = 1;> { right = 2; left = 1; };
		last = 2;
	}`)

	m := NewMap()
	require.NoError(t, yson.Unmarshal(input, m))
	require.Equal(t, []string{"first", "nested", "list", "attributed", "last"}, m.Keys())

	nested, ok := getMap(m, "nested")
	require.True(t, ok)
	require.Equal(t, []string{"second", "first"}, nested.Keys())

	marshaled, err := yson.MarshalFormat(m, yson.FormatPretty)
	require.NoError(t, err)
	require.NoError(t, yson.Valid(marshaled))

	roundTripped := NewMap()
	require.NoError(t, yson.Unmarshal(marshaled, roundTripped))
	require.Equal(t, m.Keys(), roundTripped.Keys())
}

func TestMapSetKeepsExistingKeyPosition(t *testing.T) {
	m := NewMap()
	m.Set("first", 1)
	m.Set("second", 2)
	m.Set("first", 3)

	require.Equal(t, []string{"first", "second"}, m.Keys())
	value, ok := m.Get("first")
	require.True(t, ok)
	require.EqualValues(t, 3, value)
}

func getMap(m *Map, key string) (*Map, bool) {
	value, ok := m.Get(key)
	if !ok {
		return nil, false
	}
	nested, ok := value.(*Map)
	return nested, ok
}
