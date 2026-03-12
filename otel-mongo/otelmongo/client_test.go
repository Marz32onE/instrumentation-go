package otelmongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/sdk/trace"
)

// TestConnect_AlignsWithMongoConnect ensures Connect accepts *options.ClientOptions like mongo.Connect.
func TestConnect_AlignsWithMongoConnect(t *testing.T) {
	_, _ = InitTracer("", WithTracerProviderInit(trace.NewTracerProvider()))

	opts := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := Connect(opts)
	require.NotErrorIs(t, err, ErrInitTracerRequired)
	if client != nil {
		_ = client.Disconnect(context.Background())
	}
}
