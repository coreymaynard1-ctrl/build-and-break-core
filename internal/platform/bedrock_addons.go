package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/bedrock"
	"buildandbreak/platform/internal/httpx"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
)

func (h *Handler) BedrockAddons(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	if r.Method == "GET" {
		var owned bool
		err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services s JOIN servers v ON v.id=s.server_id WHERE s.id=$1 AND s.template='minecraft-bedrock' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2))`, id, accounts.Scope(r.Context())).Scan(&owned)
		if err != nil {
			httpx.Error(w, 500, "failed to read server")
			return
		}
		if !owned {
			httpx.Error(w, 404, "Bedrock server not found")
			return
		}
		h.list(w, r, `SELECT row_to_json(q) FROM (SELECT id,pack_id,name,version,kind,state,enabled,applied_enabled FROM bedrock_addons WHERE service_id=$1 ORDER BY name) q`, id)
		return
	}
	var bundle bedrock.Bundle
	var enabled bool
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			httpx.Error(w, 413, "addon upload exceeds 16 MiB")
			return
		}
		bundle, err = bedrock.ParseAddon(data)
		if err != nil {
			httpx.Error(w, 400, err.Error())
			return
		}
	} else {
		if !httpx.UUID(r.PathValue("addonID")) {
			httpx.Error(w, 400, "invalid addon ID")
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !httpx.Decode(w, r, &req) {
			return
		}
		enabled = req.Enabled
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin addon change")
		return
	}
	defer tx.Rollback(r.Context())
	var node, server, runtime string
	var ready bool
	err = tx.QueryRow(r.Context(), `SELECT v.node_id,s.server_id,COALESCE(s.runtime_id,''),COALESCE(n.runtime_available AND n.last_seen_at>NOW()-INTERVAL '90 seconds',false) FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE s.id=$1 AND s.template='minecraft-bedrock' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF s`, id, accounts.Scope(r.Context())).Scan(&node, &server, &runtime, &ready)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "Bedrock server not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read server")
		return
	}
	if !ready || runtime == "" {
		httpx.Error(w, 409, "node must be online and server provisioned")
		return
	}
	var active bool
	if tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM provisioning_jobs WHERE service_id=$1 AND status IN ('pending','processing'))`, id).Scan(&active) != nil {
		httpx.Error(w, 500, "failed to check operations")
		return
	}
	if active {
		httpx.Error(w, 409, "wait for the current operation")
		return
	}
	payload := map[string]any{"runtime_id": runtime}
	kind := "configure_bedrock_addons"
	if r.Method == "POST" {
		var count int
		var bytes int64
		if tx.QueryRow(r.Context(), `SELECT count(*) FROM bedrock_addons WHERE service_id=$1`, id).Scan(&count) != nil || tx.QueryRow(r.Context(), `SELECT COALESCE(sum(octet_length(archive)),0) FROM bedrock_artifacts WHERE service_id=$1`, id).Scan(&bytes) != nil {
			httpx.Error(w, 500, "failed to check addon limits")
			return
		}
		if count+len(bundle.Packs) > 32 || bytes+int64(len(bundle.Archive)) > 128<<20 {
			httpx.Error(w, 409, "server addon storage limit reached (32 packs / 128 MiB)")
			return
		}
		hash := sha256.Sum256(bundle.Archive)
		var artifact string
		if tx.QueryRow(r.Context(), `INSERT INTO bedrock_artifacts(service_id,archive,sha256) VALUES($1,$2,$3) RETURNING id`, id, bundle.Archive, hex.EncodeToString(hash[:])).Scan(&artifact) != nil {
			httpx.Error(w, 500, "failed to store addon")
			return
		}
		for _, pack := range bundle.Packs {
			version, _ := json.Marshal(pack.Version)
			if _, err = tx.Exec(r.Context(), `INSERT INTO bedrock_addons(service_id,artifact_id,pack_id,name,version,kind) VALUES($1,$2,$3,$4,$5,$6)`, id, artifact, pack.ID, pack.Name, version, pack.Kind); err != nil {
				httpx.Error(w, 409, "pack UUID already installed or invalid")
				return
			}
		}
		kind = "install_bedrock_addons"
		payload["artifact_id"] = artifact
		payload["sha256"] = hex.EncodeToString(hash[:])
	} else {
		tag, err := tx.Exec(r.Context(), `UPDATE bedrock_addons SET enabled=$3 WHERE id=$1 AND service_id=$2 AND state='installed'`, r.PathValue("addonID"), id, enabled)
		if err != nil {
			httpx.Error(w, 500, "failed to update addon")
			return
		}
		if tag.RowsAffected() != 1 {
			httpx.Error(w, 404, "installed addon not found")
			return
		}
		rows, err := tx.Query(r.Context(), `SELECT pack_id,version,kind FROM bedrock_addons WHERE service_id=$1 AND enabled AND state='installed' ORDER BY id`, id)
		if err != nil {
			httpx.Error(w, 500, "failed to read enabled addons")
			return
		}
		behavior := []map[string]any{}
		resources := []map[string]any{}
		for rows.Next() {
			var pack, typ string
			var version []byte
			if rows.Scan(&pack, &version, &typ) != nil {
				rows.Close()
				httpx.Error(w, 500, "failed to read enabled addons")
				return
			}
			entry := map[string]any{"pack_id": pack, "version": json.RawMessage(version)}
			if typ == "behavior" {
				behavior = append(behavior, entry)
			} else {
				resources = append(resources, entry)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			httpx.Error(w, 500, "failed to read enabled addons")
			return
		}
		payload["behavior_packs"] = behavior
		payload["resource_packs"] = resources
	}
	job, err := enqueue(r.Context(), tx, node, server, id, kind, payload)
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to queue addon operation")
		return
	}
	httpx.JSON(w, 202, map[string]string{"job_id": job, "status": "pending"})
}

func (h *Handler) BedrockArtifact(w http.ResponseWriter, r *http.Request) {
	if !httpx.AuthNode(w, r, h.DB) {
		return
	}
	if !httpx.UUID(r.PathValue("artifactID")) {
		httpx.Error(w, 400, "invalid artifact ID")
		return
	}
	var data []byte
	err := h.DB.QueryRow(r.Context(), `SELECT a.archive FROM bedrock_artifacts a JOIN services s ON s.id=a.service_id JOIN servers v ON v.id=s.server_id WHERE a.id=$1 AND v.node_id=$2 AND s.deleted_at IS NULL`, r.PathValue("artifactID"), r.PathValue("id")).Scan(&data)
	if err != nil {
		httpx.Error(w, 404, "artifact not assigned to node")
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}
