package httpx

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"pms/internal/properties"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// customFieldDef is the small subset of a custom field the write handlers need.
type customFieldDef struct {
	ID          uuid.UUID
	FieldType   properties.CustomFieldType
	IsSensitive bool
}

// loadCustomValueDefinitions reads every field id the body mentions, in one
// query, so a foreign field id is refused with 400 rather than becoming a
// FK violation inside the property write.
func (s *Server) loadCustomValueDefinitions(
	ctx context.Context, tx pgx.Tx,
	bodies []propertyCustomValueBody,
) (map[uuid.UUID]customFieldDef, error) {
	out := map[uuid.UUID]customFieldDef{}
	ids := make([]uuid.UUID, 0, len(bodies))
	for _, b := range bodies {
		fid, err := uuid.Parse(strings.TrimSpace(b.FieldID))
		if err != nil {
			return nil, err
		}
		ids = append(ids, fid)
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, field_type, is_sensitive FROM custom_field WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d customFieldDef
		var ft string
		if err := rows.Scan(&d.ID, &ft, &d.IsSensitive); err != nil {
			return nil, err
		}
		d.FieldType = properties.CustomFieldType(ft)
		out[d.ID] = d
	}
	return out, rows.Err()
}

// buildCustomValueInputs turns the JSON request bodies into the typed inputs
// the store consumes, validating the field type for each value. A bad type
// returns a 400 with a field-level Arabic message.
func buildCustomValueInputs(
	bodies []propertyCustomValueBody,
	defs map[uuid.UUID]customFieldDef,
) ([]properties.CustomValueInput, *APIError) {
	out := make([]properties.CustomValueInput, 0, len(bodies))
	for i, b := range bodies {
		fid, err := uuid.Parse(strings.TrimSpace(b.FieldID))
		if err != nil {
			return nil, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("customValues", MsgInvalidRequest)
		}
		def, ok := defs[fid]
		if !ok {
			return nil, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("customValues["+itoa(i)+"].fieldId", MsgCustomFieldNotFound)
		}
		in := properties.CustomValueInput{
			FieldID:      fid,
			FieldType:    def.FieldType,
			IsSensitive:  def.IsSensitive,
		}
		switch def.FieldType {
		case properties.FieldText:
			if b.Text != nil {
				s := *b.Text
				if len([]rune(s)) > 1024 {
					return nil, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
						WithField("customValues["+itoa(i)+"].text", MsgCustomFieldValueSize)
				}
				in.Text = &s
			}
		case properties.FieldCheckbox:
			if b.Checked != nil {
				v := *b.Checked
				in.Checked = &v
			}
		case properties.FieldDropdown:
			if b.ChoiceID != nil && *b.ChoiceID != "" {
				cid, err := uuid.Parse(*b.ChoiceID)
				if err != nil {
					return nil, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
						WithField("customValues["+itoa(i)+"].choiceId", MsgInvalidRequest)
				}
				in.ChoiceID = &cid
			}
		case properties.FieldMultiselect:
			for _, raw := range b.ChoiceIDs {
				cid, err := uuid.Parse(raw)
				if err != nil {
					return nil, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
						WithField("customValues["+itoa(i)+"].choiceIds", MsgInvalidRequest)
				}
				in.ChoiceIDs = append(in.ChoiceIDs, cid)
			}
		default:
			return nil, NewAPIError(http.StatusBadRequest, CodeInvalidRequest, MsgInvalidRequest).
				WithField("customValues["+itoa(i)+"].fieldId", MsgInvalidRequest)
		}
		out = append(out, in)
	}
	return out, nil
}

// itoa small local helper to keep field-error keys readable.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// errorIfUnused keeps imports tidy across revisions.
var _ = errors.New
