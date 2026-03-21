package otelmongo

import (
	"testing"
)

func TestMongoDeliverSpanDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	tp, tracer := initMongoProvider("localhost", 27017)
	if tp != nil {
		t.Error("expected nil TracerProvider when OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}
	if tracer != nil {
		t.Error("expected nil Tracer when OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}
}

func TestMongoDeliverSpanEnabledWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	tp, tracer := initMongoProvider("localhost", 27017)
	if tp == nil {
		t.Error("expected non-nil TracerProvider when OTEL_EXPORTER_OTLP_ENDPOINT is set")
	}
	if tracer == nil {
		t.Error("expected non-nil Tracer when OTEL_EXPORTER_OTLP_ENDPOINT is set")
	}
	if tp != nil {
		tp.Shutdown(t.Context()) //nolint:errcheck
	}
}

func TestMongoServiceName(t *testing.T) {
	cases := []struct {
		addr string
		port int
		want string
	}{
		{"", 0, "mongodb"},
		{"localhost", 27017, "mongodb://localhost"},
		{"localhost", 27018, "mongodb://localhost:27018"},
		{"myhost", 0, "mongodb://myhost"},
	}
	for _, tc := range cases {
		got := mongoServiceName(tc.addr, tc.port)
		if got != tc.want {
			t.Errorf("mongoServiceName(%q, %d) = %q, want %q", tc.addr, tc.port, got, tc.want)
		}
	}
}
