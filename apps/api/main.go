package main

import (
	"buildandbreak/platform/internal/platform"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"buildandbreak/platform/internal/billing"
	"buildandbreak/platform/internal/database"
	"buildandbreak/platform/internal/jobs"
	"buildandbreak/platform/internal/nodes"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	response := HealthResponse{
		Status:    "ok",
		Service:   "build-and-break-core",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(
			w,
			"failed to encode response",
			http.StatusInternalServerError,
		)
	}
}

func main() {
	ctx := context.Background()

	// Connect Core to PostgreSQL.
	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatalf(
			"database connection failed: %v",
			err,
		)
	}
	defer db.Close()

	log.Println("Connected to PostgreSQL")

	// Node-related API handlers.
	nodeHandler := &nodes.Handler{
		DB: db,
	}

	// Provisioning-job API handlers.
	jobHandler := &jobs.Handler{
		DB: db,
	}

	mux := http.NewServeMux()

	// ---------------------------------------------------------
	// Core
	// ---------------------------------------------------------

	// Basic health endpoint.
	mux.HandleFunc(
		"GET /api/v1/health",
		healthHandler,
	)

	// ---------------------------------------------------------
	// Nodes
	// ---------------------------------------------------------

	// List nodes known to Core.
	mux.HandleFunc(
		"GET /api/v1/nodes",
		nodeHandler.List,
	)

	// One-time node enrollment.
	//
	// A new bnb-agent uses an enrollment token here to obtain its
	// permanent node identity and node credential.
	mux.HandleFunc(
		"POST /api/v1/nodes/enroll",
		nodeHandler.Enroll,
	)

	// Authenticated node heartbeat.
	//
	// This tells Core that the physical node is alive and reports
	// its current resources and Docker runtime information.
	mux.HandleFunc(
		"POST /api/v1/nodes/{id}/heartbeat",
		nodeHandler.Heartbeat,
	)

	// ---------------------------------------------------------
	// Provisioning jobs
	// ---------------------------------------------------------

	// Claim the next pending job assigned to this node.
	//
	// The agent polls this endpoint. Core authenticates the node
	// before allowing it to receive provisioning instructions.
	mux.HandleFunc(
		"POST /api/v1/nodes/{id}/jobs/claim",
		jobHandler.Claim,
	)

	// Report the final result of a provisioning job.
	//
	// A job must already belong to this authenticated node and
	// must currently be in the "processing" state.
	//
	// Valid transitions:
	//
	// processing -> completed
	// processing -> failed
	mux.HandleFunc(
		"POST /api/v1/nodes/{id}/jobs/{jobID}/result",
		jobHandler.Result,
	)

	platformHandler := &platform.Handler{DB: db}
	mux.HandleFunc("POST /api/v1/deployments", platformHandler.Deploy)
	mux.HandleFunc("GET /api/v1/servers", platformHandler.Servers)
	mux.HandleFunc("DELETE /api/v1/servers/{serverID}", platformHandler.DeleteServer)
	mux.HandleFunc("POST /api/v1/servers", platformHandler.CreateServer)
	mux.HandleFunc("GET /api/v1/services", platformHandler.Services)
	mux.HandleFunc("POST /api/v1/services", platformHandler.CreateService)
	mux.HandleFunc("POST /api/v1/services/{serviceID}/{action}", platformHandler.Action)
	mux.HandleFunc("DELETE /api/v1/services/{serviceID}", platformHandler.Action)
	mux.HandleFunc("GET /api/v1/templates", platformHandler.Templates)
	mux.HandleFunc("GET /api/v1/jobs", platformHandler.Jobs)
	mux.HandleFunc("GET /api/v1/nodes/{id}/services", jobHandler.Inventory)
	mux.HandleFunc("POST /api/v1/nodes/{id}/services/observations", jobHandler.Observe)
	billingHandler := &billing.Handler{DB: db, Sandbox: os.Getenv("BNB_BILLING_SANDBOX") == "1"}
	mux.HandleFunc("GET /api/v1/billing/config", billingHandler.Config)
	mux.HandleFunc("GET /api/v1/billing/plans", billingHandler.Plans)
	mux.HandleFunc("GET /api/v1/billing/invoices", billingHandler.Invoices)
	mux.HandleFunc("POST /api/v1/billing/orders", billingHandler.Order)
	mux.HandleFunc("POST /api/v1/billing/invoices/{invoiceID}/pay-sandbox", billingHandler.PaySandbox)
	// The production build can be served by Core itself, on the same origin.
	if _, err := os.Stat("apps/web/dist/index.html"); err == nil {
		mux.Handle("/", http.FileServer(http.Dir("apps/web/dist")))
	}
	addr := os.Getenv("BNB_CORE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	// ---------------------------------------------------------
	// HTTP server
	// ---------------------------------------------------------

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Println(
		"Build & Break Core starting on " + addr,
	)

	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {

		log.Fatal(err)
	}
}
