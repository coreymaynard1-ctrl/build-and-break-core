package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/moby/moby/client"
	"io"
	"path"
	"strings"
	"time"
)

func (d *DockerRuntime) CopyBedrockAddon(ctx context.Context, id, server, service string, data []byte) error {
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return err
	}
	if len(data) > 66<<20 {
		return fmt.Errorf("addon archive too large")
	}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		p := strings.Split(header.Name, "/")
		if header.Typeflag != tar.TypeReg || len(p) < 3 || (p[0] != "behavior_packs" && p[0] != "resource_packs") || path.Clean(header.Name) != header.Name || strings.ContainsAny(header.Name, "\\\x00") || strings.Contains(header.Name, "../") {
			return fmt.Errorf("invalid addon archive entry")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := d.client.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/data", Content: bytes.NewReader(data)})
	return err
}

func (d *DockerRuntime) SetBedrockPacks(ctx context.Context, id, server, service string, behavior, resource json.RawMessage) error {
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return err
	}
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for filename, content := range map[string]json.RawMessage{"world_behavior_packs.json": behavior, "world_resource_packs.json": resource} {
		var entries []struct {
			ID      string `json:"pack_id"`
			Version []int  `json:"version"`
		}
		if len(content) > 32768 || json.Unmarshal(content, &entries) != nil {
			return fmt.Errorf("invalid enabled pack list")
		}
		if err := tw.WriteHeader(&tar.Header{Name: filename, Mode: 0644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(content); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := d.client.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/data/worlds/Bedrock level", Content: &b})
	return err
}
