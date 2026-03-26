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

// envelope is the header-style wire format: trace context in headers + base64 payload.
type envelope struct {
	Headers map[string]string `json:"headers"`
	Payload []byte            `json:"payload"`
}

// embeddedWire matches JS @marz32one/otel-rxjs-ws (rxjs webSocket) output.
type embeddedWire struct {
	Traceparent string          `json:"traceparent,omitempty"`
	Tracestate  string          `json:"tracestate,omitempty"`
	Data        json.RawMessage `json:"data"`
}

func canonicalTraceHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, 2)
	if headers == nil {
		return out
	}
	if tp := headers[TraceparentHeader]; tp != "" {
		out[TraceparentHeader] = tp
	}
	if ts := headers[TracestateHeader]; ts != "" {
		out[TracestateHeader] = ts
	}
	return out
}

// marshalWire encodes the application payload as header-style envelope
// ({ "headers", "payload" }) with base64 payload bytes.
func marshalWire(carrier map[string]string, payload []byte) ([]byte, error) {
	env := envelope{
		Headers: canonicalTraceHeaders(carrier),
		Payload: payload,
	}
	return json.Marshal(env)
}

func unmarshalEnvelope(data []byte) (envelope, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return envelope{}, err
	}
	if env.Headers == nil {
		env.Headers = make(map[string]string)
	}
	return env, nil
}

// tryUnmarshalWire returns the application payload and trace headers when data
// is either embedded ({ traceparent?, tracestate?, data }) or header-style
// ({ headers, payload }). Otherwise ok is false.
func tryUnmarshalWire(data []byte) (payload []byte, headers map[string]string, ok bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil || len(m) == 0 {
		return nil, nil, false
	}
	_, hasHeaders := m["headers"]
	_, hasPayload := m["payload"]
	_, hasData := m["data"]

	if hasHeaders && hasPayload {
		env, err := unmarshalEnvelope(data)
		if err != nil {
			return nil, nil, false
		}
		return env.Payload, canonicalTraceHeaders(env.Headers), true
	}
	if hasData {
		var emb embeddedWire
		if err := json.Unmarshal(data, &emb); err != nil {
			return nil, nil, false
		}
		h := make(map[string]string)
		if emb.Traceparent != "" {
			h[TraceparentHeader] = emb.Traceparent
		}
		if emb.Tracestate != "" {
			h[TracestateHeader] = emb.Tracestate
		}
		return dataBytesFromRawJSON(emb.Data), canonicalTraceHeaders(h), true
	}
	return nil, nil, false
}

func dataBytesFromRawJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []byte(s)
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}
