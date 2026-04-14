package otelmongo

import "testing"

func TestMongoTracingEnabled_DefaultTrue(t *testing.T) {
	t.Setenv(envMongoTracingEnabled, "")
	if !mongoTracingEnabled() {
		t.Fatal("expected tracing enabled by default")
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
