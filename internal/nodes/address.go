package nodes

import (
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"
)

var addressLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func validAddress(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !addressLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func (h *Handler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"public_address"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.Error(w, 400, "invalid address request")
		return
	}
	req.Address = strings.TrimSpace(req.Address)
	if req.Address != "" && !validAddress(req.Address) {
		httpx.Error(w, 400, "enter an IP address or hostname without a port or URL")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `UPDATE nodes SET public_address=$2,updated_at=NOW() WHERE id=$1`, r.PathValue("id"), req.Address)
	if err != nil {
		httpx.Error(w, 500, "failed to save node address")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, 404, "node not found")
		return
	}
	httpx.JSON(w, 200, map[string]string{"public_address": req.Address})
}
