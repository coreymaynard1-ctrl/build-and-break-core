// Package status exposes measured component health and operator-authored incidents.
package status

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB        *pgxpool.Pool
	StartedAt time.Time
}
type component struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	components := []component{{"website", "Website & API", "operational", "Core is responding to requests."}}
	response := map[string]any{"checked_at": time.Now().UTC(), "core_uptime_seconds": int64(time.Since(h.StartedAt).Seconds()), "components": components, "incidents": []json.RawMessage{}}
	var reachable bool
	err := h.DB.QueryRow(r.Context(), `SELECT true`).Scan(&reachable)
	if err != nil {
		components = append(components, component{"database", "Database", "outage", "Core cannot reach the database."})
		response["components"] = components
		response["status"] = "outage"
		httpx.JSON(w, 503, response)
		return
	}
	components = append(components, component{"database", "Database", "operational", "Database query succeeded."})
	rows, err := h.DB.Query(r.Context(), `SELECT id,name,location,COALESCE(last_seen_at>=NOW()-INTERVAL '90 seconds',false),runtime_available FROM nodes ORDER BY name`)
	if err != nil {
		httpx.Error(w, 503, "status information unavailable")
		return
	}
	overall := "operational"
	nodeCount := 0
	for rows.Next() {
		var id, name, location string
		var online, runtime bool
		if rows.Scan(&id, &name, &location, &online, &runtime) != nil {
			rows.Close()
			httpx.Error(w, 503, "status information unavailable")
			return
		}
		nodeCount++
		state, detail := "operational", "Agent heartbeat and container runtime are healthy."
		if !online {
			state = "outage"
			detail = "Agent heartbeat is overdue."
			overall = "degraded"
		} else if !runtime {
			state = "degraded"
			detail = "Agent is online but container runtime is unavailable."
			overall = "degraded"
		}
		components = append(components, component{id, location + " · " + name, state, detail})
	}
	nodeErr := rows.Err()
	rows.Close()
	if nodeErr != nil {
		httpx.Error(w, 503, "status information unavailable")
		return
	}
	if nodeCount == 0 {
		components = append(components, component{"hosting", "Hosting nodes", "unavailable", "No hosting nodes are enrolled."})
		overall = "degraded"
	}
	var delayed int
	err = h.DB.QueryRow(r.Context(), `SELECT count(*) FROM provisioning_jobs WHERE job_type IN ('configure_minecraft','create_container','start_service','stop_service','restart_service','delete_service') AND ((status='pending' AND created_at<NOW()-INTERVAL '5 minutes') OR (status='processing' AND claimed_at<NOW()-INTERVAL '15 minutes'))`).Scan(&delayed)
	if err != nil {
		httpx.Error(w, 503, "queue status unavailable")
		return
	}
	queue := component{"queue", "Provisioning & lifecycle queue", "operational", "No overdue operations detected."}
	if delayed > 0 {
		queue.Status = "degraded"
		queue.Detail = "Some hosting operations are overdue and need operator review."
		overall = "degraded"
	}
	components = append(components, queue)
	rows, err = h.DB.Query(r.Context(), `SELECT row_to_json(q) FROM (SELECT i.id,i.title,i.message,i.impact,i.status,i.created_at,i.updated_at,i.resolved_at,COALESCE((SELECT json_agg(u ORDER BY u.created_at) FROM (SELECT message,status,created_at FROM status_incident_updates WHERE incident_id=i.id) u),'[]'::json) AS updates FROM status_incidents i WHERE i.status!='resolved' OR i.resolved_at>NOW()-INTERVAL '90 days' ORDER BY i.created_at DESC LIMIT 100) q`)
	if err != nil {
		httpx.Error(w, 503, "incident information unavailable")
		return
	}
	defer rows.Close()
	incidents := []json.RawMessage{}
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			httpx.Error(w, 503, "incident information unavailable")
			return
		}
		incidents = append(incidents, json.RawMessage(raw))
		var i struct{ Status string }
		json.Unmarshal(raw, &i)
		if i.Status != "resolved" {
			overall = "degraded"
		}
	}
	if rows.Err() != nil {
		httpx.Error(w, 503, "incident information unavailable")
		return
	}
	response["components"] = components
	response["incidents"] = incidents
	response["status"] = overall
	httpx.JSON(w, 200, response)
}

type incidentRequest struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Impact  string `json:"impact"`
	Status  string `json:"status"`
}

func validStatus(s string) bool {
	return s == "investigating" || s == "identified" || s == "monitoring" || s == "resolved"
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req incidentRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Message = strings.TrimSpace(req.Message)
	if req.Title == "" || len(req.Title) > 160 || req.Message == "" || len(req.Message) > 10000 || !validStatus(req.Status) || (req.Impact != "minor" && req.Impact != "major" && req.Impact != "maintenance") {
		httpx.Error(w, 400, "valid title, message, impact and incident status required")
		return
	}
	var id string
	err := h.DB.QueryRow(r.Context(), `INSERT INTO status_incidents(title,message,impact,status,created_by,resolved_at) VALUES($1,$2,$3,$4,$5,CASE WHEN $4='resolved' THEN NOW() END) RETURNING id`, req.Title, req.Message, req.Impact, req.Status, accounts.Current(r.Context()).ID).Scan(&id)
	if err != nil {
		httpx.Error(w, 500, "failed to create incident")
		return
	}
	httpx.JSON(w, 201, map[string]string{"id": id})
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("incidentID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid incident ID")
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
	if req.Message == "" || len(req.Message) > 10000 || !validStatus(req.Status) {
		httpx.Error(w, 400, "message and valid status required")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to update incident")
		return
	}
	defer tx.Rollback(r.Context())
	var current string
	err = tx.QueryRow(r.Context(), `SELECT status FROM status_incidents WHERE id=$1 FOR UPDATE`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "incident not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read incident")
		return
	}
	if current == "resolved" {
		httpx.Error(w, 409, "resolved incidents cannot be reopened; create a new incident")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO status_incident_updates(incident_id,message,status,created_by) VALUES($1,$2,$3,$4)`, id, req.Message, req.Status, accounts.Current(r.Context()).ID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE status_incidents SET status=$2,updated_at=NOW(),resolved_at=CASE WHEN $2='resolved' THEN NOW() END WHERE id=$1`, id, req.Status)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to save incident update")
		return
	}
	httpx.JSON(w, 200, map[string]string{"id": id, "status": req.Status})
}
