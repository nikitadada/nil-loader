package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/nikitadada/nil-loader/internal/engine"
	"github.com/nikitadada/nil-loader/internal/grpcclient"
	"github.com/nikitadada/nil-loader/internal/model"
	"github.com/nikitadada/nil-loader/internal/telemetry"
)

type Handler struct {
	engine    *engine.Engine
	state     *model.TestState
	collector *telemetry.Collector
}

func NewHandler(eng *engine.Engine, state *model.TestState, collector *telemetry.Collector) *Handler {
	return &Handler{
		engine:    eng,
		state:     state,
		collector: collector,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/start", h.handleStart)
	mux.HandleFunc("/api/stop", h.handleStop)
	mux.HandleFunc("/api/status", h.handleStatus)
	mux.HandleFunc("/api/reflect", h.handleReflect)
	mux.HandleFunc("/api/parse-proto", h.handleParseProto)
	mux.HandleFunc("/api/degradation", h.handleDegradation)
	mux.HandleFunc("/api/report", h.handleReport)
}

func (h *Handler) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg model.TestConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if cfg.Target == "" || cfg.Service == "" || cfg.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target, service, and method are required"})
		return
	}
	if cfg.StartRPS <= 0 {
		cfg.StartRPS = 10
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 30
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 200
	}
	if cfg.LoadProfile == "" {
		cfg.LoadProfile = model.LoadProfileConstant
	}

	if err := h.engine.Start(&cfg); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.engine.Stop()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := h.state.GetState()
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) handleReflect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	conn, err := grpcclient.Dial(req.Target)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	info, err := grpcclient.ListServicesViaReflection(ctx, conn)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) handleParseProto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	fds, err := grpcclient.ParseProtoContent("upload.proto", string(body))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	info := grpcclient.ListServicesFromProto(fds)
	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) handleDegradation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := h.engine.Detector().GetResult()
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := h.state.GetState()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=report.json")
	json.NewEncoder(w).Encode(st)
}

func writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
