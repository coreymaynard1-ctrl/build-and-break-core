package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// CreateContainer creates a Build & Break managed Docker container.
//
// Creation and startup are intentionally separate operations.
// This will eventually let our provisioning system create the
// container, prepare its files, and only then start the workload.
func (d *DockerRuntime) CreateContainer(
	ctx context.Context,
	spec ContainerSpec,
) (*ContainerState, error) {
	if err := validateContainerSpec(spec); err != nil {
		return nil, err
	}

	createCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	config := &container.Config{
		Image:  spec.Image,
		Env:    spec.Environment,
		Cmd:    spec.Command,
		Labels: spec.Labels(),
	}

	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory: spec.MemoryBytes,
		},
	}

	// Docker represents CPU capacity using NanoCPUs.
	//
	// 1 CPU = 1,000,000,000 NanoCPUs
	// 2 CPU = 2,000,000,000 NanoCPUs
	//
	// We're currently limiting CPU capacity rather than pinning
	// the container to particular physical CPU cores.
	hostConfig.Resources.NanoCPUs =
		int64(spec.CPUThreads) * 1_000_000_000

	response, err := d.client.ContainerCreate(
		createCtx,
		client.ContainerCreateOptions{
			Config:     config,
			HostConfig: hostConfig,
			Name:       spec.Name,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Docker container: %w",
			err,
		)
	}

	return &ContainerState{
		ID:      response.ID,
		Name:    spec.Name,
		Image:   spec.Image,
		Status:  "created",
		Running: false,
	}, nil
}

// validateContainerSpec prevents incomplete or unsafe container
// specifications from reaching Docker.
func validateContainerSpec(spec ContainerSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("container name is required")
	}

	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("container image is required")
	}

	if strings.TrimSpace(spec.ServerID) == "" {
		return fmt.Errorf("server ID is required")
	}

	if strings.TrimSpace(spec.ServiceID) == "" {
		return fmt.Errorf("service ID is required")
	}

	if spec.MemoryBytes <= 0 {
		return fmt.Errorf(
			"container memory limit must be greater than zero",
		)
	}

	if spec.CPUThreads <= 0 {
		return fmt.Errorf(
			"container CPU limit must be greater than zero",
		)
	}

	return nil
}

// InspectContainer reads the current Docker state of a container.
//
// It also acts as our ownership guard. A Docker container without
// the Build & Break managed label is not considered ours and the
// runtime refuses to manage it.
func (d *DockerRuntime) InspectContainer(
	ctx context.Context,
	containerID string,
) (*ContainerState, error) {
	if strings.TrimSpace(containerID) == "" {
		return nil, fmt.Errorf("container ID is required")
	}

	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := d.client.ContainerInspect(
		inspectCtx,
		containerID,
		client.ContainerInspectOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"inspect Docker container: %w",
			err,
		)
	}

	// The current Moby client wraps the Docker inspect response
	// inside ContainerInspectResult.Container.
	//
	// We pull that inner response out here so the rest of this
	// function is easier to read.
	inspected := result.Container

	// SAFETY:
	//
	// Build & Break must never automatically control arbitrary
	// Docker containers that happen to exist on the physical node.
	if inspected.Config == nil ||
		inspected.Config.Labels[ManagedLabel] != "true" {
		return nil, fmt.Errorf(
			"refusing to manage container not owned by Build & Break",
		)
	}

	state := &ContainerState{
		ID:    inspected.ID,
		Name:  strings.TrimPrefix(inspected.Name, "/"),
		Image: inspected.Config.Image,
	}

	if inspected.State != nil {
		state.Status = string(inspected.State.Status)
		state.Running = inspected.State.Running
		state.ExitCode = inspected.State.ExitCode

		if inspected.State.StartedAt != "" {
			startedAt, err := time.Parse(
				time.RFC3339Nano,
				inspected.State.StartedAt,
			)

			if err == nil {
				state.StartedAt = startedAt
			}
		}
	}

	return state, nil
}

// StartContainer starts an existing Build & Break container.
func (d *DockerRuntime) StartContainer(
	ctx context.Context,
	containerID string,
) error {
	// InspectContainer performs our ownership check first.
	if _, err := d.InspectContainer(ctx, containerID); err != nil {
		return err
	}

	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := d.client.ContainerStart(
		startCtx,
		containerID,
		client.ContainerStartOptions{},
	)
	if err != nil {
		return fmt.Errorf(
			"start Docker container: %w",
			err,
		)
	}

	return nil
}

// StopContainer gracefully stops an existing Build & Break container.
func (d *DockerRuntime) StopContainer(
	ctx context.Context,
	containerID string,
) error {
	// Refuse the operation if this isn't one of our containers.
	if _, err := d.InspectContainer(ctx, containerID); err != nil {
		return err
	}

	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := d.client.ContainerStop(
		stopCtx,
		containerID,
		client.ContainerStopOptions{},
	)
	if err != nil {
		return fmt.Errorf(
			"stop Docker container: %w",
			err,
		)
	}

	return nil
}

// DeleteContainer removes an existing Build & Break container.
//
// We deliberately don't force-delete running containers and we
// deliberately don't remove persistent volumes here.
func (d *DockerRuntime) DeleteContainer(
	ctx context.Context,
	containerID string,
) error {
	// Verify ownership before sending the destructive operation.
	if _, err := d.InspectContainer(ctx, containerID); err != nil {
		return err
	}

	deleteCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := d.client.ContainerRemove(
		deleteCtx,
		containerID,
		client.ContainerRemoveOptions{
			Force:         false,
			RemoveVolumes: false,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"remove Docker container: %w",
			err,
		)
	}

	return nil
}
