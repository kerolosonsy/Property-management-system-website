# Feature Specification: Properties Register

**Feature Branch**: `002-properties-crud`

**Created**: 2026-08-30

**Status**: Draft

**Input**: User description: "i want to start creating the page that contains all the properties as a list and i can add or modify or delete any property and also if i clicked on one of the list i can see all the details in it"

## Clarifications

### Session 2026-08-30

- Q: Should the list be filterable per column? → A: Yes. Every displayed column carries its own filter beneath the heading, as the design source does, and every one is applied by the server across the whole register.
- Q: What makes a property unique, given that several may share a saint's name? → A: A reference code, not the name. The system generates a sequential code on creation and an administrator may override it with the organisation's own. Names may repeat freely.
- Q: How do searchable custom fields coexist with the Principle VII rule that identity and financial values must be encrypted? → A: An administrator may mark a free-text custom field sensitive. A sensitive field's values are encrypted at rest and are excluded from the advanced search and from sorting, which is the price of encrypting them. This requires a written amendment to Principle VII before the plan gate passes, because it adds administrator-designated fields to the encrypted set.
- Q: Are administrator-defined custom property fields part of this feature? → A: Yes, in full, as the imported design shows them: administrators define fields of four types (text, dropdown, multi-select, checkbox), the fields appear on the property form and detail screen, and an advanced search filters properties on them.
- Q: Do the administrator screens for managing property types and areas ship with this feature? → A: Yes, both. Administrators manage both lists under Settings; managers are refused by the server on every route. This is the feature's manager-denied-configuration step required by Principle V.
- Q: Does removing a property delete it permanently or archive it? → A: Archive. A removed property leaves the working list but stays stored, readable, and restorable. No permanent deletion exists for anyone, including administrators; the record is the point.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Browsing and finding a property (Priority: P1)

A signed-in staff member opens the properties screen and sees every property the
organisation owns, listed one per row, with its name, type, and area. They can type
part of a name into a search box to narrow the list, restrict it to a single property
type or a single area, and move between pages when there are more properties than fit
on one screen. The whole screen is Arabic and reads right-to-left.

**Why this priority**: This is the screen the user asked for and the entry point to
everything else in the feature. On its own — with properties seeded — it already gives
staff the register they currently keep on paper, so it is a viable slice by itself.

**Independent Test**: Sign in, open the properties screen, and confirm the seeded
properties appear with correct names, types, and areas; type part of a name and confirm
the list narrows; pick a type filter and confirm only matching rows remain; step to page
two and back.

**Acceptance Scenarios**:

1. **Given** the register holds six properties, **When** a signed-in user opens the
   properties screen, **Then** all six appear with reference code, name, type, and area,
   sorted by name.
2. **Given** the properties screen is open, **When** the user types part of a property
   name or a reference code into the search box, **Then** only properties matching on
   either remain, and the count of results shown updates.
2a. **Given** two properties share the name "عقار العذراء", **When** the list shows them,
   **Then** both appear, distinguished by their reference codes and areas.
3. **Given** the properties screen is open, **When** the user selects a property type,
   **Then** only properties of that type remain and the search box keeps its effect.
4. **Given** more properties exist than one page holds, **When** the user moves to the
   next page, **Then** the following set of properties appears and the page indicator
   states which range of which total is being shown.
5. **Given** a search that matches nothing, **When** the results are empty, **Then** the
   screen states in Arabic that no properties match, rather than showing a blank area.
6. **Given** the register is empty, **When** a user opens the screen, **Then** an Arabic
   empty-state message invites them to add the first property.
7. **Given** a signed-out visitor, **When** they request the properties screen, **Then**
   they are sent to the sign-in screen and no property data is disclosed.

---

### User Story 2 - Adding a property (Priority: P2)

A staff member adds a new property by entering its name and choosing its type and area
from the configured lists. On saving, the property appears in the list immediately and
the action is recorded in the audit trail.

**Why this priority**: Without it the register can only ever hold what was seeded. It is
the first thing that makes the screen a working system of record rather than a report.

**Independent Test**: From the properties screen, add a property with a new name, then
confirm it appears in the list, can be found by search, and produced an audit record
naming the person who added it.

**Acceptance Scenarios**:

