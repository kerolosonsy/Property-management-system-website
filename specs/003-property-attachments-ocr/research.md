# Phase 0 Research: Property Attachments with Extracted Text

**Feature**: `003-property-attachments-ocr` | **Date**: 2026-08-31

Twelve decisions. Each states what was chosen, why, and what was rejected. Where the
constitution forces a decision, the principle is named.

---

## D-001 — Chunked AES-256-GCM for file bodies

**Decision**: Encrypt the body in fixed 64 KiB plaintext chunks. One data key per file,
wrapped by the KEK exactly as feature 002 wraps a value's key. Each chunk gets its own
12-byte nonce, formed from a per-file random 8-byte prefix plus a 4-byte big-endian chunk
counter. Each chunk's additional authenticated data is the chunk index and a flag marking
the final chunk. The file header stores the wrapped key, the nonce prefix, the chunk size,
and the plaintext length.

**Rationale**: GCM authenticates one message. A 50 MB file as one message needs the whole
thing buffered on the way in and again on the way out, and a viewer cannot show page one
until the last byte is verified. Chunking gives bounded memory and streaming in both
directions.

Binding the chunk index into the additional data is the part that matters and the part that
is easy to omit. Without it an attacker with write access to the store can reorder chunks,
drop the tail, or splice chunks between two files, and every individual chunk still
verifies. The final-chunk flag is what makes truncation detectable: a file cut short ends
on a chunk that does not claim to be last.

**Alternatives considered**:

- *Reuse feature 002's single-shot `Seal`/`Open`* — rejected. It is already written and
  tested, which makes it genuinely tempting. It costs two 50 MB buffers per concurrent
  upload and makes streaming impossible. Recorded in the plan's Complexity Tracking.
- *AES-CTR with a single HMAC over the file* — rejected. Streaming decryption cannot verify
  the MAC until the end, so either bytes are served before they are authenticated —
  violating "no unauthenticated content" in Principle VII — or the streaming benefit is
  lost anyway.
- *Larger chunks, 1 MiB* — rejected. Fewer tags, but 1 MiB of buffering per concurrent
  stream for no benefit at this file size.

---

## D-002 — One data key per file, not per chunk

**Decision**: A fresh 256-bit data key per attachment, wrapped once under the KEK. Every
chunk of that file uses the same data key with a distinct nonce.

**Rationale**: Nonce uniqueness under one key is what GCM requires, and the prefix-plus-
counter construction guarantees it within a file. Per-chunk keys would mean 800 wrapped
keys for a 50 MB file, all protecting the same document, with no gain: an attacker who
recovers the file key has the file either way.

**Alternatives considered**:

- *One key per chunk* — rejected as above.
- *A key derived from the KEK and the attachment id* — rejected. It removes the wrapped-key
  column, and makes key rotation require re-encrypting every file. Principle VII says
  rotation re-wraps and does not re-encrypt.

---

## D-003 — Content type from bytes, never from the filename

**Decision**: Sniff the leading bytes. Use `http.DetectContentType` from the standard
library as the first pass, then explicit magic-number checks that it does not make:
`50 4B 03 04` plus an `[Content_Types].xml` member for OOXML, `D0 CF 11 E0 A1 B1 1A E1` for
legacy OLE Office, `25 50 44 46` for PDF, and the image signatures. The declared filename
extension is metadata only and never decides anything.

**Rationale**: FR-004a. An executable renamed `deed.pdf` must be refused, and it is refused
only if the decision comes from content. `DetectContentType` alone reports OOXML as
`application/zip`, so the ZIP central directory has to be opened far enough to see whether
it is really a document.

**Alternatives considered**:

- *Trust the extension* — rejected outright by FR-004a.
- *Trust the browser's `Content-Type` on the multipart part* — rejected. It is
  client-supplied, and Principle I's rule about not reading trust from the request applies
  to more than roles.

---

## D-004 — Subprocesses are fed over pipes, never a path

**Decision**: `tesseract` is invoked as `tesseract stdin stdout -l ara+eng`, reading the
image from standard input and writing text to standard output. `pdftotext` is invoked as
`pdftotext -enc UTF-8 - -`. `pdftoppm` is invoked per page as
`pdftoppm -singlefile -f N -l N -r 300 -png -` — PDF in on standard input, PNG out on
standard output. No extraction command line ever contains a path into the attachment store
or a temporary file.

**`pdftoppm` does not follow the `- -` convention, and getting this wrong fails silently.**
Unlike `pdftotext`, whose second `-` means stdout, `pdftoppm` treats its second argument as
an output *filename prefix*. `pdftoppm -png - -` therefore writes `-.png` into the process's
working directory and emits nothing on stdout: a plaintext render of a decrypted document
on disk, with no error, no output, and an extraction that quietly yields no text. The output
root must be **omitted entirely**. This was found by running the tool rather than by reading
the code, and `TestPDFTextWritesNothingToDisk` now guards it by running an extraction inside
an empty directory and failing if any file appears.

