package otelmongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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

func Test_contextFromChangeEvent(t *testing.T) {
	traceparent := "00-12345678901234567890123456789012-0123456789012345-01"

	t.Run("insert_event_with_oteltrace_returns_enriched_ctx", func(t *testing.T) {
		event := bson.D{
			{Key: "operationType", Value: "insert"},
			{Key: "fullDocument", Value: bson.D{
				{Key: "text", Value: "hello"},
				{Key: TraceMetadataKey, Value: bson.D{
					{Key: "traceparent", Value: traceparent},
				}},
			}},
		}
		raw, err := bson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out := contextFromChangeEvent(context.Background(), raw)
		sc := trace.SpanFromContext(out).SpanContext()
		if !sc.IsValid() {
			t.Fatalf("expected valid span context")
		}
		if got, want := sc.TraceID().String(), "12345678901234567890123456789012"; got != want {
			t.Fatalf("trace id mismatch: got %s want %s", got, want)
		}
	})

	t.Run("insert_event_without_oteltrace_returns_ctx_unchanged", func(t *testing.T) {
		event := bson.D{
			{Key: "operationType", Value: "insert"},
			{Key: "fullDocument", Value: bson.D{
				{Key: "text", Value: "hello"},
			}},
		}
		raw, err := bson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out := contextFromChangeEvent(context.Background(), raw)
		sc := trace.SpanFromContext(out).SpanContext()
		if sc.IsValid() {
			t.Fatalf("expected invalid span context")
		}
	})

	t.Run("delete_event_no_fullDocument_returns_ctx_unchanged", func(t *testing.T) {
		event := bson.D{
			{Key: "operationType", Value: "delete"},
			{Key: "documentKey", Value: bson.D{
				{Key: "_id", Value: "abc"},
			}},
		}
		raw, err := bson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out := contextFromChangeEvent(context.Background(), raw)
		sc := trace.SpanFromContext(out).SpanContext()
		if sc.IsValid() {
			t.Fatalf("expected invalid span context")
		}
	})

	t.Run("fullDocument_not_a_document_returns_ctx_unchanged", func(t *testing.T) {
		event := bson.D{
			{Key: "operationType", Value: "insert"},
			{Key: "fullDocument", Value: "not-a-document"},
		}
		raw, err := bson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out := contextFromChangeEvent(context.Background(), raw)
		sc := trace.SpanFromContext(out).SpanContext()
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
