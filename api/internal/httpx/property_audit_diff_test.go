package httpx

import (
	"reflect"
	"testing"
)

// Regression: the product-owner report showed full property snapshots for a
// name-only edit, which made the actual change hard to identify.
func TestPropertyAuditProductionRegressionKeepsOnlyChangedField(t *testing.T) {
	before := propertyAuditState{
		values: map[string]any{
			"name":           "المبنى القديم",
			"propertyTypeId": "type-1",
			"areaId":         "area-1",
		},
		customValues:    map[string]any{"field-1": "ثابت"},
		sensitiveValues: map[string]string{},
	}
	after := propertyAuditState{
		values: map[string]any{
			"name":           "المبنى الجديد",
			"propertyTypeId": "type-1",
			"areaId":         "area-1",
		},
		customValues:    map[string]any{"field-1": "ثابت"},
		sensitiveValues: map[string]string{},
	}

	gotBefore, gotAfter := diffPropertyAuditStates(before, after)
	wantBefore := map[string]any{"name": "المبنى القديم"}
	wantAfter := map[string]any{"name": "المبنى الجديد"}
	if !reflect.DeepEqual(gotBefore, wantBefore) {
		t.Fatalf("before diff = %#v, want %#v", gotBefore, wantBefore)
	}
	if !reflect.DeepEqual(gotAfter, wantAfter) {
		t.Fatalf("after diff = %#v, want %#v", gotAfter, wantAfter)
	}
}

func TestDiffPropertyAuditStatesMarksSensitiveChangeWithoutValues(t *testing.T) {
	before := propertyAuditState{
		values:          map[string]any{},
		customValues:    map[string]any{},
		sensitiveValues: map[string]string{"field-secret": "قديم"},
	}
	after := propertyAuditState{
		values:          map[string]any{},
		customValues:    map[string]any{},
		sensitiveValues: map[string]string{"field-secret": "جديد"},
	}

	gotBefore, gotAfter := diffPropertyAuditStates(before, after)
	want := map[string]any{"sensitiveFields": []string{"field-secret"}}
	if !reflect.DeepEqual(gotBefore, want) {
		t.Fatalf("before diff = %#v, want %#v", gotBefore, want)
	}
	if !reflect.DeepEqual(gotAfter, want) {
		t.Fatalf("after diff = %#v, want %#v", gotAfter, want)
	}
}

func TestDiffPropertyAuditStatesUsesNilForRemovedCustomValue(t *testing.T) {
	before := propertyAuditState{
		values:          map[string]any{},
		customValues:    map[string]any{"field-1": "قيمة"},
		sensitiveValues: map[string]string{},
	}
	after := propertyAuditState{
		values:          map[string]any{},
		customValues:    map[string]any{},
		sensitiveValues: map[string]string{},
	}

	gotBefore, gotAfter := diffPropertyAuditStates(before, after)
	wantBefore := map[string]any{"customValues": map[string]any{"field-1": "قيمة"}}
	wantAfter := map[string]any{"customValues": map[string]any{"field-1": nil}}
	if !reflect.DeepEqual(gotBefore, wantBefore) {
		t.Fatalf("before diff = %#v, want %#v", gotBefore, wantBefore)
	}
	if !reflect.DeepEqual(gotAfter, wantAfter) {
		t.Fatalf("after diff = %#v, want %#v", gotAfter, wantAfter)
	}
}
