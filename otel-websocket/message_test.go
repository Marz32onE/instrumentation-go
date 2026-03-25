package otelwebsocket

import (
	"encoding/json"
	"testing"
)

func TestMarshalWire_OnlyCanonicalTraceHeaders(t *testing.T) {
	in := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
		TracestateHeader:  "k=v",
		"baggage":         "a=b",
		"custom":          "x",
	}
	raw, err := marshalWire(in, []byte(`"hello"`))
	if err != nil {
		t.Fatalf("marshalWire error: %v", err)
	}
	var emb embeddedWire
	if err := json.Unmarshal(raw, &emb); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if emb.Traceparent == "" {
		t.Fatalf("expected traceparent on wire")
	}
	if emb.Tracestate == "" {
		t.Fatalf("expected tracestate on wire")
	}
	if string(emb.Data) != `"hello"` {
		t.Fatalf("data = %q, want quoted hello", string(emb.Data))
	}
}

func TestTryUnmarshalWire_embeddedRoundTrip(t *testing.T) {
	carrier := map[string]string{
		TraceparentHeader: "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
	}
	inner := []byte(`{"body":"hi","api":"X"}`)
	wire, err := marshalWire(carrier, inner)
	if err != nil {
		t.Fatal(err)
	}
	payload, hdrs, ok := tryUnmarshalWire(wire)
	if !ok {
		t.Fatal("expected ok")
	}
	if string(payload) != string(inner) {
		t.Fatalf("payload %q want %q", payload, inner)
	}
	if hdrs[TraceparentHeader] != carrier[TraceparentHeader] {
		t.Fatalf("traceparent header mismatch")
	}
}

func TestTryUnmarshalWire_envelope(t *testing.T) {
	envBytes, err := json.Marshal(envelope{
		Headers: map[string]string{
			TraceparentHeader: "00-11111111111111111111111111111111-2222222222222222-01",
		},
		Payload: []byte(`inner`),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, hdrs, ok := tryUnmarshalWire(envBytes)
	if !ok {
		t.Fatal("expected ok for envelope")
	}
	if string(payload) != "inner" {
		t.Fatalf("payload = %q", payload)
	}
	if hdrs[TraceparentHeader] == "" {
		t.Fatal("missing traceparent")
	}
}

func TestCanonicalTraceHeaders_EmptySafe(t *testing.T) {
	got := canonicalTraceHeaders(nil)
	if got == nil {
		t.Fatal("expected non-nil map")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}
