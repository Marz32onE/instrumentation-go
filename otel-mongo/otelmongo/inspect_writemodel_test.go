package otelmongo

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestInspectWriteModelFields verifies that the field names used by reflection-based
// bulk write trace injection still exist on the driver's WriteModel types.
// If a driver upgrade renames these fields, this test will fail — update bulkwrite.go accordingly.
func TestInspectWriteModelFields(t *testing.T) {
	ins := mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "x", Value: 1}})
	typ := reflect.TypeOf(ins).Elem()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		t.Logf("InsertOneModel field: %s", f.Name)
	}

	updOne := mongo.NewUpdateOneModel().SetFilter(bson.D{}).SetUpdate(bson.D{{Key: "$set", Value: bson.D{}}})
	typ2 := reflect.TypeOf(updOne).Elem()
	for i := 0; i < typ2.NumField(); i++ {
		f := typ2.Field(i)
		t.Logf("UpdateOneModel field: %s", f.Name)
	}

	updMany := mongo.NewUpdateManyModel().SetFilter(bson.D{}).SetUpdate(bson.D{{Key: "$set", Value: bson.D{}}})
	typ3 := reflect.TypeOf(updMany).Elem()
	for i := 0; i < typ3.NumField(); i++ {
		f := typ3.Field(i)
		t.Logf("UpdateManyModel field: %s", f.Name)
	}

	// Verify the specific fields accessed by bulkwrite.go reflection
	assertField(t, ins, "Document")
	assertField(t, updOne, "Filter")
	assertField(t, updOne, "Update")
	assertField(t, updMany, "Filter")
	assertField(t, updMany, "Update")
}

func assertField(t *testing.T, model any, fieldName string) {
	t.Helper()
	v := reflect.ValueOf(model).Elem()
	f := v.FieldByName(fieldName)
	if !f.IsValid() {
		t.Errorf("%T: field %q not found — bulkwrite.go reflection will silently skip trace injection", model, fieldName)
	} else if !f.CanInterface() {
		t.Errorf("%T: field %q is unexported — bulkwrite.go reflection will silently skip trace injection", model, fieldName)
	}
}
