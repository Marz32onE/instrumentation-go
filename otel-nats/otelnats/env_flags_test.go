package otelnats

import (
	"os"
	"testing"
)

func TestNATSTracingEnabled_DefaultFalse(t *testing.T) {
	prev, existed := os.LookupEnv(envNATSTracingEnabled)
	_ = os.Unsetenv(envNATSTracingEnabled)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(envNATSTracingEnabled, prev)
		} else {
			_ = os.Unsetenv(envNATSTracingEnabled)
		}
	})
	if natsTracingEnabled() {
		t.Fatal("expected tracing disabled when env var is unset")
	}
}

func TestNATSTracingEnabled_EmptyStringIsEnabled(t *testing.T) {
	t.Setenv(envGlobalTracingEnabled, "")
	t.Setenv(envNATSTracingEnabled, "")
	if !natsTracingEnabled() {
		t.Fatal("expected empty string to mean enabled")
	}
}

func TestNATSTracingEnabled_FalseTokens(t *testing.T) {
	for _, v := range []string{"false", "0", "off", "no"} {
		t.Setenv(envNATSTracingEnabled, v)
		if natsTracingEnabled() {
			t.Fatalf("expected disabled for value %q", v)
		}
	}
}

func TestNATSTracingEnabled_GlobalOffOverridesModule(t *testing.T) {
	t.Setenv(envGlobalTracingEnabled, "false")
	t.Setenv(envNATSTracingEnabled, "true")
	if natsTracingEnabled() {
		t.Fatal("expected global flag to disable nats tracing")
	}
}
