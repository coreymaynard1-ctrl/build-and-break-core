package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	bnbruntime "buildandbreak/platform/internal/runtime"
)

type SystemInfo struct {
	Hostname         string `json:"hostname"`
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	CPUThreads       int    `json:"cpu_threads"`
	TotalMemoryBytes uint64 `json:"total_memory_bytes"`
	TotalDiskBytes   uint64 `json:"total_disk_bytes"`
}

type HealthResponse struct {
	Status        string     `json:"status"`
	Service       string     `json:"service"`
	NodeID        string     `json:"node_id"`
	System        SystemInfo `json:"system"`
	DockerReady   bool       `json:"docker_ready"`
	DockerName    string     `json:"docker_name,omitempty"`
	DockerVersion string     `json:"docker_version,omitempty"`
}

type EnrollRequest struct {
	Token            string `json:"token"`
	Name             string `json:"name"`
	Hostname         string `json:"hostname"`
	Location         string `json:"location"`
	CPUThreads       int    `json:"cpu_threads"`
	TotalMemoryBytes uint64 `json:"total_memory_bytes"`
	TotalDiskBytes   uint64 `json:"total_disk_bytes"`
}

type EnrollResponse struct {
	NodeID    string `json:"node_id"`
	NodeToken string `json:"node_token"`
}

type AgentConfig struct {
	NodeID    string `json:"node_id"`
	NodeToken string `json:"node_token"`
	CoreURL   string `json:"core_url"`
}

type HeartbeatRequest struct {
	CPUThreads  int   `json:"cpu_threads"`
	TotalMemory int64 `json:"total_memory"`
	TotalDisk   int64 `json:"total_disk"`

	RuntimeAvailable  bool   `json:"runtime_available"`
	RuntimeName       string `json:"runtime_name"`
	RuntimeVersion    string `json:"runtime_version"`
	RuntimeAPIVersion string `json:"runtime_api_version"`
}

func getSystemInfo() (SystemInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return SystemInfo{}, fmt.Errorf(
			"determine hostname: %w",
			err,
		)
	}

	memory, err := readTotalMemory()
	if err != nil {
		return SystemInfo{}, err
	}

	disk, err := readTotalDisk("/")
	if err != nil {
		return SystemInfo{}, err
	}

	return SystemInfo{
		Hostname:         hostname,
		OS:               runtime.GOOS,
		Architecture:     runtime.GOARCH,
		CPUThreads:       runtime.NumCPU(),
		TotalMemoryBytes: memory,
		TotalDiskBytes:   disk,
	}, nil
}

func readTotalMemory() (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf(
			"read /proc/meminfo: %w",
			err,
		)
	}

	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		if fields[0] != "MemTotal:" {
			continue
		}

		var memoryKB uint64

		if _, err := fmt.Sscanf(
			fields[1],
			"%d",
			&memoryKB,
		); err != nil {
			return 0, fmt.Errorf(
				"parse total memory: %w",
				err,
			)
		}

		return memoryKB * 1024, nil
	}

	return 0, fmt.Errorf(
		"MemTotal not found in /proc/meminfo",
	)
}

func readTotalDisk(path string) (uint64, error) {
	// This uses the Linux filesystem statistics available through
	// syscall.Statfs. The agent currently targets Linux nodes.
	var stat syscall.Statfs_t

	if err := syscall.Statfs(
		path,
		&stat,
	); err != nil {
		return 0, fmt.Errorf(
			"read disk information: %w",
			err,
		)
	}

	return stat.Blocks * uint64(stat.Bsize), nil
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf(
			"determine home directory: %w",
			err,
		)
	}

	return filepath.Join(
		home,
		".config",
		"build-and-break",
		"agent.json",
	), nil
}

func loadConfig() (*AgentConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config AgentConfig

	if err := json.Unmarshal(
		data,
		&config,
	); err != nil {
		return nil, fmt.Errorf(
			"decode agent config: %w",
			err,
		)
	}

	if strings.TrimSpace(config.NodeID) == "" {
		return nil, fmt.Errorf(
			"agent config is missing node ID",
		)
	}

	if strings.TrimSpace(config.NodeToken) == "" {
		return nil, fmt.Errorf(
			"agent config is missing node token",
		)
	}

	if strings.TrimSpace(config.CoreURL) == "" {
		return nil, fmt.Errorf(
			"agent config is missing Core URL",
		)
	}

	config.CoreURL = strings.TrimRight(
		config.CoreURL,
		"/",
	)

	return &config, nil
}

