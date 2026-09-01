package properties

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CustomFieldType is the field type as exposed to callers.
type CustomFieldType string

const (
	FieldText       CustomFieldType = "text"
	FieldDropdown   CustomFieldType = "dropdown"
	FieldMultiselect CustomFieldType = "multiselect"
	FieldCheckbox   CustomFieldType = "checkbox"
)

// CustomFieldChoice is one option of a dropdown or multiselect field.
type CustomFieldChoice struct {
	ID    uuid.UUID
	Label string
}

// CustomField is one row from custom_field, with its choices in display order
// and the count of properties currently holding a value for it (FR-027s5).
type CustomField struct {
	ID          uuid.UUID
	Label       string
	FieldType   CustomFieldType
	IsSensitive bool
	Choices     []CustomFieldChoice
	ValuesCount int
}

// ErrDuplicateLabel — a field with the same canonical label already exists.
var ErrDuplicateLabel = errors.New("custom field label already in use")

// ErrSensitiveTextOnly — a field marked sensitive must be of type text
// (research.md D-009 / FR-027s1).
var ErrSensitiveTextOnly = errors.New("sensitive is allowed only for text fields")

// ErrChoicesRequired — a dropdown or multiselect field must arrive with at least
// one choice (FR-027a).
var ErrChoicesRequired = errors.New("dropdown and multiselect fields require choices")

// ErrInUseValuesExist — the field cannot be removed, retyped, or have a choice
// removed while any property holds a value that the change would destroy
// (FR-027g + data-model 0013).
var ErrInUseValuesExist = errors.New("custom field has stored values")

// ListCustomFields returns every custom field, with its choices and the
// number of properties currently holding a value for it. The count is what
// the field-definition screen uses to disable the sensitivity checkbox when
// sensitivity is fixed (FR-027s5).
func (s *Store) ListCustomFields(ctx context.Context, tx pgx.Tx) ([]CustomField, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, label, field_type, is_sensitive
		FROM custom_field
		ORDER BY display_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomField
	for rows.Next() {
		var f CustomField
		if err := rows.Scan(&f.ID, &f.Label, &f.FieldType, &f.IsSensitive); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	// Load choices for every field in one query.
	choiceRows, err := tx.Query(ctx, `
		SELECT id, custom_field_id, label
		FROM custom_field_choice
		ORDER BY custom_field_id, display_order, id`)
	if err != nil {
		return nil, err
	}
	defer choiceRows.Close()
	byField := map[uuid.UUID][]CustomFieldChoice{}
	for choiceRows.Next() {
		var c CustomFieldChoice
		var fieldID uuid.UUID
		if err := choiceRows.Scan(&c.ID, &fieldID, &c.Label); err != nil {
			return nil, err
		}
		byField[fieldID] = append(byField[fieldID], c)
	}
	if err := choiceRows.Err(); err != nil {
		return nil, err
	}

	// Count properties currently holding a value per field, so the UI can
	// disable the sensitivity checkbox on a field whose values already exist
	// (FR-027s5). One query covers both single-value and multi-value rows.
	countRows, err := tx.Query(ctx, `
		SELECT cf.id,
		       (SELECT count(DISTINCT property_id) FROM property_field_value       WHERE custom_field_id = cf.id)
		     + (SELECT count(DISTINCT property_id) FROM property_field_multi_value WHERE custom_field_id = cf.id)
		FROM custom_field cf`)
	if err != nil {
		return nil, err
	}
	defer countRows.Close()
	counts := map[uuid.UUID]int{}
	for countRows.Next() {
		var id uuid.UUID
		var n int
		if err := countRows.Scan(&id, &n); err != nil {
			return nil, err
		}
		counts[id] = n
	}
	if err := countRows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		out[i].Choices = byField[out[i].ID]
		out[i].ValuesCount = counts[out[i].ID]
	}
	return out, nil
}

