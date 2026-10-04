package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

type MinecraftConfig struct {
	Startup      string   `json:"startup"`
	JavaMajor    int      `json:"-"`
	Java         string   `json:"java"`
	Type         string   `json:"type"`
	Version      string   `json:"version"`
	Build        string   `json:"build"`
	Projects     []string `json:"projects"`
	Modpack      string   `json:"modpack"`
	MOTD         string   `json:"motd"`
	MaxPlayers   int      `json:"max_players"`
	Difficulty   string   `json:"difficulty"`
	GameMode     string   `json:"game_mode"`
	ViewDistance int      `json:"view_distance"`
	AcceptEULA   bool     `json:"accept_eula"`
}

func DefaultMinecraftConfig() MinecraftConfig {
	return MinecraftConfig{Type: "VANILLA", Version: "1.21.1", Projects: []string{}, Java: "auto", MOTD: "A Minecraft Server", MaxPlayers: 20, Difficulty: "normal", GameMode: "survival", ViewDistance: 10, AcceptEULA: true}
}

var projectSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$`)
var minecraftVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
var buildVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,39}$`)

func (c *MinecraftConfig) Validate() error {
	if c.Startup == "" {
		c.Startup = "{{DEFAULT_COMMAND}}"
	}
	if err := validateStartup(c.Startup); err != nil {
		return err
	}
	switch c.Type {
	case "VANILLA", "PAPER", "FABRIC", "FORGE", "NEOFORGE", "MODRINTH", "PURPUR", "SPIGOT", "QUILT":
	default:
		return fmt.Errorf("unsupported Minecraft server type")
	}
	if c.Java == "" {
		c.Java = "auto"
	}
	switch c.Java {
	case "auto", "8", "11", "16", "17", "21", "25":
	default:
		return fmt.Errorf("invalid Java runtime")
	}
	if !minecraftVersion.MatchString(c.Version) || c.Version == "LATEST" || c.Version == "SNAPSHOT" || !c.AcceptEULA {
		return fmt.Errorf("select a published Minecraft version and accept the EULA")
	}
	if c.Build != "" && (!buildVersion.MatchString(c.Build) || c.Type == "VANILLA") {
		return fmt.Errorf("invalid build or loader version")
	}
	if (c.Type == "PAPER" || c.Type == "PURPUR") && c.Build != "" && !regexp.MustCompile(`^[0-9]{1,6}$`).MatchString(c.Build) {
		return fmt.Errorf("Paper build must be a number")
	}
	if c.Type == "MODRINTH" {
		if !projectSlug.MatchString(c.Modpack) || len(c.Projects) != 0 {
			return fmt.Errorf("select one Modrinth pack; manage its bundled mods through the pack")
		}
	} else if c.Modpack != "" {
		return fmt.Errorf("modpack requires the Modrinth server type")
	}
	if len(c.Projects) > 50 || (c.Type == "VANILLA" && len(c.Projects) != 0) {
		return fmt.Errorf("Vanilla cannot use mods/plugins; maximum 50 projects")
	}
	seen := map[string]bool{}
	for _, p := range c.Projects {
		if !projectSlug.MatchString(p) || seen[p] {
			return fmt.Errorf("projects must be unique Modrinth slugs or IDs")
		}
		seen[p] = true
	}
	if len(c.MOTD) > 200 || strings.ContainsAny(c.MOTD, "\x00\r\n") || c.MaxPlayers < 1 || c.MaxPlayers > 1000 || c.ViewDistance < 2 || c.ViewDistance > 32 {
		return fmt.Errorf("invalid MOTD, player limit or view distance")
	}
	switch c.Difficulty {
	case "peaceful", "easy", "normal", "hard":
	default:
		return fmt.Errorf("invalid difficulty")
	}
	switch c.GameMode {
	case "survival", "creative", "adventure", "spectator":
	default:
		return fmt.Errorf("invalid game mode")
	}
	if c.Projects == nil {
		c.Projects = []string{}
	}
	return nil
}

