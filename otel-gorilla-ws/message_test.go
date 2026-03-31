package otelgorillaws

import (
	"encoding/json"
	"testing"
)

func TestMarshalWire_EnvelopeFormat(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
		TracestateHeader:  "k=v",
		"baggage":         "a=b",
	}
	raw, err := marshalWire(carrier, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("marshalWire error: %v", err)
	}

	var env wireEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope error: %v", err)
	}

	// traceparent and tracestate must be in header
	if env.Header[TraceparentHeader] != "00-12345678901234567890123456789012-0123456789012345-01" {
		t.Fatalf("expected traceparent in header, got %q", env.Header[TraceparentHeader])
	}
	if env.Header[TracestateHeader] != "k=v" {
		t.Fatalf("expected tracestate in header, got %q", env.Header[TracestateHeader])
	}

	// baggage must NOT appear in header
	if _, ok := env.Header["baggage"]; ok {
		t.Fatalf("unexpected baggage in header")
	}

	// original user data must be preserved in data field
	var data map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data field error: %v", err)
	}
	var x int
	if err := json.Unmarshal(data["x"], &x); err != nil || x != 1 {
		t.Fatalf("expected x=1 in data, got %v", x)
	}

	// trace fields must NOT appear at top level or inside data
	if _, ok := data[TraceparentHeader]; ok {
		t.Fatalf("traceparent must not leak into data field")
	}
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
		if err != nil {
			t.Fatalf("marshalWire(%s) error: %v", tc.payload, err)
		}

		var env wireEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("unmarshal envelope for %s error: %v", tc.payload, err)
		}
		if env.Header == nil {
			t.Fatalf("expected non-nil header for payload %s", tc.payload)
		}
		if env.Header[TraceparentHeader] == "" {
			t.Fatalf("expected traceparent in header for payload %s", tc.payload)
		}
		if string(env.Data) != tc.wantData {
			t.Fatalf("data field for %s = %q, want %q", tc.payload, string(env.Data), tc.wantData)
		}
	}
}

func TestMarshalWire_NonJSONPayload_WrappedAsString(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
	}
	raw, err := marshalWire(carrier, []byte("plain text"))
	if err != nil {
		t.Fatalf("marshalWire error: %v", err)
	}

	var env wireEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope error: %v", err)
	}
	// data must be the JSON-encoded string
	var s string
	if err := json.Unmarshal(env.Data, &s); err != nil || s != "plain text" {
		t.Fatalf("expected data=\"plain text\", got %q", string(env.Data))
	}
}

func TestTryUnmarshalWire_EnvelopeFormat(t *testing.T) {
	input := `{"header":{"traceparent":"00-aabb-01","tracestate":"k=v"},"data":{"x":1,"y":"hello"}}`
	payload, hdrs, ok := tryUnmarshalWire([]byte(input))
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if hdrs[TraceparentHeader] != "00-aabb-01" {
		t.Fatalf("traceparent = %q, want 00-aabb-01", hdrs[TraceparentHeader])
	}
	if hdrs[TracestateHeader] != "k=v" {
		t.Fatalf("tracestate = %q, want k=v", hdrs[TracestateHeader])
	}

	// payload must be the data field contents
	var out map[string]json.RawMessage
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal payload error: %v", err)
	}
	var x int
	if err := json.Unmarshal(out["x"], &x); err != nil || x != 1 {
		t.Fatalf("expected x=1 in payload, got %v", x)
	}
	// trace fields must not appear in the returned payload
	if _, found := out[TraceparentHeader]; found {
		t.Fatalf("traceparent must not appear in returned payload")
	}
}

func TestTryUnmarshalWire_LegacyFlatFormat(t *testing.T) {
	// Old Go flat-inject format — must still be parseable for backward compat.
	input := `{"x":1,"traceparent":"00-aabb-01","tracestate":"k=v","y":"hello"}`
	payload, hdrs, ok := tryUnmarshalWire([]byte(input))
	if !ok {
		t.Fatalf("expected ok=true for legacy flat format")
	}
	if hdrs[TraceparentHeader] != "00-aabb-01" {
		t.Fatalf("traceparent = %q, want 00-aabb-01", hdrs[TraceparentHeader])
	}
	if hdrs[TracestateHeader] != "k=v" {
		t.Fatalf("tracestate = %q, want k=v", hdrs[TracestateHeader])
	}

	// trace fields must be stripped from the returned payload
	var out map[string]json.RawMessage
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal payload error: %v", err)
	}
	if _, found := out[TraceparentHeader]; found {
		t.Fatalf("traceparent must be stripped from legacy payload")
	}
	if _, found := out[TracestateHeader]; found {
		t.Fatalf("tracestate must be stripped from legacy payload")
	}
	var x int
	if err := json.Unmarshal(out["x"], &x); err != nil || x != 1 {
		t.Fatalf("expected x=1 in payload, got %v", x)
	}
}

func TestTryUnmarshalWire_NonObject_ReturnsFalse(t *testing.T) {
	for _, input := range []string{`"hello"`, `[1,2]`, `42`, `not-json`} {
		_, _, ok := tryUnmarshalWire([]byte(input))
		if ok {
			t.Fatalf("tryUnmarshalWire(%q) expected ok=false", input)
		}
	}
}
