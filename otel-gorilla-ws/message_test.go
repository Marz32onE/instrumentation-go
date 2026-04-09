package otelgorillaws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalWire_EnvelopeFormat(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
		TracestateHeader:  "k=v",
		"baggage":         "a=b",
	}
	raw, err := marshalWire(carrier, []byte(`{"x":1}`))
	require.NoError(t, err)

	var env wireEnvelope
	require.NoError(t, json.Unmarshal(raw, &env))

	// traceparent and tracestate must be in header
	assert.Equal(t, "00-12345678901234567890123456789012-0123456789012345-01", env.Header[TraceparentHeader])
	assert.Equal(t, "k=v", env.Header[TracestateHeader])

	// baggage must NOT appear in header
	assert.NotContains(t, env.Header, "baggage")

	// original user data must be preserved in data field
	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(env.Data, &data))
	var x int
	require.NoError(t, json.Unmarshal(data["x"], &x))
	assert.Equal(t, 1, x)

	// trace fields must NOT appear at top level or inside data
	assert.NotContains(t, data, TraceparentHeader, "traceparent must not leak into data field")
}

func TestMarshalWire_NonObjectPayload_WrappedInEnvelope(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
	}

	cases := []struct {
		payload  string
		wantData string
	}{
		{`"hello"`, `"hello"`},
		{`[1,2,3]`, `[1,2,3]`},
		{`42`, `42`},
	}

	for _, tc := range cases {
		raw, err := marshalWire(carrier, []byte(tc.payload))
		require.NoError(t, err, "marshalWire(%s)", tc.payload)

		var env wireEnvelope
		require.NoError(t, json.Unmarshal(raw, &env), "unmarshal envelope for %s", tc.payload)
		require.NotNil(t, env.Header, "expected non-nil header for payload %s", tc.payload)
		assert.NotEmpty(t, env.Header[TraceparentHeader], "expected traceparent in header for payload %s", tc.payload)
		assert.Equal(t, tc.wantData, string(env.Data), "data field for payload %s", tc.payload)
	}
}

func TestMarshalWire_NonJSONPayload_WrappedAsString(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
	}
	raw, err := marshalWire(carrier, []byte("plain text"))
	require.NoError(t, err)

	var env wireEnvelope
	require.NoError(t, json.Unmarshal(raw, &env))
	// data must be the JSON-encoded string
	var s string
	require.NoError(t, json.Unmarshal(env.Data, &s))
	assert.Equal(t, "plain text", s)
}

func TestTryUnmarshalWire_EnvelopeFormat(t *testing.T) {
	input := `{"header":{"traceparent":"00-aabb-01","tracestate":"k=v"},"data":{"x":1,"y":"hello"}}`
	payload, hdrs, ok := tryUnmarshalWire([]byte(input))
	require.True(t, ok)
	assert.Equal(t, "00-aabb-01", hdrs[TraceparentHeader])
	assert.Equal(t, "k=v", hdrs[TracestateHeader])

	// payload must be the data field contents
	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &out))
	var x int
	require.NoError(t, json.Unmarshal(out["x"], &x))
	assert.Equal(t, 1, x)
	// trace fields must not appear in the returned payload
	assert.NotContains(t, out, TraceparentHeader, "traceparent must not appear in returned payload")
}

func TestTryUnmarshalWire_LegacyFlatFormat(t *testing.T) {
	// Old Go flat-inject format — must still be parseable for backward compat.
	input := `{"x":1,"traceparent":"00-aabb-01","tracestate":"k=v","y":"hello"}`
	payload, hdrs, ok := tryUnmarshalWire([]byte(input))
	require.True(t, ok, "expected ok=true for legacy flat format")
	assert.Equal(t, "00-aabb-01", hdrs[TraceparentHeader])
	assert.Equal(t, "k=v", hdrs[TracestateHeader])

	// trace fields must be stripped from the returned payload
	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &out))
	assert.NotContains(t, out, TraceparentHeader, "traceparent must be stripped from legacy payload")
	assert.NotContains(t, out, TracestateHeader, "tracestate must be stripped from legacy payload")
	var x int
	require.NoError(t, json.Unmarshal(out["x"], &x))
	assert.Equal(t, 1, x)
}

func TestTryUnmarshalWire_NonObject_ReturnsFalse(t *testing.T) {
	for _, input := range []string{`"hello"`, `[1,2]`, `42`, `not-json`} {
		_, _, ok := tryUnmarshalWire([]byte(input))
		assert.False(t, ok, "tryUnmarshalWire(%q) expected ok=false", input)
	}
}
