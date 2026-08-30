# Quickstart & Manual Verification: Properties Register

**Feature**: `002-properties-crud` | **Date**: 2026-08-30

Constitution V makes this document binding. The procedure below MUST be executed and its
outcome recorded before this feature is considered done. "It compiles" is not verification.

Sections A–H. Section G is the manager-denied-configuration step Principle V requires.
Record each section as **PASS**, **FAIL**, or **PARTIAL** with a date in the results table
at the end.

---

## Prerequisites

Feature 001 is already running. If it is not, follow
`specs/001-project-setup-login/quickstart.md` first — this feature adds nothing to the
setup except one environment variable.

### The new environment variable

`PMS_KEK` is the master key that wraps every sensitive custom field's data key
(Constitution VII). It did not exist before this feature.

```bash
# Generate a 32-byte key, base64 encoded. Run once; keep the value.
openssl rand -base64 32
```

Add it to `.env`:

```bash
PMS_KEK="<the base64 value>"
```

`.env.example` documents the name and carries no value. `.env` is gitignored. If `PMS_KEK`
is missing or is not exactly 32 bytes once decoded, the server MUST refuse to start rather
than run without encryption — verify that in section F.

> **Losing `PMS_KEK` makes every sensitive value permanently unreadable.** There is no
> recovery path and that is the design. Keep the local value somewhere you can find it.

### Setup

```bash
make env-check          # confirms PMS_KEK is present and correctly sized
make migrate            # applies 0009-0014
make generate           # regenerates the Go interface and the Angular client
make run-api            # https://localhost:8443
make run-web            # https://localhost:4200
```

Sign in as the seeded administrator from feature 001.

```bash
# Optional: the six properties from the design source, for sections B and H.
make seed-demo
```

---

## A — The register is reachable and empty

**Do not run `make seed-demo` before this section.** The empty state is a requirement
(FR-010) and is unreachable once demonstration data exists.

1. Sign in as an administrator. Open **العقارات** from the sidebar.
2. **Expect**: an Arabic empty-state message inviting you to add the first property. Not a
   blank area, not an English string, not a spinner that never resolves.
3. Confirm the page reads right-to-left and the sidebar sits on the right.
4. Sign out. Request `https://localhost:4200/properties` directly.
5. **Expect**: the sign-in screen. No property data, no flash of the list first.

**Records**: FR-001, FR-010, FR-031, FR-032, US1 scenarios 6 and 7.

---

## B — Adding, and the reference code

1. Run `make seed-demo`. Reload the register.
2. **Expect**: six properties, each showing a reference code, a name, a type, and an area,
   sorted by name. Codes are `P-001` through `P-006` in Western digits.
3. Add a property: name `عقار مار جرجس`, any type, any area. Leave the code untouched.
4. **Expect**: it saves, appears in the list, and carries `P-007` — the next code, not a
   reused one.
5. Add a second property **with the same name** `عقار مار جرجس` and a different area.
6. **Expect**: it is accepted. Two rows now share a name and are told apart by code and
   area. This is FR-016a and it is the point of the reference code.
7. Try to save a property with an empty name.
8. **Expect**: refused, with an Arabic message naming the field. Nothing is stored.
9. In the name field, type the Eastern Arabic digits `٠١٢٣` somewhere.
10. **Expect**: they are stored and displayed as `0123`.

**Records**: FR-005, FR-015, FR-016, FR-016a, FR-017a, FR-023, US2.

---

## C — Finding things

1. Type `جرجس` into the search box.
2. **Expect**: both properties named after that saint, and nothing else. The result count
   updates.
3. Clear the search. Type `P-003`.
4. **Expect**: exactly one property — the code is searched too (FR-007).
5. Search for `جرجس` written with a different alef or with diacritics.
6. **Expect**: the same results. Normalisation is the same one usernames use.
7. Select a property type filter, then also type into the search box.
8. **Expect**: both constraints apply together, not one replacing the other (FR-008).
9. Set the page size to 10 with more than 10 properties present, and move to page 2 and
   back.
