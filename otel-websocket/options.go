package otelwebsocket

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Option configures a Conn.
type Option func(*Conn)

// WithPropagators sets the global TextMapPropagator so all Conn instances in this process use it.
// Call otel.SetTextMapPropagator at process startup as an alternative to passing this option.
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(c *Conn) {
		if p != nil {
			otel.SetTextMapPropagator(p)
		}
	}
}

// WithTracerProvider sets the global TracerProvider so all Conn instances in this process use it.
// Call otel.SetTracerProvider at process startup as an alternative to passing this option.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *Conn) {
		if tp != nil {
			otel.SetTracerProvider(tp)
		}
	}
}

func applyOptions(c *Conn, opts []Option) {
	for _, o := range opts {
		o(c)
	}
	// Always read tracer and propagator from otel globals (set by opts above or by process startup).
	c.propagator = otel.GetTextMapPropagator()
	c.tracer = otel.GetTracerProvider().Tracer(ScopeName, trace.WithInstrumentationVersion(Version()))
}
