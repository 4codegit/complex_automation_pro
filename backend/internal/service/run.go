// Package service is the shared composition root for every CAP deployable:
// the all-in-one development server (cmd/server) and the split microservices
// (cmd/live, cmd/ingest, cmd/historian, cmd/alarms, cmd/profiles, cmd/registry,
// cmd/identity). It wires config, storage, migrations, the live hub and the
// simulator identically in every process, so services differ only in the route
// sections they own.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cap/internal/api"
	"cap/internal/config"
	"cap/internal/hub"
	"cap/internal/simulator"
	"cap/internal/store"
)

// Options selects what a process runs.
type Options struct {
	// Name prefixes log lines and identifies the process (ingest, historian...).
	Name string

	// Sections are the route domains this process owns (see api.Routes).
	// Empty means all sections — the all-in-one development bundle.
	Sections []string

	// Seed runs the idempotent registry/roles/profile seeds. Only the first
	// process to touch a fresh database needs it; scripts/services.sh
	// starts the live service first for exactly that reason.
	Seed bool
}

// Run blocks until SIGINT/SIGTERM, then returns.
func Run(o Options) error {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[cap:" + o.Name + "] ")

	cfg, err := config.Load(".env")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx := context.Background()

	db, err := store.Open(ctx, cfg.DBURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer db.Close()

	if err := migrateWithRetry(ctx, db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if o.Seed {
		if err := store.SeedRegistry(ctx, db); err != nil {
			return fmt.Errorf("seed registry: %w", err)
		}
		if err := store.SeedRoles(ctx, db); err != nil {
			return fmt.Errorf("seed roles: %w", err)
		}
		if err := store.EnsureDefaultProfile(ctx, db); err != nil {
			return fmt.Errorf("seed profiles: %w", err)
		}
	}

	h := hub.New()
	sim := simulator.New(db, h, cfg.SimInterval, cfg.EmergencySec)
	srv := api.New(db, h, sim, cfg)

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The simulator runs in the all-in-one bundle and in the live service —
	// the process that owns the WebSocket hub it broadcasts into.
	ownsSim := len(o.Sections) == 0 || contains(o.Sections, "live")
	if cfg.SimulatorOn && ownsSim {
		sim.Start(appCtx)
		log.Printf("simulator enabled (interval=%s)", cfg.SimInterval)
	}
	// Supervisory control (ADR-003): opt-in. The PID loop runs where the
	// simulator/live hub runs; the gateway translates output into writes.
	if cfg.ControlEnabled && ownsSim {
		srv.StartControlLoop(appCtx)
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Routes(o.Sections...),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-appCtx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sim.Stop()
		_ = httpSrv.Shutdown(shCtx)
	}()

	log.Printf("listening on http://%s (db=%s)", cfg.HTTPAddr, cfg.DBURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	log.Println("shutdown complete")
	return nil
}

// migrateWithRetry tolerates a sibling service completing the same migration
// first: SQLite serialises writers, so concurrent starts can briefly collide.
func migrateWithRetry(ctx context.Context, db *sql.DB) error {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if err = store.Migrate(ctx, db); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	return err
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