func saveConfig(config *AgentConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	directory := filepath.Dir(path)

	if err := os.MkdirAll(
		directory,
		0700,
	); err != nil {
		return fmt.Errorf(
			"create agent config directory: %w",
			err,
		)
	}

	data, err := json.MarshalIndent(
		config,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"encode agent config: %w",
			err,
		)
	}

	if err := os.WriteFile(
		path,
		data,
		0600,
	); err != nil {
		return fmt.Errorf(
			"write agent config: %w",
			err,
		)
	}

	return nil
}

func enroll(
	ctx context.Context,
	coreURL string,
	token string,
	name string,
	location string,
) (*AgentConfig, error) {
	system, err := getSystemInfo()
	if err != nil {
		return nil, err
	}

	payload := EnrollRequest{
		Token:            token,
		Name:             name,
		Hostname:         system.Hostname,
		Location:         location,
		CPUThreads:       system.CPUThreads,
		TotalMemoryBytes: system.TotalMemoryBytes,
		TotalDiskBytes:   system.TotalDiskBytes,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf(
			"encode enrollment request: %w",
			err,
		)
	}

	url := strings.TrimRight(
		coreURL,
		"/",
	) + "/api/v1/nodes/enroll"

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create enrollment request: %w",
			err,
		)
	}

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf(
			"contact Core for enrollment: %w",
			err,
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(response.Body, 1024*1024),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read enrollment response: %w",
			err,
		)
	}

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"Core rejected enrollment: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)
	}

	var result EnrollResponse

	if err := json.Unmarshal(
		responseBody,
		&result,
	); err != nil {
		return nil, fmt.Errorf(
			"decode enrollment response: %w",
			err,
		)
	}

	if result.NodeID == "" ||
		result.NodeToken == "" {

		return nil, fmt.Errorf(
			"Core returned incomplete node credentials",
		)
	}

	config := &AgentConfig{
		NodeID:    result.NodeID,
		NodeToken: result.NodeToken,
		CoreURL: strings.TrimRight(
			coreURL,
			"/",
		),
	}

	if err := saveConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

func sendHeartbeat(
	ctx context.Context,
	config *AgentConfig,
	dockerRuntime *bnbruntime.DockerRuntime,
) error {
	system, err := getSystemInfo()
	if err != nil {
		return err
	}

	heartbeat := HeartbeatRequest{
		CPUThreads:  system.CPUThreads,
		TotalMemory: int64(system.TotalMemoryBytes),
		TotalDisk:   int64(system.TotalDiskBytes),
	}

	if dockerRuntime != nil {
		runtimeInfo, err := dockerRuntime.Check(ctx)

		if err != nil {
			heartbeat.RuntimeAvailable = false
			heartbeat.RuntimeName = "docker"
		} else {
			heartbeat.RuntimeAvailable = true
			heartbeat.RuntimeName = "docker"
			heartbeat.RuntimeVersion = runtimeInfo.Version
			heartbeat.RuntimeAPIVersion = runtimeInfo.APIVersion
		}
	}

	body, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf(
			"encode heartbeat: %w",
			err,
		)
	}

	url := fmt.Sprintf(
		"%s/api/v1/nodes/%s/heartbeat",
		config.CoreURL,
		config.NodeID,
	)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf(
			"create heartbeat request: %w",
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
			"send heartbeat: %w",
			err,
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(response.Body, 1024*1024),
	)
	if err != nil {
		return fmt.Errorf(
			"read heartbeat response: %w",
			err,
		)
	}

	if response.StatusCode < 200 ||
		response.StatusCode >= 300 {

		return fmt.Errorf(
			"Core rejected heartbeat: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)
	}

	return nil
}

func heartbeatLoop(
	ctx context.Context,
	config *AgentConfig,
	dockerRuntime *bnbruntime.DockerRuntime,
) {
	send := func() {
		if err := sendHeartbeat(
			ctx,
			config,
			dockerRuntime,
		); err != nil {

			log.Printf(
				"Heartbeat failed: %v",
				err,
			)

			return
		}

		log.Println(
			"Heartbeat sent successfully",
		)
	}

	send()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			send()

		case <-ctx.Done():
			return
		}
	}
}

