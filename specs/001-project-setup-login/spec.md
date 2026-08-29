# Feature Specification: Project Foundation & Login

**Feature Branch**: `001-project-setup-login`

**Created**: 2026-08-29

**Status**: Draft

**Input**: User description: "i want to start creating the project and the login for the app"

## Clarifications

### Session 2026-08-29

- Q: Which numeral system should the interface use? → A: Western Arabic digits (0123456789) everywhere; Eastern Arabic digits typed or pasted by the user are converted to Western digits on entry.
- Q: When an account's role changes while that person is signed in, when does the new role take effect? → A: Immediately — role and active state are read fresh from stored data on every request, so the change applies to the user's very next action without ending their session.
- Q: What characters may a username contain, and is it case-sensitive? → A: Arabic and Latin letters, digits, dot, underscore, and hyphen; 3–32 characters; matched case-insensitively and after Arabic normalization, so two usernames that look alike cannot both exist.
- Q: How should repeated wrong passwords be handled — account lockout or something else? → A: Progressive delay, no lockout. Each consecutive failure lengthens the wait before the next attempt is accepted. No account is ever locked and no administrator unlock exists, so nobody can lock a colleague out.
- Q: Can anyone read the audit records in this feature, or are they write-only for now? → A: Administrators get a records screen in this feature, filterable by person, date range, and action type, with paging. Managers cannot reach it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Signing in to the system (Priority: P1)

A staff member opens the application and is met by a sign-in screen in Arabic. They
enter their username and password and are taken to the main area of the system. If
their credentials are wrong, they are told so and remain on the sign-in screen.

**Why this priority**: Nothing else in the product is reachable without this. It is
the whole of the MVP: with only this story delivered, the seeded administrator can
sign in and the application is demonstrably alive and protected.

**Independent Test**: With one pre-existing administrator account, open the
application, sign in with correct credentials and confirm arrival at the main area;
sign in with wrong credentials and confirm refusal. No other feature is required.

**Acceptance Scenarios**:

1. **Given** an active account with a known password, **When** the user submits the
   correct username and password, **Then** they are signed in and the main area of
   the system is shown.
2. **Given** an active account, **When** the user submits a wrong password, **Then**
   the sign-in is refused with a message that does not reveal whether the username
   exists, and the user stays on the sign-in screen.
3. **Given** a username that does not exist, **When** the user submits it with any
   password, **Then** the refusal message is identical to the wrong-password message.
4. **Given** a deactivated account, **When** the user submits correct credentials,
   **Then** sign-in is refused.
5. **Given** an administrator signing in for the very first time with the initial
   password, **When** sign-in succeeds, **Then** the user is required to set a new
   password before reaching any other part of the system.
6. **Given** any sign-in attempt, successful or failed, **When** it completes,
   **Then** a permanent record of who, when, and from where is written.

---

### User Story 2 - Administering user accounts (Priority: P2)

The administrator creates accounts for the managers who will use the system, sets
their role, deactivates people who leave, and resets a password for someone who has
forgotten theirs. There is no public registration; every account exists because the
administrator created it.

**Why this priority**: Without it the system has exactly one usable account forever.
It is second because the seeded administrator account makes Story 1 demonstrable on
its own.

**Independent Test**: Sign in as administrator, create a manager account, sign out,
sign in as that new manager successfully. Then deactivate the manager and confirm
they can no longer sign in.

**Acceptance Scenarios**:

1. **Given** the administrator is signed in, **When** they create an account with a
   username, display name, role, and initial password, **Then** that person can sign
   in with those credentials and is required to change the password on first use.
2. **Given** a username already in use, **When** the administrator tries to create a
   second account with it, **Then** creation is refused with a clear reason.
3. **Given** an existing account, **When** the administrator deactivates it, **Then**
   any active session for that account stops working and further sign-in is refused.
4. **Given** a manager has forgotten their password, **When** the administrator
   resets it, **Then** the manager can sign in with the new password and is required
   to change it immediately.
