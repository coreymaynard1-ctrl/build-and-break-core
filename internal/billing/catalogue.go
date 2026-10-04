package billing

import (
	"buildandbreak/platform/internal/httpx"
	"net/http"
	"regexp"
	"strings"
)

var planIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func (h *Handler) AdminPlans(w http.ResponseWriter, r *http.Request) {
	// Catalogue administration is available even before a provider is configured.
	rows, err := h.DB.Query(r.Context(), `SELECT id,name,currency,amount_minor,memory_mb,disk_mb,cpu_threads,template,active,is_demo FROM billing_plans ORDER BY id`)
	if err != nil {
		httpx.Error(w, 500, "failed to load plans")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var p plan
		var active, demo bool
		if rows.Scan(&p.ID, &p.Name, &p.Currency, &p.Amount, &p.Memory, &p.Disk, &p.CPU, &p.Template, &active, &demo) != nil {
			httpx.Error(w, 500, "failed to read plans")
			return
		}
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "currency": p.Currency, "amount_minor": p.Amount, "memory_mb": p.Memory, "disk_mb": p.Disk, "cpu_threads": p.CPU, "template": "resources", "active": active, "is_demo": demo})
	}
	if rows.Err() != nil {
		httpx.Error(w, 500, "failed to read plans")
		return
	}
	httpx.JSON(w, 200, out)
}
func (h *Handler) SavePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("planID")
	var req struct {
		Name     string `json:"name"`
		Currency string `json:"currency"`
		Amount   int64  `json:"amount_minor"`
		Memory   int    `json:"memory_mb"`
		Disk     int    `json:"disk_mb"`
		CPU      int    `json:"cpu_threads"`
		Template string `json:"template"`
		Active   bool   `json:"active"`
		Demo     bool   `json:"is_demo"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !planIDPattern.MatchString(id) || req.Name == "" || len(req.Name) > 100 || strings.ContainsAny(req.Name, "\x00\n\r") || req.Currency != "GBP" || req.Amount <= 0 || req.Amount > 100000000 || req.Memory < 4096 || req.Memory > 1048576 || req.Disk < 128 || req.Disk > 1073741824 || req.CPU < 1 || req.CPU > 256 || (req.Template != "alpine-test" && req.Template != "minecraft-java" && req.Template != "minecraft-bedrock" && req.Template != "resources") || (req.Template == "minecraft-java" && req.Memory < 1536) || (req.Template == "minecraft-bedrock" && req.Memory < 1024) {
		httpx.Error(w, 400, "valid plan ID, name, GBP price in minor units, template and resources required")
		return
	}
	_, err := h.DB.Exec(r.Context(), `INSERT INTO billing_plans(id,name,currency,amount_minor,memory_mb,disk_mb,cpu_threads,template,active,is_demo) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,currency=EXCLUDED.currency,amount_minor=EXCLUDED.amount_minor,memory_mb=EXCLUDED.memory_mb,disk_mb=EXCLUDED.disk_mb,cpu_threads=EXCLUDED.cpu_threads,template=EXCLUDED.template,active=EXCLUDED.active,is_demo=EXCLUDED.is_demo`, id, req.Name, req.Currency, req.Amount, req.Memory, req.Disk, req.CPU, req.Template, req.Active, req.Demo)
	if err != nil {
		httpx.Error(w, 500, "failed to save plan")
		return
	}
	httpx.JSON(w, 200, map[string]string{"id": id})
}
