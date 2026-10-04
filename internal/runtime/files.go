package runtime

import (
	"archive/tar"
	"buildandbreak/platform/internal/serverfs"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileOperation uses a short-lived isolated helper with only this server's
// managed volume. It works while the game is stopped and never mounts host paths.
func (d *DockerRuntime) FileOperation(ctx context.Context, id, server, service, job string, req serverfs.Request) (string, error) {
	if err := serverfs.Validate(req); err != nil {
		return "", err
	}
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return "", err
	}
	inspected, err := d.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	volume := "bnb-service-" + service
	owned := false
	for _, m := range inspected.Container.Mounts {
		if m.Type == mount.TypeVolume && m.Name == volume && m.Destination == "/data" {
			owned = true
		}
	}
	if !owned {
		return "", fmt.Errorf("server has no managed data volume")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	binaryData, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "bnb-fs"))
	if err != nil {
		return "", fmt.Errorf("node file helper not installed")
	}
	if len(binaryData) > 16<<20 {
		return "", fmt.Errorf("invalid file helper size")
	}
	if err = d.EnsureImage(ctx, "alpine:3.20"); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	user := "0"
	if strings.HasPrefix(inspected.Container.Config.Image, "itzg/minecraft-server:") {
		user = "1000"
	}
	request, _ := json.Marshal(req)
	helper, err := d.client.ContainerCreate(ctx, client.ContainerCreateOptions{Name: "bnb-files-" + job, Config: &container.Config{Image: "alpine:3.20", User: user, Entrypoint: []string{"/bnb-fs", string(request)}, Labels: ContainerSpec{ServerID: server, ServiceID: service, JobID: job}.Labels()}, HostConfig: &container.HostConfig{NetworkMode: "none", CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Resources: container.Resources{Memory: 64 << 20, NanoCPUs: 1000000000, PidsLimit: new(int64(64))}, Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: volume, Target: "/data"}}}})
	if err != nil {
		return "", err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		d.client.ContainerRemove(cleanup, helper.ID, client.ContainerRemoveOptions{Force: true})
	}()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err = tw.WriteHeader(&tar.Header{Name: "bnb-fs", Mode: 0755, Size: int64(len(binaryData)), Typeflag: tar.TypeReg}); err != nil {
		return "", err
	}
	tw.Write(binaryData)
	tw.Close()
	if _, err = d.client.CopyToContainer(ctx, helper.ID, client.CopyToContainerOptions{DestinationPath: "/", Content: &archive}); err != nil {
		return "", err
	}
	if _, err = d.client.ContainerStart(ctx, helper.ID, client.ContainerStartOptions{}); err != nil {
		return "", err
	}
	for {
		state, err := d.InspectOwned(ctx, helper.ID, server, service)
		if err != nil {
			return "", err
		}
		if !state.Running {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	logs, err := d.client.ContainerLogs(ctx, helper.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}
	defer logs.Close()
	raw, err := io.ReadAll(io.LimitReader(logs, 65537))
	if err != nil || len(raw) > 65536 {
		return "", fmt.Errorf("file response exceeds limit")
	}
	var output bytes.Buffer
	for len(raw) >= 8 {
		n := int(binary.BigEndian.Uint32(raw[4:8]))
		raw = raw[8:]
		if n > len(raw) {
			return "", fmt.Errorf("invalid helper output")
		}
		output.Write(raw[:n])
		raw = raw[n:]
	}
	state, err := d.InspectOwned(ctx, helper.ID, server, service)
	if err != nil {
		return "", err
	}
	if state.ExitCode != 0 {
		return "", fmt.Errorf("file operation: %s", output.String())
	}
	if !json.Valid(output.Bytes()) {
		return "", fmt.Errorf("invalid file response")
	}
	return output.String(), nil
}
