# Quickstart & Manual Verification: Property Attachments

**Feature**: `003-property-attachments-ocr` | **Date**: 2026-08-31

Constitution V makes this document binding: the procedure MUST be executed and its outcome
recorded before the feature is done. Sections A–H. **Section F is where Principle VII is
actually proven**, and **Section G is the role boundary this feature has** in place of the
manager-denied-configuration step, because there is no configuration surface here.

---

## Prerequisites

Features 001 and 002 are running. This feature adds two system tools and two configuration
values.

### System tools — neither is fully satisfied on this machine today

```bash
# Arabic recognition. `tesseract` is present at 5.5.2 but ONLY the `eng` pack is
# installed, so Arabic scans will not be read until this is done.
brew install tesseract-lang          # adds ara, among others
tesseract --list-langs | grep -x ara # MUST print: ara

# PDF text layer and page rasterising. Currently absent entirely.
brew install poppler
command -v pdftotext pdftoppm pdfinfo   # MUST print all three paths
```

**The system runs without either.** Uploads, downloads, viewing, descriptions, and removal
all work; documents whose family needs a missing tool are marked **not eligible** with an
Arabic reason. That is by design (research.md D-012) — verify it in section E.

### Configuration

```bash
PMS_ATTACHMENT_STORE=/absolute/path/outside/the/repo   # encrypted bodies live here
PMS_ATTACHMENT_MAX_BYTES=52428800                      # 50 MB, the FR-005 default
PMS_EXTRACT_TEXT_MAX_BYTES=262144                      # 256 KB, the FR-021 default
```

`PMS_KEK` from feature 002 is reused unchanged. The store path MUST be outside the working
tree and MUST be gitignored wherever it is placed.

```bash
make env-check && make migrate && make generate && make run-api && make run-web
```

---

## A — Attaching, and what lands on disk

1. Open a property. Attach a PDF with the description `عقد الملكية`.
2. **Expect**: it appears in the attachment list with that description, your name, the date,
   and its size in Western digits. Extraction shows as pending.
3. Try to attach with an empty description. **Expect**: refused in Arabic (FR-002).
4. Try to attach a `.zip`. **Expect**: refused, listing the accepted types.
5. **Rename an executable to `deed.pdf` and attach it.**

   ```bash
   cp /bin/echo /tmp/deed.pdf
   ```

   **Expect**: refused. The type is judged from content, not from the name (FR-004a). **An
   acceptance here is a FAIL** — it means the extension is being trusted.
6. Attach a file larger than `PMS_ATTACHMENT_MAX_BYTES`. **Expect**: refused with the
   configured limit named, and nothing written to the store.
7. Lower `PMS_ATTACHMENT_MAX_BYTES`, restart, and try again. **Expect**: the refusal states
   the *new* value (FR-005a).

**Records**: FR-001 – FR-005a, SC-006a.

---

## B — Reading it back

1. Download the attachment. **Expect**: byte-identical to what you uploaded.

   ```bash
   diff original.pdf downloaded.pdf && echo IDENTICAL
   ```
2. View a PDF and an image inline. **Expect**: shown in the application without a download.
3. Open a `.docx`. **Expect**: it downloads rather than rendering (FR-013c).
4. Check the response headers on both a view and a download.

   ```bash
   curl -ski -b "$COOKIE" "$API/attachments/$ID/content?disposition=inline" | head -12
   ```

   **Expect**: `Cache-Control: no-store`. **Its absence is a FAIL** — the browser would
   write a decrypted document to disk (research.md D-009).

**Records**: FR-013 – FR-013c, SC-008.

---

## C — Descriptions, sensitivity, and removal

1. Edit a description. **Expect**: the list shows the new one.
2. Mark an ordinary attachment sensitive. **Expect**: accepted; its text leaves search
   immediately (verify in F).
3. **Try to make it non-sensitive again.** **Expect**: refused in Arabic (FR-022c).
4. Try the same past the interface:

   ```bash
   curl -sk -X PATCH -b "$COOKIE" -H 'Content-Type: application/json' \
     -d '{"isSensitive":false}' "$API/attachments/$ID"
   ```

   **Expect**: `409`. **A `200` is a FAIL.**
5. Try it directly in the database, which the trigger must also refuse:

   ```bash
   docker exec pms_postgres psql -U pms_owner -d pms \
     -c "UPDATE attachment SET is_sensitive=false WHERE id='$ID';"
   ```

   **Expect**: `ERROR: an attachment cannot be made non-sensitive`.
