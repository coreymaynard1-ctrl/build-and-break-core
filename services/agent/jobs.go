package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	bnbruntime "buildandbreak/platform/internal/runtime"
)

type ProvisioningJob struct {
	ID            string          `json:"id"`
	NodeID        string          `json:"node_id"`
	ServerID      *string         `json:"server_id"`
	ServiceID     *string         `json:"service_id"`
	JobType       string          `json:"job_type"`
	Status        string          `json:"status"`
	Payload       json.RawMessage `json:"payload"`
	Attempts      int             `json:"attempts"`
	ClaimedAt     time.Time       `json:"claimed_at"`
	CreatedAt     time.Time       `json:"created_at"`
	RuntimeID     string          `json:"-"`
	ServiceStatus string          `json:"-"`
}

type JobClaimResponse struct {
	Job *ProvisioningJob `json:"job"`
}

type JobResultRequest struct {
	Status        string `json:"status"`
	ErrorMessage  string `json:"error_message,omitempty"`
	RuntimeID     string `json:"runtime_id,omitempty"`
	ServiceStatus string `json:"service_status,omitempty"`
}

// CreateContainerPayload contains only the Docker settings that
// Core is currently allowed to ask the agent to use.
//
// We deliberately do NOT accept arbitrary Docker configuration.
// Build & Break decides which Docker capabilities customers are
// eventually allowed to use.
type CreateContainerPayload struct {
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	MemoryBytes    int64    `json:"memory_bytes"`
	CPUThreads     int      `json:"cpu_threads"`
	Environment    []string `json:"environment,omitempty"`
	Command        []string `json:"command,omitempty"`
	Port           int      `json:"port,omitempty"`
	PersistentData bool     `json:"persistent_data,omitempty"`
}

func claimJob(
	ctx context.Context,
	config *AgentConfig,
) (*ProvisioningJob, error) {
	url := fmt.Sprintf(
		"%s/api/v1/nodes/%s/jobs/claim",
		config.CoreURL,
		config.NodeID,
	)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create job claim request: %w",
			err,
		)
	}

	request.Header.Set(
		"Authorization",
		"Bearer "+config.NodeToken,
	)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf(
			"contact Core for jobs: %w",
			err,
		)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(
		io.LimitReader(response.Body, 1024*1024),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read job claim response: %w",
			err,
		)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"Core rejected job claim: HTTP %d: %s",
			response.StatusCode,
			string(body),
		)
	}

	var result JobClaimResponse

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf(
			"decode job claim response: %w",
			err,
		)
	}

	return result.Job, nil
}

func sendJobResult(
	ctx context.Context,
	config *AgentConfig,
	jobID string,
	status string,
	errorMessage string,
	runtimeID, serviceStatus string,
) error {
	result := JobResultRequest{
		Status:        status,
		ErrorMessage:  errorMessage,
		RuntimeID:     runtimeID,
		ServiceStatus: serviceStatus,
	}

	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf(
			"encode job result: %w",
			err,
		)
	}

	url := fmt.Sprintf(
		"%s/api/v1/nodes/%s/jobs/%s/result",
		config.CoreURL,
		config.NodeID,
		jobID,
	)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf(
			"create job result request: %w",
			err,
		)
	}

	request.Header.Set(
		"Authorization",
		"Bearer "+config.NodeToken,
	)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf(
			"report job result to Core: %w",
			err,
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(response.Body, 1024*1024),
	)
	if err != nil {
		return fmt.Errorf(
			"read job result response: %w",
			err,
		)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"Core rejected job result: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)
	}

	return nil
}

func executeCreateContainerJob(
	ctx context.Context,
	dockerRuntime *bnbruntime.DockerRuntime,
	job *ProvisioningJob,
) error {
	// The ownership IDs come from the database-backed job itself,
	// not from the JSON payload.
	//
	// This prevents a payload from claiming that a container belongs
	// to some unrelated server or service.
	if job.ServerID == nil ||
		strings.TrimSpace(*job.ServerID) == "" {

		return fmt.Errorf(
			"create_container job has no server ID",
		)
	}

	if job.ServiceID == nil ||
		strings.TrimSpace(*job.ServiceID) == "" {

		return fmt.Errorf(
			"create_container job has no service ID",
		)
	}

	var payload CreateContainerPayload

	if err := json.Unmarshal(
		job.Payload,
		&payload,
	); err != nil {
		return fmt.Errorf(
			"decode create_container payload: %w",
			err,
		)
	}

	if strings.TrimSpace(payload.Name) == "" {
		return fmt.Errorf(
			"container name is required",
		)
	}

	if strings.TrimSpace(payload.Image) == "" {
		return fmt.Errorf(
			"container image is required",
		)
	}

	if payload.MemoryBytes <= 0 {
		return fmt.Errorf(
			"memory limit must be greater than zero",
		)
	}

	if payload.CPUThreads <= 0 {
		return fmt.Errorf(
			"CPU limit must be greater than zero",
		)
	}

	log.Printf(
		"Ensuring Docker image is available: %s",
		payload.Image,
	)

	if err := dockerRuntime.EnsureImage(
		ctx,
		payload.Image,
	); err != nil {
		return fmt.Errorf(
			"ensure container image: %w",
			err,
		)
	}

	spec := bnbruntime.ContainerSpec{
		Name:        payload.Name,
		Image:       payload.Image,
		ServerID:    *job.ServerID,
		ServiceID:   *job.ServiceID,
		MemoryBytes: payload.MemoryBytes,
		CPUThreads:  payload.CPUThreads,
		Environment: payload.Environment,
		Command:     payload.Command,
		Port:        payload.Port,
	}

	if payload.PersistentData {
		spec.VolumeName = "bnb-service-" + *job.ServiceID
	}
	log.Printf(
		"Creating container: name=%s image=%s memory=%d cpu=%d",
		spec.Name,
		spec.Image,
		spec.MemoryBytes,
		spec.CPUThreads,
	)

	containerState, err := dockerRuntime.InspectOwned(ctx, spec.Name, spec.ServerID, spec.ServiceID)
	if bnbruntime.IsNotFound(err) {
		containerState, err = dockerRuntime.CreateContainer(ctx, spec)
	}
	if err != nil {
		return fmt.Errorf(
			"create container: %w",
			err,
		)
	}

	job.RuntimeID = containerState.ID
	log.Printf(
		"Container created: id=%s",
		containerState.ID,
	)

	if err := dockerRuntime.StartContainer(
		ctx,
		containerState.ID,
		*job.ServerID, *job.ServiceID,
	); err != nil {
		return fmt.Errorf(
			"start container %s: %w",
			containerState.ID,
			err,
		)
	}

	state, err := dockerRuntime.InspectOwned(ctx, containerState.ID, *job.ServerID, *job.ServiceID)
	if err != nil {
		return err
	}
	job.ServiceStatus = runtimeStatus(state)
	log.Printf(
		"Container started: id=%s",
		containerState.ID,
	)

	return nil
}

