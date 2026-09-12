package properties

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Operator is one of the five operator values the contract permits.
type Operator string

const (
	OpContains    Operator = "contains"
	OpEquals      Operator = "equals"
	OpIncludesAll Operator = "includesAll"
	OpIsTrue      Operator = "isTrue"
	OpIsFalse     Operator = "isFalse"
)

// ErrUnknownField is returned when a filter references a custom field that
// does not exist. Constitution I: the server decides, never the client.
var ErrUnknownField = errors.New("unknown custom field")

// ErrSensitiveField is returned when a filter names a sensitive custom field.
// FR-027s4 + Constitution VII: sensitive fields cannot be searched.
var ErrSensitiveField = errors.New("sensitive field is not searchable")

// ErrOperatorMismatch is returned when the operator does not suit the field's
// type.
var ErrOperatorMismatch = errors.New("operator does not suit field type")

// SearchFilter is one entry from AdvancedSearch.customFilters.
type SearchFilter struct {
	FieldID   uuid.UUID
	Operator  Operator
	Text      *string
	ChoiceID  *uuid.UUID
	ChoiceIDs []uuid.UUID
}

// SearchInputs is the prepared form of AdvancedSearch: built-in filters plus
// resolved (validated) custom-field filters.
type SearchInputs struct {
	Q               string
	PropertyTypeID  *uuid.UUID
	AreaID          *uuid.UUID
	IncludeArchived bool
	DocumentText    string // matches text extracted from the property's attachments
	AttachmentName  string // matches an attachment's description or original filename
	HasAttachments  string // "any" (default), "yes", or "no"
	Page            int
	PageSize        int
	CustomFilters   []SearchFilter
}

// SearchByIDs is the resolved-and-validated set of every custom field a filter
// names, in the same order as SearchInputs.CustomFilters.
type SearchByIDs struct {
	Fields map[uuid.UUID]CustomField
}

// BuildSearchInputs resolves every SearchFilter against the definitions and
// returns the cleaned inputs ready to bind into a query. The validation runs
// server-side before any SQL is built, so an unknown id, a sensitive field, or
// an operator/type mismatch is rejected before it can become part of a query.
func (s *Store) BuildSearchInputs(ctx context.Context, tx pgx.Tx, in SearchInputs) (SearchByIDs, error) {
	out := SearchByIDs{Fields: map[uuid.UUID]CustomField{}}
	for _, f := range in.CustomFilters {
		row := tx.QueryRow(ctx, `SELECT id, label, field_type, is_sensitive FROM custom_field WHERE id = $1`, f.FieldID)
		var def CustomField
		var fieldType string
		if err := row.Scan(&def.ID, &def.Label, &fieldType, &def.IsSensitive); err != nil {
			return out, ErrUnknownField
		}
		def.FieldType = CustomFieldType(fieldType)
		if def.IsSensitive {
			return out, ErrSensitiveField
		}
		if err := checkOperator(f.Operator, def.FieldType); err != nil {
			return out, err
		}
		out.Fields[def.ID] = def
	}
	return out, nil
}

func checkOperator(op Operator, t CustomFieldType) error {
	switch op {
	case OpContains:
		if t != FieldText {
			return ErrOperatorMismatch
		}
	case OpEquals:
		if t != FieldDropdown {
			return ErrOperatorMismatch
		}
	case OpIncludesAll:
		if t != FieldMultiselect {
			return ErrOperatorMismatch
		}
	case OpIsTrue, OpIsFalse:
		if t != FieldCheckbox {
			return ErrOperatorMismatch
		}
	default:
		return ErrOperatorMismatch
	}
	return nil
}