// CreateCustomField inserts the definition plus its choices, returning the row
// it stored.
func (s *Store) CreateCustomField(
	ctx context.Context, tx pgx.Tx,
	label, labelNormalized string,
	fieldType CustomFieldType,
	isSensitive bool,
	choices []string,
) (*CustomField, error) {
	if isSensitive && fieldType != FieldText {
		return nil, ErrSensitiveTextOnly
	}
	if (fieldType == FieldDropdown || fieldType == FieldMultiselect) && len(choices) == 0 {
		return nil, ErrChoicesRequired
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO custom_field (label, label_normalized, field_type, is_sensitive)
		VALUES ($1, $2, $3, $4)
		RETURNING id, label, field_type, is_sensitive`,
		label, labelNormalized, string(fieldType), isSensitive)
	f := &CustomField{}
	if err := row.Scan(&f.ID, &f.Label, &f.FieldType, &f.IsSensitive); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateLabel
		}
		return nil, err
	}
	for i, ch := range choices {
		if _, err := tx.Exec(ctx, `
			INSERT INTO custom_field_choice (custom_field_id, label, label_normalized, display_order)
			VALUES ($1, $2, $3, $4)`, f.ID, strings.TrimSpace(ch), identity.Canonical(strings.TrimSpace(ch)), i); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, ErrDuplicateLabel
			}
			return nil, err
		}
	}
	choicesOut, err := s.loadChoices(ctx, tx, f.ID)
	if err != nil {
		return nil, err
	}
	f.Choices = choicesOut
	return f, nil
}

func (s *Store) loadChoices(ctx context.Context, tx pgx.Tx, fieldID uuid.UUID) ([]CustomFieldChoice, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, label FROM custom_field_choice
		WHERE custom_field_id = $1
		ORDER BY display_order, id`, fieldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomFieldChoice
	for rows.Next() {
		var c CustomFieldChoice
		if err := rows.Scan(&c.ID, &c.Label); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCustomField renames the label and adjusts choices. Removing a choice
// is refused while any property holds it (ErrInUseValuesExist).
func (s *Store) UpdateCustomField(
	ctx context.Context, tx pgx.Tx,
	id uuid.UUID,
	label, labelNormalized string,
	addChoices []string,
	removeChoiceIDs []uuid.UUID,
) (*CustomField, error) {
	if len(removeChoiceIDs) > 0 {
		for _, cid := range removeChoiceIDs {
			var n int
			if err := tx.QueryRow(ctx, `
				SELECT (
					(SELECT count(*) FROM property_field_value WHERE choice_id = $1)
					+
					(SELECT count(*) FROM property_field_multi_value WHERE choice_id = $1)
				)`, cid).Scan(&n); err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, ErrInUseValuesExist
			}
		}
	}

	row := tx.QueryRow(ctx, `
		UPDATE custom_field
		SET label = $2, label_normalized = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, label, field_type, is_sensitive`, id, label, labelNormalized)
	f := &CustomField{}
	if err := row.Scan(&f.ID, &f.Label, &f.FieldType, &f.IsSensitive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateLabel
		}
		return nil, err
	}

	for _, cid := range removeChoiceIDs {
		if _, err := tx.Exec(ctx, `DELETE FROM custom_field_choice WHERE id = $1 AND custom_field_id = $2`, cid, id); err != nil {
			return nil, err
		}
	}
	if len(addChoices) > 0 {
		// Append to the end of the existing display order.
		var maxOrder int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(display_order), -1) FROM custom_field_choice WHERE custom_field_id = $1`, id).Scan(&maxOrder); err != nil {
			return nil, err
		}
		for i, ch := range addChoices {
			if _, err := tx.Exec(ctx, `
				INSERT INTO custom_field_choice (custom_field_id, label, label_normalized, display_order)
				VALUES ($1, $2, $3, $4)`, id, strings.TrimSpace(ch), identity.Canonical(strings.TrimSpace(ch)), maxOrder+1+i); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return nil, ErrDuplicateLabel
				}
				return nil, err
			}
		}
	}

	choicesOut, choicesErr := s.loadChoices(ctx, tx, f.ID)
	if choicesErr != nil {
		return nil, choicesErr
	}
	f.Choices = choicesOut
	return f, nil
}

// DeleteCustomField removes a field with no values referencing it. The
// ON DELETE RESTRICT constraints refuse the delete when values exist; this
// method surfaces the same as ErrInUseValuesExist and counts first so the
// Arabic message can say how many.
func (s *Store) DeleteCustomField(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT (
			(SELECT count(*) FROM property_field_value WHERE custom_field_id = $1)
			+
			(SELECT count(*) FROM property_field_multi_value WHERE custom_field_id = $1)
		)`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrInUseValuesExist
	}
	if _, err := tx.Exec(ctx, `DELETE FROM custom_field WHERE id = $1`, id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrInUseValuesExist
		}
		return err
	}
	return nil
}

// CountCustomFieldUsage returns the number of properties (active or archived)
// that hold a value for this field. Used by the in-use Arabic message.
func (s *Store) CountCustomFieldUsage(ctx context.Context, tx pgx.Tx, id uuid.UUID) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT (
			(SELECT count(DISTINCT property_id) FROM property_field_value WHERE custom_field_id = $1)
			+
			(SELECT count(DISTINCT property_id) FROM property_field_multi_value WHERE custom_field_id = $1)
		)`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count usage: %w", err)
	}
	return n, nil
}
