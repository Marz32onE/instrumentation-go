package otelgorillaws

import (
	"encoding/json"
	"testing"
)

func TestMarshalWire_FlatInjectObject(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
		TracestateHeader:  "k=v",
		"baggage":         "a=b",
	}
	raw, err := marshalWire(carrier, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("marshalWire error: %v", err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// traceparent and tracestate must be injected
	var tp string
	if err := json.Unmarshal(out[TraceparentHeader], &tp); err != nil || tp == "" {
		t.Fatalf("expected traceparent on wire, got %q", tp)
	}
	var ts string
	if err := json.Unmarshal(out[TracestateHeader], &ts); err != nil || ts == "" {
		t.Fatalf("expected tracestate on wire, got %q", ts)
	}

	// baggage must NOT be injected
	if _, ok := out["baggage"]; ok {
		t.Fatalf("unexpected baggage field on wire")
	}

	// original field must be preserved
	var x int
	if err := json.Unmarshal(out["x"], &x); err != nil || x != 1 {
		t.Fatalf("expected x=1, got %v", x)
	}
}

func TestMarshalWire_NonObject_PassThrough(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
	}

	for _, payload := range []string{`"hello"`, `[1,2,3]`, `42`} {
		out, err := marshalWire(carrier, []byte(payload))
		if err != nil {
			t.Fatalf("marshalWire(%s) error: %v", payload, err)
		}
		if string(out) != payload {
			t.Fatalf("marshalWire(%s) = %q, want unchanged", payload, string(out))
		}
	}
}

func TestTryUnmarshalWire_ExtractsTraceFields(t *testing.T) {
	input := `{"x":1,"traceparent":"00-aabb-01","tracestate":"k=v","y":"hello"}`
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

	var out map[string]json.RawMessage
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal payload error: %v", err)
	}
	if _, found := out[TraceparentHeader]; found {
		t.Fatalf("traceparent must be stripped from payload")
	}
	if _, found := out[TracestateHeader]; found {
		t.Fatalf("tracestate must be stripped from payload")
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
