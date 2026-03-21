package otelmongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
}

func TestContextFromDocumentV1(t *testing.T) {
	t.Run("full_document_with_trace_metadata_returns_valid_span_context", func(t *testing.T) {
		fullDoc := bson.M{
			"_oteltrace": bson.M{
				"traceparent": "00-12345678901234567890123456789012-0123456789012345-01",
			},
		}
		sc, ok := ContextFromDocument(context.Background(), fullDoc)
		if !ok {
			t.Fatalf("expected ok=true")
		}
		if !sc.IsValid() {
			t.Fatalf("expected valid span context")
		}
		if got, want := sc.TraceID().String(), "12345678901234567890123456789012"; got != want {
			t.Fatalf("trace id mismatch: got %s want %s", got, want)
		}
	})

	t.Run("missing_metadata_returns_false", func(t *testing.T) {
		sc, ok := ContextFromDocument(context.Background(), bson.M{"x": 1})
		if ok {
			t.Fatalf("expected ok=false")
		}
		if sc.IsValid() {
			t.Fatalf("expected invalid span context")
		}
	})

	t.Run("non_marshalable_document_returns_false", func(t *testing.T) {
		ch := make(chan int)
		sc, ok := ContextFromDocument(context.Background(), ch)
		if ok {
			t.Fatalf("expected ok=false")
		}
		if sc.IsValid() {
			t.Fatalf("expected invalid span context")
		}
	})

	t.Run("empty_traceparent_returns_false", func(t *testing.T) {
		fullDoc := bson.M{
			"_oteltrace": bson.M{
				"traceparent": "",
			},
		}
		sc, ok := ContextFromDocument(context.Background(), fullDoc)
		if ok {
			t.Fatalf("expected ok=false")
		}
		if sc.IsValid() {
			t.Fatalf("expected invalid span context")
		}
	})
}

func TestContextFromRawDocumentV1(t *testing.T) {
	traceparent := "00-12345678901234567890123456789012-0123456789012345-01"
	doc := bson.D{
		{Key: TraceMetadataKey, Value: bson.D{
			{Key: "traceparent", Value: traceparent},
		}},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := ContextFromRawDocument(context.Background(), raw)
	sc := trace.SpanFromContext(out).SpanContext()
	if !sc.IsValid() {
		t.Fatalf("expected valid span context")
	}
	if got, want := sc.TraceID().String(), "12345678901234567890123456789012"; got != want {
		t.Fatalf("trace id mismatch: got %s want %s", got, want)
	}
}

func TestStartDeliverSpanDisabled(t *testing.T) {
	coll := &Collection{deliverTracer: nil}
	ctx := context.Background()
	got, span := coll.startDeliverSpan(ctx, "testdb", "testcoll")
	defer span.End()
	if got != ctx {
		t.Error("expected unchanged ctx when deliverTracer is nil")
	}
	if trace.SpanFromContext(got).SpanContext().IsValid() {
		t.Error("expected no span in ctx when deliverTracer is nil")
	}
}

func TestStartDeliverSpanEnabled(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer(ScopeName)
	coll := &Collection{
		deliverTracer: tracer,
		serverAddr:    "localhost",
		serverPort:    27017,
	}

	// Establish a parent span so we can verify the deliver span is a child.
	parentCtx, parentSpan := tp.Tracer(ScopeName).Start(context.Background(), "parent")
	defer parentSpan.End()

	got, deliverSpan := coll.startDeliverSpan(parentCtx, "testdb", "testcoll")
	defer deliverSpan.End()

	deliverSC := trace.SpanFromContext(got).SpanContext()
	if !deliverSC.IsValid() {
		t.Fatal("expected valid span context in returned ctx")
	}
	parentSC := parentSpan.SpanContext()
	if deliverSC.SpanID() == parentSC.SpanID() {
		t.Error("deliver span ID should differ from parent span ID")
	}
	if deliverSC.TraceID() != parentSC.TraceID() {
		t.Error("deliver span should share trace ID with parent")
	}
	// span should still be recording — caller has not yet called End()
	if ro, ok := deliverSpan.(interface{ IsRecording() bool }); ok {
		if !ro.IsRecording() {
			t.Error("deliver span should still be recording before End() is called")
		}
	}
}
