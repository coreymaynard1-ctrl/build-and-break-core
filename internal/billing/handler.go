// Package billing is an explicitly sandbox-only local order/invoice ledger.
// It cannot collect money or accept payment claims from browser redirects.
package billing

import (
	"buildandbreak/platform/internal/httpx"
	"buildandbreak/platform/internal/platform"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/mail"
	"strings"
)

type Handler struct {
	DB      *pgxpool.Pool
	Sandbox bool
}

func (h *Handler) enabled(w http.ResponseWriter) bool {
	if !h.Sandbox {
		httpx.Error(w, 503, "sandbox billing is disabled; no live payment provider is configured")
		return false
	}
	return true
}
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, map[string]any{"mode": "sandbox", "enabled": h.Sandbox, "collects_money": false, "recurring_billing": false})
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request, sql string) {
	if !h.enabled(w) {
		return
	}
	rows, err := h.DB.Query(r.Context(), sql)
	if err != nil {
		httpx.Error(w, 500, "failed to query ledger")
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if rows.Scan(&b) != nil {
			httpx.Error(w, 500, "failed to read ledger")
			return
		}
		out = append(out, json.RawMessage(b))
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read ledger")
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) Plans(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, `SELECT row_to_json(p) FROM billing_plans p WHERE active ORDER BY amount_minor`)
}
func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, `SELECT row_to_json(q) FROM (SELECT i.*,o.name,o.service_id,o.server_id,o.plan_snapshot FROM billing_invoices i JOIN billing_orders o ON o.id=i.order_id ORDER BY i.created_at DESC LIMIT 100) q`)
}

type OrderRequest struct {
	PlanID     string `json:"plan_id"`
	NodeID     string `json:"node_id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Port       int    `json:"port,omitempty"`
	AcceptEULA bool   `json:"accept_eula,omitempty"`
}
type plan struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount_minor"`
	Memory   int    `json:"memory_mb"`
	Disk     int    `json:"disk_mb"`
	CPU      int    `json:"cpu_threads"`
	Template string `json:"template"`
}

func validOrder(req OrderRequest, key string) bool {
	email, err := mail.ParseAddress(req.Email)
	return err == nil && email.Address == req.Email && len(req.Email) <= 254 && httpx.UUID(req.NodeID) && strings.TrimSpace(req.Name) != "" && len(req.Name) <= 100 && !strings.ContainsAny(req.Name, "\x00\n\r") && len(key) >= 8 && len(key) <= 128 && !strings.ContainsAny(key, "\n\r")
}
func (h *Handler) Order(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	var req OrderRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	key := r.Header.Get("Idempotency-Key")
	if !validOrder(req, key) {
		httpx.Error(w, 400, "valid name, email, node UUID and Idempotency-Key (8–128 characters) are required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin order")
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize equal keys before inspecting or inserting. A different request with
	// the same key cannot create a second invoice or change the order.
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key)
	if err != nil {
		httpx.Error(w, 500, "failed to lock order")
		return
	}
	body, _ := json.Marshal(req)
	var orderID, invoiceID, status string
	var previous []byte
	err = tx.QueryRow(r.Context(), `SELECT o.id,i.id,o.request,i.status FROM billing_orders o JOIN billing_invoices i ON i.order_id=o.id WHERE o.idempotency_key=$1`, key).Scan(&orderID, &invoiceID, &previous, &status)
	if err == nil {
		var old OrderRequest
		if json.Unmarshal(previous, &old) != nil || old != req {
			httpx.Error(w, 409, "idempotency key was used for a different order")
			return
		}
		httpx.JSON(w, 200, map[string]string{"order_id": orderID, "invoice_id": invoiceID, "status": status})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 500, "failed to read order")
		return
	}
	var p plan
	err = tx.QueryRow(r.Context(), `SELECT id,name,currency,amount_minor,memory_mb,disk_mb,cpu_threads,template FROM billing_plans WHERE id=$1 AND active`, req.PlanID).Scan(&p.ID, &p.Name, &p.Currency, &p.Amount, &p.Memory, &p.Disk, &p.CPU, &p.Template)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "plan not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read plan")
		return
	}
	if p.Template == "minecraft-java" && (!req.AcceptEULA || req.Port < 1024 || req.Port > 65535) {
		httpx.Error(w, 400, "Minecraft requires EULA acceptance and valid game port")
		return
	}
	if p.Template == "alpine-test" && (req.Port != 0 || req.AcceptEULA) {
		httpx.Error(w, 400, "Alpine does not use port or EULA fields")
		return
	}
	var exists bool
	if tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM nodes WHERE id=$1)`, req.NodeID).Scan(&exists) != nil || !exists {
		httpx.Error(w, 404, "node not found")
		return
	}
	snapshot, _ := json.Marshal(p)
	err = tx.QueryRow(r.Context(), `INSERT INTO billing_orders(id,idempotency_key,request,plan_id,plan_snapshot,customer_email,name,node_id,port) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, key, body, p.ID, snapshot, req.Email, req.Name, req.NodeID, req.Port).Scan(&orderID)
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO billing_invoices(id,order_id,currency,amount_minor) VALUES(gen_random_uuid(),$1,$2,$3) RETURNING id`, orderID, p.Currency, p.Amount).Scan(&invoiceID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to create invoice")
		return
	}
	httpx.JSON(w, 201, map[string]string{"order_id": orderID, "invoice_id": invoiceID, "status": "unpaid"})
}
func (h *Handler) PaySandbox(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	id := r.PathValue("invoiceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid invoice ID")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin sandbox payment")
		return
	}
	defer tx.Rollback(r.Context())
	var status, orderID, nodeID, name string
	var snapshot []byte
	var port int
	var serviceID, serverID *string
	err = tx.QueryRow(r.Context(), `SELECT i.status,o.id,o.node_id,o.name,o.plan_snapshot,COALESCE(o.port,0),o.service_id,o.server_id FROM billing_invoices i JOIN billing_orders o ON o.id=i.order_id WHERE i.id=$1 FOR UPDATE OF i,o`, id).Scan(&status, &orderID, &nodeID, &name, &snapshot, &port, &serviceID, &serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "invoice not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read invoice")
		return
	}
	if status == "paid" {
		httpx.JSON(w, 200, map[string]any{"status": "paid", "service_id": serviceID, "server_id": serverID, "mode": "sandbox"})
		return
	}
	if status != "unpaid" {
		httpx.Error(w, 409, "invoice is not payable")
		return
	}
	var p plan
	if json.Unmarshal(snapshot, &p) != nil {
		httpx.Error(w, 500, "invalid plan snapshot")
		return
	}
	server, service, job, err := platform.Fulfill(r.Context(), tx, nodeID, name, p.Template, p.Memory, p.Disk, p.CPU, port)
	if err != nil {
		httpx.Error(w, 409, err.Error())
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO billing_payments(id,invoice_id,provider,provider_reference,currency,amount_minor) VALUES(gen_random_uuid(),$1,'sandbox',$2,$3,$4)`, id, "sandbox-"+id, p.Currency, p.Amount)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE billing_invoices SET status='paid',paid_at=NOW() WHERE id=$1`, id)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE billing_orders SET status='paid',server_id=$2,service_id=$3 WHERE id=$1`, orderID, server, service)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to commit sandbox payment")
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "paid", "mode": "sandbox", "server_id": server, "service_id": service, "job_id": job})
}