6. Remove an attachment after confirming. **Expect**: it leaves the list, its file leaves
   the store, and its extracted text is gone. The audit record of it remains.
7. Archive the property, then try to attach, edit, and remove. **Expect**: all refused
   (FR-006).

**Records**: FR-006 – FR-009, FR-022c.

---

## D — Text comes out of the documents

1. Attach a `.docx` containing the phrase `قطعة رقم ١٢٣`.
2. **Expect**: state becomes `done` without recognition being involved — OOXML is read
   directly (research.md D-005). The text contains the phrase.
3. Attach a scanned image with legible Arabic. **Expect**: `done`, with recognisable text.
4. Attach a photograph of a building. **Expect**: `empty` — a normal outcome, not an error.
5. Attach a `.doc` (legacy). **Expect**: `not_eligible`, and the file still downloads.
6. Attach a PDF that mixes a generated page with a scanned page. **Expect**: text from both,
   the first read exactly and the second recognised.
7. Confirm every one of the above is still downloadable, whatever its extraction state
   (FR-020).

**Records**: FR-017 – FR-021c, SC-005.

---

## E — Retries, corrections, and missing tools

1. **Before installing poppler**, attach a scanned PDF. **Expect**: `not_eligible` with an
   Arabic reason naming the missing capability — not `failed`, and the upload still succeeds
   (research.md D-012).
2. Install poppler, restart, press **try again**. **Expect**: it now extracts.
3. Force a failure (rename `tesseract` away, restart, attach an image). **Expect**: three
   automatic attempts at roughly 30 s, 2 min, 10 min, then `failed`. Confirm in the database
   that `extract_attempts` stops at 3:

   ```bash
   docker exec pms_postgres psql -U pms_owner -d pms \
     -c "SELECT extract_state, extract_attempts, extract_next_at FROM attachment WHERE id='$ID';"
   ```
4. Press **try again** on the failed attachment. **Expect**: it returns to `pending`.
5. Correct an attachment's text and save. **Expect**: marked corrected, with your name.
6. **Press try again on the corrected attachment.** **Expect**: refused in Arabic saying the
   correction would be lost (FR-020c). **Silently re-extracting over it is a FAIL.**
7. Restart the API while an extraction is pending. **Expect**: it is picked up again rather
   than stranded pending.

**Records**: FR-020a – FR-020e, FR-024 – FR-024b, SC-005a, SC-005b.

---

## F — Encryption, and the search exclusion

This is where Principle VII is proven. Screens can look right while the disk holds
plaintext, so these steps read the store and the database directly.

1. **Inspect a stored file.**

   ```bash
   find "$PMS_ATTACHMENT_STORE" -type f | head -3
   file "$(find "$PMS_ATTACHMENT_STORE" -type f | head -1)"
   head -c 64 "$(find "$PMS_ATTACHMENT_STORE" -type f | head -1)" | xxd | head -4
   ```

   **Expect**: opaque names with no extension, sharded two levels; `file` cannot identify
   the type; the bytes are not a PDF or image header. **Recognisable content is a FAIL.**
2. **Confirm no plaintext temp file is ever written.** With an extraction running:

   ```bash
   ps -o command= -p "$(pgrep -f 'tesseract|pdftoppm|pdftotext' | head -1)"
   ```

   **Expect**: the command line uses `-` for input and, for `tesseract`/`pdftotext`, `-`
   for output; `pdftoppm` must carry `-singlefile` and **no output root at all**. **A path
   into the store or `/tmp` on that command line is a FAIL**, and so is a bare trailing `-`
   on `pdftoppm`, which poppler reads as a filename prefix and writes to disk
   (research.md D-004).

2a. **Confirm extraction leaves no file behind.** Run an extraction and check the server's
   working directory:

   ```bash
   ls -la "$(lsof -p "$(pgrep -f 'pms.*server' | head -1)" -a -d cwd -Fn 2>/dev/null | tail -1 | cut -c2-)" | grep -E '\.png$|^-' | head
   ```

   **Expect**: no `.png` and no file named `-.png`. Any is a FAIL.
3. **Confirm no derived copies.** View a PDF inline ten times, then:

   ```bash
   find "$PMS_ATTACHMENT_STORE" -type f | wc -l
   ```

   **Expect**: exactly one file per attachment, unchanged. **Any thumbnail or rendered page
   is a FAIL** (SC-008a).
4. **Tamper with a stored file** and download it:

   ```bash
   printf 'x' | dd of="$(find "$PMS_ATTACHMENT_STORE" -type f | head -1)" bs=1 seek=200 conv=notrunc
   ```

   **Expect**: `422`, and **zero bytes of content** — not a partial or corrupted file
   (FR-014, SC-009). Restore from a copy afterwards.