1. **Given** the add-property form is open, **When** the user enters a name and selects a
   type and an area and saves, **Then** the property is stored, the form closes, and the
   new property is visible in the list.
2. **Given** the add-property form is open, **When** the user tries to save with an empty
   name, **Then** saving is refused with an Arabic message naming the missing field, and
   nothing is stored.
3. **Given** a property named "عقار مار جرجس" already exists, **When** the user saves
   another property with that same name, **Then** it is accepted, and the two are told
   apart by their reference codes.
3a. **Given** a property is being added and no code was typed, **When** it is saved,
   **Then** the system assigns the next sequential reference code and shows it.
3b. **Given** an administrator types a reference code that another property already holds,
   active or archived, **When** they save, **Then** it is refused with an Arabic message
   naming the conflict.
4. **Given** custom fields are defined, **When** the add-property form opens, **Then** each
   appears with an input suited to its type, and each may be left empty.
5. **Given** the user typed Eastern Arabic digits anywhere in the form, **When** the value
   is accepted, **Then** it is stored and displayed using Western digits (0123456789).
6. **Given** a property was added, **When** an administrator opens the records screen,
   **Then** an audit entry exists naming the actor, their role, the action, the property,
   the UTC time, and the source address.

---

### User Story 3 - Seeing everything about one property (Priority: P2)

A staff member clicks a row in the list and is taken to that property's own screen,
where every stored field is shown together with when it was created and when it was last
changed, and by whom. From there they can return to the list without losing their search
and filter selections.

**Why this priority**: This is the second half of the user's request, and it is the only
place where fields too numerous for a table row can be read.

**Independent Test**: Open the properties screen, click any row, confirm the detail
screen shows that property's name, type, area, and its creation and modification record,
then go back and confirm the previous search and filters are still applied.

**Acceptance Scenarios**:

1. **Given** the list is showing, **When** the user clicks a property row, **Then** a
   detail screen for that property opens showing its name, type, and area.
2. **Given** custom fields are defined, **When** the detail screen opens, **Then** each
   field's label and this property's value for it are shown, and a field left empty is
   shown as empty rather than omitted.
3. **Given** the detail screen is open, **When** the user reads the record section,
   **Then** they see who created the property and when, and who last modified it and
   when, in Africa/Cairo local time.
4. **Given** a user arrived at the detail screen after searching and filtering, **When**
   they return to the list, **Then** the same search text, filters, and page are still in
   effect.
5. **Given** a property identifier that does not exist, **When** it is requested directly,
   **Then** an Arabic "not found" message is shown rather than an empty or broken screen.
6. **Given** a signed-out visitor, **When** they request a property detail screen directly,
   **Then** they are sent to the sign-in screen and no property data is disclosed.

---

### User Story 4 - Correcting a property (Priority: P3)

A staff member opens a property, changes its name, type, or area, and saves. The change
is reflected everywhere the property appears and is recorded in the audit trail.

**Why this priority**: Data entered by hand is entered wrongly sooner or later. Valuable,
but the register is usable before it exists.

**Independent Test**: Open a property, change its area, save, and confirm the list shows
the new area and that an audit record names the person who made the change.

**Acceptance Scenarios**:

1. **Given** a property detail screen, **When** the user changes a field and saves,
   **Then** the new values are stored and shown, and the last-modified record updates.
2. **Given** an administrator is editing a property, **When** they change its reference
   code to one another property already holds, **Then** saving is refused with an Arabic
   message and the original code remains.
2a. **Given** a manager is editing a property, **When** they view the reference code,
   **Then** it is shown but cannot be changed, and the server refuses any attempt to change
   it by another route.
3. **Given** custom fields are defined, **When** the user changes a custom field value and
   saves, **Then** the new value is stored and shown on the detail screen.
4. **Given** a property being edited, **When** the user cancels instead of saving,
   **Then** nothing is changed.
5. **Given** a property was modified, **When** an administrator opens the records screen,
   **Then** an audit entry exists naming the actor, the action, and the property.
6. **Given** two people open the same property and both save, **When** the second save
   arrives after the first, **Then** the second person is told the property changed since
   they opened it and is shown the current values rather than silently overwriting.

---

### User Story 5 - Archiving a property (Priority: P3)

