package otelwebsocket

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Option configures a Conn.
type Option func(*Conn)

// WithPropagators sets the TextMapPropagator; it is applied to otel.SetTextMapPropagator so globals are used for inject/extract.
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(c *Conn) {
		if p != nil {
			otel.SetTextMapPropagator(p)
		}
	}
}

// WithTracerProvider sets the TracerProvider; it is applied to otel.SetTracerProvider so globals are used for tracing.
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
	c.tracer = otel.GetTracerProvider().Tracer(ScopeName, trace.WithInstrumentationVersion(SemVersion()))
}
