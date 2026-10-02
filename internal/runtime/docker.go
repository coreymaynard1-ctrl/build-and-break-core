package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/client"
)

type DockerRuntime struct {
	client *client.Client
}

type DockerInfo struct {
	Available  bool
	Version    string
	APIVersion string
}

func NewDockerRuntime() (*DockerRuntime, error) {
	dockerClient, err := client.New(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}

	return &DockerRuntime{
		client: dockerClient,
	}, nil
}

func (d *DockerRuntime) Check(ctx context.Context) (*DockerInfo, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ping, err := d.client.Ping(
		checkCtx,
		client.PingOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("ping Docker daemon: %w", err)
	}

	version, err := d.client.ServerVersion(
		checkCtx,
		client.ServerVersionOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("get Docker version: %w", err)
	}

	return &DockerInfo{
		Available:  true,
		Version:    version.Version,
		APIVersion: ping.APIVersion,
	}, nil
}

func (d *DockerRuntime) Close() error {
	return d.client.Close()
}