5. **Given** a manager is signed in, **When** they attempt to reach user
   administration by any route, **Then** access is refused.
6. **Given** any account is created, modified, deactivated, or has its password
   reset, **When** the action completes, **Then** a permanent record of who did it
   and when is written.
7. **Given** an administrator is signed in and working, **When** another
   administrator changes their role to manager, **Then** their very next action is
   evaluated as a manager and administrator areas are refused, without them signing
   out first.

---

### User Story 3 - Ending a session safely (Priority: P3)

A signed-in user can sign out deliberately. If they walk away, the system signs them
out on its own. Repeated wrong password guesses are slowed down, each attempt making
the next one wait longer.

**Why this priority**: It protects an unattended screen and blocks password guessing,
but the system is usable and demonstrable without it.

**Independent Test**: Sign in, sign out, and confirm the previous session can no
longer reach any page. Separately, leave a session idle past the timeout and confirm
sign-in is required again. Separately, submit wrong passwords repeatedly and confirm
each attempt is made to wait longer than the last, and that the correct password still
works once the wait has passed.

**Acceptance Scenarios**:

1. **Given** a signed-in user, **When** they sign out, **Then** they are returned to
   the sign-in screen and using the browser's back button does not restore access.
2. **Given** a signed-in user who has been inactive beyond the idle limit, **When**
   they next act, **Then** they are returned to the sign-in screen and must sign in
   again.
3. **Given** a session that has lasted beyond the maximum session lifetime, **When**
   the user next acts, **Then** they must sign in again even if they were active.
4. **Given** several consecutive failed sign-in attempts for one username, **When**
   another attempt is submitted before the current waiting period has passed, **Then**
   it is refused as too soon, even if the password is correct, and the user is told
   how long remains.
5. **Given** consecutive failures have built up a waiting period, **When** the wait
   has passed and the correct password is submitted, **Then** the user signs in and
   the waiting period returns to nothing.
6. **Given** a username that does not exist, **When** wrong attempts are repeated
   against it, **Then** the waiting period grows exactly as it would for a real
   account, so the delay cannot be used to discover which usernames exist.

---

### User Story 4 - Reviewing what happened (Priority: P4)

The administrator opens a records screen and sees what has happened in the system,
newest first. They narrow it to one person, to a range of dates, or to one kind of
action — for example every failed sign-in last Tuesday, or everything one manager did
last month. Nothing on the screen can change or remove a record.

**Why this priority**: The records are written from Story 1 onward whether or not
anyone can read them, so the system is safe without this. It is what makes the records
useful rather than merely present, and it is the last piece that can be added without
disturbing the others.

**Independent Test**: Sign in as administrator, perform a handful of sign-ins, a failed
attempt, and an account change, then open the records screen and confirm each action
appears, and that filtering by person, by date, and by action type each narrows the
list correctly. Sign in as a manager and confirm the screen cannot be reached.

**Acceptance Scenarios**:

1. **Given** several recorded actions exist, **When** the administrator opens the
   records screen, **Then** the records are shown newest first, each stating who acted,
   their role, what they did, which account was affected, when, and from where.
2. **Given** the records screen is open, **When** the administrator filters by a
   person, **Then** only actions performed by that person are shown.
3. **Given** the records screen is open, **When** the administrator filters by a start
   and end date, **Then** only actions within that range, inclusive of both days, are
   shown.
4. **Given** the records screen is open, **When** the administrator filters by an
   action type such as failed sign-in, **Then** only records of that type are shown.
5. **Given** several filters are applied together, **When** the list is shown, **Then**
   only records matching every applied filter appear.
6. **Given** more records exist than fit on one page, **When** the administrator moves
   to the next page, **Then** the applied filters remain in effect.
7. **Given** a filter combination that matches nothing, **When** the list is shown,
   **Then** a clear Arabic message says no records match, and the filters remain
   visible so they can be adjusted.
