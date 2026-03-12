package oteljetstream_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/Marz32onE/instrumentation-go/otel-nats/oteljetstream"
	otelnats "github.com/Marz32onE/instrumentation-go/otel-nats/otelnats"
)

func TestStreamInfo(t *testing.T) {
	url := startJetStreamServer(t)
	_ = otelnats.InitTracer("", otelnats.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := otelnats.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	streamName := "INFOTEST"
	_, err = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"info.>"},
	})
	require.NoError(t, err)

	stream, err := js.Stream(ctx, streamName)
	require.NoError(t, err)

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, streamName, info.Config.Name)
}

func TestStreamCachedInfo(t *testing.T) {
	url := startJetStreamServer(t)
	_ = otelnats.InitTracer("", otelnats.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := otelnats.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	streamName := "CACHEDINFOTEST"
	_, err = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"cached.>"},
	})
	require.NoError(t, err)

	stream, err := js.Stream(ctx, streamName)
	require.NoError(t, err)
	_, _ = stream.Info(ctx) // populate cache

	cached := stream.CachedInfo()
	require.NotNil(t, cached)
	require.Equal(t, streamName, cached.Config.Name)
}

func TestStreamConsumerNames(t *testing.T) {
	url := startJetStreamServer(t)
	_ = otelnats.InitTracer("", otelnats.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := otelnats.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	streamName := "NAMESTEST"
	_, err = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"names.>"},
	})
	require.NoError(t, err)

	stream, err := js.Stream(ctx, streamName)
	require.NoError(t, err)
	_, err = stream.CreateOrUpdateConsumer(ctx, oteljetstream.ConsumerConfig{
		Durable:       "cn1",
		FilterSubject: "names.x",
		AckPolicy:     oteljetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	lister := stream.ConsumerNames(ctx)
	var names []string
	for n := range lister.Name() {
		names = append(names, n)
	}
	require.NoError(t, lister.Err())
	require.Contains(t, names, "cn1")
}

func TestStreamCreateConsumer(t *testing.T) {
	url := startJetStreamServer(t)
	_ = otelnats.InitTracer("", otelnats.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := otelnats.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	streamName := "CREATECONSTEST"
	_, err = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"createcons.>"},
	})
	require.NoError(t, err)

	stream, err := js.Stream(ctx, streamName)
	require.NoError(t, err)

	cons, err := stream.CreateConsumer(ctx, oteljetstream.ConsumerConfig{
		Durable:       "create-only",
		FilterSubject: "createcons.a",
		AckPolicy:     oteljetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)
	require.NotNil(t, cons)
	_ = cons.CachedInfo()
}

func TestStreamDeleteConsumer(t *testing.T) {
	url := startJetStreamServer(t)
	_ = otelnats.InitTracer("", otelnats.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := otelnats.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	streamName := "DELCONSTEST"
	_, err = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"delcons.>"},
	})
	require.NoError(t, err)

	stream, err := js.Stream(ctx, streamName)
	require.NoError(t, err)
	_, err = stream.CreateOrUpdateConsumer(ctx, oteljetstream.ConsumerConfig{
		Durable:       "to-delete",
		FilterSubject: "delcons.x",
		AckPolicy:     oteljetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	err = stream.DeleteConsumer(ctx, "to-delete")
	require.NoError(t, err)

	_, err = stream.Consumer(ctx, "to-delete")
	require.Error(t, err)
}

func TestJetStreamDeleteStream(t *testing.T) {
	url := startJetStreamServer(t)
	_ = otelnats.InitTracer("", otelnats.WithTracerProviderInit(trace.NewTracerProvider()))
	conn, err := otelnats.Connect(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	require.NoError(t, err)
	ctx := context.Background()
	streamName := "DELSTREAMTEST"
	_, err = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{"delstream.>"},
	})
	require.NoError(t, err)

	err = js.DeleteStream(ctx, streamName)
	require.NoError(t, err)

	_, err = js.Stream(ctx, streamName)
	require.Error(t, err)
}
