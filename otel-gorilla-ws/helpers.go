package otelgorillaws

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// IsCloseError reports whether err is a *CloseError with one of the specified codes.
func IsCloseError(err error, codes ...int) bool {
	return websocket.IsCloseError(err, codes...)
}

// IsUnexpectedCloseError reports whether err is a *CloseError with a code not in expectedCodes.
func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	return websocket.IsUnexpectedCloseError(err, expectedCodes...)
}

// FormatCloseMessage formats closeCode and text as a WebSocket close message payload.
func FormatCloseMessage(closeCode int, text string) []byte {
	return websocket.FormatCloseMessage(closeCode, text)
}

// IsWebSocketUpgrade reports whether the request is an HTTP upgrade to WebSocket.
func IsWebSocketUpgrade(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}

// Subprotocols returns the requested WebSocket subprotocols from the request.
func Subprotocols(r *http.Request) []string {
	return websocket.Subprotocols(r)
}
