package otelmongo

import (
	"os"
	"testing"
)

func TestMongoTracingEnabled_DefaultTrue(t *testing.T) {
	prev, existed := os.LookupEnv(envMongoTracingEnabled)
	_ = os.Unsetenv(envMongoTracingEnabled)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(envMongoTracingEnabled, prev)
		} else {
			_ = os.Unsetenv(envMongoTracingEnabled)
		}
	})
	if !mongoTracingEnabled() {
		t.Fatal("expected tracing enabled when env var is unset")
	}
}

func TestMongoTracingEnabled_EmptyStringIsEnabled(t *testing.T) {
	t.Setenv(envMongoTracingEnabled, "")
	if !mongoTracingEnabled() {
		t.Fatal("expected empty string to mean enabled")
	}
}

func TestMongoTracingEnabled_FalseTokens(t *testing.T) {
	for _, v := range []string{"false", "0", "off", "no"} {
		t.Setenv(envMongoTracingEnabled, v)
		if mongoTracingEnabled() {
			t.Fatalf("expected disabled for value %q", v)
		}
	}
}

func TestMongoTracingEnabled_GlobalOffOverridesModule(t *testing.T) {
	t.Setenv(envGlobalTracingEnabled, "false")
	t.Setenv(envMongoTracingEnabled, "true")
	if mongoTracingEnabled() {
		t.Fatal("expected global flag to disable mongo tracing")
	}
}