A staff member archives a property that was entered in error or is no longer held. They
are asked to confirm first. The property leaves the working list but is not destroyed: it
can be listed among archived properties, opened, and restored.

**Why this priority**: The least frequent of the four operations, and the one whose
absence is least painful.

**Independent Test**: Archive a property, confirm it no longer appears in the working
list, confirm it appears in the archived list, restore it, and confirm an audit record
names who archived it and who restored it.

**Acceptance Scenarios**:

1. **Given** a property, **When** the user chooses to archive it, **Then** a confirmation
   naming that property is shown before anything is archived.
2. **Given** the confirmation is shown, **When** the user cancels, **Then** the property
   remains active and unchanged.
3. **Given** the confirmation is shown, **When** the user confirms, **Then** the property
   stops appearing in the working list and the user is returned to the list.
4. **Given** an archived property, **When** the user views the archived properties,
   **Then** it appears there marked as archived, with the date it was archived and by whom.
5. **Given** an archived property, **When** the user restores it, **Then** it returns to
   the working list with all of its fields as they were.
6. **Given** a property was archived and later restored, **When** an administrator opens
   the records screen, **Then** two audit entries exist — one for the archiving and one
   for the restoring — each naming the actor and the property.
7. **Given** an archived property holding reference code "P-004", **When** anyone tries to
   assign that code to another property, **Then** it is refused, because an archived code
   stays reserved so that restoring can never collide.

---

### User Story 6 - Maintaining the type and area lists (Priority: P3)

An administrator opens Settings and maintains the two lists the property form draws from:
the property types and the areas. They can add a value, rename one, and remove one that
nothing uses. A manager who reaches the same place is refused.

**Why this priority**: Without it the register cannot grow past its seeded vocabulary, but
the register itself works before it exists.

**Independent Test**: As an administrator, add a new area, then add a property using it;
rename that area and confirm the property shows the new name; try to remove it and confirm
the removal is refused because the property uses it. Then sign in as a manager and confirm
the same screen and the same operations are refused.

**Acceptance Scenarios**:

1. **Given** an administrator in Settings, **When** they add a new area, **Then** it is
   stored and immediately offered in the property form's area list.
2. **Given** an administrator, **When** they rename a property type, **Then** every
   property carrying that type shows the new name, because the property refers to the
   entry rather than copying its text.
3. **Given** a type or area no property uses, **When** the administrator removes it,
   **Then** it is removed and stops being offered.
4. **Given** a type or area at least one property uses — active or archived — **When**
   removal is attempted, **Then** it is refused with an Arabic message stating how many
   properties use it.
5. **Given** an administrator adds a value that already exists in that list, **When** they
   save, **Then** it is refused as a duplicate, compared with the same Arabic
   normalisation used for property names.
6. **Given** a signed-in manager, **When** they attempt to add, rename, or remove a
   property type or an area by any route, **Then** the server refuses, regardless of what
   the interface shows.
7. **Given** any change to either list, **When** an administrator opens the records
   screen, **Then** an audit entry exists naming the actor, the action, and which entry
   changed.

---

### User Story 7 - Defining custom property fields (Priority: P3)

An administrator decides the register needs to record something the built-in fields do not
cover — the year a building was constructed, which services it has, whether it is entered
in the real-estate registry. In Settings they define a new field, giving it an Arabic
label and choosing its type: free text, a dropdown with a fixed set of choices, a
multi-select of the same, or a yes/no checkbox. The field then appears on every property's
form and detail screen.

**Why this priority**: It lets the register record what this organisation actually tracks
without a developer. The register works before it exists, so it follows the core
operations.

**Independent Test**: As an administrator, define one field of each of the four types,
then open the add-property form and confirm all four appear with the right kind of input;
save a property with values in all four and confirm the detail screen shows them.

**Acceptance Scenarios**:

1. **Given** an administrator in Settings, **When** they define a field with an Arabic
   label and a type, **Then** it is stored and appears on the property form and detail
   screen for every property.
2. **Given** a dropdown or multi-select field is being defined, **When** the administrator
   supplies its choices, **Then** those choices — and only those — are offered to users.
3. **Given** a dropdown or multi-select field is being defined with no choices, **When**
   the administrator saves, **Then** it is refused with an Arabic message, because a
   choice list with nothing in it cannot be filled in.
