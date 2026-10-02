package jobs

import (
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB *pgxpool.Pool
}

type Job struct {
	ID        string          `json:"id"`
	NodeID    string          `json:"node_id"`
	ServerID  *string         `json:"server_id"`
	ServiceID *string         `json:"service_id"`
	JobType   string          `json:"job_type"`
	Status    string          `json:"status"`
	Payload   json.RawMessage `json:"payload"`
	Attempts  int             `json:"attempts"`
	ClaimedAt time.Time       `json:"claimed_at"`
	CreatedAt time.Time       `json:"created_at"`
}

type ClaimResponse struct {
	Job *Job `json:"job"`
}

func (h *Handler) Claim(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if !httpx.AuthNode(w, r, h.DB) {
		return
	}

	// Start a database transaction.
	//
	// Selecting the job and marking it as processing must happen
	// as one indivisible operation.
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		http.Error(
			w,
			"failed to start job claim",
			http.StatusInternalServerError,
		)
		return
	}
	defer tx.Rollback(r.Context())

	var job Job

	err = tx.QueryRow(
		r.Context(),
		`
		SELECT
			id,
			node_id,
			server_id,
			service_id,
			job_type,
			status,
			payload,
			attempts,
			created_at
		FROM provisioning_jobs
		WHERE node_id = $1
		  AND status = 'pending'
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
		`,
		nodeID,
	).Scan(
		&job.ID,
		&job.NodeID,
		&job.ServerID,
		&job.ServiceID,
		&job.JobType,
		&job.Status,
		&job.Payload,
		&job.Attempts,
		&job.CreatedAt,
	)

	// No pending job is completely normal.
	//
	// The agent will simply ask again on its next polling cycle.
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(
				w,
				"failed to complete empty job claim",
				http.StatusInternalServerError,
			)
			return
		}

		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		if err := json.NewEncoder(w).Encode(
			ClaimResponse{
				Job: nil,
			},
		); err != nil {
			return
		}

		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to find provisioning job",
			http.StatusInternalServerError,
		)
		return
	}

	// The row is still locked by our transaction here.
	//
	// Change it from pending → processing before releasing
	// the lock.
	err = tx.QueryRow(
		r.Context(),
		`
		UPDATE provisioning_jobs
		SET
			status = 'processing',
			attempts = attempts + 1,
			claimed_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
		RETURNING
			status,
			attempts,
			claimed_at
		`,
		job.ID,
	).Scan(
		&job.Status,
		&job.Attempts,
		&job.ClaimedAt,
	)

	if err != nil {
		http.Error(
			w,
			"failed to claim provisioning job",
			http.StatusInternalServerError,
		)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(
			w,
			"failed to complete job claim",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if err := json.NewEncoder(w).Encode(
		ClaimResponse{
			Job: &job,
		},
	); err != nil {
		return
	}
}
