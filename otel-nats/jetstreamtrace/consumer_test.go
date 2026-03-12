package jetstreamtrace_test

import (
	"context"
	"testing"
	"time"

	natssrv "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Marz32onE/natstrace/jetstreamtrace"
	natstrace "github.com/Marz32onE/natstrace/natstrace"
)

func startJetStreamServer(t *testing.T) string {
	t.Helper()
	opts := &natssrv.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s, err := natssrv.NewServer(opts)
	require.NoError(t, err)
	go s.Start()
	require.True(t, s.ReadyForConnections(5*time.Second), "nats-server not ready")
	t.Cleanup(s.Shutdown)
	return s.ClientURL()
}

func findSpanByKind(spans []trace.ReadOnlySpan, kind oteltrace.SpanKind) trace.ReadOnlySpan {
	for _, s := range spans {
		if s.SpanKind() == kind {
			return s
		}
	}
	return nil
}

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key, want string) {
	t.Helper()
	for _, kv := range attrs {
		if string(kv.Key) == key {
			assert.Equal(t, want, kv.Value.AsString(), "attribute %q", key)
			return
		}
	}
	t.Errorf("attribute %q not found", key)
}

func TestFetchReturnsMessagesWithTraceContext(t *testing.T) {
	url := startJetStreamServer(t)
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})
	otel.SetTextMapPropagator(prop)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))

	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)

	ctx := context.Background()
	streamName := "FETCHTEST"
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     streamName,
		Subjects: []string{"fetch.>"},
	})
	require.NoError(t, err)

	stream, err := js.Stream(ctx, streamName)
	require.NoError(t, err)

	consumerName := "fetch-consumer"
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: "fetch.test",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)

	// Publish with trace context
	tracer := tp.Tracer("publisher")
	pubCtx, pubSpan := tracer.Start(ctx, "pub-parent")
	defer pubSpan.End()
	_, err = js.Publish(pubCtx, "fetch.test", []byte("hello fetch"))
	require.NoError(t, err)

	// Fetch with retries until message is available
	var received int
	var batch jetstreamtrace.MessageBatch
	for attempt := 0; attempt < 30; attempt++ {
		var ferr error
		batch, ferr = cons.Fetch(5, jetstream.FetchMaxWait(300*time.Millisecond))
		require.NoError(t, ferr)
		for m := range batch.MessagesWithContext() {
			received++
			assert.Equal(t, "hello fetch", string(m.Data()))
			span := oteltrace.SpanFromContext(m.Context())
			assert.True(t, span.SpanContext().TraceID().IsValid(), "context should have valid trace ID")
			_ = m.Ack()
		}
		if received == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Equal(t, 1, received, "expected 1 message after retries")
	if batch != nil {
		require.NoError(t, batch.Error())
	}

	spans := sr.Ended()
	consumerSpan := findSpanByKind(spans, oteltrace.SpanKindConsumer)
	producerSpan := findSpanByKind(spans, oteltrace.SpanKindProducer)
	require.NotNil(t, consumerSpan, "no consumer span")
	assertAttr(t, consumerSpan.Attributes(), "messaging.consumer.name", consumerName)
	if producerSpan != nil && len(consumerSpan.Links()) == 1 {
		linkCtx := consumerSpan.Links()[0].SpanContext
		assert.Equal(t, producerSpan.SpanContext().TraceID(), linkCtx.TraceID())
		assert.Equal(t, producerSpan.SpanContext().SpanID(), linkCtx.SpanID())
	}
}

func TestConsumeTraceContext(t *testing.T) {
	url := startJetStreamServer(t)
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})

	otel.SetTextMapPropagator(prop)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))
	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     "CONSUMETEST",
		Subjects: []string{"consume.>"},
	})
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "CONSUMETEST")
	require.NoError(t, err)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       "consume-dup",
		FilterSubject: "consume.msg",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)

	done := make(chan struct{}, 1)
	cc, err := cons.Consume(func(m jetstreamtrace.MsgWithContext) {
		if oteltrace.SpanFromContext(m.Context()).SpanContext().TraceID().IsValid() {
			done <- struct{}{}
		}
		_ = m.Ack()
	})
	require.NoError(t, err)
	defer cc.Stop()

	tracer := tp.Tracer("pub")
	pubCtx, pubSpan := tracer.Start(ctx, "parent")
	defer pubSpan.End()
	_, _ = js.Publish(pubCtx, "consume.msg", []byte("hi"))
	time.Sleep(300 * time.Millisecond)
	select {
	case <-done:
	default:
		t.Fatal("Consume handler did not receive trace context")
	}
}

