package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type HeartbeatRequest struct {
	CPUThreads  int   `json:"cpu_threads"`
	TotalMemory int64 `json:"total_memory"`
	TotalDisk   int64 `json:"total_disk"`
}

type HeartbeatResponse struct {
	Status string `json:"status"`
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")

	if nodeID == "" {
		http.Error(w, "missing node ID", http.StatusBadRequest)
		return
	}

	authorization := r.Header.Get("Authorization")

	if !strings.HasPrefix(authorization, "Bearer ") {
		http.Error(w, "missing node credentials", http.StatusUnauthorized)
		return
	}

	nodeToken := strings.TrimPrefix(authorization, "Bearer ")

	if nodeToken == "" {
		http.Error(w, "missing node credentials", http.StatusUnauthorized)
		return
	}

	tokenSum := sha256.Sum256([]byte(nodeToken))
	tokenHash := hex.EncodeToString(tokenSum[:])

	var storedNodeID string

	err := h.DB.QueryRow(
		r.Context(),
		`
		SELECT node_id
		FROM node_credentials
		WHERE node_id = $1
		  AND token_hash = $2
		`,
		nodeID,
		tokenHash,
	).Scan(&storedNodeID)

	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "invalid node credentials", http.StatusUnauthorized)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to authenticate node",
			http.StatusInternalServerError,
		)
		return
	}

	var request HeartbeatRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid heartbeat body", http.StatusBadRequest)
		return
	}

	if request.CPUThreads <= 0 ||
		request.TotalMemory <= 0 ||
		request.TotalDisk <= 0 {
		http.Error(
			w,
			"invalid heartbeat information",
			http.StatusBadRequest,
		)
		return
	}

	result, err := h.DB.Exec(
		r.Context(),
		`
		UPDATE nodes
		SET
			status = 'online',
			cpu_threads = $2,
			total_memory = $3,
			total_disk = $4,
			last_seen_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
		`,
		nodeID,
		request.CPUThreads,
		request.TotalMemory,
		request.TotalDisk,
	)

	if err != nil {
		http.Error(
			w,
			"failed to update node",
			http.StatusInternalServerError,
		)
		return
	}

	if result.RowsAffected() != 1 {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	response := HeartbeatResponse{
		Status: "ok",
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}
