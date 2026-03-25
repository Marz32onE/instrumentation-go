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
	var emb embeddedWire
	if err := json.Unmarshal(raw, &emb); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if emb.Traceparent == "" || emb.Tracestate == "" {
		t.Fatalf("expected trace headers on wire")
	}
	if string(emb.Data) != `"hello"` {
		t.Fatalf("data = %q, want quoted hello", string(emb.Data))
	}
}
