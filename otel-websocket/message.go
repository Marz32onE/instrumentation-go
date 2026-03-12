package otelwebsocket

import "encoding/json"

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

func marshalEnvelope(headers map[string]string, payload []byte) ([]byte, error) {
	return json.Marshal(envelope{Headers: headers, Payload: payload})
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
