package main

import (
	"log"
	"net/http"

	"github.com/nikitadada/nil-loader/internal/api"
	"github.com/nikitadada/nil-loader/internal/engine"
	"github.com/nikitadada/nil-loader/internal/model"
	"github.com/nikitadada/nil-loader/internal/telemetry"
	"github.com/nikitadada/nil-loader/web"
)

func main() {
	state := model.NewTestState()
	collector := telemetry.NewCollector()
	detector := telemetry.NewDegradationDetector()
	eng := engine.NewEngine(state, collector, detector)

	handler := api.NewHandler(eng, state, collector)
	wsHandler := api.NewWSHandler(eng, collector, state)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	wsHandler.RegisterRoutes(mux)

	mux.Handle("/", http.FileServer(http.FS(web.StaticFS())))

	addr := ":8080"
	log.Printf("nil-loader started at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
