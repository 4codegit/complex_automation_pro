// Package service is the composition root of capd, the single server binary:
// config, storage, migrations, seeds, the live hub, the alarm engine, the
// supervisory control manager and the metallurgy calc service, all in one
// process behind one HTTP port (TZ §4).
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

	"cap/internal/alarms"
	"cap/internal/api"
	"cap/internal/auth"
	"cap/internal/config"
	"cap/internal/controlsvc"
	"cap/internal/hub"
	"cap/internal/metallurgy"
	"cap/internal/store"
)

// Run blocks until SIGINT/SIGTERM, then returns.
func Run() error {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[cap:capd] ")

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

	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := seedAll(ctx, db); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	h := hub.New()
	eng := alarms.New(db, h)
	srv := api.New(db, h, cfg, eng, nil)

	// The metallurgy calc service writes virtual tags through the canonical
	// ingest path (TZ §10): validation, historian and WS fan-out all apply.
	calc := metallurgy.New(db, srv.IngestCanonical, 5*time.Second)
	loops := controlsvc.New(db, h, eng, cfg.ControlStale)
	srv.SetLoops(loops)

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Communication-loss watchdog (TZ §11).
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-appCtx.Done():
				return
			case <-ticker.C:
				eng.Sweep(appCtx)
			}
		}
	}()

	if cfg.ControlEnabled {
		go loops.Run(appCtx)
		log.Printf("supervisory control enabled (watchdog %s)", cfg.ControlStale)
	} else {
		log.Printf("supervisory control disabled: loops seeded in manual, CONTROL_ENABLED=1 to start")
	}
	go calc.Run(appCtx)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-appCtx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shCtx)
	}()

	log.Printf("listening on http://%s (db=%s)", cfg.HTTPAddr, cfg.DBURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	log.Println("shutdown complete")
	return nil
}

// seedAll runs the idempotent first-start seeds: plant catalogue, rationalised
// alarm limits, control loops, roles and the initial local accounts.
func seedAll(ctx context.Context, db *sql.DB) error {
	if err := store.SeedRegistry(ctx, db); err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	if err := store.SeedAlarmLimits(ctx, db); err != nil {
		return fmt.Errorf("alarm limits: %w", err)
	}
	if err := store.SeedControlLoops(ctx, db); err != nil {
		return fmt.Errorf("control loops: %w", err)
	}
	if err := store.SeedRoles(ctx, db); err != nil {
		return fmt.Errorf("roles: %w", err)
	}
	return seedUsers(ctx, db)
}

// seedUsers creates the initial accounts on a fresh install (TZ §12).
// Passwords are development defaults and must be rotated in production.
func seedUsers(ctx context.Context, db *sql.DB) error {
	n, err := store.CountUsers(ctx, db)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	type seed struct {
		username, label, password string
	}
	for _, s := range []seed{
		{"admin", "Администратор", "admin"},
		{"operator", "Оператор", "operator"},
	} {
		hash, err := auth.HashPassword(s.password)
		if err != nil {
			return err
		}
		if err := store.CreateUser(ctx, db, &store.User{
			Username: s.username, Label: s.label, PasswordHash: hash, Active: true,
		}); err != nil {
			return err
		}
	}
	// The demo operator gets the operator system role out of the box.
	if err := store.AssignRole(ctx, db, "operator", "operator", "seed"); err != nil {
		return err
	}
	log.Printf("seeded default accounts admin/admin and operator/operator — rotate before production use")
	return nil
}
