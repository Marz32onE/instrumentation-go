package otelmongo

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestInspectWriteModelFields prints WriteModel struct field names for reflection-based bulk write.
func TestInspectWriteModelFields(t *testing.T) {
	ins := mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "x", Value: 1}})
	typ := reflect.TypeOf(ins).Elem()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		t.Logf("InsertOneModel field: %s", f.Name)
	}
	upd := mongo.NewUpdateOneModel().SetFilter(bson.D{}).SetUpdate(bson.D{{Key: "$set", Value: bson.D{}}})
	typ2 := reflect.TypeOf(upd).Elem()
	for i := 0; i < typ2.NumField(); i++ {
		f := typ2.Field(i)
		t.Logf("UpdateOneModel field: %s", f.Name)
	}
}
