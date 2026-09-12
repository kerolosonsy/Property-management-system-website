// Package main runs the property-management API. It serves TLS only — there
// is no plaintext HTTP listener created under any configuration (FR-029,
// Constitution VII).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pms/internal/attachments"
	"pms/internal/auth"
	"pms/internal/blobstore"
	"pms/internal/config"
	pmscrypto "pms/internal/crypto"
	"pms/internal/db"
	"pms/internal/extract"
	"pms/internal/gen"
	"pms/internal/httpx"
	"pms/internal/identity"
	"pms/internal/properties"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.OpenPool(ctx, cfg.DatabaseAppURL)
	if err != nil {
		slog.Error("db open failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	identityStore := &identity.Store{Pool: pool}
	propertiesStore := &properties.Store{}
	envelope, err := pmscrypto.New(cfg.FieldKEK)
	if err != nil {
		slog.Error("envelope init failed", "err", err)
		os.Exit(1)
	}

	caps := extract.Probe(ctx)
	slog.Info(caps.String())

	store := blobstore.New(cfg.AttachmentStore)
	attachments.SetStore(store)

	srv := httpx.NewServer(cfg, pool, identityStore, propertiesStore, envelope)
	srv.SetCapabilities(caps)

	// The generated handler registers routes with the OpenAPI base path
	// (here /api/v1) on its own mux; pass BaseURL so the patterns match the
	// public URL space exactly. httpx.Server.Routes applies our auth and
	// role middleware on top of that handler.
	genHandler := gen.HandlerWithOptions(srv, gen.StdHTTPServerOptions{BaseURL: "/api/v1"})

	// Sweep expired sessions every 5 minutes; expiry is derived from
	// timestamps, so the sweep is housekeeping only (research §5).
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = auth.SweepExpired(ctx, pool)
			}
		}
	}()

	// Extraction worker: polls for due rows, claims each with a conditional
	// UPDATE so two workers cannot race, and processes it. Started alongside
	// the listener and stopped cleanly on shutdown (research.md D-006).
	worker := attachments.NewWorker(pool, envelope, caps, cfg.ExtractTextMaxBytes)
	go worker.Run(ctx)

	httpServer := &http.Server{
		Addr:      cfg.ListenAddr,
		Handler:   srv.Routes(genHandler),
		TLSConfig: nil, // served via ListenAndServeTLS below
		ErrorLog:  slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo),
	}

	go func() {
		slog.Info("api listening (TLS)", "addr", cfg.ListenAddr, "cert", cfg.TLSCertPath)
		if err := httpServer.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "err", err)
	}
}

