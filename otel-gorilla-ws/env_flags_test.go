package otelgorillaws

import "testing"

func TestWSTracingEnabled_DefaultTrue(t *testing.T) {
	t.Setenv(envWSTracingEnabled, "")
	if !wsTracingEnabled() {
		t.Fatal("expected tracing enabled by default")
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