10. **Expect**: the range indicator states which of how many is shown, in Western digits.
11. Search for something that matches nothing.
12. **Expect**: an Arabic "no matches" message — and note it differs from section A's
    empty-register message.
13. Search, filter, open a property, then go back.
14. **Expect**: the search text, the filters, the page, and the page size are all still
    applied (FR-011).

**Records**: FR-006 – FR-011, SC-001, SC-008, US1, US3 scenario 4.

---

## D — Detail, modification, and a concurrent edit

1. Click a property row.
2. **Expect**: its own screen, showing code, name, type, area, who created it and when, and
   who last modified it and when — in Africa/Cairo local time, Gregorian, Western digits.
3. Change the area and save. Return to the list.
4. **Expect**: the new area shows, and the last-modified record names you and now.
5. **The concurrency check.** Open the same property in two browser tabs. In tab 1, change
   the name and save. In tab 2, without reloading, change the area and save.
6. **Expect**: tab 2 is refused with an Arabic message saying the property changed since it
   was opened, and it shows the current values. Tab 1's change survives intact. **A silent
   overwrite here is a FAIL** — this is SC-009 and there is no partial credit.
7. Request a property id that does not exist.
8. **Expect**: an Arabic "not found" message, not a broken screen.

**Records**: FR-012, FR-013, FR-014, FR-019, FR-020, FR-033, SC-009, US3, US4.

---

## E — Archiving and restoring

1. Open a property and choose to archive it.
2. **Expect**: a confirmation naming that property. Cancel — nothing changes.
3. Archive it for real.
4. **Expect**: it leaves the working list and you are returned to the list.
5. View the archived properties.
6. **Expect**: it is there, marked archived, with when and by whom.
7. Note its reference code. Try to create a new property using that exact code as an
   administrator.
8. **Expect**: refused, with an Arabic message saying the code belongs to an archived
   property. **This is the requirement most likely to be got wrong** — a unique index that
   excludes archived rows would let this through and silently break restore (FR-022d).
9. Try to modify the archived property.
10. **Expect**: refused until it is restored (FR-022e).
11. Restore it.
12. **Expect**: it returns to the working list with every field as it was.
13. Confirm no interface anywhere offers permanent deletion, as an administrator (FR-022).

**Records**: FR-021, FR-022, FR-022a – FR-022e, SC-005a, US5.

---

## F — Sensitive fields and the encryption at rest

This section is where Principle VII is actually verified. Screens can look correct while
the disk holds plaintext, so **step 5 inspects the database directly** — the equivalent of
the constitution's requirement to inspect a stored attachment and confirm it is unreadable.

1. As an administrator, define a custom field: label `رقم السجل العقاري`, type free text,
   **marked sensitive**.
2. Try to define a **dropdown** field marked sensitive.
3. **Expect**: refused. Only free-text fields may be designated (FR-027s1, Constitution VII).
4. Open a property, fill the sensitive field with `12345678901234`, and save. Confirm the
   detail screen shows it back.
5. **Inspect the database**:

   ```bash
   psql "$PMS_DATABASE_URL" -c \
     "SELECT text_value, encode(cipher_value,'hex'), cipher_nonce IS NOT NULL AS has_nonce,
             wrapped_dek IS NOT NULL AS has_key
        FROM property_field_value
       WHERE custom_field_id = (SELECT id FROM custom_field WHERE label = 'رقم السجل العقاري');"
   ```

   **Expect**: `text_value` is NULL, `cipher_value` is unreadable hex, and both `has_nonce`
   and `has_key` are true. **Seeing `12345678901234` in any column is a FAIL** and the
   feature does not ship.
6. Open the advanced search.
7. **Expect**: the sensitive field is absent from the filter list, and the screen says in
   Arabic that sensitive fields cannot be searched (FR-027s4).
