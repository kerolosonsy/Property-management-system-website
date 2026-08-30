-- +goose Up
-- +goose StatementBegin

-- Property table. Two states only (active and archived), no third. The unique
-- index on code_normalized deliberately spans archived rows so a code, once
-- issued, is never reusable — restoring an archived property must never collide
-- with something entered meanwhile. The Arabic-normalised index is the
-- implementation of FR-017b and FR-022d.

CREATE SEQUENCE property_code_seq AS bigint START 1;

CREATE TABLE property (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code             text        NOT NULL,
    code_normalized  text        NOT NULL,
    name             text        NOT NULL,
    name_normalized  text        NOT NULL,
    property_type_id uuid        NOT NULL REFERENCES property_type(id) ON DELETE RESTRICT,
    area_id          uuid        NOT NULL REFERENCES area(id)          ON DELETE RESTRICT,
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    archived_at      timestamptz NULL,
    archived_by      uuid        NULL     REFERENCES account(id) ON DELETE RESTRICT,
    CONSTRAINT property_name_len_chk CHECK (char_length(name) BETWEEN 2 AND 120),
    CONSTRAINT property_code_len_chk CHECK (char_length(code) BETWEEN 1 AND 32),
    CONSTRAINT property_archived_pair_chk
        CHECK ((archived_at IS NULL) = (archived_by IS NULL))
);

CREATE UNIQUE INDEX property_code_normalized_key   ON property(code_normalized);
CREATE        INDEX property_name_normalized_idx   ON property(name_normalized);
CREATE        INDEX property_active_name_idx      ON property(name) WHERE archived_at IS NULL;
CREATE        INDEX property_type_idx             ON property(property_type_id);
CREATE        INDEX property_area_idx             ON property(area_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS property;
DROP SEQUENCE IF EXISTS property_code_seq;

-- +goose StatementEnd
