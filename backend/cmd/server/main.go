// Command server runs the CAP monitoring API: push ingestion, pull queries,
// registry, alarms, live WebSocket fan-out and the development simulator.
package main

import (
	"context"
	"errors"
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

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[cap] ")

	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	db, err := store.Open(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := store.SeedRegistry(ctx, db); err != nil {
		log.Fatalf("seed registry: %v", err)
	}
	if err := store.SeedRoles(ctx, db); err != nil {
		log.Fatalf("seed roles: %v", err)
	}
	if err := store.EnsureDefaultProfile(ctx, db); err != nil {
		log.Fatalf("seed profiles: %v", err)
	}

	h := hub.New()
	sim := simulator.New(db, h, cfg.SimInterval, cfg.EmergencySec)
	srv := api.New(db, h, sim, cfg)

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.SimulatorOn {
		sim.Start(appCtx)
		log.Printf("simulator enabled (interval=%s)", cfg.SimInterval)
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-appCtx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sim.Stop()
		_ = httpSrv.Shutdown(shCtx)
	}()

	log.Printf("CAP listening on http://%s (db=%s)", cfg.HTTPAddr, cfg.DBURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	log.Println("shutdown complete")
}
