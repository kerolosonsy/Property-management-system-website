-- +goose Up
-- +goose StatementBegin

-- The two lookup tables. Identical shape and identical rules, which is why
-- the Angular panel is written once and instantiated twice. The label_normalized
-- value is written by the application using the same Arabic normaliser as
-- usernames; the database does not redefine the rules (research.md D-007).
-- Length is checked here; character rules are checked in Go (see 0008's note
-- on PostgreSQL's POSIX regex parser and Unicode property classes).

CREATE TABLE property_type (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    label            text        NOT NULL,
    label_normalized text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT property_type_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 60)
);
CREATE UNIQUE INDEX property_type_label_normalized_key ON property_type(label_normalized);

CREATE TABLE area (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    label            text        NOT NULL,
    label_normalized text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT area_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 60)
);
CREATE UNIQUE INDEX area_label_normalized_key ON area(label_normalized);

-- Seed values per FR-026. Property types: تجاري، سكني. Areas: the design
-- source's district list.
INSERT INTO property_type (label, label_normalized) VALUES
    ('تجاري', 'تجاري'),
    ('سكني', 'سكني');

INSERT INTO area (label, label_normalized) VALUES
    ('وسط البلد', 'وسط البلد'),
    ('مصر الجديدة', 'مصر الجديدة'),
    ('المهندسين', 'المهندسين'),
    ('الدقي', 'الدقي'),
    ('الزمالك', 'الزمالك'),
    ('مدينة نصر', 'مدينة نصر'),
    ('المعادي', 'المعادي'),
    ('حلوان', 'حلوان');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS area;
DROP TABLE IF EXISTS property_type;

-- +goose StatementEnd
