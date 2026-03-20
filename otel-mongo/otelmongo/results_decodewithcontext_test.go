package otelmongo

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartDecodeSpan_NewTraceIDAndLinksOriginTrace(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)

	tracer := tp.Tracer("test")
	originCtx, originSpan := tracer.Start(context.Background(), "origin")
	originSpanCtx := originSpan.SpanContext()
	originSpan.End()

	// Pass a context that still carries origin span context, and ensure the helper detaches it.
	newCtx, span := startDecodeSpan(originCtx, "test-decode-span", nil, originSpanCtx)
	span.End()

	recovered := trace.SpanContextFromContext(newCtx)
	if !recovered.IsValid() {
		t.Fatalf("expected returned context to contain a valid span context")
	}
	if recovered.TraceID() == originSpanCtx.TraceID() {
		t.Fatalf("expected new TraceID different from origin; got=%s origin=%s", recovered.TraceID(), originSpanCtx.TraceID())
	}

	// Validate that the span has a link-only association to the origin TraceID.
	var found bool
	for _, s := range sr.Ended() {
		if s.Name() != "test-decode-span" {
			continue
		}
		found = true
		links := s.Links()
		if len(links) == 0 {
			t.Fatalf("expected decode span to have at least 1 link")
		}
		if links[0].SpanContext.TraceID() != originSpanCtx.TraceID() {
			t.Fatalf("expected decode link TraceID=%s, got=%s", originSpanCtx.TraceID(), links[0].SpanContext.TraceID())
		}
		break
	}
	if !found {
		t.Fatalf("expected a span named %q", "test-decode-span")
	}
}

