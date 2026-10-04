package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (h *Handler) MinecraftCatalogue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	var owned bool
	err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services s JOIN servers v ON v.id=s.server_id WHERE s.id=$1 AND s.template='minecraft-java' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2))`, id, accounts.Scope(r.Context())).Scan(&owned)
	if err != nil {
		httpx.Error(w, 500, "failed to read service")
		return
	}
	if !owned {
		httpx.Error(w, 404, "Minecraft service not found")
		return
	}
	typ, version, q := r.URL.Query().Get("type"), r.URL.Query().Get("version"), strings.TrimSpace(r.URL.Query().Get("q"))
	if !minecraftVersion.MatchString(version) || len(q) > 100 {
		httpx.Error(w, 400, "invalid catalogue search")
		return
	}
	facets := [][]string{{"versions:" + version}}
	switch typ {
	case "PAPER", "PURPUR":
		facets = append(facets, []string{"categories:paper"})
	case "SPIGOT", "FABRIC", "FORGE", "NEOFORGE", "QUILT":
		facets = append(facets, []string{"categories:" + strings.ToLower(typ)})
	case "MODRINTH":
		facets = append(facets, []string{"project_type:modpack"})
	default:
		httpx.Error(w, 400, "select a plugin, mod or modpack server type")
		return
	}
	b, _ := json.Marshal(facets)
	params := url.Values{"query": {q}, "facets": {string(b)}, "limit": {"12"}, "index": {"relevance"}}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.modrinth.com/v2/search?"+params.Encode(), nil)
	if err != nil {
		httpx.Error(w, 500, "failed to create search")
		return
	}
	req.Header.Set("User-Agent", "BuildAndBreak/1.0 (Minecraft catalogue)")
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		httpx.Error(w, 502, "Modrinth catalogue is unavailable; try again later")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		httpx.Error(w, 502, "Modrinth catalogue is unavailable; try again later")
		return
	}
	var result struct {
		Hits []struct {
			ProjectType string `json:"project_type"`
			ServerSide  string `json:"server_side"`
			Slug        string `json:"slug"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Downloads   int64  `json:"downloads"`
		} `json:"hits"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result) != nil {
		httpx.Error(w, 502, "invalid catalogue response")
		return
	}
	filtered := result.Hits[:0]
	for _, hit := range result.Hits {
		if hit.ServerSide == "unsupported" || (typ != "MODRINTH" && hit.ProjectType == "modpack") {
			continue
		}
		filtered = append(filtered, hit)
	}
	result.Hits = filtered
	httpx.JSON(w, 200, result)
}
