package otelmongo

import (
	"os"
	"strings"
)

const (
	envGlobalTracingEnabled    = "OTEL_INSTRUMENTATION_GO_TRACING_ENABLED"
	envMongoTracingEnabled     = "OTEL_MONGO_TRACING_ENABLED"
	envMongoPropagationEnabled = "OTEL_MONGO_PROPAGATION_ENABLED"
)

func mongoTracingEnabled() bool {
	if !envEnabledByDefault(envGlobalTracingEnabled) {
		return false
	}
	return envEnabledByDefault(envMongoTracingEnabled)
}

func mongoPropagationEnabled() bool {
	if !envEnabledByDefault(envGlobalTracingEnabled) {
		return false
	}
	return envEnabledByDefault(envMongoPropagationEnabled)
}

func resolveFlag(override *bool, envDefault bool) bool {
	if override != nil {
		return *override
	}
	return envDefault
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