8. **Given** a manager is signed in, **When** they attempt to reach the records screen
   by any route, **Then** access is refused.
9. **Given** the records screen is open, **When** the administrator looks for a way to
   edit or delete a record, **Then** none exists, and any request to do so made by
   other means is refused.

---

### Edge Cases

- Sign-in submitted with an empty username, empty password, or whitespace only —
  refused with field-level guidance, and no attempt is counted against the account.
- A username or display name containing Arabic characters, or mixed Arabic and Latin
  text, is stored and displayed exactly as entered, except that Eastern Arabic digits
  within it are converted to Western digits.
- A user types or pastes a value containing Eastern Arabic digits, or a mix of Eastern
  and Western digits — every digit is converted to Western form, the surrounding
  Arabic text is left untouched, and the stored value contains Western digits only.
- An administrator creates a username that differs from an existing one only by letter
  case, by an Arabic diacritic, by a tatweel, or by an alef, ya, or ta marbuta variant
  — creation is refused as already in use, because the two would be indistinguishable
  on screen.
- A user signs in typing their username with different case or a different alef
  variant than it was created with — sign-in succeeds, because matching uses the
  canonical form.
- A username is submitted containing a zero-width or bidirectional control character —
  it is refused with a clear reason rather than silently stripped.
- The records screen is filtered with a start date later than the end date — the
  administrator is told the range is invalid rather than being shown an empty list.
- The records screen is opened when the system is newly installed and almost nothing
  has happened — the sign-in that opened the screen is itself already listed.
- A record refers to an account that has since been deactivated or renamed — the record
  still shows the account as it was named when the action happened, because records are
  never rewritten.
- An extremely long username or password submission is refused on length grounds
  rather than being truncated or causing an error page.
- The same account signs in from two browsers at once — both sessions work
  independently, and signing out of one does not end the other.
- An administrator deactivates their own account, or removes the last remaining
  administrator — refused, so the system can never be left with no administrator.
- A user's account is deactivated while they are mid-task — their next action is
  refused and they are returned to the sign-in screen.
- A user's role is changed while they are mid-task — their next action is judged by
  the new role. If they were viewing a screen the new role may not use, that screen's
  next action is refused rather than silently succeeding.
- The session ends while a form is part-filled — the user is returned to sign-in, and
  the unsaved entry is not silently submitted afterwards.
- Sign-in refusals take a comparable amount of time whether or not the username
  exists, so response speed cannot be used to discover valid usernames.
- The system is reached before any account exists — the initial administrator account
  is present from setup, so this state cannot occur in a correctly installed system.

## Requirements *(mandatory)*

### Functional Requirements

**Identity and access**

- **FR-001**: The system MUST identify each person by a unique username and
  authenticate them with a password.
- **FR-036**: A username MUST be 3 to 32 characters long and MUST contain only Arabic
  letters, Latin letters, digits, dot, underscore, or hyphen. Spaces are not
  permitted. Leading and trailing whitespace MUST be removed before validation.
- **FR-037**: The system MUST reject a username containing invisible or
  direction-controlling characters, including zero-width characters and bidirectional
  control marks, because such characters allow two usernames to appear identical on
  screen while differing in stored form.
- **FR-038**: The system MUST compare usernames for both sign-in and uniqueness after
  reducing them to a canonical form. That form MUST ignore letter case, MUST remove
  Arabic diacritics and the tatweel elongation mark, and MUST treat the alef variants
  (أ إ آ ا) as one letter, alef maqsura (ى) and ya (ي) as one letter, and ta marbuta
  (ة) and ha (ه) as one letter. Eastern Arabic digits MUST already have been
  converted per FR-033 before comparison.
- **FR-039**: The system MUST store and display a username exactly as the
  administrator typed it. The canonical form is used for matching only and MUST NOT
  be shown to users.
- **FR-002**: The system MUST support exactly two roles: administrator and manager.
  Every account MUST hold exactly one role.
