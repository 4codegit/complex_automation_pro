// Command gateway runs the edge collector: local sensor polling, normalization
// to the canonical contract, store-and-forward buffering (SQLite/WAL), batch
// delivery with exponential backoff and a heartbeat pulse to the server.
// See TZ_DEVELOPMENT.md section 8 (S3).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"cap/internal/gateway"
)

var version = "edge-0.1.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[cap] ")

	cfg, err := gateway.LoadConfig(".env")
	if err != nil {
		log.Fatalf("gateway config: %v", err)
	}

	runner, err := gateway.NewRunner(cfg, version)
	if err != nil {
		log.Fatalf("gateway init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runner.Run(ctx); err != nil {
		log.Fatalf("gateway: %v", err)
	}
	log.Println("shutdown complete")
}
