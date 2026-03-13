package otelmongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/trace"
)

// Collection wraps *mongo.Collection and overrides CRUD methods to inject and
// extract OpenTelemetry trace contexts via the "_oteltrace" document field (uses otel globals).
type Collection struct {
	*mongo.Collection
	tracer     trace.Tracer
	serverAddr string
	serverPort int
}

// NewCollection wraps an existing *mongo.Collection with trace propagation (tracer/propagator from otel globals).
func NewCollection(coll *mongo.Collection, tracer trace.Tracer) *Collection {
	return &Collection{Collection: coll, tracer: tracer}
}

// dbAndColl returns the database name and collection name for semconv attributes.
func (c *Collection) dbAndColl() (dbName, collName string) {
	collName = c.Name()
	if c.Database() != nil {
		dbName = c.Database().Name()
	}
	return dbName, collName
}

// InsertOne inserts document into the collection.
// The active trace context from ctx is serialised into the document under the
// "_oteltrace" field before the insert. otelmongo already creates the command span.
func (c *Collection) InsertOne(ctx context.Context, document any, opts ...options.Lister[options.InsertOneOptions]) (*InsertOneResult, error) {
	docWithTrace, err := injectTraceIntoDocument(ctx, document)
	if err != nil {
		return nil, fmt.Errorf("otelmongo: inject trace: %w", err)
	}
	res, err := c.Collection.InsertOne(ctx, docWithTrace, opts...)
	if err != nil {
		return nil, err
	}
	return &InsertOneResult{res}, nil
}

// InsertMany inserts documents into the collection.
// The active trace context from ctx is serialised into each document under the
// "_oteltrace" field before the insert. otelmongo already creates the command span.
func (c *Collection) InsertMany(ctx context.Context, documents []any, opts ...options.Lister[options.InsertManyOptions]) (*InsertManyResult, error) {
	docsWithTrace := make([]any, 0, len(documents))
	for _, doc := range documents {
		d, err := injectTraceIntoDocument(ctx, doc)
		if err != nil {
			return nil, fmt.Errorf("otelmongo: inject trace: %w", err)
		}
		docsWithTrace = append(docsWithTrace, d)
	}
	res, err := c.Collection.InsertMany(ctx, docsWithTrace, opts...)
	if err != nil {
		return nil, err
	}
	return &InsertManyResult{res}, nil
}

// Find executes a find command and returns a Cursor that can extract trace
// contexts from returned documents. Span name/attrs follow OTel DB semconv.
func (c *Collection) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (*Cursor, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("find", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "find", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	cursor, err := c.Collection.Find(ctx, filter, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &Cursor{Cursor: cursor, tracer: c.tracer, parentCtx: ctx}, nil
}

// FindOne executes a find command returning at most one document.
// The returned *SingleResult carries the span; call Decode to end the span
// and record a span link to the document's origin trace when present.
func (c *Collection) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) *SingleResult {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("findOne", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "findOne", 0, c.serverAddr, c.serverPort)...),
	)
	sr := c.Collection.FindOne(ctx, filter, opts...)
	return &SingleResult{SingleResult: sr, tracer: c.tracer, span: span, ctx: ctx}
}

