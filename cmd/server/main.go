package main

import (
	"log"
	"net/http"
	"time"

	"github.com/nikitadada/nil-loader/internal/api"
	"github.com/nikitadada/nil-loader/internal/auth"
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

	authSvc := auth.NewServiceFromEnv(24 * time.Hour)
	handler := api.NewHandler(eng, state, collector, authSvc)
	wsHandler := api.NewWSHandler(eng, collector, state, authSvc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	wsHandler.RegisterRoutes(mux)

	mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("internal/docs"))))
	mux.Handle("/", http.FileServer(http.FS(web.StaticFS())))

	addr := ":8080"
	log.Println("nil-loader started")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
