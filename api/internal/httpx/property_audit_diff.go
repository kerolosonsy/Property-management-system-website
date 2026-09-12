package httpx

import (
	"reflect"
	"sort"

	pmscrypto "pms/internal/crypto"
	"pms/internal/properties"
)

// propertyAuditState is an in-memory comparison shape. Sensitive values are
// present only long enough to determine whether a field changed; callers must
// persist only the maps returned by diffPropertyAuditStates.
type propertyAuditState struct {
	values          map[string]any
	customValues    map[string]any
	sensitiveValues map[string]string
}

func newPropertyAuditState(
	p *properties.Property,
	customValues []properties.StoredValue,
	envelope *pmscrypto.Envelope,
) (propertyAuditState, error) {
	publicCustomValues, sensitiveValues, err := auditCustomValues(customValues, envelope)
	if err != nil {
		return propertyAuditState{}, err
	}
	return propertyAuditState{
		values: map[string]any{
			"name":           p.Name,
			"propertyTypeId": p.PropertyTypeID.String(),
			"areaId":         p.AreaID.String(),
		},
		customValues:    publicCustomValues,
		sensitiveValues: sensitiveValues,
	}, nil
}

func auditCustomValues(
	storedValues []properties.StoredValue,
	envelope *pmscrypto.Envelope,
) (map[string]any, map[string]string, error) {
	publicValues := make(map[string]any)
	sensitiveValues := make(map[string]string)
	for _, storedValue := range storedValues {
		fieldID := storedValue.FieldID.String()
		if storedValue.IsSensitive {
			plaintext, err := storedValue.Decrypt(envelope)
			if err != nil {
				return nil, nil, err
			}
			sensitiveValues[fieldID] = plaintext
			continue
		}
		publicValues[fieldID] = publicAuditValue(storedValue)
	}
	return publicValues, sensitiveValues, nil
}

func publicAuditValue(storedValue properties.StoredValue) any {
	switch storedValue.FieldType {
	case properties.FieldText:
		if storedValue.Text != nil {
			return *storedValue.Text
		}
	case properties.FieldCheckbox:
		if storedValue.Bool != nil {
			return *storedValue.Bool
		}
	case properties.FieldDropdown:
		if storedValue.ChoiceID != nil {
			return storedValue.ChoiceID.String()
		}
	case properties.FieldMultiselect:
		choiceIDs := make([]string, 0, len(storedValue.ChoiceIDs))
		for _, choiceID := range storedValue.ChoiceIDs {
			choiceIDs = append(choiceIDs, choiceID.String())
		}
		sort.Strings(choiceIDs)
		return choiceIDs
	}
	return nil
}

// diffPropertyAuditStates returns symmetrical before/after objects containing
// only changed fields. Missing custom values are represented as nil so undo
// knows to remove a value rather than restore an empty snapshot.
func diffPropertyAuditStates(before, after propertyAuditState) (map[string]any, map[string]any) {
	beforeDiff, afterDiff := diffAuditMaps(before.values, after.values)
	beforeCustom, afterCustom := diffAuditMaps(before.customValues, after.customValues)
	if len(beforeCustom) > 0 {
		beforeDiff["customValues"] = beforeCustom
		afterDiff["customValues"] = afterCustom
	}

	changedSensitive := changedSensitiveFields(before.sensitiveValues, after.sensitiveValues)
	if len(changedSensitive) > 0 {
		beforeDiff["sensitiveFields"] = changedSensitive
		afterDiff["sensitiveFields"] = changedSensitive
	}
	return beforeDiff, afterDiff
}

func diffAuditMaps(before, after map[string]any) (map[string]any, map[string]any) {
	beforeDiff := make(map[string]any)
	afterDiff := make(map[string]any)
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	for key := range keys {
		beforeValue, beforeExists := before[key]
		afterValue, afterExists := after[key]
		if beforeExists == afterExists && reflect.DeepEqual(beforeValue, afterValue) {
			continue
		}
		if beforeExists {
			beforeDiff[key] = beforeValue
		} else {
			beforeDiff[key] = nil
		}
		if afterExists {
			afterDiff[key] = afterValue
		} else {
			afterDiff[key] = nil
		}
	}
	return beforeDiff, afterDiff
}

func changedSensitiveFields(before, after map[string]string) []string {
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	changed := make([]string, 0, len(keys))
	for key := range keys {
		beforeValue, beforeExists := before[key]
		afterValue, afterExists := after[key]
		if beforeExists != afterExists || beforeValue != afterValue {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed
}
