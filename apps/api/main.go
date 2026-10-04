package main

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/catalogue"
	"buildandbreak/platform/internal/platform"
	serviceStatus "buildandbreak/platform/internal/status"
	"buildandbreak/platform/internal/support"
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
	statusHandler := &serviceStatus.Handler{DB: db, StartedAt: time.Now()}
	mux.HandleFunc("GET /api/v1/status", statusHandler.Public)
	webOrigins := os.Getenv("BNB_WEB_ORIGIN")
	if webOrigins == "" {
		webOrigins = "http://127.0.0.1:5173,http://localhost:5173"
	}
	accountHandler := &accounts.Handler{DB: db, SecureCookies: os.Getenv("BNB_SECURE_COOKIES") == "1" || os.Getenv("BNB_PAYMENT_MODE") == "live", AllowedOrigin: webOrigins}
	mux.HandleFunc("GET /api/v1/auth/session", accountHandler.Me)
	mux.HandleFunc("POST /api/v1/auth/register", accountHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", accountHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", accountHandler.Protect(false, accountHandler.Logout))
	mux.HandleFunc("GET /api/v1/locations", nodeHandler.Locations)
	mux.HandleFunc("GET /api/v1/admin/accounts", accountHandler.Protect(true, accountHandler.List))
	mux.HandleFunc("POST /api/v1/admin/incidents", accountHandler.Protect(true, statusHandler.Create))
	mux.HandleFunc("PATCH /api/v1/admin/incidents/{incidentID}", accountHandler.Protect(true, statusHandler.Update))
	supportHandler := &support.Handler{DB: db}
	mux.HandleFunc("GET /api/v1/support/tickets", accountHandler.Protect(false, supportHandler.List))
	mux.HandleFunc("POST /api/v1/support/tickets", accountHandler.Protect(false, supportHandler.Create))
	mux.HandleFunc("GET /api/v1/support/tickets/{ticketID}", accountHandler.Protect(false, supportHandler.Read))
	mux.HandleFunc("PATCH /api/v1/support/tickets/{ticketID}", accountHandler.Protect(false, supportHandler.Update))

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

	mux.HandleFunc("GET /api/v1/catalogue", catalogue.List)

	// List nodes known to Core.
	mux.HandleFunc(
		"GET /api/v1/nodes",
		accountHandler.Protect(true, nodeHandler.List),
	)

	mux.HandleFunc("PUT /api/v1/admin/nodes/{id}/address", accountHandler.Protect(true, nodeHandler.UpdateAddress))
	mux.HandleFunc("POST /api/v1/admin/node-enrollment", accountHandler.Protect(true, nodeHandler.CreateEnrollment))

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
	mux.HandleFunc("POST /api/v1/deployments", accountHandler.Protect(true, platformHandler.Deploy))
	mux.HandleFunc("GET /api/v1/servers", accountHandler.Protect(false, platformHandler.Servers))
	mux.HandleFunc("DELETE /api/v1/servers/{serverID}", accountHandler.Protect(false, platformHandler.DeleteServer))
	mux.HandleFunc("POST /api/v1/servers", accountHandler.Protect(true, platformHandler.CreateServer))
	mux.HandleFunc("GET /api/v1/services", accountHandler.Protect(false, platformHandler.Services))
	mux.HandleFunc("POST /api/v1/services", accountHandler.Protect(false, platformHandler.CreateService))
	mux.HandleFunc("POST /api/v1/services/{serviceID}/{action}", accountHandler.Protect(false, platformHandler.Action))
	mux.HandleFunc("GET /api/v1/services/{serviceID}/minecraft", accountHandler.Protect(false, platformHandler.Minecraft))
	mux.HandleFunc("GET /api/v1/minecraft/releases", accountHandler.Protect(false, platformHandler.MinecraftReleases))
	mux.HandleFunc("GET /api/v1/minecraft/paper-builds", accountHandler.Protect(false, platformHandler.PaperBuilds))
	mux.HandleFunc("POST /api/v1/services/{serviceID}/minecraft/console", accountHandler.Protect(false, platformHandler.EnableConsole))
	mux.HandleFunc("POST /api/v1/services/{serviceID}/minecraft/command", accountHandler.Protect(false, platformHandler.MinecraftCommand))
	mux.HandleFunc("GET /api/v1/services/{serviceID}/minecraft/catalogue", accountHandler.Protect(false, platformHandler.MinecraftCatalogue))
	mux.HandleFunc("PUT /api/v1/services/{serviceID}/minecraft", accountHandler.Protect(false, platformHandler.Minecraft))
	mux.HandleFunc("DELETE /api/v1/services/{serviceID}", accountHandler.Protect(false, platformHandler.Action))
	mux.HandleFunc("GET /api/v1/templates", platformHandler.Templates)
	mux.HandleFunc("GET /api/v1/services/{serviceID}/bedrock/addons", accountHandler.Protect(false, platformHandler.BedrockAddons))
	mux.HandleFunc("POST /api/v1/services/{serviceID}/bedrock/addons", accountHandler.Protect(false, platformHandler.BedrockAddons))
	mux.HandleFunc("PUT /api/v1/services/{serviceID}/bedrock/addons/{addonID}", accountHandler.Protect(false, platformHandler.BedrockAddons))
	mux.HandleFunc("GET /api/v1/nodes/{id}/bedrock-artifacts/{artifactID}", platformHandler.BedrockArtifact)
	mux.HandleFunc("POST /api/v1/services/{serviceID}/files", accountHandler.Protect(false, platformHandler.Files))
	mux.HandleFunc("GET /api/v1/jobs/{jobID}", accountHandler.Protect(false, platformHandler.Operation))
	mux.HandleFunc("GET /api/v1/jobs", accountHandler.Protect(false, platformHandler.Jobs))
	mux.HandleFunc("GET /api/v1/nodes/{id}/services", jobHandler.Inventory)
	mux.HandleFunc("POST /api/v1/nodes/{id}/services/observations", jobHandler.Observe)
	billingHandler := &billing.Handler{DB: db, Sandbox: os.Getenv("BNB_BILLING_SANDBOX") == "1"}
	billingHandler.Configure()
	go billingHandler.RunFulfillment(ctx)
	mux.HandleFunc("GET /api/v1/admin/plans", accountHandler.Protect(true, billingHandler.AdminPlans))
	mux.HandleFunc("PUT /api/v1/admin/plans/{planID}", accountHandler.Protect(true, billingHandler.SavePlan))
	mux.HandleFunc("GET /api/v1/billing/config", billingHandler.Config)
	mux.HandleFunc("GET /api/v1/billing/plans", billingHandler.Plans)
	mux.HandleFunc("GET /api/v1/billing/invoices", accountHandler.Protect(false, billingHandler.Invoices))
	mux.HandleFunc("POST /api/v1/billing/orders", accountHandler.Protect(false, billingHandler.Order))
	mux.HandleFunc("POST /api/v1/billing/invoices/{invoiceID}/pay-sandbox", accountHandler.Protect(true, billingHandler.PaySandbox))
	mux.HandleFunc("POST /api/v1/billing/invoices/{invoiceID}/checkout", accountHandler.Protect(false, billingHandler.Checkout))
	mux.HandleFunc("POST /api/v1/billing/invoices/{invoiceID}/capture-paypal", accountHandler.Protect(false, billingHandler.CapturePayPal))
	mux.HandleFunc("POST /api/v1/billing/webhooks/{provider}", billingHandler.Webhook)
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
		WriteTimeout: 30 * time.Second,
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
