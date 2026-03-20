package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/nikitadada/nil-loader/internal/auth"
	"github.com/nikitadada/nil-loader/internal/engine"
	"github.com/nikitadada/nil-loader/internal/grpcclient"
	"github.com/nikitadada/nil-loader/internal/model"
	"github.com/nikitadada/nil-loader/internal/payload"
	"github.com/nikitadada/nil-loader/internal/telemetry"
)

type Handler struct {
	engine    *engine.Engine
	state     *model.TestState
	collector *telemetry.Collector
	auth      *auth.Service
}

func NewHandler(eng *engine.Engine, state *model.TestState, collector *telemetry.Collector, authSvc *auth.Service) *Handler {
	return &Handler{
		engine:    eng,
		state:     state,
		collector: collector,
		auth:      authSvc,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/login", h.handleAuthLogin)
	mux.HandleFunc("/auth/status", h.handleAuthStatus)

	mux.HandleFunc("/api/start", h.requireAuth(h.handleStart))
	mux.HandleFunc("/api/stop", h.requireAuth(h.handleStop))
	mux.HandleFunc("/api/status", h.requireAuth(h.handleStatus))
	mux.HandleFunc("/api/reflect", h.requireAuth(h.handleReflect))
	mux.HandleFunc("/api/parse-proto", h.requireAuth(h.handleParseProto))
	mux.HandleFunc("/api/auto-payload-template", h.requireAuth(h.handleAutoPayloadTemplate))
	mux.HandleFunc("/api/degradation", h.requireAuth(h.handleDegradation))
	mux.HandleFunc("/api/report", h.requireAuth(h.handleReport))
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.auth == nil || !h.auth.IsAuthenticatedRequest(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]bool{"authenticated": false})
			return
		}
		next(w, r)
	}
}

func (h *Handler) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.auth == nil {
		http.Error(w, "auth not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	// Client is expected to send JSON: {"password":"..."}.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	if req.Password == "" || !h.auth.CheckPassword(req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if err := h.auth.SetAuthCookie(w, r, time.Now()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (h *Handler) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.auth == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}

	ok := h.auth.IsAuthenticatedRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]bool{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
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

func (h *Handler) handleAutoPayloadTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Target        string `json:"target"`
		Service       string `json:"service"`
		Method        string `json:"method"`
		UseReflection bool   `json:"useReflection"`
		ProtoContent  string `json:"protoContent,omitempty"`
		MaxDepth      int    `json:"maxDepth,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.Target == "" || req.Service == "" || req.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target, service, and method are required"})
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = 5
	}

	caller, err := grpcclient.NewCaller(
		req.Target,
		req.UseReflection,
		req.Service,
		req.Method,
		req.ProtoContent,
	)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer caller.Close()

	inputType := caller.GetInputType()
	tpl, warnings, err := payload.GeneratePayloadTemplate(inputType, req.MaxDepth)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"payloadTemplate": tpl,
		"warnings":        warnings,
	})
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
