package otelmongo

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

// InsertOneResult wraps *mongo.InsertOneResult. Use when calling Collection.InsertOne.
type InsertOneResult struct {
	*mongo.InsertOneResult
}

// InsertManyResult wraps *mongo.InsertManyResult. Use when calling Collection.InsertMany.
type InsertManyResult struct {
	*mongo.InsertManyResult
}

// UpdateResult wraps *mongo.UpdateResult. Use when calling UpdateOne, UpdateMany, ReplaceOne, UpdateByID.
type UpdateResult struct {
	*mongo.UpdateResult
}

// DeleteResult wraps *mongo.DeleteResult. Use when calling DeleteOne, DeleteMany.
type DeleteResult struct {
	*mongo.DeleteResult
}

// BulkWriteResult wraps *mongo.BulkWriteResult. Use when calling Collection.BulkWrite.
type BulkWriteResult struct {
	*mongo.BulkWriteResult
}

// ChangeStream wraps *mongo.ChangeStream. Use when calling Collection.Watch.
// Use DecodeWithContext to automatically restore trace context from fullDocument,
// or ContextFromDocument(ctx, event.FullDocument) for manual extraction.
type ChangeStream struct {
	*mongo.ChangeStream
}

// Next advances the change stream to the next change document. See *mongo.ChangeStream.Next.
func (cs *ChangeStream) Next(ctx context.Context) bool {
	return cs.ChangeStream.Next(ctx)
}

// Decode decodes the current change document into val. See *mongo.ChangeStream.Decode.
func (cs *ChangeStream) Decode(val any) error {
	return cs.ChangeStream.Decode(val)
}

// DecodeWithContext decodes the current change document into val and returns a
// context enriched with trace context extracted from fullDocument's "_oteltrace"
// field. When the field is absent (e.g. delete events) or invalid, the returned
// context is unchanged. The val parameter can be any user-defined struct — it
// does not need a fullDocument field; extraction uses the raw BSON internally.
func (cs *ChangeStream) DecodeWithContext(ctx context.Context, val any) (context.Context, error) {
	if err := cs.ChangeStream.Decode(val); err != nil {
		return ctx, err
	}
	return contextFromChangeEvent(ctx, cs.Current), nil
}

// Close closes the change stream. See *mongo.ChangeStream.Close.
func (cs *ChangeStream) Close(ctx context.Context) error {
	return cs.ChangeStream.Close(ctx)
}

// Err returns the last error. See *mongo.ChangeStream.Err.
func (cs *ChangeStream) Err() error {
	return cs.ChangeStream.Err()
}
