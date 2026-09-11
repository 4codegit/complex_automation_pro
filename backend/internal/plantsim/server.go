package plantsim

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// HTTPServer exposes the stand's control interface (TZ §8.7): /healthz,
// /scenario and /state. It is a demonstration accessory — a real plant has
// nothing like it.
type HTTPServer struct {
	model     *Model
	modbus    *ModbusServer
	startedAt time.Time
}

// NewHTTPServer wires the HTTP interface.
func NewHTTPServer(model *Model, modbus *ModbusServer) *HTTPServer {
	return &HTTPServer{model: model, modbus: modbus, startedAt: time.Now()}
}

// Handler builds the HTTP routes.
func (h *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /scenario", h.scenario)
	mux.HandleFunc("GET /state", h.state)
	return mux
}

func (h *HTTPServer) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"tick":         h.model.State().Tick,
		"uptime_s":     time.Since(h.startedAt).Seconds(),
		"modbus_conns": h.modbus.Connections(),
		"comm_loss":    h.model.CommLoss(),
	})
}

func (h *HTTPServer) scenario(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  int `json:"code"`
		Value int `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed JSON"})
		return
	}
	// Keep the scenario command mirrored in the scenario registers so the
	// operator can see the last command over Modbus as well.
	_ = h.model.WriteHolding(RegSIMCMD, uint16(clampInt(req.Code, 0, 255)))
	_ = h.model.WriteHolding(RegSIMVAL, uint16(uint16(int16(clampInt(req.Value, -32768, 32767)))))
	desc := h.model.Scenario(req.Code, req.Value)
	log.Printf("[plantsim] scenario %d/%d: %s", req.Code, req.Value, desc)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": desc})
}

func (h *HTTPServer) state(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.model.State())
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Stand bundles the model, its Modbus server and the HTTP control interface.
type Stand struct {
	Model  *Model
	Modbus *ModbusServer
	HTTP   *HTTPServer
}

// RunStand runs a complete stand until the context is cancelled. It is the
// heart of cmd/plantsim.
func RunStand(ctx context.Context, modbusAddr, httpAddr string, unitID uint8, tick time.Duration, seed uint64) (*Stand, error) {
	model := New(seed, time.Now())
	modbusSrv, err := NewModbusServer(modbusAddr, model, unitID)
	if err != nil {
		return nil, err
	}
	httpSrv := NewHTTPServer(model, modbusSrv)
	httpListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		modbusSrv.Close()
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				model.Tick(tick)
			}
		}
	}()

	srv := &http.Server{Handler: httpSrv.Handler()}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
		modbusSrv.Close()
	}()
	go func() {
		log.Printf("[plantsim] modbus on %s (unit %d), http on %s, tick %s",
			modbusSrv.Addr(), unitID, httpListener.Addr(), tick)
		if err := srv.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			log.Printf("[plantsim] http: %v", err)
		}
	}()

	return &Stand{Model: model, Modbus: modbusSrv, HTTP: httpSrv}, nil
}

// Main is the entry point of cmd/plantsim (kept here for testability).
func Main() {
	modbusAddr := envOr("PLANTSIM_MODBUS_ADDR", ":1502")
	httpAddr := envOr("PLANTSIM_HTTP_ADDR", ":15080")
	unit := uint8(envIntOr("PLANTSIM_UNIT_ID", 1))
	tick := time.Duration(envIntOr("PLANTSIM_TICK_MS", 1000)) * time.Millisecond
	seed := uint64(envIntOr("PLANTSIM_SEED", 42))

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[cap:plantsim] ")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if _, err := RunStand(ctx, modbusAddr, httpAddr, unit, tick, seed); err != nil {
		log.Fatalf("%v", err)
	}
	<-ctx.Done()
	log.Println("shutdown complete")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
