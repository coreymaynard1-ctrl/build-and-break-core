package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func (h *Handler) MinecraftCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	var req struct {
		Command string `json:"command"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" || len(req.Command) > 512 || strings.ContainsAny(req.Command, "\x00\r\n") {
		httpx.Error(w, 400, "enter one Minecraft command (maximum 512 bytes)")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin command")
		return
	}
	defer tx.Rollback(r.Context())
	var node, server, runtime string
	var ready bool
	err = tx.QueryRow(r.Context(), `SELECT v.node_id,s.server_id,COALESCE(s.runtime_id,''),COALESCE(s.status='running' AND s.console_input_enabled AND n.runtime_available AND n.last_seen_at>NOW()-INTERVAL '90 seconds',false) FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE s.id=$1 AND s.template='minecraft-java' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF s`, id, accounts.Scope(r.Context())).Scan(&node, &server, &runtime, &ready)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "Minecraft service not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read service")
		return
	}
	if !ready || runtime == "" {
		httpx.Error(w, 409, "enable command input and wait for server startup")
		return
	}
	var active bool
	if tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM provisioning_jobs WHERE service_id=$1 AND status IN ('pending','processing'))`, id).Scan(&active) != nil {
		httpx.Error(w, 500, "failed to check operations")
		return
	}
	if active {
		httpx.Error(w, 409, "wait for the current operation")
		return
	}
	job, err := enqueue(r.Context(), tx, node, server, id, "minecraft_command", map[string]string{"runtime_id": runtime, "command": req.Command})
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to queue command")
		return
	}
	httpx.JSON(w, 202, map[string]string{"job_id": job, "status": "pending"})
}
