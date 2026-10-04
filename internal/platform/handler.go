// Package platform is the runtime-agnostic development control plane.
package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
)

type Handler struct {
	DB                    *pgxpool.Pool
	MinecraftJavaResolver func(context.Context, string) (int, error)
}
type Template struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MinMemoryMB  int    `json:"min_memory_mb"`
	RequiresEULA bool   `json:"requires_eula"`
	RequiresPort bool   `json:"requires_port"`
}

var templates = []Template{{"alpine-test", "Alpine development workload", 32, false, false}, {"minecraft-java", "Minecraft Java", 1536, true, true}, {"minecraft-bedrock", "Minecraft Bedrock", 1024, true, true}}

func (h *Handler) Templates(w http.ResponseWriter, r *http.Request) { httpx.JSON(w, 200, templates) }
func (h *Handler) list(w http.ResponseWriter, r *http.Request, sql string, args ...any) {
	rows, err := h.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpx.Error(w, 500, "failed to query records")
		return
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			httpx.Error(w, 500, "failed to read records")
			return
		}
		out = append(out, json.RawMessage(b))
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read records")
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) Servers(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, `SELECT row_to_json(q) FROM (SELECT * FROM servers WHERE status!='deleted' AND ($1::uuid IS NULL OR account_id=$1) ORDER BY created_at) q`, accounts.Scope(r.Context()))
}
func (h *Handler) Services(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("server_id")
	if serverID != "" && !httpx.UUID(serverID) {
		httpx.Error(w, 400, "invalid server_id")
		return
	}
	h.list(w, r, `SELECT row_to_json(q) FROM (SELECT s.*,v.node_id,CASE WHEN n.last_seen_at >= NOW()-INTERVAL '90 seconds' THEN 'online' ELSE 'offline' END AS node_status,(SELECT j.status FROM provisioning_jobs j WHERE j.service_id=s.id ORDER BY j.created_at DESC LIMIT 1) AS job_status FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE ($1::uuid IS NULL OR s.server_id=$1) AND ($2::uuid IS NULL OR v.account_id=$2) AND s.deleted_at IS NULL ORDER BY s.created_at) q`, nullableID(serverID), accounts.Scope(r.Context()))
}
func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("service_id")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "valid service_id is required")
		return
	}
	h.list(w, r, `SELECT row_to_json(q) FROM (SELECT j.id,j.node_id,j.server_id,j.service_id,j.job_type,j.status,j.attempts,j.error_message,j.result,j.claimed_at,j.completed_at,j.created_at,j.updated_at FROM provisioning_jobs j JOIN servers v ON v.id=j.server_id WHERE j.service_id=$1 AND ($2::uuid IS NULL OR v.account_id=$2) ORDER BY j.created_at DESC LIMIT 50) q`, id, accounts.Scope(r.Context()))
}
func nullableID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

type CreateServerRequest struct {
	NodeID     string `json:"node_id"`
	Name       string `json:"name"`
	MemoryMB   int    `json:"memory_mb"`
	DiskMB     int    `json:"disk_mb"`
	CPUThreads int    `json:"cpu_threads"`
}

