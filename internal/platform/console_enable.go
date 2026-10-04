package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

// EnableConsole changes only the input pipe option in the last applied workload.
func (h *Handler) EnableConsole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin console activation")
		return
	}
	defer tx.Rollback(r.Context())
	var server, node, runtime string
	var online, enabled bool
	err = tx.QueryRow(r.Context(), `SELECT s.server_id,v.node_id,COALESCE(s.runtime_id,''),COALESCE(n.runtime_available AND n.last_seen_at>NOW()-INTERVAL '90 seconds',false),s.console_input_enabled FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE s.id=$1 AND s.template='minecraft-java' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF s`, id, accounts.Scope(r.Context())).Scan(&server, &node, &runtime, &online, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "Minecraft service not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read service")
		return
	}
	if enabled {
		httpx.JSON(w, 200, map[string]bool{"enabled": true})
		return
	}
	if !online || runtime == "" {
		httpx.Error(w, 409, "node must be online and server provisioned")
		return
	}
	var active bool
	if tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM provisioning_jobs WHERE service_id=$1 AND status IN ('pending','processing'))`, id).Scan(&active) != nil {
		httpx.Error(w, 500, "failed to read operations")
		return
	}
	if active {
		httpx.Error(w, 409, "wait for the current operation")
		return
	}
	var raw []byte
	if tx.QueryRow(r.Context(), `SELECT payload FROM provisioning_jobs WHERE service_id=$1 AND job_type IN ('create_container','configure_minecraft') AND status='completed' ORDER BY created_at DESC LIMIT 1`, id).Scan(&raw) != nil {
		httpx.Error(w, 409, "no applied workload is available")
		return
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		httpx.Error(w, 409, "invalid workload snapshot")
		return
	}
	env, ok := payload["environment"].([]any)
	if !ok {
		httpx.Error(w, 409, "no Minecraft environment is available")
		return
	}
	next := []string{}
	for _, v := range env {
		if value, ok := v.(string); ok && !strings.HasPrefix(value, "CREATE_CONSOLE_IN_PIPE=") {
			next = append(next, value)
		}
	}
	payload["environment"] = append(next, "CREATE_CONSOLE_IN_PIPE=true")
	payload["runtime_id"] = runtime
	job, err := enqueue(r.Context(), tx, node, server, id, "configure_minecraft", payload)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE services SET status='installing',error_message=NULL,updated_at=NOW() WHERE id=$1`, id)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to queue console activation")
		return
	}
	httpx.JSON(w, 202, map[string]string{"job_id": job, "status": "installing"})
}
