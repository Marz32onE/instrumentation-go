package otelmongo

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

// TestConnect_AlignsWithMongoConnect ensures Connect accepts *options.ClientOptions like mongo.Connect.
func TestConnect_AlignsWithMongoConnect(t *testing.T) {
	otel.SetTracerProvider(trace.NewTracerProvider())

	opts := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := Connect(opts)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
	}
	if client != nil {
		_ = client.Disconnect(context.Background())
	}
}
