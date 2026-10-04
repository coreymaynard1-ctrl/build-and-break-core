package platform

import (
	"buildandbreak/platform/internal/httpx"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type MinecraftRelease struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

var releaseCache struct {
	sync.Mutex
	Until    time.Time
	Versions []MinecraftRelease
}

func publisherJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "BuildAndBreak/1.0 (https://buildandbreak.local)")
	c := &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("publisher is unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("publisher has no matching release")
	}
	if json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out) != nil {
		return fmt.Errorf("invalid publisher response")
	}
	return nil
}

func minecraftReleases(ctx context.Context) ([]MinecraftRelease, error) {
	releaseCache.Lock()
	defer releaseCache.Unlock()
	if time.Now().Before(releaseCache.Until) {
		return releaseCache.Versions, nil
	}
	var manifest struct {
		Versions []MinecraftRelease `json:"versions"`
	}
	if err := publisherJSON(ctx, "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &manifest); err != nil {
		return nil, err
	}
	releaseCache.Versions = manifest.Versions
	releaseCache.Until = time.Now().Add(10 * time.Minute)
	return releaseCache.Versions, nil
}

func javaForVersion(ctx context.Context, version string) (int, error) {
	versions, err := minecraftReleases(ctx)
	if err != nil {
		return 0, err
	}
	for _, release := range versions {
		if release.ID == version {
			u, err := url.Parse(release.URL)
			if err != nil || u.Scheme != "https" || (u.Host != "piston-meta.mojang.com" && u.Host != "launchermeta.mojang.com") {
				return 0, fmt.Errorf("invalid publisher metadata origin")
			}
			var meta struct {
				Java struct {
					Major int `json:"majorVersion"`
				} `json:"javaVersion"`
				Downloads map[string]json.RawMessage `json:"downloads"`
			}
			if err := publisherJSON(ctx, release.URL, &meta); err != nil {
				return 0, err
			}
			if meta.Downloads["server"] == nil {
				return 0, fmt.Errorf("this Minecraft version has no published dedicated server")
			}
			if meta.Java.Major == 0 {
				return 8, nil
			}
			return meta.Java.Major, nil
		}
	}
	return 0, fmt.Errorf("Minecraft version is not in the official release manifest")
}

func (h *Handler) MinecraftReleases(w http.ResponseWriter, r *http.Request) {
	versions, err := minecraftReleases(r.Context())
	if err != nil {
		httpx.Error(w, 502, err.Error())
		return
	}
	typ := r.URL.Query().Get("type")
	var supported map[string]bool
	if typ == "PAPER" {
		var project struct {
			Versions map[string][]string `json:"versions"`
		}
		if err := publisherJSON(r.Context(), "https://fill.papermc.io/v3/projects/paper", &project); err != nil {
			httpx.Error(w, 502, err.Error())
			return
		}
		supported = map[string]bool{}
		for _, group := range project.Versions {
			for _, v := range group {
				supported[v] = true
			}
		}
	}
	if typ == "PURPUR" {
		var project struct {
			Versions []string `json:"versions"`
		}
		if err := publisherJSON(r.Context(), "https://api.purpurmc.org/v2/purpur", &project); err != nil {
			httpx.Error(w, 502, err.Error())
			return
		}
		supported = map[string]bool{}
		for _, v := range project.Versions {
			supported[v] = true
		}
	}
	out := []MinecraftRelease{}
	for _, release := range versions {
		if supported == nil || supported[release.ID] {
			out = append(out, MinecraftRelease{ID: release.ID, Type: release.Type})
		}
	}
	httpx.JSON(w, 200, out)
}

func (h *Handler) PaperBuilds(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if !minecraftVersion.MatchString(version) {
		httpx.Error(w, 400, "invalid Minecraft version")
		return
	}
	var builds []struct {
		ID      int    `json:"id"`
		Channel string `json:"channel"`
	}
	err := publisherJSON(r.Context(), "https://fill.papermc.io/v3/projects/paper/versions/"+url.PathEscape(version)+"/builds", &builds)
	if err != nil {
		httpx.Error(w, 502, err.Error())
		return
	}
	httpx.JSON(w, 200, builds)
}

func minecraftJavaImage(major int) (string, error) {
	switch major {
	case 8, 11, 16, 17, 21, 25:
		return fmt.Sprintf("itzg/minecraft-server:java%d", major), nil
	}
	return "", fmt.Errorf("Java %d runtime image is not configured", major)
}
