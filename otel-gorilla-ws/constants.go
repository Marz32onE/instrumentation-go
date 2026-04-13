package otelgorillaws

import "github.com/gorilla/websocket"

// Re-export gorilla/websocket message type constants so users can import only this package.
const (
	TextMessage   = websocket.TextMessage
	BinaryMessage = websocket.BinaryMessage
	CloseMessage  = websocket.CloseMessage
	PingMessage   = websocket.PingMessage
	PongMessage   = websocket.PongMessage
)