4. **Given** a defined field, **When** the administrator renames its label, **Then** every
   property screen shows the new label and no stored value is lost.
5. **Given** a field that at least one property holds a value for, **When** removal of the
   field, a change of its type, or removal of one of its choices is attempted, **Then** it
   is refused with an Arabic message stating how many properties hold values that would be
   destroyed.
6. **Given** a field no property holds a value for, **When** the administrator removes it,
   **Then** it disappears from the property form, the detail screen, and the advanced
   search.
7. **Given** a signed-in manager, **When** they attempt to define, rename, or remove a
   custom field by any route, **Then** the server refuses, regardless of what the interface
   shows.
8. **Given** any change to a field definition, **When** an administrator opens the records
   screen, **Then** an audit entry exists naming the actor, the action, and which field
   changed.

---

### User Story 8 - Searching on custom fields (Priority: P3)

A staff member needs properties matching something recorded in a custom field — every
property with a lift, or every one built before 1990. They open an advanced search, set
values for the built-in filters and any custom fields they care about, and get the
matching properties. They can clear it all and start over.

**Why this priority**: It is what makes custom fields worth recording rather than merely
storing. It depends on User Story 7 and so follows it.

**Independent Test**: With custom fields defined and properties holding varied values,
open the advanced search, filter on one custom field, and confirm exactly the properties
holding that value are returned; add a second filter and confirm the results narrow.

**Acceptance Scenarios**:

1. **Given** custom fields are defined, **When** the user opens the advanced search,
   **Then** each custom field appears as a filter suited to its type.
2. **Given** several filters are set, **When** the search runs, **Then** only properties
   satisfying every set filter are returned, and unset filters constrain nothing.
3. **Given** a multi-select filter with two choices selected, **When** the search runs,
   **Then** only properties holding both choices are returned.
4. **Given** a checkbox filter, **When** the user sets it to yes, to no, or leaves it
   unset, **Then** the results are respectively the properties where it is ticked, where it
   is not, and all properties.
5. **Given** filters are set, **When** the user clears them, **Then** every filter returns
   to unset and the full result set returns.
6. **Given** a search matching nothing, **When** the results are empty, **Then** an Arabic
   message says so.
7. **Given** archived properties exist, **When** an advanced search runs, **Then** they are
   excluded unless the user asks to include them.

---

### Edge Cases

- What happens when two people add properties at the same instant? Both succeed and each
  receives its own reference code; no two properties can ever be issued the same code.
- What happens when an administrator overrides a code with one that a property created a
  moment earlier already took? Exactly one succeeds; the other is refused with the
  duplicate-code message.
- What happens when someone edits a property that another person archived a moment
  earlier? The save is refused with an Arabic message saying the property has been
  archived, and the user is returned to the list.
- What happens when a property's type or area was removed from the configured lists after
  that property was saved? It cannot happen: an entry in use cannot be removed.
- What happens to properties saved before a custom field was defined? They hold no value
  for it, and it shows as empty on their detail screen until someone fills it in.
- What happens when a multi-select field's choice list is reordered? Stored values are
  unaffected, because a value refers to a choice by identity rather than by position.
- What happens when a code matches an archived property's code? It is refused; the archived
  record still holds that code and restoring it must never collide. The message says the
  code belongs to an archived property, so the user knows to restore rather than re-enter.
- What happens when a name is entered with leading, trailing, or repeated inner spaces?
  It is trimmed and inner runs of whitespace collapsed before storage, so the register does
  not accumulate names that differ only invisibly.
- What happens when two properties have the same name? Nothing — it is allowed. They are
  told apart by reference code and area, both of which the list shows.
- What happens when two codes differ only by letter case, Arabic letter variants, diacritics,
  or tatweel? They are treated as the same code and the second is refused, using the same
  normalisation that matches usernames in feature 001.
- What happens to the sequence when a property is archived? Nothing is reused. The next
  generated code continues past the highest ever issued, so an archived code can never be
  handed to a different property.
- What happens when the search text contains characters that have special meaning to the
  search? They are treated as literal text, never as search operators.
- What happens when a user requests a page number beyond the last page? The last page is
  shown rather than an empty screen.