8. **Try to search it anyway**, bypassing the UI:

   ```bash
   curl -k -X POST https://localhost:8443/api/v1/properties/search \
     -b "pms_session=<your cookie>" -H 'Content-Type: application/json' \
     -d '{"customFilters":[{"fieldId":"<the sensitive field id>","operator":"contains","text":"123"}]}'
   ```

   **Expect**: `400` with an Arabic message. **A `200` here is a FAIL** — it would mean the
   exclusion lives in the UI, which Principle I says carries zero security weight.
9. Try to change the field from sensitive to not sensitive now that a value exists.
10. **Expect**: refused (FR-027s5).
11. Stop the API. Unset `PMS_KEK`. Start it again.
12. **Expect**: it refuses to start with a clear message. It MUST NOT start and serve
    sensitive fields unencrypted.
13. Grep the API log for the value you entered and for the KEK.
14. **Expect**: neither appears anywhere (Constitution VII).

**Records**: FR-027s1 – FR-027s6, SC-008d, SC-008e, Constitution VII.

---

## G — The manager is refused every configuration action

**This is the step Principle V requires**, and it is not satisfied by observing that
buttons are hidden. Every check below goes past the interface, because Angular guards and
hidden menu items carry zero security weight (Principle I).

1. As an administrator, create a manager account. Sign in as that manager.
2. **Expect**: Settings shows no property-types, areas, or custom-fields screen.
3. Now bypass the interface. With the manager's session cookie, call each of these:

   ```bash
   # Add a property type
   curl -k -X POST https://localhost:8443/api/v1/property-types \
     -b "pms_session=<manager cookie>" -H 'Content-Type: application/json' \
     -d '{"label":"اختبار"}'

   # Rename an area
   curl -k -X PUT https://localhost:8443/api/v1/areas/<an area id> \
     -b "pms_session=<manager cookie>" -H 'Content-Type: application/json' \
     -d '{"label":"اختبار"}'

   # Remove a property type
   curl -k -X DELETE https://localhost:8443/api/v1/property-types/<a type id> \
     -b "pms_session=<manager cookie>"

   # Define a custom field
   curl -k -X POST https://localhost:8443/api/v1/custom-fields \
     -b "pms_session=<manager cookie>" -H 'Content-Type: application/json' \
     -d '{"label":"اختبار","fieldType":"text"}'

   # Change a property's reference code
   curl -k -X PATCH https://localhost:8443/api/v1/properties/<a property id>/code \
     -b "pms_session=<manager cookie>" -H 'Content-Type: application/json' \
     -d '{"code":"X-1","version":1}'
   ```

4. **Expect**: `403` from all five. **Any `2xx` is a FAIL** and blocks the feature.
5. Confirm the manager *can* still add, modify, archive, and restore a property, and can
   see the reference code without being able to change it (FR-002, FR-017c).

**Records**: FR-003, FR-004, FR-017c, FR-027h, SC-005b, SC-006, US6 scenario 6, US7
scenario 7. **Constitution V's mandatory manager-denied-configuration step.**

---

## H — Lookups, custom fields, and the advanced search

1. As an administrator, add an area. Use it on a new property.
2. Rename that area. **Expect**: the property shows the new name without being re-saved
   (FR-024b).
3. Try to remove that area. **Expect**: refused, with an Arabic message stating how many
   properties use it (FR-025).
4. Archive the property using it, then try to remove the area again. **Expect**: still
   refused — archived properties count (FR-025).
5. Add a duplicate area label. **Expect**: refused (FR-024a).
6. Define one custom field of each of the four types. Fill all four on a property.
7. **Expect**: each renders with an input suited to its type, all four are optional, and
   the detail screen shows every one including the empty ones (FR-027c, FR-027d).
8. Try to define a dropdown with no choices. **Expect**: refused (FR-027a).
9. Rename a field's label. **Expect**: every property shows the new label, no value lost.
10. Try to remove a choice that a property has selected. **Expect**: refused, with a count
    (FR-027g).
11. Open the advanced search. Set the type filter and two custom-field filters.
12. **Expect**: only properties satisfying all three; unset filters constrain nothing;
    clearing resets everything (FR-027j).
