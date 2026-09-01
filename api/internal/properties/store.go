package properties

import (
	"context"
	"errors"
	"strings"
	"time"

	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDuplicateCode means the supplied reference code already exists, active or
// archived. The unique index on code_normalized spans archived rows (data-model
// 0012) so this error fires for either state.
var ErrDuplicateCode = errors.New("property code already in use")

// ErrArchived means the property is archived and the operation is not allowed.
var ErrArchived = errors.New("property is archived")

// ErrNotFound means no property has that id.
var ErrNotFound = errors.New("property not found")

// ErrVersionConflict means the property was changed between read and write.
var ErrVersionConflict = errors.New("property version conflict")

// Store holds the dependencies every property-domain call site shares. It
// does not own a connection pool of its own; methods accept either a pgx.Tx
// (writes — so the change and its audit row commit together) or a pgxpool.Pool
// (queries that don't need to share a transaction).
type Store struct {
	Pool interface {
		// intentionally empty: the field is set by the server, and the
		// methods here take a pgx.Tx or pgxpool.Pool directly so the store
		// is not bound to a specific connection type.
	}
}

// Property is the in-memory representation of one row. Custom values are
// loaded separately through custom_value_store.go.
type Property struct {
	ID             uuid.UUID
	Code           string
	CodeNormalized string
	Name           string
	PropertyTypeID uuid.UUID
	AreaID         uuid.UUID
	Version        int
	CreatedAt      time.Time
	CreatedBy      uuid.UUID
	CreatedByName  string
	UpdatedAt      time.Time
	UpdatedBy      uuid.UUID
	UpdatedByName  string
	ArchivedAt     *time.Time
	ArchivedBy     *uuid.UUID
	ArchivedByName *string
	ArchiveNote    *string

	PropertyTypeLabel  string
	AreaLabel          string
	MatchedAttachments []MatchedAttachment
}

type MatchedAttachment struct {
	ID          uuid.UUID
	Description string
}

// Lookup is one row from property_type or area.
type Lookup struct {
	ID    uuid.UUID
	Label string
}

// normalizeName trims surrounding whitespace and collapses inner runs of
// whitespace into a single space. Arabic and Latin letters are kept verbatim
// so the typist sees what they typed.
func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace && b.Len() > 0 {
				b.WriteRune(' ')
				inSpace = true
			}
			continue
		}
		b.WriteRune(r)
		inSpace = false
	}
	return strings.TrimRight(b.String(), " ")
}

// ListFilters are the inputs to ListProperties.
type ListFilters struct {
	Q               string
	Code            string // per-column filter: reference code only
	Name            string // per-column filter: name only
	PropertyTypeID  *uuid.UUID
	AreaID          *uuid.UUID
	IncludeArchived bool
	Page            int
	PageSize        int
}

// ListProperties returns a page of properties plus the total count, applying
// the search and filters after Arabic normalisation.
func (s *Store) ListProperties(ctx context.Context, tx pgx.Tx, f ListFilters) ([]Property, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 25
	}

	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if !f.IncludeArchived {
		where = append(where, "p.archived_at IS NULL")
	}

	q := strings.TrimSpace(f.Q)
	if q != "" {
		// Reuse the same Arabic normaliser as usernames. The application
		// always stores the canonical form in name_normalized, so a match
		// here is the exact equivalent of "match by name or code in any of
		// the variants FR-007 lists".
		canonical := identity.CanonicalQuery(q)
		where = append(where, "(p.name_normalized LIKE $"+itoa(idx)+" OR p.code_normalized LIKE $"+itoa(idx)+")")
		args = append(args, "%"+canonical+"%")
		idx++
	}
	// Per-column filters. Each narrows a single column, and combines with the
	// header search and with every other filter by AND. Both go through the
	// same normaliser as the stored column, so a column filter matches the
	// same Arabic variants the header search does.
	if code := strings.TrimSpace(f.Code); code != "" {
		where = append(where, "p.code_normalized LIKE $"+itoa(idx))
		args = append(args, "%"+identity.CanonicalQuery(code)+"%")
		idx++
	}
	if name := strings.TrimSpace(f.Name); name != "" {
		where = append(where, "p.name_normalized LIKE $"+itoa(idx))
		args = append(args, "%"+identity.CanonicalQuery(name)+"%")
		idx++
	}
	if f.PropertyTypeID != nil {
		where = append(where, "p.property_type_id = $"+itoa(idx))
		args = append(args, *f.PropertyTypeID)
		idx++
	}
	if f.AreaID != nil {
		where = append(where, "p.area_id = $"+itoa(idx))
		args = append(args, *f.AreaID)
		idx++
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM property p WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.code, p.name, p.property_type_id, p.area_id,
		       p.version, p.created_at, p.created_by, cu.username,
		       p.updated_at, p.updated_by, uu.username,
		       p.archived_at, p.archived_by, au.username,
		       p.archive_note, pt.label, a.label
		FROM property p
		JOIN property_type pt ON pt.id = p.property_type_id
		JOIN area          a  ON a.id  = p.area_id
		JOIN account cu ON cu.id = p.created_by
		JOIN account uu ON uu.id = p.updated_by
		LEFT JOIN account au ON au.id = p.archived_by
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
			&p.ArchiveNote, &p.PropertyTypeLabel, &p.AreaLabel); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CreateProperty inserts a property row and returns its identity. The caller
