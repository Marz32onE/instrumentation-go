package otelgorillaws

import (
	"encoding/json"
)

const (
	// TraceparentHeader is the canonical W3C trace context header key.
	TraceparentHeader = "traceparent"
	// TracestateHeader is the canonical W3C trace context header key.
	TracestateHeader = "tracestate"
)

// marshalWire injects traceparent/tracestate into the JSON object payload (flat format).
// Non-object payloads (strings, arrays, etc.) are returned unchanged with no trace injection.
func marshalWire(carrier map[string]string, payload []byte) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		// Not a JSON object: send as-is, no trace injection.
		return payload, nil
	}
	if tp := carrier[TraceparentHeader]; tp != "" {
		b, _ := json.Marshal(tp)
		obj[TraceparentHeader] = b
	}
	if ts := carrier[TracestateHeader]; ts != "" {
		b, _ := json.Marshal(ts)
		obj[TracestateHeader] = b
	}
	return json.Marshal(obj)
}

// tryUnmarshalWire extracts traceparent/tracestate from the top-level JSON object,
// removes them, and returns the remaining fields as payload.
// Returns ok=false for non-object JSON (strings, arrays, parse errors).
func tryUnmarshalWire(data []byte) (payload []byte, headers map[string]string, ok bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil || len(m) == 0 {
		return nil, nil, false
	}

	h := make(map[string]string)
	if raw, exists := m[TraceparentHeader]; exists {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			h[TraceparentHeader] = s
		}
		delete(m, TraceparentHeader)
	}
	if raw, exists := m[TracestateHeader]; exists {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			h[TracestateHeader] = s
		}
		delete(m, TracestateHeader)
	}

	out, err := json.Marshal(m)
	if err != nil {
		return nil, nil, false
	}
	return out, h, true
}
