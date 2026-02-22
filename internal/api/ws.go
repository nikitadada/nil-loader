package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nikitadada/nil-loader/internal/engine"
	"github.com/nikitadada/nil-loader/internal/model"
	"github.com/nikitadada/nil-loader/internal/telemetry"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSHandler struct {
	engine    *engine.Engine
	collector *telemetry.Collector
	state     *model.TestState
}

func NewWSHandler(eng *engine.Engine, collector *telemetry.Collector, state *model.TestState) *WSHandler {
	return &WSHandler{
		engine:    eng,
		collector: collector,
		state:     state,
	}
}

func (h *WSHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws/metrics", h.handleMetrics)
	mux.HandleFunc("/ws/logs", h.handleLogs)
	mux.HandleFunc("/ws/errors", h.handleErrors)
}

func (h *WSHandler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ch := h.collector.Subscribe()
	defer h.collector.Unsubscribe(ch)

	statusTicker := time.NewTicker(1 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case snap, ok := <-ch:
			if !ok {
				return
			}
			msg := model.WSMessage{Type: "metrics", Data: snap}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-statusTicker.C:
			statusMsg := model.WSMessage{
				Type: "status",
				Data: map[string]interface{}{
					"status":    h.state.GetStatus(),
					"startedAt": h.state.GetState().StartedAt,
				},
			}
			if err := conn.WriteJSON(statusMsg); err != nil {
				return
			}
			degMsg := model.WSMessage{Type: "degradation", Data: h.engine.Detector().GetResult()}
			if err := conn.WriteJSON(degMsg); err != nil {
				return
			}
		}
	}
}

func (h *WSHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	logCh := h.engine.LogChannel()

	for {
		select {
		case msg, ok := <-logCh:
			if !ok {
				return
			}
			wsMsg := model.WSMessage{Type: "log", Data: msg}
			data, _ := json.Marshal(wsMsg)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

func (h *WSHandler) handleErrors(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ch := h.collector.SubscribeErrors()
	defer h.collector.UnsubscribeErrors(ch)

	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return
			}
			wsMsg := model.WSMessage{Type: "error", Data: entry}
			if err := conn.WriteJSON(wsMsg); err != nil {
				return
			}
		}
	}
}
