package properties

import (
	"context"
	"errors"
	"strings"

	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Suggestion sources for the built-in filters. The contract's enum mirrors
// these names exactly.
const (
	SuggestCode           = "code"
	SuggestName           = "name"
	SuggestPropertyType   = "propertyType"
	SuggestArea           = "area"
	SuggestAttachmentName = "attachmentName"
)

// maxSuggestions is the contract's cap on /properties/field-suggestions.
const maxSuggestions = 10

// ErrSuggestionTarget — neither or both of field/fieldId was supplied.
var ErrSuggestionTarget = errors.New("exactly one of field or fieldId is required")

// ErrCheckboxNoSuggestions — a checkbox field holds booleans, not values to
// complete against.
var ErrCheckboxNoSuggestions = errors.New("checkbox fields have no suggestions")

// FieldSuggestions returns at most 10 distinct values for one filter, ordered
// by descending usage count then by label. It is a pure read: no audit row,
// no write. A sensitive custom field is refused (ErrSensitiveField) before any
// query runs — a suggestion must never be derived from an encrypted value, and
// nothing here decrypts (Constitution VII).
func (s *Store) FieldSuggestions(
	ctx context.Context, tx pgx.Tx,
	field string, fieldID *uuid.UUID, q string,
) ([]string, error) {
	if (field == "" && fieldID == nil) || (field != "" && fieldID != nil) {
		return nil, ErrSuggestionTarget
	}

	if fieldID != nil {
		return s.customFieldSuggestions(ctx, tx, *fieldID, q)
	}

	// q narrows the candidates by substring after the same Arabic
	// normalisation every other search uses; an absent q leaves the
	// candidates unconstrained so the most common values come back.
	canonical := identity.CanonicalQuery(strings.TrimSpace(q))
	like := ""
	args := []any{}
	if canonical != "" {
		like = "%" + canonical + "%"
		args = append(args, like)
	}

	switch field {
	case SuggestCode:
		return s.suggest(ctx, tx, `
			SELECT p.code, count(*) AS usage
			FROM property p`+whereLike("p.code_normalized", len(args))+`
			GROUP BY p.code
			ORDER BY usage DESC, p.code
			LIMIT `+itoa(maxSuggestions), args)
	case SuggestName:
		return s.suggest(ctx, tx, `
			SELECT p.name, count(*) AS usage
			FROM property p`+whereLike("p.name_normalized", len(args))+`
			GROUP BY p.name
			ORDER BY usage DESC, p.name
			LIMIT `+itoa(maxSuggestions), args)
	case SuggestPropertyType:
		return s.suggest(ctx, tx, `
			SELECT t.label, count(p.id) AS usage
			FROM property_type t
			LEFT JOIN property p ON p.property_type_id = t.id`+
			whereLike("t.label_normalized", len(args))+`
			GROUP BY t.id, t.label
			ORDER BY usage DESC, t.label
			LIMIT `+itoa(maxSuggestions), args)
	case SuggestArea:
		return s.suggest(ctx, tx, `
			SELECT t.label, count(p.id) AS usage
			FROM area t
			LEFT JOIN property p ON p.area_id = t.id`+
			whereLike("t.label_normalized", len(args))+`
			GROUP BY t.id, t.label
			ORDER BY usage DESC, t.label
			LIMIT `+itoa(maxSuggestions), args)
	case SuggestAttachmentName:
		// Descriptions only — never extracted document text — and sensitive
		// attachments are included: sensitivity conceals a document's
		// contents, never its existence or its description (FR-022d). Rows
		// predating migration 0023's backfill hold NULL description_normalized
		// and fall back to a raw case-insensitive match, exactly as the
		// attachmentName search filter does.
		where := ""
		if like != "" {
			args = append(args, "%"+strings.ToLower(strings.TrimSpace(q))+"%")
			where = ` WHERE a.description_normalized LIKE $1
			   OR (a.description_normalized IS NULL AND lower(a.description) LIKE $2)`
		}
		return s.suggest(ctx, tx, `
			SELECT a.description, count(*) AS usage
			FROM attachment a`+where+`
			GROUP BY a.description
			ORDER BY usage DESC, a.description
			LIMIT `+itoa(maxSuggestions), args)
	default:
		return nil, ErrSuggestionTarget
	}
}

// customFieldSuggestions serves fieldId=…: a custom field's stored text values
// (text and autocomplete) or its defined choice labels (dropdown, multiselect).
func (s *Store) customFieldSuggestions(
	ctx context.Context, tx pgx.Tx, fieldID uuid.UUID, q string,
) ([]string, error) {
	var fieldType CustomFieldType
	var isSensitive bool
	err := tx.QueryRow(ctx,
		`SELECT field_type, is_sensitive FROM custom_field WHERE id = $1`, fieldID).
		Scan(&fieldType, &isSensitive)
	if err != nil {
		return nil, ErrUnknownField
	}
	if isSensitive {
		// Refused before any query, exactly as BuildSearchInputs refuses a
		// filter naming a sensitive field. Sensitive values live only in the
		// cipher columns and this code never touches them.
		return nil, ErrSensitiveField
	}
	if fieldType == FieldCheckbox {
		return nil, ErrCheckboxNoSuggestions
	}

	canonical := identity.CanonicalQuery(strings.TrimSpace(q))
	like := ""
	args := []any{fieldID}
	if canonical != "" {
		like = "%" + canonical + "%"
		args = append(args, like)
	}

	switch fieldType {
	case FieldDropdown, FieldMultiselect:
		// The field's defined choice labels, most used first; a choice nobody
		// has picked yet is still a choice and stays suggestible. Counting
		// through custom_field_id uses the field indexes migration 0024
		// added; counting per choice_id would seq-scan per choice.
		//
		// The field id is a predicate of its own ($1), not just a join
		// condition: without it the query would read every field's choices.
		// The label LIKE is bound only when a query text exists, as its own
		// parameter in the position it occupies in args — whereLike is not
		// used here because its argc-based contract assumes the LIKE pattern
		// is the only parameter, which is not true of this branch.
		valueTable := "property_field_value"
		if fieldType == FieldMultiselect {
			valueTable = "property_field_multi_value"
		}
		choiceFilter := ""
		if like != "" {
			choiceFilter = " AND c.label_normalized LIKE $" + itoa(len(args))
		}
		return s.suggest(ctx, tx, `
			SELECT c.label, count(v.property_id) AS usage
			FROM custom_field_choice c
			LEFT JOIN `+valueTable+` v
				ON v.custom_field_id = c.custom_field_id AND v.choice_id = c.id
			WHERE c.custom_field_id = $1`+choiceFilter+`
			GROUP BY c.id, c.label
			ORDER BY usage DESC, c.label
			LIMIT `+itoa(maxSuggestions), args)
	default: // FieldText, FieldAutocomplete
		// Stored text values. The LIKE matches the canonicalised query against
		// text_value the same way the advanced-search `contains` operator
		// does; suggestions and search therefore agree on what matches.
		textFilter := ""
		if like != "" {
			textFilter = " AND v.text_value LIKE $" + itoa(len(args))
		}
		return s.suggest(ctx, tx, `
			SELECT v.text_value, count(*) AS usage
			FROM property_field_value v
			WHERE v.custom_field_id = $1 AND v.text_value IS NOT NULL`+textFilter+`
			GROUP BY v.text_value
			ORDER BY usage DESC, v.text_value
			LIMIT `+itoa(maxSuggestions), args)
	}
}

// suggest runs one suggestion query and flattens the rows.
func (s *Store) suggest(ctx context.Context, tx pgx.Tx, sql string, args []any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, maxSuggestions)
	for rows.Next() {
		var value string
		var usage int
		if err := rows.Scan(&value, &usage); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// whereLike appends a WHERE LIKE clause binding the next parameter. It is for
// queries where the LIKE pattern is the ONLY parameter: argc both decides
// whether a query text exists and numbers the placeholder. Callers whose args
// already hold other values (the custom-field branches) must build their LIKE
// predicate by hand instead, or the placeholder collides with those values.
func whereLike(column string, argc int) string {
	if argc == 0 {
		return ""
	}
	return " WHERE " + column + " LIKE $" + itoa(argc)
}
