package nodes

import (
	"buildandbreak/platform/internal/httpx"
	"net/http"
)

// Locations exposes the catalogue without internal hostnames, credentials or capacity.
func (h *Handler) Locations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT id,name,location,public_address,COALESCE(last_seen_at>=NOW()-INTERVAL '90 seconds' AND runtime_available,false) FROM nodes ORDER BY name`)
	if err != nil {
		httpx.Error(w, 500, "failed to load locations")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, location, address string
		var online bool
		if rows.Scan(&id, &name, &location, &address, &online) != nil {
			httpx.Error(w, 500, "failed to read locations")
			return
		}
		status := "offline"
		if online {
			status = "online"
		}
		out = append(out, map[string]any{"id": id, "name": name, "location": location, "public_address": address, "status": status, "runtime_available": online})
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read locations")
		return
	}
	httpx.JSON(w, 200, out)
}