func validName(name string) bool {
	return strings.TrimSpace(name) != "" && len(name) <= 100 && !strings.ContainsAny(name, "\x00\n\r")
}
func validResources(memory, disk, cpu int) bool {
	return memory >= 32 && memory <= 1048576 && disk >= 128 && disk <= 1073741824 && cpu >= 1 && cpu <= 256
}
func (h *Handler) CreateServer(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !httpx.UUID(req.NodeID) || !validName(req.Name) || !validResources(req.MemoryMB, req.DiskMB, req.CPUThreads) {
		httpx.Error(w, 400, "invalid node, name, or resource limits")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin allocation")
		return
	}
	defer tx.Rollback(r.Context())
	var online, available bool
	var memory, disk int64
	var cpu int
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(last_seen_at>=NOW()-INTERVAL '90 seconds',false),runtime_available,total_memory,total_disk,cpu_threads FROM nodes WHERE id=$1 FOR UPDATE`, req.NodeID).Scan(&online, &available, &memory, &disk, &cpu)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "node not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read node")
		return
	}
	if !online || !available {
		httpx.Error(w, 409, "node or runtime is offline")
		return
	}
	var usedMem, usedDisk, usedCPU int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(memory_mb),0),COALESCE(SUM(disk_mb),0),COALESCE(SUM(cpu_threads),0) FROM servers WHERE node_id=$1 AND status!='deleted'`, req.NodeID).Scan(&usedMem, &usedDisk, &usedCPU); err != nil {
		httpx.Error(w, 500, "failed to read capacity")
		return
	}
	if (usedMem+int64(req.MemoryMB))*1048576 > memory || (usedDisk+int64(req.DiskMB))*1048576 > disk || usedCPU+int64(req.CPUThreads) > int64(cpu) {
		httpx.Error(w, 409, "insufficient unallocated node capacity")
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO servers(id,node_id,name,status,memory_mb,disk_mb,cpu_threads,account_id) VALUES(gen_random_uuid(),$1,$2,'active',$3,$4,$5,$6) RETURNING id`, req.NodeID, req.Name, req.MemoryMB, req.DiskMB, req.CPUThreads, accounts.Owner(r.Context())).Scan(&id)
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to create allocation")
		return
	}
	httpx.JSON(w, 201, map[string]string{"id": id, "status": "active"})
}

type CreateServiceRequest struct {
	ServerID   string `json:"server_id"`
	Name       string `json:"name"`
	Template   string `json:"template"`
	MemoryMB   int    `json:"memory_mb"`
	CPUThreads int    `json:"cpu_threads"`
	Port       int    `json:"port,omitempty"`
	AcceptEULA bool   `json:"accept_eula,omitempty"`
}

func validateService(req CreateServiceRequest) error {
	if !httpx.UUID(req.ServerID) || !validName(req.Name) || req.MemoryMB < 32 || req.MemoryMB > 1048576 || req.CPUThreads < 1 || req.CPUThreads > 256 {
		return fmt.Errorf("invalid server, name, or resource limits")
	}
	switch req.Template {
	case "alpine-test":
		if req.Port != 0 || req.AcceptEULA {
			return fmt.Errorf("Alpine does not use port or EULA fields")
		}
	case "minecraft-java":
		if req.MemoryMB < 1536 || req.Port < 1024 || req.Port > 65535 || !req.AcceptEULA {
			return fmt.Errorf("Minecraft requires at least 1536 MiB, a port from 1024 to 65535, and explicit EULA acceptance")
		}
	case "minecraft-bedrock":
		if req.MemoryMB < 1024 || req.Port < 1024 || req.Port > 65519 || !req.AcceptEULA {
			return fmt.Errorf("Bedrock requires 1024 MiB, an allocated UDP port and EULA acceptance")
		}
	default:
		return fmt.Errorf("unknown template")
	}
	return nil
}
func (h *Handler) CreateService(w http.ResponseWriter, r *http.Request) {
	var req CreateServiceRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validateService(req); err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin provisioning")
		return
	}
	defer tx.Rollback(r.Context())
	var nodeID string
	var mem, cpu int
	var online bool
	err = tx.QueryRow(r.Context(), `SELECT v.node_id,v.memory_mb,v.cpu_threads,COALESCE(n.last_seen_at>=NOW()-INTERVAL '90 seconds' AND n.runtime_available,false) FROM servers v JOIN nodes n ON n.id=v.node_id WHERE v.id=$1 AND v.status='active' AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF v`, req.ServerID, accounts.Scope(r.Context())).Scan(&nodeID, &mem, &cpu, &online)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "active server not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read server")
		return
	}
	if !online {
		httpx.Error(w, 409, "node or runtime is offline")
		return
	}
	var usedMem, usedCPU int64
	// Legacy services without recorded limits conservatively consume the allocation budget.
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(COALESCE(memory_mb,$2)),0),COALESCE(SUM(COALESCE(cpu_threads,$3)),0) FROM services WHERE server_id=$1 AND deleted_at IS NULL`, req.ServerID, mem, cpu).Scan(&usedMem, &usedCPU)
	if err != nil {
		httpx.Error(w, 500, "failed to read service budgets")
		return
	}
	if usedMem+int64(req.MemoryMB) > int64(mem) || usedCPU+int64(req.CPUThreads) > int64(cpu) {
		httpx.Error(w, 409, "service exceeds remaining server allocation")
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO services(id,server_id,name,template,status,memory_mb,cpu_threads,port) VALUES(gen_random_uuid(),$1,$2,$3,'installing',$4,$5,$6) RETURNING id`, req.ServerID, req.Name, req.Template, req.MemoryMB, req.CPUThreads, nullablePort(req.Port)).Scan(&id)
	if err != nil {
		httpx.Error(w, 500, "failed to create service")
		return
	}
	if req.Port != 0 {
		if _, err = tx.Exec(r.Context(), `INSERT INTO service_ports(node_id,port,service_id) VALUES($1,$2,$3)`, nodeID, req.Port, id); err != nil {
			var e *pgconn.PgError
			if errors.As(err, &e) && e.Code == "23505" {
				httpx.Error(w, 409, "port is already reserved on node")
			} else {
				httpx.Error(w, 500, "failed to reserve port")
			}
			return
		}
	}
	payload := WorkloadPayload(id, req.Template, req.MemoryMB, req.CPUThreads, req.Port)
	if err = reserveExtraPorts(r.Context(), tx, nodeID, id, payload); err != nil {
		httpx.Error(w, 409, err.Error())
		return
	}
	jobID, err := enqueue(r.Context(), tx, nodeID, req.ServerID, id, "create_container", payload)
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to enqueue provisioning")
		return
	}
	httpx.JSON(w, 202, map[string]string{"id": id, "job_id": jobID, "status": "installing"})
}
func nullablePort(port int) any {
	if port == 0 {
		return nil
	}
	return port
}
func enqueue(ctx context.Context, tx pgx.Tx, nodeID, serverID, serviceID, kind string, payload any) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO provisioning_jobs(id,node_id,server_id,service_id,job_type,payload) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5) RETURNING id`, nodeID, serverID, serviceID, kind, b).Scan(&id)
	return id, err
}
func (h *Handler) Action(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	action := r.PathValue("action")
	if r.Method == http.MethodDelete {
		action = "delete"
	}
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	states := map[string]string{"start": "starting", "stop": "stopping", "restart": "restarting", "delete": "deleting", "retry": "installing"}
	next, ok := states[action]
	if !ok {
		httpx.Error(w, 404, "unknown action")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin operation")
		return
	}
	defer tx.Rollback(r.Context())
	var serverID, nodeID, status string
	var runtimeID *string
	var online bool
	err = tx.QueryRow(r.Context(), `SELECT s.server_id,v.node_id,s.runtime_id,s.status,COALESCE(n.last_seen_at>=NOW()-INTERVAL '90 seconds' AND n.runtime_available,false) FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE s.id=$1 AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF s`, id, accounts.Scope(r.Context())).Scan(&serverID, &nodeID, &runtimeID, &status, &online)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "service not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read service")
		return
	}
	if !online || (runtimeID == nil && action != "retry") {
		httpx.Error(w, 409, "node is offline or service has no runtime identity")
		return
	}
	var active bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM provisioning_jobs WHERE service_id=$1 AND status IN ('pending','processing'))`, id).Scan(&active)
	if err != nil {
		httpx.Error(w, 500, "failed to read operation")
		return
	}
	if active {
		httpx.Error(w, 409, "service already has an active operation")
		return
	}
	var jobID string
	if action == "retry" {
		if status != "error" {
			httpx.Error(w, 409, "retry requires a failed service")
			return
		}
		var payload []byte
		var retryKind string
		err = tx.QueryRow(r.Context(), `SELECT payload,job_type FROM provisioning_jobs WHERE service_id=$1 AND job_type IN ('create_container','configure_minecraft') ORDER BY created_at DESC LIMIT 1`, id).Scan(&payload, &retryKind)
		if err != nil {
			httpx.Error(w, 409, "no provisioning request available to retry")
			return
		}
		if retryKind == "configure_minecraft" && runtimeID != nil {
			var p map[string]any
			if json.Unmarshal(payload, &p) != nil {
				httpx.Error(w, 409, "invalid configuration retry payload")
				return
			}
			p["runtime_id"] = *runtimeID
			payload, _ = json.Marshal(p)
		}
		jobID, err = enqueue(r.Context(), tx, nodeID, serverID, id, retryKind, json.RawMessage(payload))
	} else {
		jobID, err = enqueue(r.Context(), tx, nodeID, serverID, id, action+"_service", map[string]string{"runtime_id": *runtimeID})
	}

	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE services SET status=$2,error_message=NULL,updated_at=NOW() WHERE id=$1`, id, next)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to enqueue operation")
		return
	}
	httpx.JSON(w, 202, map[string]string{"id": id, "job_id": jobID, "status": next})
}

