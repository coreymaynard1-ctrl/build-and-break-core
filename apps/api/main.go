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

	// Connect Build & Break Core to PostgreSQL.
	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	log.Println("Connected to PostgreSQL")

	// Give the node handlers access to our PostgreSQL connection.
	nodeHandler := &nodes.Handler{
		DB: db,
	}

	// Create the HTTP router.
	mux := http.NewServeMux()

	// Core health endpoint.
	mux.HandleFunc(
		"GET /api/v1/health",
		healthHandler,
	)

	// Return all registered Build & Break nodes.
	mux.HandleFunc(
		"GET /api/v1/nodes",
		nodeHandler.List,
	)

	// Allow a new bnb-agent to enroll using a valid
	// one-time enrollment token.
	mux.HandleFunc(
		"POST /api/v1/nodes/enroll",
		nodeHandler.Enroll,
	)

	// Configure the Core HTTP server.
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
