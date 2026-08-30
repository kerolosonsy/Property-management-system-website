package properties

import (
	"context"
	"errors"

	pmscrypto "pms/internal/crypto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CustomValueInput is the per-field payload supplied by the caller when saving
// a property. Exactly one of the scalar members is set, matching the field's
// type. Multiselect is the only exception: it carries a set of choices.
type CustomValueInput struct {
	FieldID   uuid.UUID
	FieldType CustomFieldType
	IsSensitive bool

	Text     *string
	Checked  *bool
	ChoiceID *uuid.UUID
	ChoiceIDs []uuid.UUID
}

// SavedValue is what the store reads back. Decryption of sensitive values is
// not the store's job — that happens in the handler, inside the role-checked
// path, so the store never logs, returns in errors, or audits plaintext
// (Constitution VII).
type SavedValue struct {
	FieldID uuid.UUID
	// Plaintext is filled by the handler after the role check has passed.
	Plaintext *string
	Bool      *bool
	ChoiceID  *uuid.UUID
	ChoiceIDs []uuid.UUID
}

// SaveCustomValues writes every per-property value in one transaction alongside
// the property insert/update, so the property, its values, and its audit row
// all commit together. Sensitive values are sealed with the envelope before
// they reach the database; the cipher columns are written, never text_value.
func (s *Store) SaveCustomValues(
	ctx context.Context, tx pgx.Tx,
	env *pmscrypto.Envelope,
	propertyID uuid.UUID,
	inputs []CustomValueInput,
) error {
	for _, v := range inputs {
		if err := s.saveOne(ctx, tx, env, propertyID, v); err != nil {
			return err
		}
	}
	return nil
}

// DeleteCustomValues removes every value for the given property; used by the
// update path so the new value set is the source of truth.
func (s *Store) DeleteCustomValues(ctx context.Context, tx pgx.Tx, propertyID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM property_field_value WHERE property_id = $1`, propertyID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM property_field_multi_value WHERE property_id = $1`, propertyID); err != nil {
		return err
	}
	return nil
}

