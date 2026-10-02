package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"buildandbreak/platform/internal/database"
	"buildandbreak/platform/internal/nodes"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:    "ok",
		Service:   "build-and-break-core",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func main() {
	ctx := context.Background()

	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	log.Println("Connected to PostgreSQL")

	nodeHandler := &nodes.Handler{
		DB: db,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", healthHandler)
	mux.HandleFunc("GET /api/v1/nodes", nodeHandler.List)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Println("Build & Break Core starting on port 8080")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
