package nodes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB *pgxpool.Pool
}

type Node struct {
	RuntimeAvailable    bool       `json:"runtime_available"`
	RuntimeName         *string    `json:"runtime_name"`
	RuntimeVersion      *string    `json:"runtime_version"`
	AllocatedMemoryMB   int64      `json:"allocated_memory_mb"`
	AllocatedCPUThreads int64      `json:"allocated_cpu_threads"`
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Hostname            string     `json:"hostname"`
	Location            string     `json:"location"`
	Status              string     `json:"status"`
	CPUThreads          int        `json:"cpu_threads"`
	TotalMemory         int64      `json:"total_memory"`
	TotalDisk           int64      `json:"total_disk"`
	LastSeenAt          *time.Time `json:"last_seen_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(
		r.Context(),
		`
		SELECT
			id,
			name,
			hostname,
			location,

			CASE
				WHEN last_seen_at IS NOT NULL
					AND last_seen_at >= NOW() - INTERVAL '90 seconds'
				THEN 'online'
				ELSE 'offline'
			END AS calculated_status,

			cpu_threads,
			total_memory,
			total_disk,
			last_seen_at,
			created_at,
			updated_at,
 runtime_available,runtime_name,runtime_version,
 (SELECT COALESCE(SUM(memory_mb),0) FROM servers WHERE node_id=nodes.id AND status!='deleted'),
 (SELECT COALESCE(SUM(cpu_threads),0) FROM servers WHERE node_id=nodes.id AND status!='deleted')
		FROM nodes
		ORDER BY name
		`,
	)

	if err != nil {
		http.Error(
			w,
			"failed to query nodes",
			http.StatusInternalServerError,
		)
		return
	}
	defer rows.Close()

	nodes := make([]Node, 0)

	for rows.Next() {
		var node Node

		err := rows.Scan(
			&node.ID,
			&node.Name,
			&node.Hostname,
			&node.Location,
			&node.Status,
			&node.CPUThreads,
			&node.TotalMemory,
			&node.TotalDisk,
			&node.LastSeenAt,
			&node.CreatedAt,
			&node.UpdatedAt,
			&node.RuntimeAvailable, &node.RuntimeName, &node.RuntimeVersion, &node.AllocatedMemoryMB, &node.AllocatedCPUThreads,
		)

		if err != nil {
			http.Error(
				w,
				"failed to read node",
				http.StatusInternalServerError,
			)
			return
		}

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		http.Error(
			w,
			"failed while reading nodes",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		return
	}
}
