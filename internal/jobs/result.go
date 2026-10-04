package jobs

import (
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

type ResultRequest struct {
	Data          string `json:"data,omitempty"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message,omitempty"`
	RuntimeID     string `json:"runtime_id,omitempty"`
	ServiceStatus string `json:"service_status,omitempty"`
}

func (h *Handler) Result(w http.ResponseWriter, r *http.Request) {
	if !httpx.AuthNode(w, r, h.DB) {
		return
	}
	jobID := r.PathValue("jobID")
	if !httpx.UUID(jobID) {
		httpx.Error(w, 400, "invalid job ID")
		return
	}
	var req ResultRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Status != "completed" && req.Status != "failed" {
		httpx.Error(w, 400, "status must be completed or failed")
		return
	}
	if len(req.Data) > 49152 || (req.Data != "" && !json.Valid([]byte(req.Data))) {
		httpx.Error(w, 400, "invalid file result")
		return
	}
	if len(req.RuntimeID) > 128 || len(req.ErrorMessage) > 4096 {
		httpx.Error(w, 400, "result exceeds field limits")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin result")
		return
	}
	defer tx.Rollback(r.Context())
	var status, kind string
	var serviceID, serverID *string
	var previous []byte
	err = tx.QueryRow(r.Context(), `SELECT status,job_type,service_id,server_id,result FROM provisioning_jobs WHERE id=$1 AND node_id=$2 FOR UPDATE`, jobID, r.PathValue("id")).Scan(&status, &kind, &serviceID, &serverID, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "job not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read job")
		return
	}
	if status != "processing" {
		var old ResultRequest
		if json.Unmarshal(previous, &old) == nil && old == req {
			httpx.JSON(w, 200, map[string]string{"status": status})
			return
		}
		httpx.Error(w, 409, "job is not processing or result differs")
		return
	}
	managed := kind == "install_bedrock_addons" || kind == "configure_bedrock_addons" || kind == "configure_minecraft" || kind == "create_container" || kind == "start_service" || kind == "stop_service" || kind == "restart_service" || kind == "delete_service"
	if managed {
		if serviceID == nil || serverID == nil {
			httpx.Error(w, 409, "job has no allocation identity")
			return
		}
		if req.Status == "completed" && !validCompletedResult(kind, req) {
			httpx.Error(w, 400, "invalid runtime identity or service status for operation")
			return
		}
		var runtimeID *string
		err = tx.QueryRow(r.Context(), `SELECT s.runtime_id FROM services s JOIN servers v ON v.id=s.server_id WHERE s.id=$1 AND s.server_id=$2 AND v.node_id=$3 FOR UPDATE OF s`, *serviceID, *serverID, r.PathValue("id")).Scan(&runtimeID)
		if err != nil {
			httpx.Error(w, 409, "service allocation does not match job")
			return
		}
		if kind != "create_container" && kind != "configure_minecraft" && (runtimeID == nil || (req.RuntimeID != "" && req.RuntimeID != *runtimeID)) {
			httpx.Error(w, 409, "runtime identity does not match service")
			return
		}
		if kind == "create_container" && runtimeID != nil && req.RuntimeID != "" && *runtimeID != req.RuntimeID {
			httpx.Error(w, 409, "service already has a different runtime")
			return
		}
		if kind == "configure_minecraft" {
			var expected string
			if tx.QueryRow(r.Context(), `SELECT payload->>'runtime_id' FROM provisioning_jobs WHERE id=$1`, jobID).Scan(&expected) != nil || runtimeID == nil || *runtimeID != expected {
				httpx.Error(w, 409, "original runtime identity does not match configuration job")
				return
			}
		}
		next := req.ServiceStatus
		if req.Status == "failed" {
			next = "error"
		}
		var rid any = runtimeID
		if req.RuntimeID != "" {
			rid = req.RuntimeID
		}
		if next == "deleted" {
			rid = nil
		}
		_, err = tx.Exec(r.Context(), `UPDATE services SET runtime_id=$2,status=$3,error_message=$4,observed_status=CASE WHEN $5='completed' THEN $3 ELSE observed_status END,observed_at=CASE WHEN $5='completed' THEN NOW() ELSE observed_at END,deleted_at=CASE WHEN $3='deleted' THEN NOW() ELSE deleted_at END,updated_at=NOW() WHERE id=$1`, *serviceID, rid, next, nullableError(req.Status, req.ErrorMessage), req.Status)
		if err == nil && (kind == "create_container" || kind == "configure_minecraft") && req.Status == "completed" {
			_, err = tx.Exec(r.Context(), `UPDATE services SET console_input_enabled=COALESCE((SELECT payload->'environment' @> '["CREATE_CONSOLE_IN_PIPE=true"]'::jsonb FROM provisioning_jobs WHERE id=$2),false) WHERE id=$1`, *serviceID, jobID)
		}
		if err == nil && (kind == "configure_minecraft" || kind == "create_container") && req.Status == "completed" {
			_, err = tx.Exec(r.Context(), `UPDATE services SET minecraft_applied_config=(SELECT payload->'minecraft_config' FROM provisioning_jobs WHERE id=$2) WHERE id=$1`, *serviceID, jobID)
		}
		if err == nil && next == "deleted" {
			_, err = tx.Exec(r.Context(), `DELETE FROM service_ports WHERE service_id=$1`, *serviceID)
		}
		if err != nil {
			httpx.Error(w, 500, "failed to update service")
			return
		}
	}
	body, _ := json.Marshal(req)
	_, err = tx.Exec(r.Context(), `UPDATE provisioning_jobs SET status=$2,error_message=$3,result=$4,completed_at=NOW(),updated_at=NOW() WHERE id=$1`, jobID, req.Status, nullableError(req.Status, req.ErrorMessage), body)
	if err == nil && serviceID != nil && (kind == "install_bedrock_addons" || kind == "configure_bedrock_addons") {
		if kind == "install_bedrock_addons" {
			state := "error"
			if req.Status == "completed" {
				state = "installed"
			}
			_, err = tx.Exec(r.Context(), `UPDATE bedrock_addons SET state=$2 WHERE service_id=$1 AND artifact_id=(SELECT (payload->>'artifact_id')::uuid FROM provisioning_jobs WHERE id=$3)`, *serviceID, state, jobID)
		} else if req.Status == "completed" {
			_, err = tx.Exec(r.Context(), `UPDATE bedrock_addons SET applied_enabled=enabled WHERE service_id=$1`, *serviceID)
		}
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to commit result")
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": req.Status})
}
func validCompletedResult(kind string, r ResultRequest) bool {
	if strings.TrimSpace(r.RuntimeID) == "" {
		return false
	}
	switch kind {
	case "delete_service":
		return r.ServiceStatus == "deleted"
	case "stop_service":
		return r.ServiceStatus == "stopped"
	default:
		return r.ServiceStatus == "running" || r.ServiceStatus == "stopped"
	}
}
func nullableError(status, message string) any {
	if status != "failed" {
		return nil
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "job failed without an error message"
	}
	return message
}
