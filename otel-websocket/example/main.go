// Example demonstrates how to initialize the OpenTelemetry TracerProvider and
// TextMapPropagator at process startup, then use otelwebsocket.
// The instrumentation package does not provide InitTracer; the application is
// responsible for creating and setting the global provider and propagator
// (per OTel Go Contrib instrumentation guidelines).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	// 1) Create TracerProvider and set global provider + propagator.
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

	// 2) Use instrumentation: after gorilla/websocket Upgrader.Upgrade, wrap with
	//    conn := otelwebsocket.NewConn(upgradedConn)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Set OTEL_EXPORTER_OTLP_ENDPOINT and run with a real WebSocket server to test."))
	})
	log.Println("Example server (no WebSocket upgrade in this minimal example). Run with OTEL_EXPORTER_OTLP_ENDPOINT set.")
	_ = http.ListenAndServe(":0", nil)
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
			attribute.String("service.name", "otel-websocket-example"),
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