// SearchProperties runs the validated advanced search and returns the page of
// properties plus the total count. Built-in filters combine with AND with the
// custom filters; archived properties are excluded unless included.
func (s *Store) SearchProperties(
	ctx context.Context, tx pgx.Tx,
	in SearchInputs, byIDs SearchByIDs,
) ([]Property, int, error) {
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 100 {
		in.PageSize = 25
	}

	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if !in.IncludeArchived {
		where = append(where, "p.archived_at IS NULL")
	}

	q := strings.TrimSpace(in.Q)
	if q != "" {
		canonical := identity.CanonicalQuery(q)
		where = append(where, "(p.name_normalized LIKE $"+itoa(idx)+" OR p.code_normalized LIKE $"+itoa(idx)+")")
		args = append(args, "%"+canonical+"%")
		idx++
	}

	// Document text is a filter like any other and combines by AND. The
	// EXISTS keeps one property to one row however many of its attachments
	// match. Sensitive attachments carry no body_normalized at all, so this
	// cannot reach them however the query is written (research.md D-008) —
	// the exclusion is structural, not a predicate that could be forgotten.
	documentPattern := ""
	if dt := strings.TrimSpace(in.DocumentText); dt != "" {
		documentPattern = "%" + identity.CanonicalQuery(dt) + "%"
		where = append(where, `EXISTS (
			SELECT 1 FROM attachment a
			JOIN attachment_text t ON t.attachment_id = a.id
			WHERE a.property_id = p.id
			  AND t.body_normalized IS NOT NULL
			  AND t.body_normalized LIKE $`+itoa(idx)+`)`)
		args = append(args, documentPattern)
		idx++
	}
	// Attachment name: description or original filename. Unlike document text
	// this DOES reach sensitive attachments — sensitivity conceals a document's
	// contents, never its existence or what it is called (FR-022d).
	//
	// The normalised columns are populated by the Go normaliser; rows predating
	// migration 0023 hold NULL until the backfill runs, so a raw case-insensitive
	// match stands in for them rather than making them invisible.
	if an := strings.TrimSpace(in.AttachmentName); an != "" {
		canonical := "%" + identity.CanonicalQuery(an) + "%"
		raw := "%" + strings.ToLower(an) + "%"
		where = append(where, `EXISTS (
			SELECT 1 FROM attachment a
			WHERE a.property_id = p.id
			  AND (
			        a.description_normalized LIKE $`+itoa(idx)+`
			     OR a.filename_normalized    LIKE $`+itoa(idx)+`
			     OR (a.description_normalized IS NULL AND lower(a.description)       LIKE $`+itoa(idx+1)+`)
			     OR (a.filename_normalized    IS NULL AND lower(a.original_filename) LIKE $`+itoa(idx+1)+`)
			  ))`)
		args = append(args, canonical, raw)
		idx += 2
	}

	// Whether the property holds any attachment at all.
	switch strings.TrimSpace(in.HasAttachments) {
	case "yes":
		where = append(where, `EXISTS (SELECT 1 FROM attachment a WHERE a.property_id = p.id)`)
	case "no":
		where = append(where, `NOT EXISTS (SELECT 1 FROM attachment a WHERE a.property_id = p.id)`)
	}

	if in.PropertyTypeID != nil {
		where = append(where, "p.property_type_id = $"+itoa(idx))
		args = append(args, *in.PropertyTypeID)
		idx++
	}
	if in.AreaID != nil {
		where = append(where, "p.area_id = $"+itoa(idx))
		args = append(args, *in.AreaID)
		idx++
	}

	// Each custom filter contributes a clause that joins against the values
	// table (or the multi-values table for multiselect) and binds its
	// operand as a parameter — never interpolates into the SQL string.
	join := ""
	for _, f := range in.CustomFilters {
		def := byIDs.Fields[f.FieldID]
		clause, nextIdx, err := buildCustomFilter(f, def, idx, join, args)
		if err != nil {
			return nil, 0, err
		}
		// The helper returns the join snippet separately so a multiselect
		// AND of multiple choices can keep adding joins.
		join = clause.join
		where = append(where, clause.where...)
		args = clause.args
		idx = nextIdx
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT p.id) FROM property p `+join+` WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, in.PageSize, (in.Page-1)*in.PageSize)
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id, p.code, p.name, p.property_type_id, p.area_id,
		       p.version, p.created_at, p.created_by, cu.username,
		       p.updated_at, p.updated_by, uu.username,
		       p.archived_at, p.archived_by, au.username,
		       pt.label, a.label
		FROM property p
		JOIN property_type pt ON pt.id = p.property_type_id
		JOIN area          a  ON a.id  = p.area_id
		JOIN account cu ON cu.id = p.created_by
		JOIN account uu ON uu.id = p.updated_by
		LEFT JOIN account au ON au.id = p.archived_by
		`+join+`
		WHERE `+whereSQL+`
		ORDER BY p.name, p.id
		LIMIT $`+itoa(idx)+` OFFSET $`+itoa(idx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Property
	for rows.Next() {
		var p Property
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.PropertyTypeID, &p.AreaID,
			&p.Version, &p.CreatedAt, &p.CreatedBy, &p.CreatedByName,
			&p.UpdatedAt, &p.UpdatedBy, &p.UpdatedByName,
			&p.ArchivedAt, &p.ArchivedBy, &p.ArchivedByName,
			&p.PropertyTypeLabel, &p.AreaLabel); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()

	if documentPattern != "" && len(out) > 0 {
		matchesByProperty, err := findMatchedAttachments(ctx, tx, out, documentPattern)
		if err != nil {
			return nil, 0, err
		}
		for index := range out {
			out[index].MatchedAttachments = matchesByProperty[out[index].ID]
		}
	}
	return out, total, nil
}

func findMatchedAttachments(
	ctx context.Context,
	tx pgx.Tx,
	propertyRows []Property,
	documentPattern string,
) (map[uuid.UUID][]MatchedAttachment, error) {
	propertyIDs := make([]uuid.UUID, 0, len(propertyRows))
	for _, propertyRow := range propertyRows {
		propertyIDs = append(propertyIDs, propertyRow.ID)
	}
	rows, err := tx.Query(ctx, `
		SELECT a.property_id, a.id, a.description
		FROM attachment a
		JOIN attachment_text t ON t.attachment_id = a.id
		WHERE a.property_id = ANY($1)
		  AND t.body_normalized IS NOT NULL
		  AND t.body_normalized LIKE $2
		ORDER BY a.property_id, a.created_at, a.id`, propertyIDs, documentPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := make(map[uuid.UUID][]MatchedAttachment, len(propertyRows))
	for rows.Next() {
		var propertyID uuid.UUID
		var attachment MatchedAttachment
		if err := rows.Scan(&propertyID, &attachment.ID, &attachment.Description); err != nil {
			return nil, err
		}
		matches[propertyID] = append(matches[propertyID], attachment)
	}
	return matches, rows.Err()
}

// customClause is the result of building one filter's contribution to the
// search SQL. Args are appended in order and idx moves past them.
type customClause struct {
	join  string
	where []string
	args  []any
}

func buildCustomFilter(
	f SearchFilter, def CustomField, idx int, joinIn string, argsIn []any,
) (customClause, int, error) {
	out := customClause{join: joinIn, args: argsIn}
	switch f.Operator {
	case OpContains:
		if f.Text == nil {
			return out, idx, ErrOperatorMismatch
		}
		canonical := identity.CanonicalQuery(*f.Text)
		alias := fmt.Sprintf("v%d", idx)
		out.join += fmt.Sprintf(" JOIN property_field_value %s ON %s.property_id = p.id AND %s.custom_field_id = $%d",
			alias, alias, alias, idx)
		out.args = append(out.args, def.ID)
		out.where = append(out.where, fmt.Sprintf("%s.text_value LIKE $%d", alias, idx+1))
		out.args = append(out.args, "%"+canonical+"%")
		return out, idx + 2, nil
	case OpEquals:
		if f.ChoiceID == nil {
			return out, idx, ErrOperatorMismatch
		}
		alias := fmt.Sprintf("v%d", idx)
		out.join += fmt.Sprintf(" JOIN property_field_value %s ON %s.property_id = p.id AND %s.custom_field_id = $%d",
			alias, alias, alias, idx)
		out.args = append(out.args, def.ID)
		out.where = append(out.where, fmt.Sprintf("%s.choice_id = $%d", alias, idx+1))
		out.args = append(out.args, *f.ChoiceID)
		return out, idx + 2, nil
	case OpIsTrue:
		alias := fmt.Sprintf("v%d", idx)
		out.join += fmt.Sprintf(" JOIN property_field_value %s ON %s.property_id = p.id AND %s.custom_field_id = $%d",
			alias, alias, alias, idx)
		out.args = append(out.args, def.ID)
		out.where = append(out.where, fmt.Sprintf("%s.bool_value = true", alias))
		return out, idx + 1, nil
	case OpIsFalse:
		alias := fmt.Sprintf("v%d", idx)
		out.join += fmt.Sprintf(" JOIN property_field_value %s ON %s.property_id = p.id AND %s.custom_field_id = $%d",
			alias, alias, alias, idx)
		out.args = append(out.args, def.ID)
		out.where = append(out.where, fmt.Sprintf("(%s.bool_value = false OR %s.bool_value IS NULL)", alias, alias))
		return out, idx + 1, nil
	case OpIncludesAll:
		if len(f.ChoiceIDs) == 0 {
			return out, idx, ErrOperatorMismatch
		}
		// Each choice adds one AND-joined subquery so the property must hold
		// every requested choice for this field.
		next := idx
		out.args = append(out.args, def.ID)
		first := true
		for _, cid := range f.ChoiceIDs {
			if first {
				first = false
				out.where = append(out.where, fmt.Sprintf(`EXISTS (SELECT 1 FROM property_field_multi_value WHERE property_id = p.id AND custom_field_id = $%d AND choice_id = $%d)`, next, next+1))
			} else {
				out.where = append(out.where, fmt.Sprintf(`EXISTS (SELECT 1 FROM property_field_multi_value WHERE property_id = p.id AND custom_field_id = $%d AND choice_id = $%d)`, next, next+1))
			}
			out.args = append(out.args, cid)
			next += 2
		}
		// Move past the field-id param and the choice ids.
		return out, next, nil
	}
	return out, idx, ErrOperatorMismatch
}
