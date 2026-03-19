package otelmongo

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type recordingExporter struct {
	last []sdktrace.ReadOnlySpan
}

func (e *recordingExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.last = spans
	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error { return nil }

func TestSkipDBOperationsExporter_DropsGetMore(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")

	_, s1 := tracer.Start(context.Background(), "span-getmore")
	s1.SetAttributes(attribute.String(dbOperationNameKey, "getMore"))
	s1.End()

	_, s2 := tracer.Start(context.Background(), "span-find")
	s2.SetAttributes(attribute.String(dbOperationNameKey, "find"))
	s2.End()

	ended := sr.Ended()
	if len(ended) != 2 {
		t.Fatalf("expected 2 ended spans, got %d", len(ended))
	}

	rec := &recordingExporter{}
	exp := SkipDBOperationsExporter(rec, []string{"getMore"})
	if err := exp.ExportSpans(context.Background(), ended); err != nil {
		t.Fatalf("export spans: %v", err)
	}
	if len(rec.last) != 1 {
		t.Fatalf("expected 1 span after filter, got %d", len(rec.last))
	}
	if rec.last[0].Name() != "span-find" {
		t.Fatalf("expected span-find, got %q", rec.last[0].Name())
	}
}

func TestSkipDBOperationsExporter_EmptySkipPassThrough(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")

	_, s := tracer.Start(context.Background(), "span-any")
	s.SetAttributes(attribute.String(dbOperationNameKey, "getMore"))
	s.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}

	rec := &recordingExporter{}
	exp := SkipDBOperationsExporter(rec, nil)
	if err := exp.ExportSpans(context.Background(), ended); err != nil {
		t.Fatalf("export spans: %v", err)
	}
	if len(rec.last) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(rec.last))
	}
}
