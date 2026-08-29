// Command migrate runs goose migrations against the database using the OWNER
// role credentials. Never invoked from the running service.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	if cmd != "up" && cmd != "down" && cmd != "status" {
		usage()
		os.Exit(1)
	}

	dsn := os.Getenv("PMS_DATABASE_OWNER_URL")
	if dsn == "" {
		fatal("PMS_DATABASE_OWNER_URL must be set (owner role only).")
	}

	migrationsDir, err := migrationsDir()
	if err != nil {
		fatal(err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatal("open: " + err.Error())
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		fatal("set dialect: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch cmd {
	case "up":
		if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
			fatal("goose up: " + err.Error())
		}
	case "down":
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			fatal("goose down: " + err.Error())
		}
	case "status":
		current, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			fatal(err)
		}
		fmt.Println("current version: " + strconv.Itoa(int(current)))
		entries, err := os.ReadDir(migrationsDir)
		if err != nil {
			fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".sql") {
				continue
			}
			fmt.Println("  " + name)
		}
	}
	slog.Info("migrate " + cmd + " ok")
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate up|down|status")
}

func fatal(v any) {
	slog.Error("migrate failed", "err", fmt.Sprint(v))
	os.Exit(1)
}

func migrationsDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(cwd, "migrations"),
		filepath.Join(cwd, "api", "migrations"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("migrations directory not found (looked in ./migrations and ./api/migrations)")
}
