package otelmongo

import (
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Database wraps *mongo.Database for document-level tracing (uses otel globals).
type Database struct {
	*mongo.Database
	serverAddr string
	serverPort int
}

// Collection returns a Collection with document-level trace propagation (tracer/propagator from otel globals).
func (d *Database) Collection(name string, opts ...*options.CollectionOptions) *Collection {
	tracer := otel.GetTracerProvider().Tracer(ScopeName, trace.WithInstrumentationVersion(Version()))
	return &Collection{
		Collection: d.Database.Collection(name, opts...),
		tracer:     tracer,
		serverAddr: d.serverAddr,
		serverPort: d.serverPort,
	}
}
