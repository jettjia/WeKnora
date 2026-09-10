package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/semantic/cubeclient"
)

func TestParseCubeQueryArgsStringifiedFields(t *testing.T) {
	// The agent pipeline may deliver array/object arguments as JSON strings.
	raw := json.RawMessage(`{
		"measures": "[\"orders.count\"]",
		"dimensions": "[\"orders.status\"]",
		"filters": "[{\"member\":\"orders.status\",\"operator\":\"equals\",\"values\":[\"active\"]}]",
		"order": "{\"orders.count\":\"desc\"}"
	}`)
	in, err := parseCubeQueryArgs(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"orders.count"}, in.Measures)
	assert.Equal(t, []string{"orders.status"}, in.Dimensions)
	require.Len(t, in.Filters, 1)
	assert.Equal(t, "orders.status", in.Filters[0].Member)
	assert.Equal(t, map[string]string{"orders.count": "desc"}, in.Order)
}

func TestParseCubeQueryArgsNativeJSON(t *testing.T) {
	raw := json.RawMessage(`{"measures": ["orders.count"], "limit": 10}`)
	in, err := parseCubeQueryArgs(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"orders.count"}, in.Measures)
}

func TestParseCubeQueryArgsRequiresMember(t *testing.T) {
	_, err := parseCubeQueryArgs(json.RawMessage(`{}`))
	assert.Error(t, err)
	_, err = parseCubeQueryArgs(nil)
	assert.Error(t, err)
}

func TestParseCubeQueryArgsPlainStringFieldNotDecoded(t *testing.T) {
	// A filter value that is a legitimate string must not be mangled: only
	// the container fields listed for the lenient pass get unwrapped.
	raw := json.RawMessage(`{"dimensions": "orders.status"}`)
	_, err := parseCubeQueryArgs(raw)
	// "orders.status" is not valid JSON for an array, so binding fails.
	assert.Error(t, err)
}

func TestFilterBoundCubes(t *testing.T) {
	cubes := []cubeclient.MetaCube{{Name: "orders"}, {Name: "users"}, {Name: "events"}}
	bound := filterBoundCubes(cubes, []string{"orders", "events"})
	require.Len(t, bound, 2)
	assert.Equal(t, "orders", bound[0].Name)
	assert.Equal(t, "events", bound[1].Name)

	// Empty bound list means no orchestration scoping.
	assert.Len(t, filterBoundCubes(cubes, nil), 3)
}

func TestFormatCubeRows(t *testing.T) {
	out, n := formatCubeRows([]map[string]interface{}{
		{"orders.status": "active", "orders.count": 42},
	})
	assert.Equal(t, 1, n)
	assert.Contains(t, out, "orders.status: active")
	assert.Contains(t, out, "orders.count: 42")

	empty, n := formatCubeRows(nil)
	assert.Equal(t, 0, n)
	assert.Equal(t, "(no data)", empty)
}
