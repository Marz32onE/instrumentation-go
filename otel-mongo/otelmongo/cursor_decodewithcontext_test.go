package otelmongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestCursorDecodeWithContext_NewTraceIDAndLinksOriginTrace(t *testing.T) {
	// Ensure the document trace metadata encoding/decoding is deterministic in tests.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

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

	// Build a cursor document that contains the origin trace metadata.
	injected, err := injectTraceIntoDocument(originCtx, bson.D{{Key: "field", Value: "v"}})
	if err != nil {
		t.Fatalf("injectTraceIntoDocument: %v", err)
	}
	rawDoc, err := bson.Marshal(injected)
	if err != nil {
		t.Fatalf("bson.Marshal injected doc: %v", err)
	}

	cur, err := mongo.NewCursorFromDocuments([]interface{}{rawDoc}, nil, nil)
	if err != nil {
		t.Fatalf("NewCursorFromDocuments: %v", err)
	}
	defer func() { _ = cur.Close(context.Background()) }()
	if !cur.Next(context.Background()) {
		t.Fatalf("expected cursor.Next=true")
	}

	wrapped := &Cursor{Cursor: cur, parentCtx: context.Background()}

	var out bson.D
	enrichedCtx, err := wrapped.DecodeWithContext(context.Background(), &out)
	if err != nil {
		t.Fatalf("DecodeWithContext: %v", err)
	}

	recovered := trace.SpanContextFromContext(enrichedCtx)
	if !recovered.IsValid() {
		t.Fatalf("expected returned context to contain a valid span context")
	}
	if recovered.TraceID() == originSpanCtx.TraceID() {
		t.Fatalf("expected new TraceID different from origin; got=%s origin=%s", recovered.TraceID(), originSpanCtx.TraceID())
	}

	// Validate that the internal decode span has a link to the origin TraceID.
	var found bool
	for _, s := range sr.Ended() {
		if s.Name() != "mongo.cursor.decode" {
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
		t.Fatalf("expected a span named %q", "mongo.cursor.decode")
	}
}
