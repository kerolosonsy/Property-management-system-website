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
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: admintool seed-admin [--username NAME] | reset-admin [--username NAME]")
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

	_ = audit.Write(ctx, tx, audit.Entry{
		Action:         audit.AccountCreated,
		ActorAccountID: nil, // pre-existing actor; this is itself the bootstrap
		ActorUsername:  raw,
		ActorRole:      strPtr("admin"),
		TargetAccountID: &acct.ID,
		TargetUsername:  strPtr(acct.Username),
		SourceIP:        "127.0.0.1",
		Detail:          map[string]string{"source": "seed-admin"},
	})
	_ = audit.Write(ctx, tx, audit.Entry{
		Action:         audit.AdminRecoveryUsed,
		ActorAccountID: &acct.ID,
		ActorUsername:  acct.Username,
		ActorRole:      strPtr(acct.Role),
		SourceIP:       "127.0.0.1",
		Detail:         map[string]string{"operation": "seed-admin"},
	})

	if err := tx.Commit(ctx); err != nil {
		fatal("commit: " + err.Error())
	}

	slog.Info("seeded administrator", "username", raw, "id", acct.ID.String())
}

func resetAdmin(args []string) {
	fs := flag.NewFlagSet("reset-admin", flag.ExitOnError)
	username := fs.String("username", "admin", "username of the account to reset")
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

	pw, err := promptPassword("New password (>= 12 chars; not echoed): ")
	if err != nil {
		fatal(err)
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

	if err := store.UpdatePassword(ctx, tx, acct.ID, hash, true); err != nil {
		fatal(err)
	}
	if err := auth.RevokeAllForAccount(ctx, tx, acct.ID); err != nil {
		fatal(err)
	}

	_ = audit.Write(ctx, tx, audit.Entry{
		Action:         audit.PasswordReset,
		ActorAccountID: nil,
		ActorUsername:  raw,
		ActorRole:      nil,
		TargetAccountID: &acct.ID,
		TargetUsername:  strPtr(acct.Username),
		SourceIP:        "127.0.0.1",
		Detail:          map[string]string{"source": "admintool reset-admin"},
	})
	_ = audit.Write(ctx, tx, audit.Entry{
		Action:         audit.AdminRecoveryUsed,
		ActorAccountID: nil,
		ActorUsername:  raw,
		SourceIP:       "127.0.0.1",
		Detail:         map[string]string{"operation": "reset-admin", "target": acct.Username},
	})

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
