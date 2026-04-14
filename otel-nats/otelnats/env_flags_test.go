package otelnats

import "testing"

func TestNATSTracingEnabled_DefaultTrue(t *testing.T) {
	t.Setenv(envNATSTracingEnabled, "")
	if !natsTracingEnabled() {
		t.Fatal("expected tracing enabled by default")
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
