package otelmongo

import (
	"context"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/trace"
)

// Cursor wraps *mongo.Cursor so that callers can optionally extract the trace
// context stored in each document as they iterate.
type Cursor struct {
	*mongo.Cursor
	tracer    trace.Tracer
	parentCtx context.Context
}

// DecodeWithContext decodes the current document into val and returns a context
// enriched with the trace context extracted from the document's "_oteltrace"
// field. When the field is absent the returned context is unchanged.
//
// Use this instead of Decode when you need to propagate the document's origin
// trace context downstream.
func (c *Cursor) DecodeWithContext(ctx context.Context, val any) (context.Context, error) {
	if err := c.Cursor.Decode(val); err != nil {
		return ctx, err
	}
	raw := c.Current
	if meta, ok := extractMetadataFromRaw(raw); ok {
		ctx = contextFromTraceMetadata(ctx, meta)
	}
	return ctx, nil
}

// Decode decodes the current document into val.
// It delegates directly to the underlying *mongo.Cursor.Decode.
func (c *Cursor) Decode(val any) error {
	return c.Cursor.Decode(val)
}

// SingleResult wraps *mongo.SingleResult so that the stored trace context can
// be propagated as a span link and extracted by callers.
type SingleResult struct {
	*mongo.SingleResult
	tracer  trace.Tracer
	span    trace.Span
	ctx     context.Context
	endOnce sync.Once
}

// endSpan ensures the associated span is ended exactly once.
func (r *SingleResult) endSpan() {
	r.endOnce.Do(func() { r.span.End() })
}

// Decode decodes the document and records any stored trace context as a span
// link on the FindOne span before ending it.
// The span is ended exactly once regardless of how many times Decode is called.
func (r *SingleResult) Decode(v any) error {
	defer r.endSpan()

	raw, err := r.SingleResult.Raw()
	if err != nil {
		recordSpanError(r.span, err)
		return err
	}

	if meta, ok := extractMetadataFromRaw(raw); ok {
		originCtx := contextFromTraceMetadata(context.Background(), meta)
		originSpanCtx := trace.SpanContextFromContext(originCtx)
		if originSpanCtx.IsValid() {
			r.span.AddLink(trace.Link{SpanContext: originSpanCtx})
		}
	}

	return r.SingleResult.Decode(v)
}

// TraceContext returns a context enriched with the trace context stored in the
// fetched document's "_oteltrace" field. It must be called after Decode or Raw.
// The span is ended exactly once when this method is called.
func (r *SingleResult) TraceContext() context.Context {
	defer r.endSpan()

	raw, err := r.SingleResult.Raw()
	if err != nil {
		return r.ctx
	}
	if meta, ok := extractMetadataFromRaw(raw); ok {
		return contextFromTraceMetadata(r.ctx, meta)
	}
	return r.ctx
}

// Raw returns the raw BSON document and ends the span exactly once.
func (r *SingleResult) Raw() (bson.Raw, error) {
	defer r.endSpan()
	return r.SingleResult.Raw()
}
