package otelmongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// buildDocWithTrace creates a raw BSON document that contains an _oteltrace field
// matching the span context in ctx.
func buildDocWithTrace(t *testing.T, ctx context.Context) bson.Raw { //nolint:revive // ctx is second parameter intentionally for test helpers
	t.Helper()
	doc := bson.D{{Key: "value", Value: "test"}}
	injected, err := injectTraceIntoDocument(ctx, doc)
	require.NoError(t, err)
	raw, err := bson.Marshal(injected)
	require.NoError(t, err)
	return raw
}

func TestCursorDecodeWithContext_ExtractsTrace(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")

	// Start a span so we have a valid trace context to inject.
	ctx, span := tracer.Start(context.Background(), "origin")
	originSpanCtx := span.SpanContext()
	span.End()

	rawDoc := buildDocWithTrace(t, ctx)

	cursor, err := mongo.NewCursorFromDocuments([]any{rawDoc}, nil, nil)
	require.NoError(t, err)
	defer func() { _ = cursor.Close(context.Background()) }()

	require.True(t, cursor.Next(context.Background()))

	c := &Cursor{Cursor: cursor, parentCtx: ctx}

	var result bson.D
	enriched, err := c.DecodeWithContext(context.Background(), &result)
	require.NoError(t, err)

	recovered := trace.SpanContextFromContext(enriched)
	assert.True(t, recovered.IsValid())
	assert.Equal(t, originSpanCtx.TraceID(), recovered.TraceID())
}

func TestCursorDecodeWithContext_NoTrace(t *testing.T) {
	// Document without trace metadata.
	raw, err := bson.Marshal(bson.D{{Key: "x", Value: 1}})
	require.NoError(t, err)

	cursor, err := mongo.NewCursorFromDocuments([]any{raw}, nil, nil)
	require.NoError(t, err)
	defer func() { _ = cursor.Close(context.Background()) }()

	require.True(t, cursor.Next(context.Background()))

	baseCtx := context.Background()
	c := &Cursor{Cursor: cursor, parentCtx: baseCtx}

	var result bson.D
	enriched, err := c.DecodeWithContext(baseCtx, &result)
	require.NoError(t, err)

	// Context should not carry a remote span.
	recovered := trace.SpanContextFromContext(enriched)
	assert.False(t, recovered.IsValid())
}

func TestSingleResultDecodeLinksOriginTrace(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")

	// Create a context with an active span so we can inject trace metadata.
	ctx, originSpan := tracer.Start(context.Background(), "origin")
	originSpan.End()

	rawDoc := buildDocWithTrace(t, ctx)

	mongoSR := mongo.NewSingleResultFromDocument(rawDoc, nil, nil)
	_, findSpan := tracer.Start(context.Background(), "findone")

	wrapped := &SingleResult{
		SingleResult: mongoSR,
		tracer:       tracer,
		span:         findSpan,
		ctx:          ctx,
	}

	var out bson.D
	err := wrapped.Decode(&out)
	require.NoError(t, err)

	// The findone span should now be ended and have a link to the origin trace.
	ended := sr.Ended()
	require.NotEmpty(t, ended)

	var findoneSpan sdktrace.ReadOnlySpan
	for _, s := range ended {
		if s.Name() == "findone" {
			findoneSpan = s
			break
		}
	}
	require.NotNil(t, findoneSpan, "findone span should have ended")

	links := findoneSpan.Links()
	require.NotEmpty(t, links, "expected link to origin trace")
	assert.Equal(t, originSpan.SpanContext().TraceID(), links[0].SpanContext.TraceID())
}

func TestSingleResultTraceContext(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")

	ctx, originSpan := tracer.Start(context.Background(), "origin")
	originSpanCtx := originSpan.SpanContext()
	originSpan.End()

	rawDoc := buildDocWithTrace(t, ctx)

	mongoSR := mongo.NewSingleResultFromDocument(rawDoc, nil, nil)
	_, findSpan := tracer.Start(context.Background(), "findone2")

	wrapped := &SingleResult{
		SingleResult: mongoSR,
		tracer:       tracer,
		span:         findSpan,
		ctx:          ctx,
	}

	enriched := wrapped.TraceContext()
	recovered := trace.SpanContextFromContext(enriched)
	assert.True(t, recovered.IsValid())
	assert.Equal(t, originSpanCtx.TraceID(), recovered.TraceID())
}

func TestCursorDecode(t *testing.T) {
	raw, err := bson.Marshal(bson.D{{Key: "field", Value: "v"}})
	require.NoError(t, err)

	cursor, err := mongo.NewCursorFromDocuments([]any{raw}, nil, nil)
	require.NoError(t, err)
	defer func() { _ = cursor.Close(context.Background()) }()

	require.True(t, cursor.Next(context.Background()))

	c := &Cursor{Cursor: cursor, parentCtx: context.Background()}

	var result bson.D
	err = c.Decode(&result)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestSingleResultRaw(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")

	ctx, originSpan := tracer.Start(context.Background(), "origin")
	originSpan.End()

	rawDoc := buildDocWithTrace(t, ctx)
	mongoSR := mongo.NewSingleResultFromDocument(rawDoc, nil, nil)
	_, findSpan := tracer.Start(context.Background(), "findone-raw")

	wrapped := &SingleResult{
		SingleResult: mongoSR,
		tracer:       tracer,
		span:         findSpan,
		ctx:          ctx,
	}

	raw, err := wrapped.Raw()
	require.NoError(t, err)
	assert.NotEmpty(t, raw)

	// Span should have ended after Raw()
	ended := sr.Ended()
	var found bool
	for _, s := range ended {
		if s.Name() == "findone-raw" {
			found = true
			break
		}
	}
	assert.True(t, found, "findone-raw span should be ended after Raw()")
}

func TestSingleResultDecodeSpanEndedOnce(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")

	ctx, originSpan := tracer.Start(context.Background(), "origin")
	originSpan.End()

	rawDoc := buildDocWithTrace(t, ctx)
	mongoSR := mongo.NewSingleResultFromDocument(rawDoc, nil, nil)
	_, findSpan := tracer.Start(context.Background(), "findone-once")

	wrapped := &SingleResult{
		SingleResult: mongoSR,
		tracer:       tracer,
		span:         findSpan,
		ctx:          ctx,
	}

	// Call Decode twice – span should be ended only once.
	var out bson.D
	_ = wrapped.Decode(&out)
	_ = wrapped.Decode(&out)

	var count int
	for _, s := range sr.Ended() {
		if s.Name() == "findone-once" {
			count++
		}
	}
	assert.Equal(t, 1, count, "span must be ended exactly once")
}