5. **A sensitive attachment's text must be ciphertext:**

   ```bash
   docker exec pms_postgres psql -U pms_owner -d pms -c \
     "SELECT body IS NULL AS body_null, body_normalized IS NULL AS norm_null,
             encode(cipher_body,'hex') <> '' AS has_cipher
        FROM attachment_text WHERE attachment_id='$SENSITIVE_ID';"
   ```

   **Expect**: `t | t | t`. **A readable `body` is a FAIL**, and `body_normalized` being
   non-null would mean it is still searchable.
6. **Search for a phrase that exists only in the sensitive document.**

   ```bash
   curl -sk -X POST -b "$ADMIN_COOKIE" -H 'Content-Type: application/json' \
     -d '{"q":"<the phrase>"}' "$API/attachments/search"
   ```

   **Expect**: no hits, **as an administrator**, and `sensitiveExcluded: true` in the
   response. Repeat as a manager: same result.
7. Search for a phrase in an ordinary document. **Expect**: the property is returned, naming
   the matching attachment.
8. **Check for leaks:**

   ```bash
   grep -c "<phrase from the sensitive doc>" api.log     # expect 0
   grep -Fc "$PMS_KEK" api.log                           # expect 0
   docker exec pms_postgres psql -U pms_owner -d pms -tAc \
     "SELECT count(*) FROM audit_log WHERE detail::text LIKE '%<phrase>%';"   # expect 0
   ```

**Records**: FR-010 – FR-016, FR-022 – FR-022d, FR-029, SC-002, SC-003, SC-008a, SC-009,
SC-011 – SC-013.

---

## G — The role boundary

This feature has **no configuration operations**, so Constitution V's manager-denied-
configuration step does not apply literally. The equivalent boundary is verified instead:
the rules that bind *everyone*, checked past the interface.

1. As a **manager**, confirm you can attach, view, download, describe, correct text, and
   remove. **Expect**: all succeed — attachments are operational data (FR-031).
2. As a **manager**, try to demote a sensitive attachment by direct API call. **Expect**:
   `409`.
3. As an **administrator**, try the same. **Expect**: `409`. **Administrators are not
   exempt.**
4. As an **administrator**, search for the sensitive phrase. **Expect**: no hits.
5. **Signed out**, request an attachment's content directly by its address. **Expect**:
   refused, no bytes.
6. Confirm nothing serves the store directory over HTTP:

   ```bash
   curl -sk -o /dev/null -w '%{http_code}\n' "https://localhost:8443/attachments/"
   curl -sk -o /dev/null -w '%{http_code}\n' "https://localhost:4200/attachments/"
   ```

   **Expect**: 404 from both. **A 200 or a directory listing is a FAIL.**

**Records**: FR-013, FR-022c, FR-029 – FR-031, SC-005b, SC-008.

---

## H — Audit and search behaviour

1. Perform one of each: attach, view inline, download, edit description, correct text,
   request re-extraction, remove.
2. Open the records screen as an administrator. **Expect**: an entry for every one,
   including **both the view and the download** — viewing is reading (FR-013d).
3. **Expect no entry** for merely listing a property's attachments.
4. **Expect no decrypted text or file content** in any record (FR-033).
5. Search a phrase written with different alef or ya variants. **Expect**: the same hits, as
   for property names (FR-027).
6. Archive a property with attachments and search. **Expect**: excluded unless archived are
   explicitly included (FR-028).
7. Attach a very long document. **Expect**: text is capped, the attachment is marked
   truncated, and a phrase past the cut is not found — which the truncation flag explains.

**Records**: FR-025 – FR-034, SC-004, SC-007, SC-015, SC-016.

---

## Results

| Section | What it proves | Result | Date | Notes |
|---|---|---|---|---|
| A | Upload, type-by-content, configured size limit | | | |
| B | Round trip, inline viewing, no browser caching | | | |
| C | Description, one-way sensitivity, removal, archived refusal | | | |
| D | The three extraction families | | | |
| E | Bounded retry, correction, graceful degradation | | | |
| F | **Encryption at rest, no temp files, no derived copies, search exclusion** | | | |
| G | **The role boundary, including that admins are not exempt** | | | |
| H | Audit completeness, search behaviour | | | |

**The feature is not done until every row reads PASS.** A PARTIAL is a recorded gap, not a
pass; write what is missing so the next person does not have to rediscover it.
