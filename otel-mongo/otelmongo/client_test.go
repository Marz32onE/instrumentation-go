package otelmongo

import (
	"go.mongodb.org/mongo-driver/mongo/options"
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

func TestParseServerFromClientOptions(t *testing.T) {
	t.Run("nil options", func(t *testing.T) {
		addr, port := parseServerFromClientOptions(nil)
		if addr != "" || port != 0 {
			t.Fatalf("expected empty addr/port, got %q:%d", addr, port)
		}
	})

	t.Run("without apply uri", func(t *testing.T) {
		addr, port := parseServerFromClientOptions(options.Client())
		if addr != "" || port != 0 {
			t.Fatalf("expected empty addr/port, got %q:%d", addr, port)
		}
	})

	t.Run("with apply uri", func(t *testing.T) {
		addr, port := parseServerFromClientOptions(options.Client().ApplyURI("mongodb://mongo:27018"))
		if addr != "mongo" || port != 27018 {
			t.Fatalf("expected mongo:27018, got %q:%d", addr, port)
		}
	})
}