- How does the system handle a manager attempting to reach the configuration of property
  types or areas by any route? It is refused by the server, whatever the interface shows.

## Requirements *(mandatory)*

### Functional Requirements

#### Access and authorization

- **FR-001**: The system MUST require a valid signed-in session for every properties
  operation — listing, viewing, adding, modifying, and removing.
- **FR-002**: The system MUST permit both administrators and managers to list, view, add,
  modify, archive, and restore properties, because properties are operational data. The
  single exception is the reference code, which only administrators may supply or change.
- **FR-003**: The system MUST refuse, at the server, any attempt by a manager to add,
  modify, or remove a property type or an area, regardless of what the interface offers.
- **FR-004**: The system MUST decide every authorization question from the server-verified
  session, never from a value supplied by the browser.

#### The properties list

- **FR-005**: The system MUST present all properties as a list, one property per row,
  showing at least the reference code, the property name, its type, and its area.
- **FR-006**: The system MUST sort the list by property name by default.
- **FR-007**: Users MUST be able to narrow the list by typing text that is matched against
  both the property name and the reference code, with matching insensitive to case, Arabic
  letter variants, diacritics, and tatweel.
- **FR-008**: Users MUST be able to restrict the list to a single property type and to a
  single area, and these restrictions MUST combine with the search text rather than
  replace it.
- **FR-009**: The system MUST page the list, offering page sizes of 10, 25, 50, and 100
  rows, and MUST state which range of which total is being shown.
- **FR-009a**: The system MUST offer a filter on every displayed column of the list —
  reference code, name, property type, and area — presented as a row of controls beneath
  the column headings. Text columns match by substring after Arabic normalisation;
  columns drawn from a configured list offer that list. Every column filter MUST be
  applied by the server across the whole register rather than to the visible page, and
  MUST combine with the header search and with every other filter by AND.
- **FR-010**: The system MUST show an Arabic message when the register is empty, and a
  different Arabic message when the register is not empty but nothing matches the current
  search and filters.
- **FR-011**: The system MUST preserve the current search text, filters, page number, and
  page size when the user opens a property and returns to the list.

#### Property details

- **FR-012**: Users MUST be able to open a single property from the list and see every
  stored field for it on one screen, its reference code included.
- **FR-013**: The detail screen MUST show when the property was created and by whom, when
  it was last modified and by whom, and — if the property is archived — that it is
  archived, when, and by whom.
- **FR-014**: The system MUST show an Arabic "not found" message when a property that does
  not exist is requested directly.

#### Adding, modifying, archiving

- **FR-015**: Users MUST be able to add a property by supplying its name, its type, and
  its area.
- **FR-016**: The system MUST require a property name of 2 to 120 characters after trimming
  and whitespace collapsing, and MUST require a type and an area to be chosen.
- **FR-016a**: The system MUST allow two or more properties to share a name, because
  buildings named after the same saint are common; the name is descriptive, not identifying.
- **FR-017**: Every property MUST carry a reference code that is unique across the whole
  register, active and archived alike, compared after trimming, case folding, and the same
  Arabic normalisation used for usernames in feature 001.
- **FR-017a**: The system MUST generate a reference code automatically when a property is
  created and no code was supplied, using a fixed prefix followed by the next number in a
  sequence, rendered in Western digits.
- **FR-017b**: The system MUST NOT reuse a reference code that has ever been issued,
  including one held by an archived property; the sequence only moves forward.
- **FR-017c**: The system MUST permit only administrators to supply or change a property's
  reference code, and MUST refuse any such attempt by a manager at the server, whatever the
  interface shows. Managers see the code but cannot edit it.
- **FR-017d**: A supplied reference code MUST be 1 to 32 characters after trimming and MUST
  be rejected if it duplicates any existing code.
- **FR-017e**: The system MUST write an audit record when a property's reference code is
  changed, naming the old and the new code.
- **FR-018**: The system MUST offer property types and areas only from the configured
  lists; a free-typed type or area MUST NOT be accepted.
- **FR-019**: Users MUST be able to modify a property's name, type, and area.
- **FR-020**: The system MUST refuse a modification whose underlying property changed
  since the user opened it, and MUST tell the user rather than overwrite the other
  person's change.
- **FR-021**: Users MUST be able to archive a property, and the system MUST require an
  explicit confirmation naming that property first.
