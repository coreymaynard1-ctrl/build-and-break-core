package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type EnrollRequest struct {
	Token       string `json:"token"`
	Hostname    string `json:"hostname"`
	CPUThreads  int    `json:"cpu_threads"`
	TotalMemory int64  `json:"total_memory"`
	TotalDisk   int64  `json:"total_disk"`
}

type EnrollResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Location string `json:"location"`
	Status   string `json:"status"`
}

func (h *Handler) Enroll(w http.ResponseWriter, r *http.Request) {
	var request EnrollRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if request.Token == "" ||
		request.Hostname == "" ||
		request.CPUThreads <= 0 ||
		request.TotalMemory <= 0 ||
		request.TotalDisk <= 0 {
		http.Error(w, "missing or invalid enrollment information", http.StatusBadRequest)
		return
	}

	tokenSum := sha256.Sum256([]byte(request.Token))
	tokenHash := hex.EncodeToString(tokenSum[:])

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start enrollment", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var tokenID string
	var nodeName string
	var location string
	var expiresAt time.Time
	var usedAt *time.Time

	err = tx.QueryRow(
		r.Context(),
		`
		SELECT
			id,
			node_name,
			location,
			expires_at,
			used_at
		FROM node_enrollment_tokens
		WHERE token_hash = $1
		FOR UPDATE
		`,
		tokenHash,
	).Scan(
		&tokenID,
		&nodeName,
		&location,
		&expiresAt,
		&usedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "invalid enrollment token", http.StatusUnauthorized)
		return
	}

	if err != nil {
		http.Error(w, "failed to verify enrollment token", http.StatusInternalServerError)
		return
	}

	if usedAt != nil {
		http.Error(w, "enrollment token has already been used", http.StatusUnauthorized)
		return
	}

	if time.Now().After(expiresAt) {
		http.Error(w, "enrollment token has expired", http.StatusUnauthorized)
		return
	}

	var nodeID string

	err = tx.QueryRow(
		r.Context(),
		`
		INSERT INTO nodes (
			id,
			name,
			hostname,
			location,
			status,
			cpu_threads,
			total_memory,
			total_disk
		)
		VALUES (
			gen_random_uuid(),
			$1,
			$2,
			$3,
			'online',
			$4,
			$5,
			$6
		)
		RETURNING id
		`,
		nodeName,
		request.Hostname,
		location,
		request.CPUThreads,
		request.TotalMemory,
		request.TotalDisk,
	).Scan(&nodeID)

	if err != nil {
		http.Error(w, "failed to create node", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(
		r.Context(),
		`
		UPDATE node_enrollment_tokens
		SET used_at = NOW()
		WHERE id = $1
		`,
		tokenID,
	)

	if err != nil {
		http.Error(w, "failed to consume enrollment token", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to complete enrollment", http.StatusInternalServerError)
		return
	}

	response := EnrollResponse{
		ID:       nodeID,
		Name:     nodeName,
		Hostname: request.Hostname,
		Location: location,
		Status:   "online",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}
