// Package mongotrace: OTel database and MongoDB semantic convention helpers.
// See https://opentelemetry.io/docs/specs/semconv/db/database-spans/ and
// https://opentelemetry.io/docs/specs/semconv/db/mongodb/

package mongotrace

import (
	"errors"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DB semconv attribute keys (stable per OTel spec).
const (
	keyDBSystemName         = "db.system.name"
	keyDBCollection         = "db.collection.name"
	keyDBNamespace          = "db.namespace"
	keyDBOperationName      = "db.operation.name"
	keyDBOpBatchSize        = "db.operation.batch.size"
	keyDBResponseStatusCode = "db.response.status_code"
	keyErrorType            = "error.type"
	keyServerAddress        = "server.address"
	keyServerPort           = "server.port"
)

const (
	dbSystemMongoDB = "mongodb"
	// ErrorTypeOther is the fallback error.type per OTel when no custom value applies.
	ErrorTypeOther = "_OTHER"
)

// dbSpanName returns the span name per OTel: "{db.operation.name} {target}".
// Target is the collection name for MongoDB.
func dbSpanName(operation, collectionName string) string {
	if collectionName == "" {
		return operation
	}
	return operation + " " + collectionName
}

// dbAttributes returns attributes for a MongoDB client span.
// batchSize is 0 for single-doc ops; for batch ops (insertMany, updateMany, deleteMany) pass size >= 2.
// serverAddr and serverPort are optional; server.port is only set when address is set and port != 27017.
func dbAttributes(dbName, collName, operation string, batchSize int, serverAddr string, serverPort int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(keyDBSystemName, dbSystemMongoDB),
		attribute.String(keyDBCollection, collName),
		attribute.String(keyDBOperationName, operation),
	}
	if dbName != "" {
		attrs = append(attrs, attribute.String(keyDBNamespace, dbName))
	}
	if batchSize >= 2 {
		attrs = append(attrs, attribute.Int(keyDBOpBatchSize, batchSize))
	}
	if serverAddr != "" {
		attrs = append(attrs, attribute.String(keyServerAddress, serverAddr))
		if serverPort > 0 && serverPort != 27017 {
			attrs = append(attrs, attribute.Int(keyServerPort, serverPort))
		}
	}
	return attrs
}

// recordSpanError sets span status to Error and records db.response.status_code and error.type
// per OTel MongoDB/database semantic conventions when err is non-nil.
func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	var writeErr mongo.WriteException
	if errors.As(err, &writeErr) {
		errCodes := writeErr.ErrorCodes()
		if len(errCodes) > 0 {
			codeStr := strconv.Itoa(errCodes[0])
			span.SetAttributes(
				attribute.String(keyDBResponseStatusCode, codeStr),
				attribute.String(keyErrorType, codeStr),
			)
			return
		}
	}

	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		codeStr := strconv.Itoa(int(cmdErr.Code))
		span.SetAttributes(
			attribute.String(keyDBResponseStatusCode, codeStr),
			attribute.String(keyErrorType, codeStr),
		)
		return
	}

	span.SetAttributes(attribute.String(keyErrorType, ErrorTypeOther))
}