func (s *Store) saveOne(
	ctx context.Context, tx pgx.Tx,
	env *pmscrypto.Envelope,
	propertyID uuid.UUID,
	in CustomValueInput,
) error {
	// An empty input means "no value for this field on this property".
	// Absence is normal (FR-027d).
	if in.FieldType == "" {
		return nil
	}
	switch in.FieldType {
	case FieldText:
		if in.Text == nil {
			return nil
		}
		if in.IsSensitive {
			if env == nil {
				return errors.New("envelope is required to seal a sensitive value")
			}
			sealed, err := env.Seal([]byte(*in.Text))
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO property_field_value (
					property_id, custom_field_id, cipher_value, cipher_nonce,
					wrapped_dek, wrap_nonce
				) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (property_id, custom_field_id) DO UPDATE
				SET cipher_value = EXCLUDED.cipher_value,
				    cipher_nonce = EXCLUDED.cipher_nonce,
				    wrapped_dek = EXCLUDED.wrapped_dek,
				    wrap_nonce = EXCLUDED.wrap_nonce,
				    text_value = NULL, bool_value = NULL, choice_id = NULL`,
				propertyID, in.FieldID,
				sealed.Ciphertext, sealed.CipherNonce,
				sealed.WrappedDEK, sealed.WrapNonce)
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO property_field_value (property_id, custom_field_id, text_value)
			VALUES ($1, $2, $3)
			ON CONFLICT (property_id, custom_field_id) DO UPDATE
			SET text_value = EXCLUDED.text_value,
			    cipher_value = NULL, cipher_nonce = NULL,
			    wrapped_dek = NULL, wrap_nonce = NULL,
			    bool_value = NULL, choice_id = NULL`,
			propertyID, in.FieldID, *in.Text)
		return err
	case FieldCheckbox:
		if in.Checked == nil {
			return nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO property_field_value (property_id, custom_field_id, bool_value)
			VALUES ($1, $2, $3)
			ON CONFLICT (property_id, custom_field_id) DO UPDATE
			SET bool_value = EXCLUDED.bool_value,
			    text_value = NULL, cipher_value = NULL,
			    cipher_nonce = NULL, wrapped_dek = NULL, wrap_nonce = NULL,
			    choice_id = NULL`,
			propertyID, in.FieldID, *in.Checked)
		return err
	case FieldDropdown:
		if in.ChoiceID == nil {
			return nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO property_field_value (property_id, custom_field_id, choice_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (property_id, custom_field_id) DO UPDATE
			SET choice_id = EXCLUDED.choice_id,
			    text_value = NULL, cipher_value = NULL,
			    cipher_nonce = NULL, wrapped_dek = NULL, wrap_nonce = NULL,
			    bool_value = NULL`,
			propertyID, in.FieldID, *in.ChoiceID)
		return err
	case FieldMultiselect:
		// Replace strategy: delete this field's prior multiselect rows and
		// insert the new set inside the same transaction.
		if _, err := tx.Exec(ctx, `DELETE FROM property_field_multi_value WHERE property_id = $1 AND custom_field_id = $2`, propertyID, in.FieldID); err != nil {
			return err
		}
		for _, cid := range in.ChoiceIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO property_field_multi_value (property_id, custom_field_id, choice_id)
				VALUES ($1, $2, $3)
				ON CONFLICT DO NOTHING`, propertyID, in.FieldID, cid); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("unknown custom field type")
}

// ReadCustomValues returns every stored value for one property. Sensitive
// values stay sealed; the caller decrypts after the role check has passed
// (FR-027s3, Constitution VII). Multiselect values come back as the set of
// choice ids.
func (s *Store) ReadCustomValues(ctx context.Context, tx pgx.Tx, propertyID uuid.UUID) ([]StoredValue, error) {
	rows, err := tx.Query(ctx, `
		SELECT custom_field_id, field_type, is_sensitive,
		       text_value, bool_value, choice_id,
		       cipher_value, cipher_nonce, wrapped_dek, wrap_nonce
		FROM property_field_value v
		JOIN custom_field f ON f.id = v.custom_field_id
		WHERE v.property_id = $1`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredValue
	for rows.Next() {
		var v StoredValue
		var fieldType string
		if err := rows.Scan(&v.FieldID, &fieldType, &v.IsSensitive,
			&v.Text, &v.Bool, &v.ChoiceID,
			&v.CipherValue, &v.CipherNonce, &v.WrappedDEK, &v.WrapNonce); err != nil {
			return nil, err
		}
		v.FieldType = CustomFieldType(fieldType)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	multiRows, err := tx.Query(ctx, `
		SELECT custom_field_id, choice_id
		FROM property_field_multi_value
		WHERE property_id = $1
		ORDER BY custom_field_id, choice_id`, propertyID)
	if err != nil {
		return nil, err
	}
	defer multiRows.Close()
	for multiRows.Next() {
		var fieldID, choiceID uuid.UUID
		if err := multiRows.Scan(&fieldID, &choiceID); err != nil {
			return nil, err
		}
		out = append(out, StoredValue{
			FieldID:   fieldID,
			FieldType: FieldMultiselect,
			IsMultiselect: true,
			ChoiceIDs: []uuid.UUID{choiceID},
		})
	}
	if err := multiRows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// StoredValue is the raw form read from the database. Cipher members are
// filled only when the field is sensitive; the plaintext is never present
// here (the handler decrypts).
type StoredValue struct {
	FieldID      uuid.UUID
	FieldType    CustomFieldType
	IsSensitive  bool
	IsMultiselect bool
	Text         *string
	Bool         *bool
	ChoiceID     *uuid.UUID
	ChoiceIDs    []uuid.UUID
	CipherValue  []byte
	CipherNonce  []byte
	WrappedDEK   []byte
	WrapNonce    []byte
}

// Decrypt returns the plaintext bytes for a sensitive value using the supplied
// envelope. Non-sensitive fields return ("", nil). Constitution VII: the result
// MUST NOT be written to a log, an error body, or an audit row.
func (v StoredValue) Decrypt(env *pmscrypto.Envelope) (string, error) {
	if !v.IsSensitive {
		if v.Text == nil {
			return "", nil
		}
		return *v.Text, nil
	}
	plaintext, err := env.Open(&pmscrypto.Sealed{
		Ciphertext:  v.CipherValue,
		CipherNonce: v.CipherNonce,
		WrappedDEK:  v.WrappedDEK,
		WrapNonce:   v.WrapNonce,
	})
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