**Rationale**: This is the single most likely way to violate Principle VII in this feature.
Both tools take a filename happily, and the natural implementation decrypts to a temp file,
runs the tool, and deletes it. Principle VII forbids exactly that — "a plaintext temp file
is a violation even if deleted afterwards" — and a crash between the two steps leaves the
document lying in `/tmp`. Piping keeps plaintext in memory and in the pipe only.

**Alternatives considered**:

- *A temp file in a directory that is wiped at startup* — rejected. It narrows the window
  rather than closing it, and the constitution's wording admits no window.
- *An in-memory filesystem* — rejected. Platform-specific, and still a filesystem another
  process can read.
- *cgo bindings to libtesseract* — rejected. It removes the subprocess but adds a cgo
  toolchain requirement to a project that builds with plain `go build` today.

---

## D-005 — Three extraction families, chosen per page for PDF

**Decision**: OOXML (`docx`, `xlsx`, `pptx`) is unzipped in-process and its XML text nodes
read with `archive/zip` and `encoding/xml` — standard library only, no dependency. Images
go to recognition. A PDF is examined page by page: a page with a usable text layer is read
with `pdftotext`, a page without is rasterised at 300 dpi and recognised. Legacy `doc`,
`xls`, and `ppt` are stored and marked **not eligible**.

**Rationale**: FR-017 names the three families. Choosing per page rather than per file
matters because the common Egyptian registry document is a PDF that mixes a generated cover
page with scanned annexes; treating the whole file as one kind loses either exactness or
the scans.

Legacy binary Office formats are the honest gap. Parsing OLE compound files in Go has no
mature option, and the spec anticipated this by making "not eligible" a normal outcome
rather than a failure — nothing is wrong with the document.

**How the per-page decision is actually made**: `pdfinfo -` reports the page count, then for
each page `pdftotext -f N -l N - -` is tried. A page whose text layer yields fewer than
sixteen characters is treated as a scan and rasterised with `pdftoppm -f N -l N`, then
recognised. The sixteen-character floor exists because a scanned page routinely carries a
few stray characters — a header stamp, a footer page number — and treating those as "this
page has text" would skip recognition and lose the page's real content. A document beyond
200 pages is reported `too_large` rather than occupying the worker indefinitely.

**Alternatives considered**:

- *Rasterise and recognise every PDF* — rejected. It throws away exact text and replaces it
  with a worse guess.
- *A Go PDF-parsing library to count pages* — considered and **not taken**. `pdfinfo` ships
  with the poppler package this feature already requires and answers the same question, so
  the dependency would have bought nothing. Principle VI.
- *`pdftotext` on the whole file and give up if empty* — rejected. It cannot tell "this
  page is a scan" from "extraction failed", and the two need different states.
- *Convert legacy Office with LibreOffice in headless mode* — rejected. A very large
  dependency, and it wants to write files.

---

## D-006 — Job state lives on the attachment row

**Decision**: The attachment row carries `extract_state`, `extract_attempts`,
`extract_next_at`, and `extract_error`. An in-process worker polls for rows that are due,
claims one with `UPDATE ... WHERE id = $1 AND extract_state = 'pending'` returning the row,
and processes it. Backoff is 30 s, 2 min, 10 min. After three attempts the state becomes
`failed` and the row stops being due.

**Rationale**: The spec requires a restart to resume pending work rather than stranding it
(an edge case), which rules out an in-memory queue. Putting the state on the row rather than
in a separate jobs table keeps one source of truth for "what is happening to this
attachment", which is also what the UI displays.

The conditional `UPDATE` is the claim: two workers cannot both take the same row, because
only one update matches.

**Alternatives considered**:

- *A message broker* — rejected. A new service for a queue that will hold single digits of
  work at a time. Principle VI.
- *A goroutine started at upload with no persistence* — rejected. A restart mid-extraction
  leaves the attachment pending forever, which the spec names explicitly.
- *`LISTEN`/`NOTIFY` instead of polling* — rejected as an optimisation for a workload that
  polls once every few seconds against a handful of rows.

---

## D-007 — Sensitive text is sealed with the existing single-shot envelope

**Decision**: A sensitive attachment's extracted text is encrypted with the `Seal`/`Open`
pair feature 002 already built, not with the chunked stream. Ciphertext, nonce, wrapped
key, and wrap nonce live on the text row exactly as they do on a sensitive custom value.

**Rationale**: The text is capped at 256 KB (FR-021), which is small enough to seal in one
piece, and reusing 002's envelope means one audited implementation covers both. The chunked
stream exists for file bodies because they are large; text is not.

**Alternatives considered**:

- *Chunk the text too, for uniformity* — rejected. Uniformity for its own sake, on data
  that never needs streaming.
- *Store sensitive text in the same column as ordinary text with a flag* — rejected. A
  single column holding sometimes-readable, sometimes-encrypted content is exactly the
  shape that produces an accidental plaintext read.

