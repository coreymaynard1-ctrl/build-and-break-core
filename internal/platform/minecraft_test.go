package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMinecraftSoftwareAndContentRules(t *testing.T) {
	for _, typ := range []string{"VANILLA", "PAPER", "FABRIC", "FORGE", "NEOFORGE", "MODRINTH", "PURPUR", "SPIGOT", "QUILT"} {
		c := DefaultMinecraftConfig()
		c.Type = typ
		if typ == "MODRINTH" {
			c.Modpack = "test-pack"
		} else if typ != "VANILLA" {
			c.Projects = []string{"test-project"}
		}
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		p := MinecraftPayload("service", 2048, 2, 25565, c)
		b, _ := json.Marshal(p)
		if !strings.Contains(string(b), "TYPE="+typ) || !strings.Contains(string(b), "persistent_data") {
			t.Fatal("settings not connected to workload")
		}
	}
	for _, edit := range []func(*MinecraftConfig){
		func(c *MinecraftConfig) { c.Projects = []string{"plugin"} },
		func(c *MinecraftConfig) { c.Type = "PAPER"; c.Projects = []string{"https://127.0.0.1/plugin.jar"} },
		func(c *MinecraftConfig) { c.Type = "PAPER"; c.Projects = []string{"@/data/private"} },
		func(c *MinecraftConfig) { c.Type = "PAPER"; c.Projects = []string{"fabric:fabric-api"} },
		func(c *MinecraftConfig) { c.Type = "PAPER"; c.Build = "latest\nURL=http://private" },
		func(c *MinecraftConfig) { c.AcceptEULA = false },
		func(c *MinecraftConfig) { c.Type = "MODRINTH"; c.Modpack = "../pack" },
		func(c *MinecraftConfig) { c.Version = "LATEST" },
		func(c *MinecraftConfig) { c.MaxPlayers = 0 },
	} {
		c := DefaultMinecraftConfig()
		edit(&c)
		if c.Validate() == nil {
			t.Fatalf("unsafe/unsupported settings accepted: %+v", c)
		}
	}
}
