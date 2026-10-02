package jobs

import (
	"buildandbreak/platform/internal/httpx"
	"net/http"
	"time"
)

// Inventory only returns this authenticated node's assignments. Legacy identity
// recovery is restricted to the name recorded in a completed/failed create job.
func (h *Handler) Inventory(w http.ResponseWriter, r *http.Request) {
	if !httpx.AuthNode(w, r, h.DB) {
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT s.id,s.server_id,s.runtime_id,s.updated_at,(SELECT payload->>'name' FROM provisioning_jobs j WHERE j.service_id=s.id AND j.node_id=$1 AND j.job_type='create_container' AND j.status IN ('completed','failed') ORDER BY created_at DESC LIMIT 1) FROM services s JOIN servers v ON v.id=s.server_id WHERE v.node_id=$1 AND s.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM provisioning_jobs j WHERE j.service_id=s.id AND j.status IN ('pending','processing'))`, r.PathValue("id"))
	if err != nil {
		httpx.Error(w, 500, "failed to query assignments")
		return
	}
	defer rows.Close()
	type item struct {
		ID          string    `json:"id"`
		ServerID    string    `json:"server_id"`
		RuntimeID   *string   `json:"runtime_id"`
		Revision    time.Time `json:"revision"`
		RuntimeName *string   `json:"runtime_name"`
	}
	out := []item{}
	for rows.Next() {
		var i item
		if rows.Scan(&i.ID, &i.ServerID, &i.RuntimeID, &i.Revision, &i.RuntimeName) != nil {
			httpx.Error(w, 500, "failed to read assignments")
			return
		}
		out = append(out, i)
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read assignments")
		return
	}
	httpx.JSON(w, 200, out)
}

type Observation struct {
	ServiceID string    `json:"service_id"`
	RuntimeID string    `json:"runtime_id"`
	Status    string    `json:"status"`
	Revision  time.Time `json:"revision"`
}

func (h *Handler) Observe(w http.ResponseWriter, r *http.Request) {
	if !httpx.AuthNode(w, r, h.DB) {
		return
	}
	var req struct {
		Services []Observation `json:"services"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	if len(req.Services) > 200 {
		httpx.Error(w, 400, "maximum 200 observations per request")
		return
	}
	for _, s := range req.Services {
		if s.Revision.IsZero() || !httpx.UUID(s.ServiceID) || s.RuntimeID == "" || len(s.RuntimeID) > 128 || (s.Status != "running" && s.Status != "stopped" && s.Status != "missing" && s.Status != "unknown") {
			httpx.Error(w, 400, "invalid observation")
			return
		}
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin observation")
		return
	}
	defer tx.Rollback(r.Context())
	for _, s := range req.Services {
		// Lock before checking active jobs; action enqueue locks the same service.
		var runtimeID, name *string
		var status string
		var revision time.Time
		err = tx.QueryRow(r.Context(), `SELECT s.runtime_id,s.status,s.updated_at,(SELECT payload->>'name' FROM provisioning_jobs j WHERE j.service_id=s.id AND j.node_id=$2 AND j.job_type='create_container' AND j.status IN ('completed','failed') ORDER BY j.created_at DESC LIMIT 1) FROM services s JOIN servers v ON v.id=s.server_id WHERE s.id=$1 AND v.node_id=$2 AND s.deleted_at IS NULL FOR UPDATE OF s`, s.ServiceID, r.PathValue("id")).Scan(&runtimeID, &status, &revision, &name)
		if err != nil {
			httpx.Error(w, 409, "observation does not match assigned service")
			return
		}
		if !revision.Equal(s.Revision) {
			continue
		}
		if runtimeID != nil && *runtimeID != s.RuntimeID {
			httpx.Error(w, 409, "runtime identity mismatch")
			return
		}
		if runtimeID == nil && (name == nil || (s.Status != "running" && s.Status != "stopped")) {
			httpx.Error(w, 409, "service has no recoverable runtime identity")
			return
		}
		var active bool
		if tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM provisioning_jobs WHERE service_id=$1 AND status IN ('pending','processing'))`, s.ServiceID).Scan(&active) != nil {
			httpx.Error(w, 500, "failed to read jobs")
			return
		}
		if active {
			continue
		}
		// Failed jobs remain visible until an explicit lifecycle action succeeds.
		_, err = tx.Exec(r.Context(), `UPDATE services SET runtime_id=$2,observed_status=$3,observed_at=NOW(),status=CASE WHEN error_message IS NULL THEN $3 ELSE status END,updated_at=NOW() WHERE id=$1`, s.ServiceID, s.RuntimeID, s.Status)
		if err != nil {
			httpx.Error(w, 500, "failed to record observation")
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to commit observations")
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "ok"})
}