- **FR-003**: The system MUST refuse any request that is not accompanied by a valid,
  server-verified session, other than the sign-in request itself.
- **FR-004**: The system MUST decide every permission on the server. Hiding or
  disabling something on screen MUST NOT be the only thing preventing a manager from
  performing an administrator action.
- **FR-005**: The system MUST refuse a manager access to user administration and to
  every configuration area, regardless of how the request is made.
- **FR-006**: The system MUST NOT provide public self-registration. Accounts MUST be
  created only by an administrator.
- **FR-007**: The system MUST ship with one initial administrator account created
  during installation, and MUST require that account's password to be changed before
  any other action is possible.

**Credentials**

- **FR-008**: The system MUST store passwords in a non-reversible form. It MUST NOT
  be possible for the system, or anyone with access to its stored data, to read,
  display, print, or send an existing password.
- **FR-009**: The system MUST reject passwords shorter than 12 characters.
- **FR-010**: The system MUST require a password change on first sign-in after an
  account is created or after an administrator resets its password.
- **FR-011**: Users MUST be able to change their own password by supplying their
  current password.
- **FR-012**: Administrators MUST be able to reset any account's password. Managers
  MUST NOT be able to reset any password other than their own.

**Refusals and rate limiting**

- **FR-013**: The system MUST return an identical refusal message whether the
  username is unknown, the password is wrong, or the account is deactivated.
- **FR-014**: The system MUST slow repeated sign-in failures by imposing a waiting
  period that grows with each consecutive failure for the same submitted username: no
  wait before the first failure, then 1, 2, 4, 8, 16, 32 seconds, capped at 60 seconds
  for every failure beyond that.
- **FR-015**: The system MUST refuse any sign-in attempt submitted before the current
  waiting period has elapsed, even when the password is correct, and MUST tell the
  user how much of the wait remains.
- **FR-016**: The system MUST reset the waiting period to nothing after a successful
  sign-in.
- **FR-040**: The system MUST apply the same waiting period to usernames that do not
  exist as to real accounts, so neither the message nor the timing reveals whether an
  account exists.
- **FR-041**: The system MUST NOT lock any account. There MUST be no administrator
  action that unlocks an account, because no account can become locked. An account
  MUST NOT become permanently unusable through failed sign-in attempts.
- **FR-042**: The system MUST discard a username's failure count after 15 minutes with
  no further failed attempt.

**Sessions**

- **FR-017**: The system MUST end a session after 30 minutes of inactivity.
- **FR-018**: The system MUST end a session 8 hours after it began, regardless of
  activity.
- **FR-019**: Users MUST be able to sign out, and doing so MUST make that session
  immediately unusable, including via the browser's history.
- **FR-020**: The system MUST invalidate all existing sessions of an account when
  that account is deactivated or has its password changed or reset.
- **FR-021**: The system MUST NOT expose a session identifier in a page address, and
  MUST NOT make it readable by scripts running in the page.
- **FR-034**: A session MUST carry identity only. The system MUST read the account's
  current role and active state from stored data when handling each request, and MUST
  decide that request's permissions from what it reads. A role recorded at sign-in
  time MUST NOT be trusted for any later request.
- **FR-035**: A change of role MUST take effect on the affected user's next action,
  without requiring them to sign out or sign in again.

**Record keeping**

- **FR-022**: The system MUST write a permanent record of every successful sign-in,
  failed sign-in, sign-out, password change, password reset, account creation, role
  change, and activation or deactivation.
- **FR-023**: Each record MUST state who acted, their role, what they did, which
  account was affected, when it happened in coordinated universal time, and the
  network address the request came from.
- **FR-024**: Records MUST NOT be editable or deletable by anyone, including an
  administrator.
- **FR-025**: Records MUST NOT contain passwords in any form.
- **FR-043**: Administrators MUST be able to view the recorded actions. Managers MUST
  NOT be able to view them by any route.
