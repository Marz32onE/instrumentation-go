package otelwebsocket

import "encoding/json"

const (
	// TraceparentHeader is the canonical W3C trace context header key.
	TraceparentHeader = "traceparent"
	// TracestateHeader is the canonical W3C trace context header key.
	TracestateHeader = "tracestate"
)

// envelope is the on-wire JSON wrapper that carries both the application
// payload and the W3C Trace Context headers so that receivers can
// reconstruct the distributed-trace span from the caller.
type envelope struct {
	// Headers holds the propagated trace-context key/value pairs
	// (e.g. "traceparent", "tracestate", and any baggage entries).
	Headers map[string]string `json:"headers"`

	// Payload is the original application message body.
	Payload []byte `json:"payload"`
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

func marshalEnvelope(headers map[string]string, payload []byte) ([]byte, error) {
	return json.Marshal(envelope{Headers: canonicalTraceHeaders(headers), Payload: payload})
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
