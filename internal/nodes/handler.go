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
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Hostname    string    `json:"hostname"`
	Location    string    `json:"location"`
	Status      string    `json:"status"`
	CPUThreads  int       `json:"cpu_threads"`
	TotalMemory int64     `json:"total_memory"`
	TotalDisk   int64     `json:"total_disk"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
			status,
			cpu_threads,
			total_memory,
			total_disk,
			created_at,
			updated_at
		FROM nodes
		ORDER BY name
		`,
	)

	if err != nil {
		http.Error(w, "failed to query nodes", http.StatusInternalServerError)
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
			&node.CreatedAt,
			&node.UpdatedAt,
		)

		if err != nil {
			http.Error(w, "failed to read node", http.StatusInternalServerError)
			return
		}

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "failed while reading nodes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		return
	}
}