func MinecraftPayload(id string, memory, cpu, port int, c MinecraftConfig) map[string]any {
	p := WorkloadPayload(id, "minecraft-java", memory, cpu, port)
	env := []string{"EULA=TRUE", "TYPE=" + c.Type, "VERSION=" + c.Version, fmt.Sprintf("MEMORY=%dM", memory-512), "ENABLE_RCON=false", "CREATE_CONSOLE_IN_PIPE=true", "MODRINTH_DOWNLOAD_DEPENDENCIES=required", "MOTD=" + c.MOTD, fmt.Sprintf("MAX_PLAYERS=%d", c.MaxPlayers), "DIFFICULTY=" + c.Difficulty, "MODE=" + c.GameMode, fmt.Sprintf("VIEW_DISTANCE=%d", c.ViewDistance)}
	keys := map[string]string{"PAPER": "PAPER_BUILD", "FABRIC": "FABRIC_LOADER_VERSION", "QUILT": "QUILT_LOADER_VERSION", "PURPUR": "PURPUR_BUILD", "FORGE": "FORGE_VERSION", "NEOFORGE": "NEOFORGE_VERSION"}
	if key, ok := keys[c.Type]; ok && c.Build != "" {
		env = append(env, key+"="+c.Build)
	}
	if c.Type == "SPIGOT" {
		env = append(env, "BUILD_FROM_SOURCE=true")
	}
	if c.Type == "MODRINTH" {
		env = append(env, "MODRINTH_MODPACK="+c.Modpack)
		if c.Build != "" {
			env = append(env, "MODRINTH_VERSION="+c.Build)
		}
	} else {
		env = append(env, "MODRINTH_PROJECTS="+strings.Join(c.Projects, ","))
	}
	// Java runtime selection comes from official version metadata.
	if c.JavaMajor != 0 {
		if image, err := minecraftJavaImage(c.JavaMajor); err == nil {
			p["image"] = image
		}
	}
	env = append(env, "BNB_STARTUP_COMMAND="+c.Startup, "SERVER_RUNNER=/usr/local/bin/bnb-startup", fmt.Sprintf("BNB_HEAP_MB=%d", memory-512), "BNB_INTERNAL_PORT=25565")
	p["startup_runner"] = true
	p["environment"] = env
	return p
}

func (h *Handler) Minecraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("serviceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid service ID")
		return
	}
	if r.Method == http.MethodGet {
		var requested, applied []byte
		var startup string
		err := h.DB.QueryRow(r.Context(), `SELECT s.minecraft_config,s.minecraft_applied_config,s.template_startup FROM services s JOIN servers v ON v.id=s.server_id WHERE s.id=$1 AND s.template='minecraft-java' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2)`, id, accounts.Scope(r.Context())).Scan(&requested, &applied, &startup)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, 404, "Minecraft service not found")
			return
		}
		if err != nil {
			httpx.Error(w, 500, "failed to read Minecraft settings")
			return
		}
		if requested == nil {
			requested, _ = json.Marshal(DefaultMinecraftConfig())
		}
		httpx.JSON(w, 200, map[string]any{"requested": json.RawMessage(requested), "applied": json.RawMessage(applied), "egg": map[string]any{"id": "minecraft-java", "name": "Minecraft Java", "default_startup": displayStartup(startup), "startup_resolved": startup != "", "variables": []string{"DEFAULT_COMMAND", "SERVER_JARFILE", "SERVER_MEMORY", "SERVER_PORT"}}})
		return
	}
	var cfg MinecraftConfig
	if !httpx.Decode(w, r, &cfg) {
		return
	}
	if err := cfg.Validate(); err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin settings update")
		return
	}
	defer tx.Rollback(r.Context())
	var server, node string
	var runtime *string
	var memory, cpu, port int
	var online bool
	err = tx.QueryRow(r.Context(), `SELECT s.server_id,v.node_id,s.runtime_id,s.memory_mb,s.cpu_threads,s.port,COALESCE(n.last_seen_at>NOW()-INTERVAL '90 seconds' AND n.runtime_available,false) FROM services s JOIN servers v ON v.id=s.server_id JOIN nodes n ON n.id=v.node_id WHERE s.id=$1 AND s.template='minecraft-java' AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR v.account_id=$2) FOR UPDATE OF s`, id, accounts.Scope(r.Context())).Scan(&server, &node, &runtime, &memory, &cpu, &port, &online)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "Minecraft service not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read Minecraft service")
		return
	}
	if !online || runtime == nil {
		httpx.Error(w, 409, "node must be online and the service provisioned")
		return
	}
	var active bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM provisioning_jobs WHERE service_id=$1 AND status IN ('pending','processing'))`, id).Scan(&active); err != nil {
		httpx.Error(w, 500, "failed to check operations")
		return
	}
	if active {
		httpx.Error(w, 409, "wait for the current service operation to finish")
		return
	}
	cfg, err = ResolveMinecraft(r.Context(), cfg, h.MinecraftJavaResolver)
	if err != nil {
		httpx.Error(w, 502, err.Error())
		return
	}
	p := MinecraftPayload(id, memory, cpu, port, cfg)
	p["runtime_id"] = *runtime
	p["minecraft_config"] = cfg
	job, err := enqueue(r.Context(), tx, node, server, id, "configure_minecraft", p)
	b, _ := json.Marshal(cfg)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE services SET minecraft_config=$2,template_startup='',status='installing',error_message=NULL,updated_at=NOW() WHERE id=$1`, id, b)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		httpx.Error(w, 500, "failed to queue Minecraft update")
		return
	}
	httpx.JSON(w, 202, map[string]string{"job_id": job, "status": "installing"})
}