func executeJob(
	ctx context.Context,
	dockerRuntime *bnbruntime.DockerRuntime,
	job *ProvisioningJob,
) error {
	switch job.JobType {

	case "test":
		log.Printf(
			"Executing test job: %s",
			string(job.Payload),
		)

		return nil

	case "create_container":
		return executeCreateContainerJob(
			ctx,
			dockerRuntime,
			job,
		)

	case "start_service", "stop_service", "restart_service", "delete_service":
		return executeLifecycleJob(ctx, dockerRuntime, job)
	default:
		return fmt.Errorf(
			"unsupported job type %q",
			job.JobType,
		)
	}
}

func processJob(
	ctx context.Context,
	config *AgentConfig,
	dockerRuntime *bnbruntime.DockerRuntime,
	job *ProvisioningJob,
) {
	log.Printf(
		"Provisioning job claimed: id=%s type=%s attempt=%d",
		job.ID,
		job.JobType,
		job.Attempts,
	)

	err := executeJob(
		ctx,
		dockerRuntime,
		job,
	)

	if err != nil {
		log.Printf(
			"Provisioning job failed: id=%s error=%v",
			job.ID,
			err,
		)

		if reportErr := reportJobResult(
			ctx,
			config,
			job.ID,
			"failed",
			err.Error(),
			job.RuntimeID, job.ServiceStatus,
		); reportErr != nil {

			log.Printf(
				"Failed to report job failure: id=%s error=%v",
				job.ID,
				reportErr,
			)
		}

		return
	}

	if err := reportJobResult(
		ctx,
		config,
		job.ID,
		"completed",
		"",
		job.RuntimeID, job.ServiceStatus,
	); err != nil {

		log.Printf(
			"Failed to report job completion: id=%s error=%v",
			job.ID,
			err,
		)

		return
	}

	log.Printf(
		"Provisioning job completed: id=%s",
		job.ID,
	)
}

func jobPollingLoop(
	ctx context.Context,
	config *AgentConfig,
	dockerRuntime *bnbruntime.DockerRuntime,
) {
	poll := func() {
		job, err := claimJob(
			ctx,
			config,
		)
		if err != nil {
			log.Printf(
				"Job polling failed: %v",
				err,
			)
			return
		}

		if job == nil {
			return
		}

		processJob(
			ctx,
			config,
			dockerRuntime,
			job,
		)
	}

	poll()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			poll()

		case <-ctx.Done():
			return
		}
	}
}

func runtimeStatus(state *bnbruntime.ContainerState) string {
	if state.Running {
		return "running"
	}
	return "stopped"
}
func executeLifecycleJob(ctx context.Context, rt *bnbruntime.DockerRuntime, job *ProvisioningJob) error {
	if job.ServerID == nil || job.ServiceID == nil {
		return fmt.Errorf("lifecycle job requires allocation IDs")
	}
	var payload struct {
		RuntimeID string `json:"runtime_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.RuntimeID == "" {
		return fmt.Errorf("lifecycle job requires runtime_id")
	}
	state, err := rt.InspectOwned(ctx, payload.RuntimeID, *job.ServerID, *job.ServiceID)
	if err != nil {
		return err
	}
	job.RuntimeID = state.ID
	switch job.JobType {
	case "start_service":
		err = rt.StartContainer(ctx, state.ID, *job.ServerID, *job.ServiceID)
	case "stop_service":
		err = rt.StopContainer(ctx, state.ID, *job.ServerID, *job.ServiceID)
	case "restart_service":
		err = rt.StopContainer(ctx, state.ID, *job.ServerID, *job.ServiceID)
		if err == nil {
			err = rt.StartContainer(ctx, state.ID, *job.ServerID, *job.ServiceID)
		}
	case "delete_service":
		err = rt.StopContainer(ctx, state.ID, *job.ServerID, *job.ServiceID)
		if err == nil {
			err = rt.DeleteContainer(ctx, state.ID, *job.ServerID, *job.ServiceID)
		}
		if err == nil {
			job.ServiceStatus = "deleted"
			return nil
		}
	}
	if err != nil {
		return err
	}
	state, err = rt.InspectOwned(ctx, state.ID, *job.ServerID, *job.ServiceID)
	if err != nil {
		return err
	}
	job.ServiceStatus = runtimeStatus(state)
	return nil
}

func reportJobResult(ctx context.Context, config *AgentConfig, jobID, status, message, runtimeID, serviceStatus string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = sendJobResult(ctx, config, jobID, status, message, runtimeID, serviceStatus)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return err
}
