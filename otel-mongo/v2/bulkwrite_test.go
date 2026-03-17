package otelmongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestBuildBulkWriteModelsWithTrace_InsertOneModel(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "a", Value: 1}}),
	}
	out, err := buildBulkWriteModelsWithTrace(ctx, models)
	require.NoError(t, err)
	require.Len(t, out, 1)
	ins, ok := out[0].(*mongo.InsertOneModel)
	require.True(t, ok)
	doc, ok := getInsertOneModelDocument(ins)
	require.True(t, ok)
	docD, ok := doc.(bson.D)
	require.True(t, ok)
	hasTrace := false
	for _, e := range docD {
		if e.Key == TraceMetadataKey {
			hasTrace = true
			break
		}
	}
	assert.True(t, hasTrace, "InsertOneModel document should contain _oteltrace")
}

func TestBuildBulkWriteModelsWithTrace_UpdateOneModel(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	models := []mongo.WriteModel{
		mongo.NewUpdateOneModel().SetFilter(bson.D{{Key: "x", Value: 1}}).SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "y", Value: 2}}}}),
	}
	out, err := buildBulkWriteModelsWithTrace(ctx, models)
	require.NoError(t, err)
	require.Len(t, out, 1)
	upd, ok := out[0].(*mongo.UpdateOneModel)
	require.True(t, ok)
	_, update, ok := getUpdateModelFilterUpdate(upd)
	require.True(t, ok)
	updateD, ok := update.(bson.D)
	require.True(t, ok)
	hasSetTrace := false
	for _, e := range updateD {
		if e.Key == "$set" {
			setDoc, _ := e.Value.(bson.D)
			for _, s := range setDoc {
				if s.Key == TraceMetadataKey {
					hasSetTrace = true
					break
				}
			}
			break
		}
	}
	assert.True(t, hasSetTrace, "UpdateOneModel update $set should contain _oteltrace")
}

func TestBuildBulkWriteModelsWithTrace_OtherModelsUnchanged(t *testing.T) {
	ctx := context.Background()
	del := mongo.NewDeleteOneModel().SetFilter(bson.D{{Key: "_id", Value: 1}})
	models := []mongo.WriteModel{del}
	out, err := buildBulkWriteModelsWithTrace(ctx, models)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Same(t, del, out[0])
}
