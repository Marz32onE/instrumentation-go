package otelmongo

import (
	"os"
	"strings"
)

const (
	envGlobalTracingEnabled = "OTEL_INSTRUMENTATION_GO_TRACING_ENABLED"
	envMongoTracingEnabled  = "OTEL_MONGO_TRACING_ENABLED"
)

func mongoTracingEnabled() bool {
	if !envEnabledByDefault(envGlobalTracingEnabled) {
		return false
	}
	return envEnabledByDefault(envMongoTracingEnabled)
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
