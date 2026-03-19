package otelmongo

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const dbOperationNameKey = "db.operation.name"

// SkipDBOperationsExporter wraps an exporter and drops spans whose db.operation.name
// matches one of skipOps (case-insensitive), e.g. "getMore".
func SkipDBOperationsExporter(delegate sdktrace.SpanExporter, skipOps []string) sdktrace.SpanExporter {
	return &skipDBOperationsExporter{
		delegate: delegate,
		skipOps:  normalizeSkipOps(skipOps),
	}
}

type skipDBOperationsExporter struct {
	delegate sdktrace.SpanExporter
	skipOps  map[string]struct{}
}

func (e *skipDBOperationsExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 || len(e.skipOps) == 0 {
		return e.delegate.ExportSpans(ctx, spans)
	}
	filtered := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, s := range spans {
		if shouldSkipSpanByOperation(s.Attributes(), e.skipOps) {
			continue
		}
		filtered = append(filtered, s)
	}
	if len(filtered) == 0 {
		return nil
	}
	return e.delegate.ExportSpans(ctx, filtered)
}

func (e *skipDBOperationsExporter) Shutdown(ctx context.Context) error {
	return e.delegate.Shutdown(ctx)
}

func (e *skipDBOperationsExporter) ForceFlush(ctx context.Context) error {
	type forceFlusher interface {
		ForceFlush(context.Context) error
	}
	ff, ok := e.delegate.(forceFlusher)
	if !ok {
		return nil
	}
	return ff.ForceFlush(ctx)
}

func normalizeSkipOps(ops []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ops))
	for _, op := range ops {
		normalized := strings.ToLower(strings.TrimSpace(op))
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func shouldSkipSpanByOperation(attrs []attribute.KeyValue, skip map[string]struct{}) bool {
	for _, kv := range attrs {
		if string(kv.Key) != dbOperationNameKey {
			continue
		}
		if kv.Value.Type() != attribute.STRING {
			return false
		}
		_, ok := skip[strings.ToLower(strings.TrimSpace(kv.Value.AsString()))]
		return ok
	}
	return false
}
