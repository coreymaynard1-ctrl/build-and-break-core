package nodes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"buildandbreak/platform/internal/httpx"
)

// CreateEnrollment is wrapped by administrator authentication and CSRF checks.
func (h *Handler) CreateEnrollment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		httpx.Error(w, 400, "invalid enrollment request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Location = strings.TrimSpace(req.Location)
	if req.Name == "" || req.Location == "" || len(req.Name) > 100 || len(req.Location) > 100 {
		httpx.Error(w, 400, "name and location are required (maximum 100 bytes)")
		return
	}
	token := "bnb_enroll_" + rand.Text()
	sum := sha256.Sum256([]byte(token))
	expires := time.Now().UTC().Add(30 * time.Minute)
	_, err := h.DB.Exec(r.Context(), `INSERT INTO node_enrollment_tokens(id,token_hash,node_name,location,expires_at) VALUES(gen_random_uuid(),$1,$2,$3,$4)`, hex.EncodeToString(sum[:]), req.Name, req.Location, expires)
	if err != nil {
		httpx.Error(w, 500, "failed to create enrollment token")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, 201, map[string]any{"token": token, "expires_at": expires, "name": req.Name, "location": req.Location})
}