// supplies the reference code or "" to have the generator pick the next.
// Property row insert and audit row insert happen in the same transaction the
// caller controls (Constitution VIII).
func (s *Store) CreateProperty(
	ctx context.Context, tx pgx.Tx,
	name string, nameNormalized string,
	code string, codeNormalized string,
	propertyTypeID, areaID uuid.UUID,
	createdBy uuid.UUID,
) (*Property, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO property (
			code, code_normalized, name, name_normalized,
			property_type_id, area_id, version,
			created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $7)
		RETURNING id, code, name, property_type_id, area_id, version,
		          created_at, updated_at, created_by, updated_by,
		          archived_at, archived_by`,
		code, codeNormalized, name, nameNormalized,
		propertyTypeID, areaID, createdBy,
	)
	p := &Property{}
	if err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PropertyTypeID, &p.AreaID, &p.Version,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
		&p.ArchivedAt, &p.ArchivedBy); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateCode
		}
		return nil, err
	}
	return p, nil
}

// FindByID returns a property including creator/modifier/archiver usernames.
// Archived properties are returned, never hidden (FR-022a).
func (s *Store) FindByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Property, error) {
	row := tx.QueryRow(ctx, `
		SELECT p.id, p.code, p.name, p.property_type_id, p.area_id,
		       p.version, p.created_at, p.created_by, cu.username,
		       p.updated_at, p.updated_by, uu.username,
		       p.archived_at, p.archived_by, au.username,
		       p.archive_note, pt.label, a.label
		FROM property p
		JOIN property_type pt ON pt.id = p.property_type_id
		JOIN area          a  ON a.id  = p.area_id
		JOIN account cu ON cu.id = p.created_by
		JOIN account uu ON uu.id = p.updated_by
		LEFT JOIN account au ON au.id = p.archived_by
		WHERE p.id = $1`, id)
	p := &Property{}
	if err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PropertyTypeID, &p.AreaID,
		&p.Version, &p.CreatedAt, &p.CreatedBy, &p.CreatedByName,
		&p.UpdatedAt, &p.UpdatedBy, &p.UpdatedByName,
		&p.ArchivedAt, &p.ArchivedBy, &p.ArchivedByName,
		&p.ArchiveNote, &p.PropertyTypeLabel, &p.AreaLabel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// UpdateProperty applies the change and returns the new version. The expected
// version is compared-and-incremented in one SQL statement; an archived
// property is refused with ErrArchived.
func (s *Store) UpdateProperty(
	ctx context.Context, tx pgx.Tx,
	id uuid.UUID,
	name, nameNormalized string,
	propertyTypeID, areaID uuid.UUID,
	updatedBy uuid.UUID,
	expectedVersion int,
) (*Property, error) {
	row := tx.QueryRow(ctx, `
		UPDATE property
		SET name = $2,
		    name_normalized = $3,
		    property_type_id = $4,
		    area_id = $5,
		    version = version + 1,
		    updated_at = now(),
		    updated_by = $6
		WHERE id = $1 AND version = $7 AND archived_at IS NULL
		RETURNING id, code, name, property_type_id, area_id, version,
		          created_at, updated_at, created_by, updated_by,
		          archived_at, archived_by, archive_note`,
		id, name, nameNormalized, propertyTypeID, areaID, updatedBy, expectedVersion,
	)
	p := &Property{}
	if err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PropertyTypeID, &p.AreaID, &p.Version,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
		&p.ArchivedAt, &p.ArchivedBy, &p.ArchiveNote); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish archived from version conflict from missing.
			if cur, qErr := s.FindByID(ctx, tx, id); qErr == nil && cur != nil && cur.ArchivedAt != nil {
				return nil, ErrArchived
			} else if qErr != nil {
				return nil, qErr
			} else {
				return nil, ErrVersionConflict
			}
		}
		return nil, err
	}
	return p, nil
}

// ChangeCode updates a property's reference code. Refuses a duplicate code
// (ErrDuplicateCode) and an archived property (ErrArchived). Returns the old
// and new codes so the audit row can record both (FR-017e).
func (s *Store) ChangeCode(
	ctx context.Context, tx pgx.Tx,
	id uuid.UUID, newCode, newNormalized string,
	updatedBy uuid.UUID, expectedVersion int,
) (oldCode string, p *Property, err error) {
	row := tx.QueryRow(ctx, `
		UPDATE property
		SET code = $2,
		    code_normalized = $3,
		    version = version + 1,
		    updated_at = now(),
		    updated_by = $4
		WHERE id = $1 AND version = $5 AND archived_at IS NULL
		RETURNING code, id, name, property_type_id, area_id, version,
		          created_at, updated_at, created_by, updated_by,
		          archived_at, archived_by, archive_note`,
		id, newCode, newNormalized, updatedBy, expectedVersion,
	)
	p = &Property{}
	if err = row.Scan(&oldCode, &p.ID, &p.Name, &p.PropertyTypeID, &p.AreaID, &p.Version,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
		&p.ArchivedAt, &p.ArchivedBy, &p.ArchiveNote); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if cur, qErr := s.FindByID(ctx, tx, id); qErr == nil && cur != nil && cur.ArchivedAt != nil {
				err = ErrArchived
				return
			} else if qErr != nil {
				err = qErr
				return
			} else {
				err = ErrVersionConflict
				return
			}
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			err = ErrDuplicateCode
			return
		}
		return
	}
	return
}

// Archive sets the archived pair, the optional note, and increments version.
// Refuses an already archived property with ErrArchived and a stale version
// with ErrVersionConflict. An empty note is stored as NULL.
func (s *Store) Archive(
	ctx context.Context, tx pgx.Tx,
	id uuid.UUID, actorID uuid.UUID, expectedVersion int,
	note *string,
) (*Property, error) {
	row := tx.QueryRow(ctx, `
		UPDATE property
		SET archived_at = now(),
		    archived_by = $2,
		    archive_note = $3,
		    version = version + 1,
		    updated_at = now(),
		    updated_by = $2
		WHERE id = $1 AND version = $4 AND archived_at IS NULL
		RETURNING id, code, name, property_type_id, area_id, version,
		          created_at, updated_at, created_by, updated_by,
		          archived_at, archived_by, archive_note`,
		id, actorID, note, expectedVersion,
	)
	p := &Property{}
	if err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PropertyTypeID, &p.AreaID, &p.Version,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
		&p.ArchivedAt, &p.ArchivedBy, &p.ArchiveNote); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if cur, qErr := s.FindByID(ctx, tx, id); qErr == nil && cur != nil && cur.ArchivedAt != nil {
				return nil, ErrArchived
			} else if qErr != nil {
				return nil, qErr
			} else {
				return nil, ErrVersionConflict
			}
		}
		return nil, err
	}
	return p, nil
}

// Restore clears the archived pair and the archive note, and increments
// version. Refuses an already-active property with ErrArchived and a stale
// version with ErrVersionConflict.
func (s *Store) Restore(
	ctx context.Context, tx pgx.Tx,
	id uuid.UUID, actorID uuid.UUID, expectedVersion int,
) (*Property, error) {
	row := tx.QueryRow(ctx, `
		UPDATE property
		SET archived_at = NULL,
		    archived_by = NULL,
		    archive_note = NULL,
		    version = version + 1,
		    updated_at = now(),
		    updated_by = $2
		WHERE id = $1 AND version = $3 AND archived_at IS NOT NULL
		RETURNING id, code, name, property_type_id, area_id, version,
		          created_at, updated_at, created_by, updated_by,
		          archived_at, archived_by, archive_note`,
		id, actorID, expectedVersion,
	)
	p := &Property{}
	if err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PropertyTypeID, &p.AreaID, &p.Version,
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
		&p.ArchivedAt, &p.ArchivedBy, &p.ArchiveNote); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if cur, qErr := s.FindByID(ctx, tx, id); qErr == nil && cur != nil && cur.ArchivedAt == nil {
				return nil, ErrArchived
			} else if qErr != nil {
				return nil, qErr
			} else {
				return nil, ErrVersionConflict
			}
		}
		return nil, err
	}
	return p, nil
}

// CodeExists returns true if any property — active or archived — already holds
// the given normalized code.
func (s *Store) CodeExists(ctx context.Context, tx pgx.Tx, codeNormalized string) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM property WHERE code_normalized = $1)`, codeNormalized).Scan(&found)
	return found, err
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
