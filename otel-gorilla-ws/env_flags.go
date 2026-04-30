package otelgorillaws

import (
	"os"
	"strings"
)

const (
	envGlobalTracingEnabled = "OTEL_INSTRUMENTATION_GO_TRACING_ENABLED"
	envWSTracingEnabled     = "OTEL_GORILLA_WS_TRACING_ENABLED"
)

func wsTracingEnabled() bool {
	if !envEnabledByDefault(envGlobalTracingEnabled) {
		return false
	}
	return envEnabledByDefault(envWSTracingEnabled)
}

func envEnabledByDefault(key string) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