- **FR-022**: The system MUST NOT provide any means of permanently deleting a property, to
  any role including administrators. Archiving is the only removal.
- **FR-022a**: The system MUST exclude archived properties from the working list, its
  search, and its filters by default, and MUST let users view the archived properties
  separately.
- **FR-022b**: The system MUST record, for each archived property, when it was archived and
  by whom, and MUST show both on the archived listing and on the property's detail screen.
- **FR-022c**: Users MUST be able to restore an archived property, returning it to the
  working list with every field as it was.
- **FR-022d**: The system MUST keep an archived property's reference code reserved, so that
  assigning a code held by an archived property is refused with an Arabic message saying the
  code belongs to an archived property.
- **FR-022e**: The system MUST refuse to modify an archived property; it MUST be restored
  first.
- **FR-023**: The system MUST convert Eastern Arabic digits (٠١٢٣٤٥٦٧٨٩) entered by the
  user into Western digits (0123456789) on entry, and MUST display all numerals, dates,
  and counts in Western digits.

#### Configuration of types and areas

- **FR-024**: The system MUST hold the list of property types and the list of areas as
  configuration, and MUST provide administrators a screen under Settings to add to, rename,
  and remove entries in each list.
- **FR-024a**: The system MUST refuse a new or renamed entry that duplicates an existing one
  in the same list, compared with the same normalisation rule used for property names.
- **FR-024b**: A property MUST refer to its type and its area by identity rather than by
  copied text, so that renaming an entry is reflected everywhere that property appears.
- **FR-025**: The system MUST refuse to remove a property type or an area that is used by at
  least one property, active or archived, and MUST state in Arabic how many properties use
  it.
- **FR-026**: The system MUST ship a seeded set of property types and areas so the register
  is usable before an administrator configures anything.
- **FR-026a**: The system MUST write an audit record for every addition, rename, and removal
  in either list, naming the actor, the action, and which entry changed.

#### Custom property fields

- **FR-027**: The system MUST let administrators define custom property fields, each with
  an Arabic label and one of exactly four types: free text, single-choice dropdown,
  multi-select, and yes/no checkbox.
- **FR-027s1**: The system MUST let an administrator mark a **free-text** custom field as
  sensitive at the time it is defined. The three choice-based types MUST NOT be markable
  sensitive, because their values are drawn from a short published list and encrypting them
  conceals nothing.
- **FR-027s2**: The system MUST encrypt a sensitive field's values at rest using the
  envelope scheme required by Principle VII, storing only the wrapped data key and the
  nonce alongside the ciphertext, and MUST verify the authentication tag on every read.
- **FR-027s3**: The system MUST decrypt a sensitive value only inside a handler that has
  already resolved and checked the caller's role, and MUST NOT write a decrypted value to
  any log, error message, or audit record.
- **FR-027s4**: The system MUST exclude sensitive fields from the advanced search, from
  sorting, and from any filter, and MUST state on the advanced search screen, in Arabic,
  that sensitive fields cannot be searched.
- **FR-027s5**: The system MUST fix a field's sensitivity at definition time and MUST refuse
  to change it while any property holds a value for that field, on the same rule that
  governs changing a field's type.
- **FR-027s6**: The system MUST show, on the field-definition screen, an Arabic note that
  national ID, passport, bank account, and IBAN values belong in sensitive fields, and that
  a field holding them and left non-sensitive is a violation of the project's encryption
  rule.
- **FR-027a**: The system MUST refuse to save a dropdown or multi-select field that has no
  choices defined.
- **FR-027b**: The system MUST refuse a field label that duplicates an existing one,
  compared with the same normalisation rule used for property names.
- **FR-027c**: The system MUST show every defined field on the add-property form, the edit
  form, and the property detail screen, using an input suited to its type.
- **FR-027d**: Custom fields MUST all be optional; a property MUST be saveable with every
  custom field left empty.
- **FR-027e**: The system MUST offer, for a dropdown or multi-select field, only that
  field's defined choices; a free-typed value MUST NOT be accepted.
- **FR-027f**: A property's custom values MUST refer to the field definition by identity,
  so that renaming a field's label changes it everywhere without touching stored values.