- **FR-044**: The records view MUST show newest first, and MUST be filterable by the
  person who acted, by a start and end date inclusive of both days, by action type, and
  by the account affected. Filters MUST be combinable, and applying them MUST NOT
  change which records exist.
- **FR-045**: The records view MUST be paged, and MUST keep the applied filters when
  moving between pages.
- **FR-046**: The records view MUST offer no means of editing or deleting a record, and
  the system MUST refuse any such request made by other means, consistent with FR-024.
- **FR-047**: Times in the records view MUST be shown in the installation's local time
  using Western Arabic digits, while remaining stored in coordinated universal time.
- **FR-048**: When no records match the applied filters, the system MUST say so in
  Arabic and keep the filters on screen for adjustment.

**Presentation**

- **FR-026**: Every screen in this feature MUST be presented in Arabic with a
  right-to-left layout.
- **FR-027**: Every message the user sees — including refusals, validation, and
  errors — MUST be in Arabic and MUST state what the user should do next.
- **FR-028**: The system MUST NOT reveal internal failure detail to the user. An
  unexpected failure MUST show a general Arabic message while the detail is recorded
  for the operator.
- **FR-032**: The system MUST display every number — counts, dates, times, and
  amounts — using Western Arabic digits (0123456789). Eastern Arabic digits
  (٠١٢٣٤٥٦٧٨٩) MUST NOT appear in any displayed value.
- **FR-033**: When a user types or pastes Eastern Arabic digits into any field, the
  system MUST convert them to the equivalent Western Arabic digits before the value
  is validated or stored. The conversion MUST be visible to the user as they enter
  the value, and MUST NOT alter non-numeric Arabic text.

**Data protection in transit**

- **FR-029**: The system MUST NOT accept credentials over an unprotected connection
  in any environment, including a developer's own machine.

**Foundation**

- **FR-030**: The application MUST be startable, and its sign-in screen reachable,
  from a documented set of commands on a clean machine with no prior project
  knowledge.
- **FR-031**: No credential, key, or secret required to run the system MUST be stored
  in the project's version history.

### Key Entities

- **User Account**: A person who may sign in. Holds a unique username, a display
  name, exactly one role, an active or deactivated state, a protected password, and a
  flag indicating whether the password must be changed at next sign-in.
- **Role**: One of exactly two values — administrator or manager — determining what
  the account may do.
- **Session**: A period of authenticated access belonging to one account. Has a start
  time, a last-activity time, and an end. Ends by sign-out, inactivity, age, or
  account change.
- **Failure Counter**: The number of consecutive failed sign-ins for one submitted
  username and the time of the most recent one, used to decide how long the next
  attempt must wait. Exists for usernames that do not correspond to any account.
  Cleared by a successful sign-in, and discarded after 15 minutes without a failure.
- **Audit Entry**: A permanent, unchangeable record of one action: who, their role,
  what, which account was affected, when, and from where.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user who knows their credentials can go from opening the application
  to working in the main area in under 15 seconds, entering no more than two fields.
- **SC-002**: Across a test of unknown usernames, wrong passwords, and deactivated
  accounts, 100% of attempts are refused with a message a tester cannot use to tell
  the three cases apart.
- **SC-003**: 100% of attempts by a manager to reach user administration or any
  configuration area are refused, including attempts that bypass the on-screen
  navigation entirely.
- **SC-004**: 100% of sign-ins, failed attempts, sign-outs, and account changes
  performed during verification appear as records, and no attempt to alter or remove
  one of those records succeeds.
- **SC-005**: After five consecutive wrong passwords, a sixth attempt must wait at
  least 16 seconds; once that wait passes, the correct password signs the user in
  immediately. No sequence of failed attempts makes an account permanently unusable,
  and no administrator action is ever needed to restore access.
- **SC-014**: Repeating wrong attempts against a username that does not exist produces
  the same messages and the same waiting periods as against a real account, so a
  tester cannot tell the two apart.
- **SC-015**: An administrator can find every action one named person took on one named
  day in under 30 seconds, using no more than three filter choices.
