package otelgorillaws

import (
	"os"
	"testing"
)

func TestWSTracingEnabled_DefaultFalse(t *testing.T) {
	prev, existed := os.LookupEnv(envWSTracingEnabled)
	_ = os.Unsetenv(envWSTracingEnabled)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(envWSTracingEnabled, prev)
		} else {
			_ = os.Unsetenv(envWSTracingEnabled)
		}
	})
	if wsTracingEnabled() {
		t.Fatal("expected tracing disabled when env var is unset")
	}
}

func TestWSTracingEnabled_EmptyStringIsEnabled(t *testing.T) {
	t.Setenv(envGlobalTracingEnabled, "")
	t.Setenv(envWSTracingEnabled, "")
	if !wsTracingEnabled() {
		t.Fatal("expected empty string to mean enabled")
	}
}

func TestWSTracingEnabled_FalseTokens(t *testing.T) {
	for _, v := range []string{"false", "0", "off", "no"} {
		t.Setenv(envWSTracingEnabled, v)
		if wsTracingEnabled() {
			t.Fatalf("expected disabled for value %q", v)
		}
	}
}

func TestWSTracingEnabled_GlobalOffOverridesModule(t *testing.T) {
	t.Setenv(envGlobalTracingEnabled, "false")
	t.Setenv(envWSTracingEnabled, "true")
	if wsTracingEnabled() {
		t.Fatal("expected global flag to disable ws tracing")
	}
}
