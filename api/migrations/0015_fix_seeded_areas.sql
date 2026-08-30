-- +goose Up
-- +goose StatementBegin

-- Migration 0011 seeded Cairo districts into `area`; the design source
-- is an Assiut diocese (research.md D-011) and the brief requires the
-- Assiut names. Forward-only: rename in place, do not delete-and-reinsert,
-- because each seed-demo property holds a foreign key into the existing
-- rows. New label_normalized values are the same output the Go
-- normaliser produces (NFKC, strip diacritics and tatweel, unify
-- alef/ya/ta-marbuta, case-fold). The migration matches by the values
-- migration 0011 actually inserted — those are un-normalised literal
-- strings, so the WHERE clause here is un-normalised too. Future area
-- rows will carry the canonical form because every Go path routes
-- through identity.Canonical before INSERT/UPDATE.

-- Old: وسط البلد                  → New: وسط البلد، أسيوط
UPDATE area SET label = 'وسط البلد، أسيوط', label_normalized = 'وسط البلد، اسيوط'
    WHERE label = 'وسط البلد';

-- Old: مصر الجديدة                → New: حي الحمراء
UPDATE area SET label = 'حي الحمراء', label_normalized = 'حي الحمراء'
    WHERE label = 'مصر الجديدة';

-- Old: المهندسين                  → New: شارع الجمهورية
UPDATE area SET label = 'شارع الجمهورية', label_normalized = 'شارع الجمهوريه'
    WHERE label = 'المهندسين';

-- Old: الدقي                       → New: أرض الشهيد
UPDATE area SET label = 'أرض الشهيد', label_normalized = 'ارض الشهيد'
    WHERE label = 'الدقي';

-- Old: الزمالك                    → New: كورنيش النيل
UPDATE area SET label = 'كورنيش النيل', label_normalized = 'كورنيش النيل'
    WHERE label = 'الزمالك';

-- Old: مدينة نصر                  → New: حي بني عدي
UPDATE area SET label = 'حي بني عدي', label_normalized = 'حي بني عدي'
    WHERE label = 'مدينة نصر';

-- Remove the two original seeded areas that no seed-demo property uses
-- (المعادي, حلوان). Guard the DELETE with NOT EXISTS so a property added
-- later, referencing the area, keeps it in place — the migration never
-- destroys a row that is still in use.
DELETE FROM area WHERE label = 'المعادي'
    AND NOT EXISTS (SELECT 1 FROM property WHERE area_id = area.id);
DELETE FROM area WHERE label = 'حلوان'
    AND NOT EXISTS (SELECT 1 FROM property WHERE area_id = area.id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reverting the rename is lossy: original rows are gone if the area was
-- referenced only by the seed data and we have no record of which old
-- name belonged to which new name. The Down migration restores the
-- original label values where the source row still exists, and reinserts
-- the two removed areas if no row by that name has appeared in the
-- meantime.
UPDATE area SET label = 'وسط البلد', label_normalized = 'وسط البلد'
    WHERE label = 'وسط البلد، أسيوط';
UPDATE area SET label = 'مصر الجديدة', label_normalized = 'مصر الجديدة'
    WHERE label = 'حي الحمراء';
UPDATE area SET label = 'المهندسين', label_normalized = 'المهندسين'
    WHERE label = 'شارع الجمهورية';
UPDATE area SET label = 'الدقي', label_normalized = 'الدقي'
    WHERE label = 'أرض الشهيد';
UPDATE area SET label = 'الزمالك', label_normalized = 'الزمالك'
    WHERE label = 'كورنيش النيل';
UPDATE area SET label = 'مدينة نصر', label_normalized = 'مدينة نصر'
    WHERE label = 'حي بني عدي';

INSERT INTO area (label, label_normalized)
    SELECT 'المعادي', 'المعادي'
    WHERE NOT EXISTS (SELECT 1 FROM area WHERE label = 'المعادي');
INSERT INTO area (label, label_normalized)
    SELECT 'حلوان', 'حلوان'
    WHERE NOT EXISTS (SELECT 1 FROM area WHERE label = 'حلوان');

-- +goose StatementEnd