- **FR-027g**: The system MUST refuse to remove a custom field, change its type, or remove
  one of its choices while any property — active or archived — holds a value that the
  change would destroy, and MUST state in Arabic how many properties are affected.
- **FR-027h**: The system MUST refuse, at the server, any attempt by a manager to define,
  rename, or remove a custom field, because field definitions are configuration.
- **FR-027i**: The system MUST provide an advanced search offering every built-in filter
  plus one filter per non-sensitive custom field, suited to that field's type: substring match for text,
  a single choice for a dropdown, a set of required choices for a multi-select, and
  yes/no/unset for a checkbox.
- **FR-027j**: The advanced search MUST combine every set filter so that only properties
  satisfying all of them are returned, MUST treat an unset filter as no constraint, and
  MUST let the user clear all filters at once.
- **FR-027k**: The advanced search MUST exclude archived properties unless the user asks
  for them to be included.
- **FR-027l**: The system MUST write an audit record for every custom field defined,
  renamed, retyped, and removed, naming the actor, the action, and which field changed.

#### Recording what happened

- **FR-028**: The system MUST write an audit record for every property added, modified,
  archived, and restored, naming the actor, their role, the action, the property, the UTC
  time, and the source address. Archiving and restoring MUST be distinguishable actions in
  the record, not both reported as a modification.
- **FR-029**: The system MUST write that audit record in the same transaction as the
  change it records, so that a failed audit write prevents the change.
- **FR-030**: The system MUST NOT write an audit record for merely reading the properties
  list or a property's detail.

#### Language and layout

- **FR-031**: Every user-visible string added by this feature MUST be Arabic, including
  column headings, filter labels, buttons, confirmations, empty states, and error
  messages.
- **FR-032**: Every screen added by this feature MUST lay out right-to-left using logical
  layout rules, and directional icons such as paging arrows and row chevrons MUST mirror.
- **FR-033**: Dates and times MUST be rendered in Africa/Cairo local time using the
  Gregorian calendar.

### Key Entities

- **Property** — a building or site the organisation holds. Attributes: a reference code
  unique across the whole register including archived entries and never reused, a
  descriptive name that need not be unique, a property type drawn from the configured list,
  an area drawn from the configured list, whether it is active or archived, and the record
  of who created, last modified, and archived it and when. The reference code is the
  identity; the name is description. A property moves between exactly two states, active
  and archived, in either direction; it is never destroyed.
- **Property Type** — a configured classification such as commercial or residential.
  Attributes: an identity that properties refer to, and a display name unique within the
  list. Administered by administrators only; cannot be removed while any property refers to
  it, archived properties included.
- **Area** — a configured geographic area such as a district or street. Same shape and same
  rules as Property Type, maintained through the same screen.
- **Custom Field** — an administrator-defined field extending what a property records.
  Attributes: an identity that stored values refer to, an Arabic label unique across the
  fields, a type of exactly one of text, dropdown, multi-select, or checkbox, for the two
  choice types an ordered set of choices, and for free text a fixed sensitivity flag.
  Configuration, so administrators only.
- **Custom Field Value** — one property's value for one custom field. Optional; absent means
  the field was left empty. Shaped by the field's type: text, one choice, a set of choices,
  or a yes/no. A value belonging to a sensitive field is stored as ciphertext with its
  wrapped data key and nonce, never as readable text.
- **Audit Record** — the existing append-only record from feature 001, extended in use to
  cover property additions, modifications, and removals.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A staff member can find a specific property among 500 by name or by reference
  code in under 10 seconds from opening the screen.
- **SC-002**: A staff member can add a complete property in under 60 seconds, counted from
  opening the add form to seeing the property in the list.
- **SC-003**: The properties list shows its first page within 2 seconds of the screen
  opening, with 500 properties in the register.
- **SC-004**: 100% of property additions, modifications, archivings, and restorings appear
  in the audit records screen with the correct actor and a distinct action for each.
- **SC-005**: 0 properties can be created with a reference code that duplicates any code
  ever issued, active or archived, and 0 can be created with an empty name, a type outside
  the configured list, or an area outside the configured list. Duplicate names are accepted,
  not refused.
- **SC-005b**: 0 managers can set or change a reference code, by any route, in 100% of
  attempts.
