package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"buildandbreak/platform/internal/serverfs"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (h *Handler) Files(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	var req serverfs.Request
	if !httpx.Decode(w, r, &req) {
		return
	}
	if err := serverfs.Validate(req); err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin file operation")
		return
	}
	defer tx.Rollback(r.Context())
	var node, server, runtime string
	var online bool
	err = tx.QueryRow(r.Context(), `SELECT v.node_id,s.server_id,COALESCE(s.runtime_id,''),COALESCE(n.runtime_available AND n.last_seen_at>NOW()-INTERVAL '90 seconds',false) FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE s.id=$1 AND s.template IN ('minecraft-java','minecraft-bedrock') AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF s`, id, accounts.Scope(r.Context())).Scan(&node, &server, &runtime, &online)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "managed game volume not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read server")
		return
	}
	if !online || runtime == "" {
		httpx.Error(w, 409, "node must be online and server provisioned")
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
	job, err := enqueue(r.Context(), tx, node, server, id, "server_files", map[string]any{"runtime_id": runtime, "request": req})
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to queue file operation")
		return
	}
	httpx.JSON(w, 202, map[string]string{"job_id": job, "status": "pending"})
}

func (h *Handler) Operation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("jobID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid operation ID")
		return
	}
	var data []byte
	err := h.DB.QueryRow(r.Context(), `SELECT row_to_json(q) FROM (SELECT j.id,j.status,j.error_message,j.result FROM provisioning_jobs j JOIN servers v ON v.id=j.server_id WHERE j.id=$1 AND ($2::uuid IS NULL OR v.account_id=$2)) q`, id, accounts.Scope(r.Context())).Scan(&data)
	if err != nil {
		httpx.Error(w, 404, "operation not found")
		return
	}
	httpx.JSON(w, 200, json.RawMessage(data))
}
