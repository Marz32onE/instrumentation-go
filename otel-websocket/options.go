package otelwebsocket

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Option configures a Conn.
type Option func(*Conn)

// WithPropagators sets the TextMapPropagator used to inject and extract
// trace context. Defaults to otel.GetTextMapPropagator(). Aligns with OTel contrib naming (WithPropagators).
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(c *Conn) {
		if p != nil {
			c.propagator = p
		}
	}
}

// WithTracerProvider sets the TracerProvider used to create spans.
// Defaults to otel.GetTracerProvider(). Per OTel contrib: accept TracerProvider, not Tracer.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *Conn) {
		if tp != nil {
			c.tracer = tp.Tracer(ScopeName, trace.WithInstrumentationVersion(SemVersion()))
		}
	}
}

func applyOptions(c *Conn, opts []Option) {
	// Set defaults (OTel contrib: use global provider/propagator when not supplied).
	c.propagator = otel.GetTextMapPropagator()
	c.tracer = otel.GetTracerProvider().Tracer(ScopeName, trace.WithInstrumentationVersion(SemVersion()))

	for _, o := range opts {
		o(c)
	}
}