func healthHandler(
	config *AgentConfig,
	system SystemInfo,
	dockerRuntime *bnbruntime.DockerRuntime,
) http.HandlerFunc {
	return func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		response := HealthResponse{
			Status:  "ok",
			Service: "build-and-break-agent",
			NodeID:  config.NodeID,
			System:  system,
		}

		if dockerRuntime != nil {
			runtimeInfo, err := dockerRuntime.Check(
				r.Context(),
			)

			if err == nil {
				response.DockerReady = true
				response.DockerName = "docker"
				response.DockerVersion = runtimeInfo.Version
			}
		}

		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		if err := json.NewEncoder(w).Encode(
			response,
		); err != nil {

			http.Error(
				w,
				"failed to encode response",
				http.StatusInternalServerError,
			)
		}
	}
}

func runAgent() error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf(
			"load agent config: %w",
			err,
		)
	}

	log.Printf(
		"Loaded node identity: %s",
		config.NodeID,
	)

	log.Printf(
		"Connected Core: %s",
		config.CoreURL,
	)

	system, err := getSystemInfo()
	if err != nil {
		return fmt.Errorf(
			"read system information: %w",
			err,
		)
	}

	// Connect to the local Docker daemon.
	dockerRuntime, err := bnbruntime.NewDockerRuntime()
	if err != nil {
		return fmt.Errorf(
			"create Docker runtime: %w",
			err,
		)
	}
	defer dockerRuntime.Close()

	runtimeInfo, err := dockerRuntime.Check(
		context.Background(),
	)
	if err != nil {
		return fmt.Errorf(
			"Docker runtime unavailable: %w",
			err,
		)
	}

	log.Println(
		"Docker runtime available",
	)

	log.Printf(
		"Docker version: %s",
		runtimeInfo.Version,
	)

	log.Printf(
		"Docker API version: %s",
		runtimeInfo.APIVersion,
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	defer cancel()

	// Heartbeats continuously tell Core that this physical node
	// is alive and report its current Docker/runtime state.
	go heartbeatLoop(
		ctx,
		config,
		dockerRuntime,
	)

	// Provisioning jobs are fetched from Core and executed locally.
	//
	// We pass dockerRuntime into the job worker because jobs such as
	// create_container now need to perform real Docker operations.
	go jobPollingLoop(
		ctx,
		config,
		dockerRuntime,
	)

	go serviceSyncLoop(ctx, config, dockerRuntime)
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /health",
		healthHandler(
			config,
			system,
			dockerRuntime,
		),
	)

	server := &http.Server{
		Addr:         "127.0.0.1:9090",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Shut the local HTTP server down cleanly when the agent receives
	// Ctrl+C or a normal service termination signal.
	signalChannel := make(
		chan os.Signal,
		1,
	)

	signal.Notify(
		signalChannel,
		os.Interrupt,
		syscall.SIGTERM,
	)

	go func() {
		<-signalChannel

		log.Println(
			"Agent shutdown requested",
		)

		cancel()

		shutdownCtx, shutdownCancel :=
			context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
		defer shutdownCancel()

		if err := server.Shutdown(
			shutdownCtx,
		); err != nil {

			log.Printf(
				"Agent HTTP shutdown failed: %v",
				err,
			)
		}
	}()

	log.Println(
		"Build & Break Agent listening on 127.0.0.1:9090",
	)

	err = server.ListenAndServe()

	if errors.Is(
		err,
		http.ErrServerClosed,
	) {
		return nil
	}

	return err
}

func generateRandomToken(
	bytesLength int,
) (string, error) {
	data := make([]byte, bytesLength)

	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	result := make([]byte, len(data))

	for i, value := range data {
		result[i] = alphabet[int(value)%len(alphabet)]
	}

	return string(result), nil
}

func main() {
	if len(os.Args) > 1 &&
		os.Args[1] == "enroll" {

		log.Fatal(
			"This agent is already designed to use the existing enrollment workflow. Use the established enrollment command/config rather than re-enrolling this node.",
		)
	}

	if err := runAgent(); err != nil {
		log.Fatal(err)
	}
}
