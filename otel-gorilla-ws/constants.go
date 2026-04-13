package otelgorillaws

import "github.com/gorilla/websocket"

// CloseError re-exports gorilla/websocket close error type.
type CloseError = websocket.CloseError

// Re-export gorilla/websocket message type constants so users can import only this package.
const (
	TextMessage   = websocket.TextMessage
	BinaryMessage = websocket.BinaryMessage
	CloseMessage  = websocket.CloseMessage
	PingMessage   = websocket.PingMessage
	PongMessage   = websocket.PongMessage
)

// Re-export gorilla/websocket close code constants for close/error handling.
const (
	CloseNormalClosure           = websocket.CloseNormalClosure
	CloseGoingAway               = websocket.CloseGoingAway
	CloseProtocolError           = websocket.CloseProtocolError
	CloseUnsupportedData         = websocket.CloseUnsupportedData
	CloseNoStatusReceived        = websocket.CloseNoStatusReceived
	CloseAbnormalClosure         = websocket.CloseAbnormalClosure
	CloseInvalidFramePayloadData = websocket.CloseInvalidFramePayloadData
	ClosePolicyViolation         = websocket.ClosePolicyViolation
	CloseMessageTooBig           = websocket.CloseMessageTooBig
	CloseMandatoryExtension      = websocket.CloseMandatoryExtension
	CloseInternalServerErr       = websocket.CloseInternalServerErr
	CloseServiceRestart          = websocket.CloseServiceRestart
	CloseTryAgainLater           = websocket.CloseTryAgainLater
	CloseTLSHandshake            = websocket.CloseTLSHandshake
)