- **SC-005a**: 0 properties are ever destroyed — every property that has existed remains
  retrievable, and 100% of archived properties can be restored with all fields intact.
- **SC-006**: A manager is refused every attempt to add, rename, or remove a property type
  or an area, by every route, in 100% of attempts.
- **SC-006a**: An administrator can add a new area and use it on a property in under 60
  seconds, without any developer involvement or database change.
- **SC-007**: 100% of screens and messages added by this feature are Arabic, and 0
  physical left/right layout rules are used in their styling.
- **SC-008**: A user who searches, filters, opens a property, and returns finds their
  search and filters intact in 100% of attempts.
- **SC-008a**: An administrator can define a custom field of any of the four types and see
  it on the property form in under 60 seconds, without any developer involvement.
- **SC-008b**: 0 stored custom values are destroyed by a configuration change — every
  attempt to remove a field, change its type, or remove a choice that is in use is refused.
- **SC-008d**: 0 sensitive custom values are readable in the database, in any log, in any
  error message, or in any audit record — inspecting the stored row yields ciphertext only.
- **SC-008e**: 100% of sensitive fields are absent from the advanced search, and the screen
  says so in Arabic rather than silently omitting them.
- **SC-008c**: An advanced search combining a built-in filter and two custom-field filters
  returns exactly the properties satisfying all three, with 0 false positives, against a
  register of 500 properties, within 2 seconds.
- **SC-009**: When two people modify the same property concurrently, 0 changes are lost
  silently — the later save is always either refused or explicitly confirmed.

## Assumptions

- Properties are operational data, not configuration, so managers may add, modify, and
  remove them. Property types and areas are configuration, so only administrators may
  change those. This follows directly from Principle I of the constitution.
- A property's stored fields are its name, type, and area, plus creation and modification
  records. This mirrors the imported design, which carries exactly these three fields per
  property.
- Custom field definitions are configuration and custom field values are operational data.
  Administrators alone define the fields; both roles fill them in. This follows Principle I.
- Units, tenants, leases, rent, occupancy figures, and attachments are **out of scope**
  for this feature and are expected in later ones. The property detail screen therefore
  shows the property's own fields only, with no unit list and no occupancy percentage,
  even though the imported design shows those on its combined screen.
- The reference code, not the name, identifies a property. The organisation names buildings
  after saints, so names repeat across districts and cannot serve as identity. Codes are
  generated so staff never have to invent one, and administrator-overridable so an existing
  paper or plaque code can be carried over.
- Reference codes are never recycled. A code freed by archiving stays retired, so a code
  appearing in an old audit record or on a printed document always means the same property.
- The code's generated form is a fixed prefix followed by the next number in one sequence
  shared by the whole register, in Western digits. The exact prefix is a planning detail.
- Archiving is the only removal, so no cascade or dependency check is needed: units and
  leases added in later features can always point at a valid property, whether it is active
  or archived. The archived name stays reserved so restoring can never collide with a name
  entered in the meantime.
- Marking a custom field sensitive extends the encrypted set beyond the four columns named
  in Principle VII, so Principle VII must be amended in writing before this feature's plan
  gate can pass. That amendment is a prerequisite of planning, not of implementation.
- This feature therefore builds the AES-256-GCM envelope encryption layer, one feature
  earlier than attachments would have required it. The same layer serves attachments later.
- Both roles may read a sensitive field's value, because properties are operational data.
  Sensitivity governs how a value is stored and whether it can be searched, not who may see
  it.
- Custom fields are shared across all properties rather than defined per property type;
  every defined field appears on every property. This matches the imported design, which
  applies its dynamic fields to every property uniformly.
- The two list-management screens are the same screen twice over one shared shape, not two
  separately designed screens, matching how the imported design handles all of its lookup
  categories through one panel.
- Managing the property type and area lists is this feature's manager-denied-configuration
  step, satisfying the obligation in Principle V.
- The sign-in, session, role, audit, and Arabic-normalisation behaviour from feature 001
  is reused unchanged; this feature adds no new authentication mechanism.
- Western Arabic digits are used throughout, as decided in feature 001.
- The design source at `Property Management System.html` is a visual reference for layout
  and wording, not shippable markup, and its combined properties-and-units screen is split
  so that this feature delivers the properties half only.
