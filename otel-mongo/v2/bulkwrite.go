package otelmongo

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// buildBulkWriteModelsWithTrace returns a new slice of WriteModels with _oteltrace
// injected into InsertOneModel, UpdateOneModel, and UpdateManyModel. Other model types are unchanged.
func buildBulkWriteModelsWithTrace(ctx context.Context, models []mongo.WriteModel) ([]mongo.WriteModel, error) {
	out := make([]mongo.WriteModel, 0, len(models))
	for _, m := range models {
		switch vm := m.(type) {
		case *mongo.InsertOneModel:
			doc, ok := getInsertOneModelDocument(vm)
			if !ok {
				out = append(out, m)
				continue
			}
			docWithTrace, err := injectTraceIntoDocument(ctx, doc)
			if err != nil {
				return nil, fmt.Errorf("otelmongo: bulk insert inject trace: %w", err)
			}
			out = append(out, mongo.NewInsertOneModel().SetDocument(docWithTrace))
		case *mongo.UpdateOneModel:
			filter, update, ok := getUpdateModelFilterUpdate(vm)
			if !ok {
				out = append(out, m)
				continue
			}
			updateWithTrace, err := injectTraceIntoUpdate(ctx, update)
			if err != nil {
				return nil, fmt.Errorf("otelmongo: bulk updateOne inject trace: %w", err)
			}
			out = append(out, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(updateWithTrace))
		case *mongo.UpdateManyModel:
			filter, update, ok := getUpdateManyModelFilterUpdate(vm)
			if !ok {
				out = append(out, m)
				continue
			}
			updateWithTrace, err := injectTraceIntoUpdate(ctx, update)
			if err != nil {
				return nil, fmt.Errorf("otelmongo: bulk updateMany inject trace: %w", err)
			}
			out = append(out, mongo.NewUpdateManyModel().SetFilter(filter).SetUpdate(updateWithTrace))
		default:
			out = append(out, m)
		}
	}
	return out, nil
}

// getInsertOneModelDocument returns the document from *mongo.InsertOneModel via reflection.
func getInsertOneModelDocument(m *mongo.InsertOneModel) (any, bool) {
	if m == nil {
		return nil, false
	}
	v := reflect.ValueOf(m).Elem()
	f := v.FieldByName("Document")
	if !f.IsValid() || !f.CanInterface() {
		return nil, false
	}
	return f.Interface(), true
}

// getUpdateModelFilterUpdate returns filter and update from *mongo.UpdateOneModel via reflection.
func getUpdateModelFilterUpdate(m *mongo.UpdateOneModel) (filter, update any, ok bool) {
	if m == nil {
		return nil, nil, false
	}
	v := reflect.ValueOf(m).Elem()
	filterF := v.FieldByName("Filter")
	updateF := v.FieldByName("Update")
	if !filterF.IsValid() || !updateF.IsValid() || !filterF.CanInterface() || !updateF.CanInterface() {
		return nil, nil, false
	}
	return filterF.Interface(), updateF.Interface(), true
}

// getUpdateManyModelFilterUpdate returns filter and update from *mongo.UpdateManyModel via reflection.
func getUpdateManyModelFilterUpdate(m *mongo.UpdateManyModel) (filter, update any, ok bool) {
	if m == nil {
		return nil, nil, false
	}
	v := reflect.ValueOf(m).Elem()
	filterF := v.FieldByName("Filter")
	updateF := v.FieldByName("Update")
	if !filterF.IsValid() || !updateF.IsValid() || !filterF.CanInterface() || !updateF.CanInterface() {
		return nil, nil, false
	}
	return filterF.Interface(), updateF.Interface(), true
}
