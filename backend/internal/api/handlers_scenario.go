package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// Scenario forwards an operator scenario command to the process stand
// (plantsim HTTP interface, TZ §7.3/§16). The stand is a demo component; in a
// real plant this endpoint is simply not configured and returns 503.
func (s *Server) Scenario(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PlantsimURL == "" {
		writeProblem(w, http.StatusServiceUnavailable, "no_plantsim",
			"процессный стенд не настроен (PLANTSIM_URL пуст)")
		return
	}
	var req struct {
		Code  int `json:"code"`
		Value int `json:"value"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		trimRight(s.cfg.PlantsimURL)+"/scenario", bytes.NewReader(body))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "plantsim_unreachable", "стенд недоступен: "+err.Error())
		return
	}
	defer resp.Body.Close()

	s.audit(r, "scenario.run", "scenario", "", "code,value")
	writeJSON(w, http.StatusAccepted, map[string]int{"code": req.Code, "value": req.Value})
}

func trimRight(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
