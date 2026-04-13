package otelgorillaws

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// Upgrader wraps websocket.Upgrader and adds otel-ws subprotocol negotiation.
// Use Upgrade instead of websocket.Upgrader.Upgrade to get spec-compliant
// trace propagation negotiation on the server side.
//
// Server-side spec behaviour:
//   - Scenario F: client sends no subprotocols → accept, tracing disabled (passthrough)
//   - Scenario G: client sends "otel-ws,json" → respond "otel-ws+json", tracing enabled
//   - Scenario H: client sends "json" (no otel-ws) → accept normally, tracing disabled
type Upgrader struct {
	// Inner is the underlying gorilla Upgrader. Set CheckOrigin, ReadBufferSize,
	// WriteBufferSize, Error, etc. on Inner as needed.
	// Do NOT set Inner.Subprotocols — use AppSubprotocols instead.
	Inner websocket.Upgrader

	// AppSubprotocols lists the application-level protocols this server supports
	// (e.g. ["json", "binary"]). The first match from the client's proposed list
	// is chosen. If nil, the first application protocol the client proposes is
	// accepted (accept-any semantics).
	AppSubprotocols []string
}

// Upgrade upgrades the HTTP connection to WebSocket with otel-ws negotiation.
//
// When the client includes "otel-ws" in its Sec-WebSocket-Protocol header,
// the server responds with "otel-ws+<negotiated>" and the returned Conn has
// tracing enabled (Scenario G). Otherwise the connection is accepted with
// normal protocol selection and the returned Conn operates in passthrough
// mode (Scenarios F and H).
//
// gorilla constraint: when Inner.Subprotocols is nil, gorilla's selectSubprotocol
// reads the subprotocol from responseHeader. When Inner.Subprotocols is non-nil,
// gorilla ignores responseHeader for protocol selection. This method sets
// Inner.Subprotocols=nil and injects the otel-ws+<proto> value via responseHeader
// for the otel-ws path; for the non-otel path it restores Inner.Subprotocols so
// gorilla performs normal matching.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
	clientProtos := websocket.Subprotocols(r)
	otelRequested := containsProto(clientProtos, otelWSProtocol)

	// Work on a copy of Inner so we never mutate the caller's upgrader.
	inner := u.Inner

	if otelRequested {
		// Strip "otel-ws" from the client list, find the first matching app protocol.
		appProtos := filterProto(clientProtos, otelWSProtocol)
		negotiated := selectFirst(appProtos, u.AppSubprotocols)

		// Respond with "otel-ws+<negotiated>" so the client can detect otel-ws support.
		// inner.Subprotocols must be nil so gorilla reads from responseHeader.
		inner.Subprotocols = nil
		responseHeader = cloneHeader(responseHeader)
		responseHeader.Set("Sec-Websocket-Protocol", otelWSProtocol+"+"+negotiated)
	} else {
		// Scenarios F and H: normal gorilla protocol selection from AppSubprotocols.
		inner.Subprotocols = u.AppSubprotocols
	}

	raw, err := inner.Upgrade(w, r, responseHeader)
	if err != nil {
		return nil, err
	}

	return newConn(raw, otelRequested), nil
}

// cloneHeader returns a shallow clone of h (or a new empty header if h is nil).
func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}

// containsProto reports whether proto appears in list.
func containsProto(list []string, proto string) bool {
	for _, p := range list {
		if p == proto {
			return true
		}
	}
	return false
}

// filterProto returns a new slice with all occurrences of proto removed.
func filterProto(list []string, proto string) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		if p != proto {
			out = append(out, p)
		}
	}
	return out
}

// selectFirst returns the first element of clientProtos that also appears in
// serverProtos. If serverProtos is nil, the first element of clientProtos is
// returned (accept-any semantics). Returns "" if no match is found.
func selectFirst(clientProtos, serverProtos []string) string {
	if len(clientProtos) == 0 {
		return ""
	}
	if serverProtos == nil {
		return clientProtos[0]
	}
	for _, cp := range clientProtos {
		for _, sp := range serverProtos {
			if cp == sp {
				return cp
			}
		}
	}
	return ""
}
