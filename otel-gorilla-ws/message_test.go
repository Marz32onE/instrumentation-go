package otelgorillaws

import (
	"encoding/json"
	"testing"
)

func TestMarshalWire_OnlyCanonicalTraceHeaders(t *testing.T) {
	in := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
		TracestateHeader:  "k=v",
		"baggage":         "a=b",
	}
	raw, err := marshalWire(in, []byte(`"hello"`))
	if err != nil {
		t.Fatalf("marshalWire error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if env.Headers[TraceparentHeader] == "" || env.Headers[TracestateHeader] == "" {
		t.Fatalf("expected canonical trace headers on wire")
	}
	if _, ok := env.Headers["baggage"]; ok {
		t.Fatalf("unexpected non-canonical header baggage on wire")
	}
	if string(env.Payload) != `"hello"` {
		t.Fatalf("payload = %q, want quoted hello", string(env.Payload))
	}
}