---

## D-008 — Search is a normalised column with `LIKE`, and no new extension

**Decision**: Ordinary extracted text is stored twice: as written, and normalised through
the same Arabic normaliser used for usernames and property names. Document search matches
`LIKE` against the normalised column. Sensitive rows have no normalised column value at all
— not an empty one, no value — so they cannot be matched even by a query that forgets to
filter them.

**Rationale**: FR-027 requires the same Arabic matching as property names, and reusing the
Go normaliser keeps one definition. Making the exclusion structural rather than a `WHERE`
clause is the important part: FR-022b and FR-029 forbid sensitive text from search, and a
row with nothing to match cannot be returned by a mistake in a query.

`pg_trgm` is again omitted, as in feature 002 D-007. At hundreds of attachments a sequential
scan answers well inside the target, and Principle VI forbids the dependency until it is
needed.

**Alternatives considered**:

- *PostgreSQL full-text search* — rejected. Staff search for fragments — a registry number,
  part of a name — which is a substring problem, and the Arabic stemming configuration
  would be a second normalisation rule competing with the Go one.
- *Filtering sensitive rows in the query only* — rejected. One forgotten predicate leaks
  the thing the feature exists to protect.

---

## D-009 — Nothing is cached, including by the browser

**Decision**: Download and inline-view responses carry `Cache-Control: no-store` and
`Pragma: no-cache`. Inline viewing sets `Content-Disposition: inline` for PDF and images
and `attachment` for everything else. No rendering, thumbnail, or page image is ever
written to disk by the server.

**Rationale**: Constitution VII as amended in v1.3.0 says derived content inherits the
document's confidentiality, and names thumbnails specifically. The browser's disk cache is
the route that gets forgotten: a decrypted deed sitting in a cache folder is the same
exposure as one sitting in the store, and it is outside the store's protections entirely.

**Alternatives considered**:

- *Cached thumbnails for the attachment list* — rejected by the amendment, and it was the
  option most likely to be chosen for the list's sake.
- *`Cache-Control: private`* — rejected. It permits disk caching; it only forbids shared
  caches.

---

## D-010 — The read audit row commits before the first byte

**Decision**: A download or view opens a transaction, writes the audit row, commits, and
only then begins streaming. The audit write does not wrap the streaming.

**Rationale**: Principle VIII requires the audit row to be in the same transaction as the
change it records, so that a failed audit rolls the change back. A read is not a change and
cannot be rolled back once bytes are on the wire — so the ordering has to be the other way
around: if the audit write fails, nothing is served. Committing first means a served byte
always has a record, which is the property that matters for "who downloaded that ID scan".

The consequence is honest and worth stating: a read that is audited and then fails mid-
stream leaves a record of a read that did not complete. Over-recording a read is the safe
direction.

**Alternatives considered**:

- *Audit after a successful stream* — rejected. A read that fails at the last byte was still
  a read of everything before it, and would go unrecorded.
- *Hold the transaction open across the stream* — rejected. It pins a database connection
  for the length of a 50 MB download.

---

## D-011 — The store is a sharded directory of opaque names

**Decision**: Files live under a configured store root outside the repository, named by the
attachment's UUID with no extension, sharded two levels by the first four hex characters:
`<root>/ab/cd/abcdef01-....`. The store root is gitignored regardless of where it is placed.

**Rationale**: Principle VII requires opaque names and forbids the original filename
determining the path. Sharding keeps directory listings small. No extension means nothing
about the content is inferable from the store, and nothing is directly servable even if the
directory were accidentally exposed.

**Alternatives considered**:

- *Bytes in a `bytea` column* — rejected. It puts 50 MB blobs in every backup and every
  query plan, and PostgreSQL is not a file server.
- *A flat directory* — rejected. Tens of thousands of entries in one directory for no
  reason.
- *Keeping the original extension for convenience* — rejected by Principle VII.

---

## D-012 — Missing tools degrade, they do not break

**Decision**: At startup the server probes for `tesseract` (and the `ara` language pack) and
for `pdftotext`/`pdftoppm`, and records what is available. Uploads always succeed. An
attachment whose family needs a tool that is absent is marked **not eligible** with a reason
the UI shows in Arabic. Missing tools never fail an upload and never fail a read.

**Rationale**: FR-020 says extraction failing must never make a document inaccessible, and
neither should a missing dependency. **On this machine right now, `tesseract` has only the
`eng` pack and poppler is absent entirely** — so without this decision, every scanned
document would fail rather than being stored and marked honestly. The quickstart documents
installing both; the system is useful before they are installed and better after.

**Alternatives considered**:

- *Refuse to start without the tools* — rejected. It makes an optional capability a hard
  dependency of the whole service, including the parts that have nothing to do with text.
- *Fail the extraction and mark it failed* — rejected. "Failed" invites the user to press
  "try again" forever; "not eligible, Arabic recognition is not installed" tells them the
  truth.
