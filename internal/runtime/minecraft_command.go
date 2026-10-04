package runtime

import (
	"context"
	"fmt"
	"github.com/moby/moby/client"
	"strings"
	"time"
)

func (d *DockerRuntime) MinecraftCommand(ctx context.Context, id, server, service, command string) error {
	if strings.TrimSpace(command) == "" || len(command) > 512 || strings.ContainsAny(command, "\x00\r\n") {
		return fmt.Errorf("invalid Minecraft command")
	}
	state, err := d.InspectOwned(ctx, id, server, service)
	if err != nil {
		return err
	}
	if !state.Running {
		return fmt.Errorf("Minecraft container is stopped")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	exec, err := d.client.ExecCreate(ctx, state.ID, client.ExecCreateOptions{User: "1000", Cmd: []string{"mc-send-to-console", command}})
	if err != nil {
		return err
	}
	if _, err = d.client.ExecStart(ctx, exec.ID, client.ExecStartOptions{Detach: true}); err != nil {
		return err
	}
	for {
		result, err := d.client.ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
		if err != nil {
			return err
		}
		if !result.Running {
			if result.ExitCode != 0 {
				return fmt.Errorf("command delivery failed; wait until Minecraft finishes starting")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