func TestMessagesNextTraceContext(t *testing.T) {
	url := startJetStreamServer(t)
	tp := trace.NewTracerProvider()
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})

	otel.SetTextMapPropagator(prop)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))
	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     "MSGTEST",
		Subjects: []string{"msg.>"},
	})
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "MSGTEST")
	require.NoError(t, err)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       "msg-dup",
		FilterSubject: "msg.one",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)

	iter, err := cons.Messages()
	require.NoError(t, err)
	defer iter.Stop()

	_, _ = js.Publish(ctx, "msg.one", []byte("data"))
	time.Sleep(300 * time.Millisecond)

	msgCtx, msg, err := iter.Next()
	require.NoError(t, err)
	assert.Equal(t, "data", string(msg.Data()))
	assert.True(t, oteltrace.SpanFromContext(msgCtx).SpanContext().TraceID().IsValid(), "Next should return context with trace")
	_ = msg.Ack()
}

func TestFetchNoWaitReturnsTraceContext(t *testing.T) {
	url := startJetStreamServer(t)
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(tracetest.NewSpanRecorder()))
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})

	otel.SetTextMapPropagator(prop)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))
	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     "NOWAIT",
		Subjects: []string{"nowait.>"},
	})
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "NOWAIT")
	require.NoError(t, err)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       "nowait-c",
		FilterSubject: "nowait.x",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)

	_, _ = js.Publish(ctx, "nowait.x", []byte("v"))
	time.Sleep(200 * time.Millisecond)

	batch, err := cons.FetchNoWait(5)
	require.NoError(t, err)
	n := 0
	for m := range batch.MessagesWithContext() {
		n++
		assert.True(t, oteltrace.SpanFromContext(m.Context()).SpanContext().TraceID().IsValid(), "context should have trace")
		_ = m.Ack()
	}
	assert.Equal(t, 1, n)
}

func TestFetchBytesTraceContext(t *testing.T) {
	url := startJetStreamServer(t)
	tp := trace.NewTracerProvider()
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})

	otel.SetTextMapPropagator(prop)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(tp))
	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     "BYTESTEST",
		Subjects: []string{"bytes.>"},
	})
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "BYTESTEST")
	require.NoError(t, err)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       "bytes-c",
		FilterSubject: "bytes.a",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)

	_, _ = js.Publish(ctx, "bytes.a", []byte("hello"))
	time.Sleep(200 * time.Millisecond)

	batch, err := cons.FetchBytes(1024, jetstream.FetchMaxWait(5*time.Second))
	require.NoError(t, err)
	for m := range batch.MessagesWithContext() {
		assert.True(t, oteltrace.SpanFromContext(m.Context()).SpanContext().TraceID().IsValid(), "context should have trace")
		_ = m.Ack()
	}
}

func TestConsumerInfo(t *testing.T) {
	url := startJetStreamServer(t)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     "CONSINFOTEST",
		Subjects: []string{"consinfo.>"},
	})
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "CONSINFOTEST")
	require.NoError(t, err)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       "info-cons",
		FilterSubject: "consinfo.x",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)

	info, err := cons.Info(ctx)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "info-cons", info.Name)
}

func TestConsumerCachedInfo(t *testing.T) {
	url := startJetStreamServer(t)
	_ = natstrace.InitTracer("", natstrace.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := natstrace.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := jetstreamtrace.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = js.CreateOrUpdateStream(ctx, jetstreamtrace.StreamConfig{
		Name:     "CACHEDCONSINFO",
		Subjects: []string{"cachedcons.>"},
	})
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "CACHEDCONSINFO")
	require.NoError(t, err)
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstreamtrace.ConsumerConfig{
		Durable:       "cached-cons",
		FilterSubject: "cachedcons.x",
		AckPolicy:     jetstreamtrace.AckExplicitPolicy,
	})
	require.NoError(t, err)
	_, _ = cons.Info(ctx) // populate cache

	cached := cons.CachedInfo()
	require.NotNil(t, cached)
	require.Equal(t, "cached-cons", cached.Name)
}
