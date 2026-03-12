package otelmongo

import (
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Database wraps *mongo.Database for document-level tracing (uses global TracerProvider).
type Database struct {
	*mongo.Database
	serverAddr string
	serverPort int
	propagator propagation.TextMapPropagator
}

// Collection returns a Collection with document-level trace propagation.
func (d *Database) Collection(name string, opts ...options.Lister[options.CollectionOptions]) *Collection {
	tracer := otel.GetTracerProvider().Tracer(ScopeName, trace.WithInstrumentationVersion(Version()))
	return &Collection{
		Collection: d.Database.Collection(name, opts...),
		tracer:     tracer,
		serverAddr: d.serverAddr,
		serverPort: d.serverPort,
		propagator: d.propagator,
	}
}
