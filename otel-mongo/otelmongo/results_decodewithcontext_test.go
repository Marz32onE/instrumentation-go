package otelmongo

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestBuildConsumerCtx_NewTraceIDAndLinksOriginTrace(t *testing.T) {
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
	newCtx, span := buildConsumerCtx(originCtx, nil, "", nil, "test-decode-span", nil, originSpanCtx)
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

func TestBuildConsumerCtx_WithDeliverTracer_ChildOfDeliver(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)

	deliverSR := tracetest.NewSpanRecorder()
	deliverTP := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(deliverSR),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	t.Cleanup(func() { _ = deliverTP.Shutdown(context.Background()) })
	deliverTracer := deliverTP.Tracer("deliver-test")

	tracer := tp.Tracer("test")
	_, originSpan := tracer.Start(context.Background(), "origin")
	originSpanCtx := originSpan.SpanContext()
	originSpan.End()

	newCtx, span := buildConsumerCtx(context.Background(), deliverTracer, "test deliver", nil, "test-consumer-span", nil, originSpanCtx)
	span.End()

	consumerSpanCtx := trace.SpanContextFromContext(newCtx)
	if !consumerSpanCtx.IsValid() {
		t.Fatalf("expected returned context to contain a valid span context")
	}
	if consumerSpanCtx.TraceID() == originSpanCtx.TraceID() {
		t.Fatalf("expected new TraceID different from origin")
	}

	// Deliver span should exist in deliverSR with a link to origin.
	var deliverFound bool
	for _, s := range deliverSR.Ended() {
		if s.Name() != "test deliver" {
			continue
		}
		deliverFound = true
		if len(s.Links()) == 0 {
			t.Fatalf("expected deliver span to have a link to origin")
		}
		if s.Links()[0].SpanContext.TraceID() != originSpanCtx.TraceID() {
			t.Fatalf("deliver span link should point to origin TraceID")
		}
		// Consumer span should share deliver span's TraceID.
		if consumerSpanCtx.TraceID() != s.SpanContext().TraceID() {
			t.Fatalf("consumer span TraceID=%s should match deliver span TraceID=%s",
				consumerSpanCtx.TraceID(), s.SpanContext().TraceID())
		}
		break
	}
	if !deliverFound {
		t.Fatalf("expected a deliver span named %q", "test deliver")
	}
}