// Deployment is the customer create-server workflow: allocation + first service
// + queue job either all commit or none do.
type DeploymentRequest struct {
	NodeID     string `json:"node_id"`
	Name       string `json:"name"`
	Template   string `json:"template"`
	MemoryMB   int    `json:"memory_mb"`
	DiskMB     int    `json:"disk_mb"`
	CPUThreads int    `json:"cpu_threads"`
	Port       int    `json:"port,omitempty"`
	AcceptEULA bool   `json:"accept_eula,omitempty"`
}

func (h *Handler) Deploy(w http.ResponseWriter, r *http.Request) {
	var req DeploymentRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	serviceReq := CreateServiceRequest{ServerID: req.NodeID, Name: req.Name, Template: req.Template, MemoryMB: req.MemoryMB, CPUThreads: req.CPUThreads, Port: req.Port, AcceptEULA: req.AcceptEULA}
	if !validResources(req.MemoryMB, req.DiskMB, req.CPUThreads) {
		httpx.Error(w, 400, "invalid resource limits")
		return
	}
	if err := validateService(serviceReq); err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin deployment")
		return
	}
	defer tx.Rollback(r.Context())
	server, service, job, err := Fulfill(r.Context(), tx, req.NodeID, req.Name, req.Template, req.MemoryMB, req.DiskMB, req.CPUThreads, req.Port)
	if err != nil {
		httpx.Error(w, 409, err.Error())
		return
	}
	if tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to commit deployment")
		return
	}
	httpx.JSON(w, 202, map[string]string{"server_id": server, "service_id": service, "job_id": job, "status": "installing"})
}

// DeleteServer archives an empty allocation and releases its reserved budget.
// Services and job history are retained; live services must be deleted first.
func (h *Handler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serverID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid server ID")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin archive")
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	err = tx.QueryRow(r.Context(), `SELECT status FROM servers WHERE id=$1 AND ($2::uuid IS NULL OR account_id=$2) FOR UPDATE`, id, accounts.Scope(r.Context())).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "server not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read server")
		return
	}
	var live bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services WHERE server_id=$1 AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM provisioning_jobs WHERE server_id=$1 AND status IN ('pending','processing'))`, id).Scan(&live)
	if err != nil {
		httpx.Error(w, 500, "failed to read server services")
		return
	}
	if live {
		httpx.Error(w, 409, "delete all services and finish their jobs before archiving this server")
		return
	}
	_, err = tx.Exec(r.Context(), `UPDATE servers SET status='deleted',updated_at=NOW() WHERE id=$1`, id)
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to archive server")
		return
	}
	httpx.JSON(w, 200, map[string]string{"id": id, "status": "deleted"})
}
