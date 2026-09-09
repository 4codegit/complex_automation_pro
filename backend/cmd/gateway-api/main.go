// Command gateway-api is the single public entry point in split mode: it
// routes /api/v1/* and the dashboard to the backing microservices and holds
// no database of its own. Edge gateways and browsers talk only to this
// address; upstreams default to the scripts/services.sh ports and are
// overridable per service via <NAME>_UPSTREAM env vars.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cap/internal/config"
)

// route is one mux prefix and the service that owns it behind this gateway.
type route struct {
	prefix string
	envKey string
	def    string
}

var routes = []route{
	{"/api/v1/ingest/", "INGEST_UPSTREAM", "http://127.0.0.1:8002"},
	{"/api/v1/telemetry", "HISTORIAN_UPSTREAM", "http://127.0.0.1:8003"},
	{"/api/v1/telemetry/", "HISTORIAN_UPSTREAM", "http://127.0.0.1:8003"},
	{"/api/v1/alerts", "HISTORIAN_UPSTREAM", "http://127.0.0.1:8003"},
	{"/api/v1/alerts/", "HISTORIAN_UPSTREAM", "http://127.0.0.1:8003"},
	{"/api/v1/reports/", "HISTORIAN_UPSTREAM", "http://127.0.0.1:8003"},
	{"/api/v1/alarms", "ALARMS_UPSTREAM", "http://127.0.0.1:8004"},
	{"/api/v1/alarms/", "ALARMS_UPSTREAM", "http://127.0.0.1:8004"},
	{"/api/v1/profiles", "PROFILES_UPSTREAM", "http://127.0.0.1:8005"},
	{"/api/v1/profiles/", "PROFILES_UPSTREAM", "http://127.0.0.1:8005"},
	{"/api/v1/assets", "REGISTRY_UPSTREAM", "http://127.0.0.1:8006"},
	{"/api/v1/assets/", "REGISTRY_UPSTREAM", "http://127.0.0.1:8006"},
	{"/api/v1/tags", "REGISTRY_UPSTREAM", "http://127.0.0.1:8006"},
	{"/api/v1/tags/", "REGISTRY_UPSTREAM", "http://127.0.0.1:8006"},
	{"/api/v1/gateways", "REGISTRY_UPSTREAM", "http://127.0.0.1:8006"},
	{"/api/v1/access/", "IDENTITY_UPSTREAM", "http://127.0.0.1:8007"},
	{"/api/v1/ws", "LIVE_UPSTREAM", "http://127.0.0.1:8001"},
	{"/api/v1/simulator/", "LIVE_UPSTREAM", "http://127.0.0.1:8001"},
	{"/api/v1/health", "LIVE_UPSTREAM", "http://127.0.0.1:8001"},
	{"/", "LIVE_UPSTREAM", "http://127.0.0.1:8001"}, // dashboard SPA
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[cap:gateway-api] ")

	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	mux := http.NewServeMux()
	for _, r := range routes {
		mux.Handle(r.prefix, proxyTo(getEnv(r.envKey, r.def)))
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-appCtx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Printf("listening on http://%s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	log.Println("shutdown complete")
}

// proxyTo builds a reverse proxy to one upstream service. FlushInterval -1
// keeps WebSocket frames streaming instead of buffering.
func proxyTo(raw string) http.Handler {
	u, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("upstream %q: %v", raw, err)
	}
	return &httputil.ReverseProxy{
		Rewrite:       func(pr *httputil.ProxyRequest) { pr.SetURL(u) },
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "bad_gateway",
				"message": err.Error(),
			})
		},
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
