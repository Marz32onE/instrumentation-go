// Package otelmongo provides a MongoDB driver v2 wrapper that propagates
// OpenTelemetry trace contexts to and from documents stored in MongoDB.
// Trace metadata is stored in a reserved field named "_oteltrace" in each
// document, enabling full lifecycle tracing of data across services.
package otelmongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TraceMetadataKey is the BSON field name used to store trace metadata in documents.
const TraceMetadataKey = "_oteltrace"

// TraceMetadata holds the W3C Trace Context fields stored alongside a MongoDB document.
type TraceMetadata struct {
	// Traceparent holds the W3C traceparent header value.
	Traceparent string `bson:"traceparent"`
	// Tracestate holds the W3C tracestate header value (optional).
	Tracestate string `bson:"tracestate,omitempty"`
}

// traceMetadataFromContext extracts W3C trace context from ctx into TraceMetadata using otel.GetTextMapPropagator().
func traceMetadataFromContext(ctx context.Context) (*TraceMetadata, bool) {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		return nil, false
	}
	prop := otel.GetTextMapPropagator()
	carrier := propagation.MapCarrier{}
	prop.Inject(ctx, carrier)
	return &TraceMetadata{
		Traceparent: carrier.Get("traceparent"),
		Tracestate:  carrier.Get("tracestate"),
	}, true
}

// injectTraceIntoDocument marshals document to bson.D and, when the span context in ctx is valid,
// appends an "_oteltrace" field using otel.GetTextMapPropagator(). The original document is not modified.
func injectTraceIntoDocument(ctx context.Context, document any) (bson.D, error) {
	raw, err := bson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("otelmongo: marshal document: %w", err)
	}

	doc := make(bson.D, 0, 1)
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("otelmongo: unmarshal document: %w", err)
	}

	meta, ok := traceMetadataFromContext(ctx)
	if !ok {
		return doc, nil
	}
	doc = append(doc, bson.E{Key: TraceMetadataKey, Value: *meta})
	return doc, nil
}

// extractMetadataFromRaw looks up the "_oteltrace" field in raw and, when found,
// unmarshals it into a TraceMetadata. Returns (nil, false) when the field is absent
// or cannot be decoded.
func extractMetadataFromRaw(raw bson.Raw) (*TraceMetadata, bool) {
	val, err := raw.LookupErr(TraceMetadataKey)
	if err != nil {
		return nil, false
	}

	rawDoc, ok := val.DocumentOK()
	if !ok {
		return nil, false
	}

	var meta TraceMetadata
	if err := bson.Unmarshal(rawDoc, &meta); err != nil {
		return nil, false
	}
	if meta.Traceparent == "" {
		return nil, false
	}
	return &meta, true
}

// ContextFromDocument returns a context enriched with the trace context stored in
// the document's "_oteltrace" field. Intended for consumers that read documents
// outside of the Collection CRUD helpers (e.g. change stream fullDocument).
// Uses the global propagator (otel.GetTextMapPropagator()). When _oteltrace is absent or invalid, the original ctx is returned unchanged.
func ContextFromDocument(ctx context.Context, raw bson.Raw) context.Context {
	meta, ok := extractMetadataFromRaw(raw)
	if !ok {
		return ctx
	}
	return contextFromTraceMetadata(ctx, meta)
}

// contextFromTraceMetadata injects the remote span context encoded in meta into ctx using otel.GetTextMapPropagator().
func contextFromTraceMetadata(ctx context.Context, meta *TraceMetadata) context.Context {
	prop := otel.GetTextMapPropagator()
	carrier := propagation.MapCarrier{
		"traceparent": meta.Traceparent,
	}
	if meta.Tracestate != "" {
		carrier["tracestate"] = meta.Tracestate
	}
	return prop.Extract(ctx, carrier)
}

// injectTraceIntoUpdate inspects update, and when ctx carries a valid span context,
// embeds the trace metadata using otel.GetTextMapPropagator().
//   - For operator updates (first key starts with "$") the metadata is added to "$set".
//   - For replacement documents the metadata is appended as a top-level field.
func injectTraceIntoUpdate(ctx context.Context, update any) (any, error) {
	meta, ok := traceMetadataFromContext(ctx)
	if !ok {
		return update, nil
	}

	raw, err := bson.Marshal(update)
	if err != nil {
		return update, fmt.Errorf("otelmongo: marshal update: %w", err)
	}

	doc := make(bson.D, 0, 1)
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return update, fmt.Errorf("otelmongo: unmarshal update: %w", err)
	}

	if len(doc) > 0 && len(doc[0].Key) > 0 && doc[0].Key[0] == '$' {
		// Operator update: inject into $set.
		doc = upsertSetField(doc, *meta)
		return doc, nil
	}

	// Replacement document: append as top-level field (same as injectTraceIntoDocument).
	doc = append(doc, bson.E{Key: TraceMetadataKey, Value: *meta})
	return doc, nil
}

// upsertSetField finds or creates the "$set" element in an operator update document
// and appends the trace metadata key to it.
func upsertSetField(doc bson.D, meta TraceMetadata) bson.D {
	for i, elem := range doc {
		if elem.Key == "$set" {
			var setDoc bson.D
			switch v := elem.Value.(type) {
			case bson.D:
				setDoc = v
			default:
				raw, err := bson.Marshal(v)
				if err != nil {
					break
				}
				_ = bson.Unmarshal(raw, &setDoc)
			}
			setDoc = append(setDoc, bson.E{Key: TraceMetadataKey, Value: meta})
			doc[i].Value = setDoc
			return doc
		}
	}
	// No existing $set — create one.
	doc = append(doc, bson.E{Key: "$set", Value: bson.D{{Key: TraceMetadataKey, Value: meta}}})
	return doc
}