// UpdateOne injects the current trace context into the update so the document's
// _oteltrace is replaced. No extra read; one round-trip only. Span follows OTel DB semconv.
func (c *Collection) UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*UpdateResult, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("updateOne", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "updateOne", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	updateWithTrace, err := injectTraceIntoUpdate(ctx, update)
	if err != nil {
		updateWithTrace = update
	}

	res, err := c.Collection.UpdateOne(ctx, filter, updateWithTrace, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{res}, nil
}

// UpdateMany injects the current trace context into the update so matched
// documents record the most recent writer's trace. No per-document origin
// read (batch). Span follows OTel DB semconv.
func (c *Collection) UpdateMany(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateManyOptions]) (*UpdateResult, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("updateMany", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "updateMany", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	updateWithTrace, _ := injectTraceIntoUpdate(ctx, update)

	res, err := c.Collection.UpdateMany(ctx, filter, updateWithTrace, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{res}, nil
}

// ReplaceOne injects the current trace context into the replacement document.
// otelmongo already creates the command span.
func (c *Collection) ReplaceOne(ctx context.Context, filter any, replacement any, opts ...options.Lister[options.ReplaceOptions]) (*UpdateResult, error) {
	replacementWithTrace, err := injectTraceIntoDocument(ctx, replacement)
	if err != nil {
		return nil, fmt.Errorf("otelmongo: inject trace: %w", err)
	}
	res, err := c.Collection.ReplaceOne(ctx, filter, replacementWithTrace, opts...)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{res}, nil
}

// DeleteOne deletes one document. No extra read; one round-trip only.
// Span follows OTel DB semconv.
func (c *Collection) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*DeleteResult, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("deleteOne", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "deleteOne", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	res, err := c.Collection.DeleteOne(ctx, filter, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &DeleteResult{res}, nil
}

// DeleteMany executes a delete command against all documents matching filter.
// Span follows OTel DB semconv. No per-document origin read (batch).
func (c *Collection) DeleteMany(ctx context.Context, filter any, opts ...options.Lister[options.DeleteManyOptions]) (*DeleteResult, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("deleteMany", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "deleteMany", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	res, err := c.Collection.DeleteMany(ctx, filter, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &DeleteResult{res}, nil
}

// CountDocuments counts documents matching filter. Span follows OTel DB semconv (read-only, no _oteltrace).
func (c *Collection) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("countDocuments", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "countDocuments", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	n, err := c.Collection.CountDocuments(ctx, filter, opts...)
	recordSpanError(span, err)
	return n, err
}

// Distinct returns distinct values for fieldName. Span follows OTel DB semconv (read-only).
// Call the result's Err() after decoding to check for errors.
func (c *Collection) Distinct(ctx context.Context, fieldName string, filter any, opts ...options.Lister[options.DistinctOptions]) *mongo.DistinctResult {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("distinct", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "distinct", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	res := c.Collection.Distinct(ctx, fieldName, filter, opts...)
	return res
}

// Aggregate runs an aggregation pipeline and returns a Cursor that supports
// DecodeWithContext for document trace propagation. Span follows OTel DB semconv.
func (c *Collection) Aggregate(ctx context.Context, pipeline any, opts ...options.Lister[options.AggregateOptions]) (*Cursor, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("aggregate", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "aggregate", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	cursor, err := c.Collection.Aggregate(ctx, pipeline, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &Cursor{Cursor: cursor, tracer: c.tracer, parentCtx: ctx}, nil
}

// UpdateByID updates one document by _id. Injects current trace into update
// (document _oteltrace is replaced). Span follows OTel DB semconv.
func (c *Collection) UpdateByID(ctx context.Context, id any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*UpdateResult, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("updateOne", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "updateOne", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	updateWithTrace, _ := injectTraceIntoUpdate(ctx, update)
	res, err := c.Collection.UpdateByID(ctx, id, updateWithTrace, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{res}, nil
}

// DeleteOneByID deletes one document by _id. Span follows OTel DB semconv (no _oteltrace on delete).
func (c *Collection) DeleteOneByID(ctx context.Context, id any, opts ...options.Lister[options.DeleteOneOptions]) (*DeleteResult, error) {
	return c.DeleteOne(ctx, map[string]any{"_id": id}, opts...)
}

// FindOneByID returns a SingleResult for the document with the given _id.
// Decode or TraceContext to get span link from document's _oteltrace when present.
func (c *Collection) FindOneByID(ctx context.Context, id any, opts ...options.Lister[options.FindOneOptions]) *SingleResult {
	return c.FindOne(ctx, map[string]any{"_id": id}, opts...)
}

// FindByIDs returns a Cursor over documents whose _id is in the given ids slice.
// Use Cursor.DecodeWithContext for trace propagation from each document's _oteltrace.
func (c *Collection) FindByIDs(ctx context.Context, ids []any, opts ...options.Lister[options.FindOptions]) (*Cursor, error) {
	return c.Find(ctx, map[string]any{"_id": map[string]any{"$in": ids}}, opts...)
}

// BulkWrite runs multiple write operations. Span follows OTel DB semconv.
// _oteltrace is injected into InsertOneModel, UpdateOneModel, and UpdateManyModel.
func (c *Collection) BulkWrite(ctx context.Context, models []mongo.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (*BulkWriteResult, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("bulkWrite", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "bulkWrite", len(models), c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	injected, err := buildBulkWriteModelsWithTrace(ctx, models)
	if err != nil {
		recordSpanError(span, err)
		return nil, err
	}
	res, err := c.Collection.BulkWrite(ctx, injected, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &BulkWriteResult{res}, nil
}

// Watch starts a change stream. Use ContextFromDocument(ctx, event.FullDocument) to
// restore trace context from fullDocument. Span follows OTel DB semconv.
func (c *Collection) Watch(ctx context.Context, pipeline any, opts ...options.Lister[options.ChangeStreamOptions]) (*ChangeStream, error) {
	dbName, collName := c.dbAndColl()
	spanName := dbSpanName("watch", collName)
	ctx, span := c.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(dbAttributes(dbName, collName, "watch", 0, c.serverAddr, c.serverPort)...),
	)
	defer span.End()

	cs, err := c.Collection.Watch(ctx, pipeline, opts...)
	recordSpanError(span, err)
	if err != nil {
		return nil, err
	}
	return &ChangeStream{cs}, nil
}