- **SC-016**: 100% of manager attempts to reach the records view are refused, including
  attempts that bypass the on-screen navigation.
- **SC-006**: A session left untouched for 30 minutes requires signing in again, and
  so does a session 8 hours old regardless of use.
- **SC-007**: 100% of text visible on this feature's screens is Arabic, and the
  layout reads right to left, with no element visually mirrored the wrong way.
- **SC-008**: After signing out, no page reachable through browser history or a saved
  address shows the previous user's data.
- **SC-011**: Across every screen and stored value in this feature, 100% of digits are
  Western Arabic form, including values a tester deliberately entered using Eastern
  Arabic digits.
- **SC-012**: An account demoted from administrator to manager during an active
  session is refused every administrator action from its next request onward, with no
  sign-out required and no window in which the old role still applies.
- **SC-013**: No two accounts can exist whose usernames render identically on screen.
  A tester attempting to create case, diacritic, tatweel, alef, ya, and ta marbuta
  variants of an existing username is refused every time.
- **SC-009**: A new developer with only the project's written setup steps reaches a
  working sign-in screen without asking anyone a question.
- **SC-010**: A search of the project's stored data and version history finds no
  readable password and no working credential.

## Assumptions

- **Initial administrator**: Installation creates exactly one administrator account
  with a starting password supplied at install time, which must be changed at first
  sign-in. There is no other bootstrap path.
- **No self-service password recovery**: The system runs locally with no mail
  delivery, so there is no "forgot password" email flow. A manager who has forgotten
  their password asks an administrator to reset it. An administrator who loses
  their own password is recovered through an offline administrative utility run on
  the machine itself, which is treated as installation tooling and specified with the
  setup work rather than as a user-facing screen.
- **Username, not email**: People sign in with a username. Email addresses are not
  collected by this feature and are not used for identity. Usernames may be written in
  Arabic or Latin script; the Arabic display name remains the primary way a person is
  identified on screen.
- **Single factor**: Two-factor authentication, one-time codes, and hardware keys are
  out of scope for this feature.
- **No "remember me"**: Sessions are not persisted beyond the stated lifetimes, and
  there is no long-lived stay-signed-in option.
- **Thresholds**: The 12-character minimum, the doubling wait that starts at 1 second
  and caps at 60, the 15-minute failure-count expiry, the 30-minute idle limit, and the
  8-hour session lifetime are chosen as sensible defaults; none was specified by the
  requester and each can be adjusted before implementation.
- **No lockout, deliberately**: Locking an account after repeated failures would let
  anyone who knows a colleague's username keep that colleague locked out at will, on a
  network where everyone can reach the sign-in screen. A growing wait stops guessing
  without handing out that power. The accepted cost is that a determined attacker can
  keep guessing indefinitely, at roughly one attempt per minute.
- **Concurrent sessions allowed**: One account may be signed in from more than one
  browser at a time; sessions are independent.
- **Numerals**: The interface is Arabic with Western Arabic digits. Users typing on an
  Arabic keyboard layout may produce Eastern Arabic digits, so entry is normalized
  rather than rejected — a wrong-looking digit is a typing artifact, not a mistake
  worth an error message.
- **Scale**: The system serves a small number of staff, on the order of tens rather
  than thousands, on a single local installation.
- **Project foundation**: "Creating the project" is understood as the repository
  structure, local build and run commands, database setup, and protected local
  connection needed for the sign-in screen to work. It produces no user-visible
  behavior of its own beyond FR-030, and is therefore covered by the setup phase of
  this feature rather than by a user story.
- **Records viewing scope**: The records screen reads and filters. It does not export,
  print, or summarise, and opening it is not itself recorded — with only a small number
  of administrators, recording every read would add noise without adding an answer to
  any question likely to be asked.
- **Out of scope for this feature**: tenant and owner portals, external identity
  providers, any property, unit, lease, tenant, or payment screen, any file upload, and
  exporting or printing the records. Those arrive in later features.