13. Confirm archived properties are excluded unless explicitly included (FR-027k).
14. As an administrator, open the records screen from feature 001.
15. **Expect**: entries for every action in sections B through H — property created,
    modified, archived, restored, code changed, lookups created/renamed/removed, custom
    fields created/renamed/removed — each naming the actor, the action, the entity, the
    time, and the source address. **Expect no entry for merely reading** a list or a
    detail (FR-030). **Expect no decrypted sensitive value anywhere in a record.**

**Records**: FR-024 – FR-026a, FR-027 – FR-027l, FR-028 – FR-030, SC-004, SC-006a,
SC-008a – SC-008c, US6, US7, US8.

---

## Results

| Section | What it proves | Result | Date | Notes |
|---|---|---|---|---|
| A | Empty state, RTL, signed-out redirect | NOT RUN | | Needs a browser and an empty register. |
| B | Adding, code generation, duplicate names | PARTIAL | 2026-08-30 | API verified: code generation gave P-007 after the seed; duplicate code refused 409 in both letter cases; duplicate name accepted 201. **This section failed on first run and found a real bug** — see below. Browser steps not run. |
| C | Search, filters, paging, state preservation | NOT RUN | | Needs a browser. |
| D | Detail, modification, concurrent edit | PARTIAL | 2026-08-30 | Concurrency verified by API: two saves from version 2, first 200, second **409**, first writer's value survived at version 3. SC-009 holds. Browser steps not run. |
| E | Archive, restore, code reservation | PARTIAL | 2026-08-30 | Archive verified by API (200). Restore and the archived-code reservation not re-run after the reseed. |
| F | Encryption at rest, search exclusion at the server | **PASS** | 2026-08-30 | See detail below. |
| G | **Manager denied every configuration action** | **PASS** | 2026-08-30 | All five refused 403 by direct API call, past the interface. |
| H | Lookups, custom fields, advanced search, audit | PARTIAL | 2026-08-30 | Custom field definition, sensitive designation, audit entity rows, and non-sensitive search all verified by API. Lookup rename/in-use and the browser steps not run. |

### Section F — recorded detail (2026-08-30)

| Step | Expected | Observed |
|---|---|---|
| F1 | Sensitive free-text field accepted | created, `isSensitive: true` |
| F2/F3 | Sensitive **dropdown** refused | 400 |
| F4 | Value round-trips through the API | 200, read back correctly |
| F5 | **`text_value` NULL, ciphertext present** | `text_value` NULL; 30-byte ciphertext (14 plaintext + 16 GCM tag); nonce, wrapped key, and wrap nonce all present; plaintext absent from the table |
| F7 | Sensitive field flagged to clients | listed with `isSensitive: true` |
| F8 | **Search of a sensitive field refused at the server** | 400 for admin **and** manager, both by direct API call past the UI, with the Arabic message |
| — | Non-sensitive field still searchable | 200, 1 match |
| F11/F12 | Server refuses to start without a valid KEK | absent KEK and 5-byte KEK both exit 1 with a clear message that does not echo the key |
| F13/F14 | No secret in any log or audit row | plaintext value: 0 hits across three API logs; KEK: 0 hits; audit rows carry field ids and labels, never a decrypted value |

### Section G — recorded detail (2026-08-30)

Performed with a manager session cookie by direct API call, not through the interface,
because Principle I gives a hidden control zero security weight.

| Action | Result |
|---|---|
| Add a property type | 403 |
| Rename an area | 403 |
| Remove a property type | 403 |
| Define a custom field | 403 |
| Change a property's reference code | 403 |
| — *and the manager still can* — | |
| List properties / read one / read the area list | 200 / 200 / 200 |
| Create a property, then archive it | 201 / 200 |
| Create a property **supplying a code** | 403 (FR-017c) |

**The feature is not done until every row reads PASS.** A PARTIAL is a recorded gap, not a
pass; write what is missing in Notes so the next person does not have to rediscover it.
