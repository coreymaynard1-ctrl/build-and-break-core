package support

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ DB *pgxpool.Pool }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT row_to_json(q) FROM (SELECT t.id,t.subject,t.category,t.status,t.created_at,t.updated_at,a.name AS customer_name FROM support_tickets t JOIN accounts a ON a.id=t.account_id WHERE ($1::uuid IS NULL OR t.account_id=$1) ORDER BY t.updated_at DESC LIMIT 100) q`, accounts.Scope(r.Context()))
	if err != nil {
		httpx.Error(w, 500, "failed to load tickets")
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			httpx.Error(w, 500, "failed to read tickets")
			return
		}
		out = append(out, json.RawMessage(raw))
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read tickets")
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("ticketID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid ticket ID")
		return
	}
	var raw []byte
	err := h.DB.QueryRow(r.Context(), `SELECT row_to_json(q) FROM (SELECT t.id,t.subject,t.category,t.status,t.created_at,t.updated_at,COALESCE((SELECT json_agg(m ORDER BY m.created_at,m.id) FROM (SELECT m.id,m.message,m.created_at,a.name AS author,a.role AS author_role FROM support_messages m JOIN accounts a ON a.id=m.account_id WHERE m.ticket_id=t.id) m),'[]'::json) AS messages FROM support_tickets t WHERE t.id=$1 AND ($2::uuid IS NULL OR t.account_id=$2)) q`, id, accounts.Scope(r.Context())).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "ticket not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read ticket")
		return
	}
	httpx.JSON(w, 200, json.RawMessage(raw))
}
func validMessage(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 10000 && !strings.ContainsRune(s, 0)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject  string `json:"subject"`
		Category string `json:"category"`
		Message  string `json:"message"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Message = strings.TrimSpace(req.Message)
	if req.Subject == "" || len(req.Subject) > 160 || strings.ContainsAny(req.Subject, "\x00\n\r") || !validMessage(req.Message) || (req.Category != "technical" && req.Category != "billing" && req.Category != "account" && req.Category != "other") {
		httpx.Error(w, 400, "subject, category and message required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to create ticket")
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	owner := accounts.Current(r.Context()).ID
	err = tx.QueryRow(r.Context(), `INSERT INTO support_tickets(account_id,subject,category) VALUES($1,$2,$3) RETURNING id`, owner, req.Subject, req.Category).Scan(&id)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO support_messages(ticket_id,account_id,message) VALUES($1,$2,$3)`, id, owner, req.Message)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to save ticket")
		return
	}
	httpx.JSON(w, 201, map[string]string{"id": id, "status": "open"})
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("ticketID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid ticket ID")
		return
	}
	var req struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if (req.Message == "" && req.Status == "") || (req.Message != "" && !validMessage(req.Message)) || (req.Status != "" && req.Status != "open" && req.Status != "closed") {
		httpx.Error(w, 400, "message or open/closed status required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to update ticket")
		return
	}
	defer tx.Rollback(r.Context())
	var current string
	err = tx.QueryRow(r.Context(), `SELECT status FROM support_tickets WHERE id=$1 AND ($2::uuid IS NULL OR account_id=$2) FOR UPDATE`, id, accounts.Scope(r.Context())).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "ticket not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read ticket")
		return
	}
	if req.Message != "" && current == "closed" && req.Status != "open" {
		httpx.Error(w, 409, "reopen the ticket before replying")
		return
	}
	next := current
	if req.Message != "" {
		_, err = tx.Exec(r.Context(), `INSERT INTO support_messages(ticket_id,account_id,message) VALUES($1,$2,$3)`, id, accounts.Current(r.Context()).ID, req.Message)
		next = "open"
		if accounts.IsAdmin(r.Context()) {
			next = "answered"
		}
	}
	if req.Status != "" {
		next = req.Status
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE support_tickets SET status=$2,updated_at=NOW() WHERE id=$1`, id, next)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to save ticket update")
		return
	}
	httpx.JSON(w, 200, map[string]string{"id": id, "status": next})
}
