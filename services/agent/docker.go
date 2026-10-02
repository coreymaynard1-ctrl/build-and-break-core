package main

import (
	"context"
	"fmt"
	"log"

	bnbruntime "buildandbreak/platform/internal/runtime"
)

type RuntimeStatus struct {
	Available  bool
	Name       string
	Version    string
	APIVersion string
}

func createDockerRuntime(ctx context.Context) (*bnbruntime.DockerRuntime, error) {
	dockerRuntime, err := bnbruntime.NewDockerRuntime()
	if err != nil {
		return nil, fmt.Errorf("initialize Docker runtime: %w", err)
	}

	info, err := dockerRuntime.Check(ctx)
	if err != nil {
		dockerRuntime.Close()
		return nil, fmt.Errorf("Docker runtime unavailable: %w", err)
	}

	log.Printf("Docker runtime available")
	log.Printf("Docker version: %s", info.Version)
	log.Printf("Docker API version: %s", info.APIVersion)

	return dockerRuntime, nil
}

func getRuntimeStatus(
	ctx context.Context,
	dockerRuntime *bnbruntime.DockerRuntime,
) RuntimeStatus {
	info, err := dockerRuntime.Check(ctx)

	if err != nil {
		log.Printf("Docker runtime health check failed: %v", err)

		return RuntimeStatus{
			Available: false,
			Name:      "docker",
		}
	}

	return RuntimeStatus{
		Available:  true,
		Name:       "docker",
		Version:    info.Version,
		APIVersion: info.APIVersion,
	}
}
