// Example demonstrates how to initialize the OpenTelemetry TracerProvider and
// TextMapPropagator at process startup, then use otelnats and oteljetstream.
// The instrumentation packages do not provide InitTracer; the application is
// responsible for creating and setting the global provider and propagator
// (per OTel Go Contrib instrumentation guidelines).
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Marz32onE/instrumentation-go/otel-nats/oteljetstream"
	"github.com/Marz32onE/instrumentation-go/otel-nats/otelnats"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	// 1) Create TracerProvider (e.g. OTLP exporter) and set global provider + propagator.
	tp, err := newTracerProvider()
	if err != nil {
		log.Fatalf("newTracerProvider: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 2) Use instrumentation: Connect and optional JetStream.
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	conn, err := otelnats.Connect(natsURL, nil)
	if err != nil {
		log.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	js, err := oteljetstream.New(conn)
	if err != nil {
		log.Printf("JetStream not available: %v", err)
		return
	}
	ctx := context.Background()
	_, _ = js.CreateOrUpdateStream(ctx, oteljetstream.StreamConfig{
		Name:     "EXAMPLE",
		Subjects: []string{"example.>"},
	})
	if _, err := js.Publish(ctx, "example.hello", []byte("world")); err != nil {
		log.Printf("Publish: %v", err)
	}
	log.Println("Example done.")
}

func newTracerProvider() (*sdktrace.TracerProvider, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	exp, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", "otel-nats-example"),
			attribute.String("service.version", "0.0.1"),
		),
	)
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	), nil
}
