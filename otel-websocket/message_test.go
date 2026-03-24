package otelwebsocket

import (
	"encoding/json"
	"testing"
)

func TestMarshalEnvelope_OnlyCanonicalTraceHeaders(t *testing.T) {
	in := map[string]string{
		TraceparentHeader: "00-12345678901234567890123456789012-0123456789012345-01",
		TracestateHeader:  "k=v",
		"baggage":         "a=b",
		"custom":          "x",
	}
	raw, err := marshalEnvelope(in, []byte("hello"))
	if err != nil {
		t.Fatalf("marshalEnvelope error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if len(env.Headers) != 2 {
		t.Fatalf("expected exactly 2 headers, got %d", len(env.Headers))
	}
	if env.Headers[TraceparentHeader] == "" {
		t.Fatalf("expected %s in envelope headers", TraceparentHeader)
	}
	if env.Headers[TracestateHeader] == "" {
		t.Fatalf("expected %s in envelope headers", TracestateHeader)
	}
	if _, ok := env.Headers["baggage"]; ok {
		t.Fatal("expected baggage to be excluded from envelope headers")
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
