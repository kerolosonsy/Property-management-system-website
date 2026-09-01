// Command admintool is the offline administrator utility. It runs in two
// modes:
//   - seed-admin: create the very first administrator (created_by IS NULL).
//   - reset-admin: recovery flow; requires the OWNER credentials and direct
//     shell access on the host. Never reachable over HTTP. Writes an
//     admin_recovery_used audit row.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"pms/internal/audit"
	"pms/internal/auth"
	"pms/internal/identity"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "seed-admin":
		seedAdmin(os.Args[2:])
	case "reset-admin":
		resetAdmin(os.Args[2:])
	case "backfill-attachment-search":
		backfillAttachmentSearch(os.Args[2:])
	case "seed-demo":
		seedDemo(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: admintool seed-admin [--username NAME] | reset-admin [--username NAME] | seed-demo")
}

func seedAdmin(args []string) {
	fs := flag.NewFlagSet("seed-admin", flag.ExitOnError)
	username := fs.String("username", "admin", "username for the initial administrator")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	dsn := os.Getenv("PMS_DATABASE_OWNER_URL")
	if dsn == "" {
		fatal("PMS_DATABASE_OWNER_URL must be set (owner role only).")
	}

	raw := strings.TrimSpace(*username)
	if err := validateSeedUsername(raw); err != nil {
		fatal(err)
	}

	pw, err := promptPassword("Password for new administrator (>= 12 chars; not echoed): ")
	if err != nil {
		fatal(err)
	}
	if len([]rune(pw)) < 12 {
		fatal("password must be at least 12 characters")
	}
	pw2, err := promptPassword("Confirm password: ")
	if err != nil {
		fatal(err)
	}
	if pw != pw2 {
		fatal("passwords do not match")
	}

	hash, err := auth.HashPassword(pw)
	if err != nil {
		fatal(err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fatal("pool: " + err.Error())
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		fatal(err)
	}
	defer tx.Rollback(ctx)

	store := &identity.Store{Pool: pool}
	canonical := identity.Canonical(raw)

	existing, err := store.FindByCanonical(ctx, tx, canonical)
	if err != nil {
		fatal(err)
	}
	if existing != nil {
		fatal("an account with this canonical username already exists")
	}

	acct := &identity.Account{
		Username:          raw,
		UsernameCanonical: canonical,
		DisplayName:       raw,
		Role:              "admin",
	}
	if err := store.CreateAccount(ctx, tx, acct, hash, nil); err != nil {
		fatal("create: " + err.Error())
	}

	if err := audit.Write(ctx, tx, audit.Entry{
		Action:          audit.AccountCreated,
		ActorAccountID:  nil, // pre-existing actor; this is itself the bootstrap
		ActorUsername:   raw,
		ActorRole:       strPtr("admin"),
		TargetAccountID: &acct.ID,
		TargetUsername:  strPtr(acct.Username),
		SourceIP:        "127.0.0.1",
		Detail:          map[string]string{"source": "seed-admin"},
	}); err != nil {
		fatal("audit: " + err.Error())
	}
	if err := audit.Write(ctx, tx, audit.Entry{
		Action:         audit.AdminRecoveryUsed,
		ActorAccountID: &acct.ID,
		ActorUsername:  acct.Username,
		ActorRole:      strPtr(acct.Role),
		SourceIP:       "127.0.0.1",
		Detail:         map[string]string{"operation": "seed-admin"},
	}); err != nil {
		fatal("audit: " + err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		fatal("commit: " + err.Error())
	}

	slog.Info("seeded administrator", "username", raw, "id", acct.ID.String())
}

func resetAdmin(args []string) {
	fs := flag.NewFlagSet("reset-admin", flag.ExitOnError)
	username := fs.String("username", "admin", "username of the account to reset")
	// Non-interactive path for local development. The password is read from the named
	// environment variable, never from a flag: a flag value is visible in `ps` output
	// and in shell history, which Constitution VII rules out for any secret.
	passwordEnv := fs.String("password-env", "", "read the new password from this environment variable instead of prompting")
	mustChange := fs.Bool("must-change", true, "require the account to change this password at next sign-in")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	dsn := os.Getenv("PMS_DATABASE_OWNER_URL")
	if dsn == "" {
		fatal("PMS_DATABASE_OWNER_URL must be set (owner role only).")
	}

	raw := strings.TrimSpace(*username)
	if err := validateSeedUsername(raw); err != nil {
		fatal(err)
	}

	var pw string
	if *passwordEnv != "" {
		pw = os.Getenv(*passwordEnv)
		if pw == "" {
			fatal(*passwordEnv + " is empty or unset; set it in .env (which is gitignored) and re-run")
		}
	} else {
		var err error
		pw, err = promptPassword("New password (>= 12 chars; not echoed): ")
		if err != nil {
			fatal(err)
		}
	}
	if len([]rune(pw)) < 12 {
		fatal("password must be at least 12 characters")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		fatal(err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fatal("pool: " + err.Error())
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		fatal(err)
	}
	defer tx.Rollback(ctx)

	store := &identity.Store{Pool: pool}
	canonical := identity.Canonical(raw)
	acct, err := store.FindByCanonical(ctx, tx, canonical)
	if err != nil {
		fatal(err)
	}
	if acct == nil {
		fatal("account not found")
	}

	if err := store.UpdatePassword(ctx, tx, acct.ID, hash, *mustChange); err != nil {
		fatal(err)
	}
	if err := auth.RevokeAllForAccount(ctx, tx, acct.ID); err != nil {
		fatal(err)
	}

	if err := audit.Write(ctx, tx, audit.Entry{
		Action:          audit.PasswordReset,
		ActorAccountID:  nil,
		ActorUsername:   raw,
		ActorRole:       nil,
		TargetAccountID: &acct.ID,
		TargetUsername:  strPtr(acct.Username),
		SourceIP:        "127.0.0.1",
		Detail:          map[string]string{"source": "admintool reset-admin", "must_change_password": fmt.Sprintf("%t", *mustChange)},
	}); err != nil {
		fatal("audit: " + err.Error())
	}
	if err := audit.Write(ctx, tx, audit.Entry{
		Action:         audit.AdminRecoveryUsed,
		ActorAccountID: nil,
		ActorUsername:  raw,
		SourceIP:       "127.0.0.1",
		Detail:         map[string]string{"operation": "reset-admin", "target": acct.Username},
	}); err != nil {
		fatal("audit: " + err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		fatal("commit: " + err.Error())
	}

	slog.Info("reset password", "username", raw, "id", acct.ID.String())
}

func validateSeedUsername(s string) error {
	if len([]rune(s)) < 3 || len([]rune(s)) > 32 {
		return errors.New("username must be 3-32 characters")
	}
	if identity.HasInvisibleOrBidi(s) {
		return errors.New("username contains invisible or direction-control characters")
	}
	for _, r := range s {
		if !identity.AllowedUsernameRune(r) {
			return errors.New("username contains a character that is not a letter, digit, dot, underscore, or hyphen")
		}
	}
	return nil
}

// stdinReader is a single bufio.Reader shared by every promptPassword call.
// Creating a fresh bufio.Reader per call would race with the kernel pipe buffer:
// the first reader would silently consume bytes into its internal buffer and
// never release them, and the second reader would see an empty stream.
var stdinReader = bufio.NewReader(os.Stdin)

func promptPassword(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	fd := int(os.Stdin.Fd())
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		// Fall back to plain read if not on a terminal (CI etc.).
		line, e := stdinReader.ReadString('\n')
		if e != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	return string(pw), nil
}

// fatal reports a startup or input failure and exits non-zero. It takes any
// so call sites can pass either a message or an error.
func fatal(v any) {
	fmt.Fprintln(os.Stderr, "admintool:", v)
	os.Exit(1)
}

func strPtr(s string) *string { return &s }

// seedDemo inserts the six design properties the quickstart manual procedure
// expects to see (FR-026 demonstration seed; feature 002-properties-crud T032).
// The owner role is required so the inserts work even though the table is
// otherwise touched only by pms_app.
func seedDemo(args []string) {
	dsn := os.Getenv("PMS_DATABASE_OWNER_URL")
	if dsn == "" {
		fatal("PMS_DATABASE_OWNER_URL must be set (owner role only).")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fatal("pool: " + err.Error())
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		fatal("begin: " + err.Error())
	}
	defer tx.Rollback(ctx)

	var adminID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM account WHERE role = 'admin' ORDER BY created_at NULLS FIRST LIMIT 1`).Scan(&adminID); err != nil {
		fatal("locate admin: " + err.Error())
	}

	// Seed data taken from the design source (an Assiut diocese). Each
	// property carries one of the three تجاري types (الكتدرائية المرقسية,
	// الأنبا كيرلس, مار مينا) or one of the three سكني types (مار جرجس,
	// العذراء, الملاك ميخائيل), and the area matches the design pairing.
	pairs := [][2]string{
		{"تجاري", "وسط البلد، أسيوط"},
		{"سكني", "حي الحمراء"},
		{"تجاري", "شارع الجمهورية"},
		{"تجاري", "أرض الشهيد"},
		{"سكني", "كورنيش النيل"},
		{"سكني", "حي بني عدي"},
	}
	names := []string{
		"عقار الكاتدرائية المرقسية",
		"عقار مار جرجس",
		"مجمع الأنبا كيرلس",
		"عقار العذراء",
		"مبنى مار مينا",
		"عقار الملاك ميخائيل",
	}

	for i, p := range pairs {
		var typeID, areaID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM property_type WHERE label = $1`, p[0]).Scan(&typeID); err != nil {
			fatal("lookup property_type " + p[0] + ": " + err.Error())
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM area WHERE label = $1`, p[1]).Scan(&areaID); err != nil {
			fatal("lookup area " + p[1] + ": " + err.Error())
		}
		name := names[i]
		code := fmt.Sprintf("P-%03d", i+1)
		// The normalised columns MUST go through the same normaliser every API write
		// uses. Writing the raw value here defeats the unique index on
		// code_normalized: the API canonicalises to "p-001" while a raw seed stores
		// "P-001", so the two never collide and the same visible code is issued twice.
		if _, err := tx.Exec(ctx, `
			INSERT INTO property (code, code_normalized, name, name_normalized, property_type_id, area_id, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
			code, identity.Canonical(code), name, identity.Canonical(name), typeID, areaID, adminID); err != nil {
			fatal("insert " + code + ": " + err.Error())
		}
	}

	// These rows were inserted with literal codes rather than drawn from the sequence,
	// so the sequence still points at 1 and the next API-generated code would collide
	// with P-001. Advance it past everything seeded (FR-017b: codes are never reused).
	if _, err := tx.Exec(ctx, `SELECT setval('property_code_seq', $1, true)`, len(names)); err != nil {
		fatal("advance property_code_seq: " + err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		fatal("commit: " + err.Error())
	}

	slog.Info("seeded demo properties", "count", len(pairs))
}

// backfillAttachmentSearch fills description_normalized and filename_normalized
// for attachments stored before migration 0023. The canonical form is produced
// by the Go normaliser rather than an SQL rewrite, so it matches exactly what
// every other search compares against (research.md D-007).
func backfillAttachmentSearch(args []string) {
	fs := flag.NewFlagSet("backfill-attachment-search", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	dsn := os.Getenv("PMS_DATABASE_OWNER_URL")
	if dsn == "" {
		fatal("PMS_DATABASE_OWNER_URL must be set (owner role only).")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fatal("pool: " + err.Error())
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT id, description, original_filename
		FROM attachment
		WHERE description_normalized IS NULL OR filename_normalized IS NULL`)
	if err != nil {
		fatal(err)
	}
	type row struct {
		id       uuid.UUID
		desc     string
		filename string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.desc, &r.filename); err != nil {
			rows.Close()
			fatal(err)
		}
		pending = append(pending, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		fatal(err)
	}

	for _, r := range pending {
		if _, err := pool.Exec(ctx, `
			UPDATE attachment
			   SET description_normalized = $2, filename_normalized = $3
			 WHERE id = $1`,
			r.id, identity.Canonical(r.desc), identity.Canonical(r.filename)); err != nil {
			fatal(err)
		}
	}
	slog.Info("backfilled attachment search columns", "rows", len(pending))
}
