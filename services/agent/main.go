package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type SystemInfo struct {
	Hostname        string
	CPUThreads      int
	TotalMemory     uint64
	AvailableMemory uint64
	MemoryPercent   float64
	TotalDisk       uint64
	FreeDisk        uint64
	DiskPercent     float64
}

type HealthResponse struct {
	Status          string  `json:"status"`
	Agent           string  `json:"agent"`
	Hostname        string  `json:"hostname"`
	CPUThreads      int     `json:"cpu_threads"`
	TotalMemory     uint64  `json:"total_memory"`
	AvailableMemory uint64  `json:"available_memory"`
	MemoryPercent   float64 `json:"memory_percent"`
	TotalDisk       uint64  `json:"total_disk"`
	FreeDisk        uint64  `json:"free_disk"`
	DiskPercent     float64 `json:"disk_percent"`
	Timestamp       string  `json:"timestamp"`
}

type EnrollRequest struct {
	Token       string `json:"token"`
	Hostname    string `json:"hostname"`
	CPUThreads  int    `json:"cpu_threads"`
	TotalMemory uint64 `json:"total_memory"`
	TotalDisk   uint64 `json:"total_disk"`
}

type EnrollResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	Location  string `json:"location"`
	Status    string `json:"status"`
	NodeToken string `json:"node_token"`
}

type AgentConfig struct {
	NodeID    string `json:"node_id"`
	NodeToken string `json:"node_token"`
	CoreURL   string `json:"core_url"`
}

type HeartbeatRequest struct {
	CPUThreads  int    `json:"cpu_threads"`
	TotalMemory uint64 `json:"total_memory"`
	TotalDisk   uint64 `json:"total_disk"`
}

func getSystemInfo() (*SystemInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("determine hostname: %w", err)
	}

	memory, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("determine memory: %w", err)
	}

	storage, err := disk.Usage("/")
	if err != nil {
		return nil, fmt.Errorf("determine disk: %w", err)
	}

	return &SystemInfo{
		Hostname:        hostname,
		CPUThreads:      runtime.NumCPU(),
		TotalMemory:     memory.Total,
		AvailableMemory: memory.Available,
		MemoryPercent:   memory.UsedPercent,
		TotalDisk:       storage.Total,
		FreeDisk:        storage.Free,
		DiskPercent:     storage.UsedPercent,
	}, nil
}

func configPath() (string, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config directory: %w", err)
	}

	return filepath.Join(
		configDirectory,
		"build-and-break",
		"agent.json",
	), nil
}

func saveConfig(config AgentConfig) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}

	directory := filepath.Dir(path)

	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode agent configuration: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write agent configuration: %w", err)
	}

	if err := os.Chmod(path, 0600); err != nil {
		return "", fmt.Errorf("secure agent configuration: %w", err)
	}

	return path, nil
}

func loadConfig() (*AgentConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent configuration: %w", err)
	}

	var config AgentConfig

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("decode agent configuration: %w", err)
	}

	if config.NodeID == "" ||
		config.NodeToken == "" ||
		config.CoreURL == "" {
		return nil, fmt.Errorf("agent configuration is incomplete")
	}

	return &config, nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	system, err := getSystemInfo()
	if err != nil {
		http.Error(
			w,
			"failed to determine system information",
			http.StatusInternalServerError,
		)
		return
	}

	response := HealthResponse{
		Status:          "online",
		Agent:           "bnb-agent",
		Hostname:        system.Hostname,
		CPUThreads:      system.CPUThreads,
		TotalMemory:     system.TotalMemory,
		AvailableMemory: system.AvailableMemory,
		MemoryPercent:   system.MemoryPercent,
		TotalDisk:       system.TotalDisk,
		FreeDisk:        system.FreeDisk,
		DiskPercent:     system.DiskPercent,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(
			w,
			"failed to encode response",
			http.StatusInternalServerError,
		)
	}
}

func enroll() error {
	coreURL := os.Getenv("BNB_CORE_URL")
	token := os.Getenv("BNB_ENROLL_TOKEN")

	if coreURL == "" {
		return fmt.Errorf("BNB_CORE_URL is not set")
	}

	if token == "" {
		return fmt.Errorf("BNB_ENROLL_TOKEN is not set")
	}

	system, err := getSystemInfo()
	if err != nil {
		return err
	}

	request := EnrollRequest{
		Token:       token,
		Hostname:    system.Hostname,
		CPUThreads:  system.CPUThreads,
		TotalMemory: system.TotalMemory,
		TotalDisk:   system.TotalDisk,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode enrollment request: %w", err)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	response, err := client.Post(
		coreURL+"/api/v1/nodes/enroll",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("contact Core: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read Core response: %w", err)
	}

	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf(
			"Core rejected enrollment: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)
	}

	var result EnrollResponse

	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("decode Core response: %w", err)
	}

	if result.ID == "" || result.NodeToken == "" {
		return fmt.Errorf("Core returned incomplete node credentials")
	}

	config := AgentConfig{
		NodeID:    result.ID,
		NodeToken: result.NodeToken,
		CoreURL:   coreURL,
	}

	path, err := saveConfig(config)
	if err != nil {
		return fmt.Errorf("save node identity: %w", err)
	}

	fmt.Println("Build & Break node enrolled successfully")
	fmt.Printf("Node ID:  %s\n", result.ID)
	fmt.Printf("Name:     %s\n", result.Name)
	fmt.Printf("Hostname: %s\n", result.Hostname)
	fmt.Printf("Location: %s\n", result.Location)
	fmt.Printf("Status:   %s\n", result.Status)
	fmt.Printf("Identity saved securely to: %s\n", path)

	return nil
}

func sendHeartbeat(config *AgentConfig) error {
	system, err := getSystemInfo()
	if err != nil {
		return err
	}

	heartbeat := HeartbeatRequest{
		CPUThreads:  system.CPUThreads,
		TotalMemory: system.TotalMemory,
		TotalDisk:   system.TotalDisk,
	}

	body, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("encode heartbeat: %w", err)
	}

	url := fmt.Sprintf(
		"%s/api/v1/nodes/%s/heartbeat",
		config.CoreURL,
		config.NodeID,
	)

	request, err := http.NewRequest(
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("create heartbeat request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(
		"Authorization",
		"Bearer "+config.NodeToken,
	)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("contact Core: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read heartbeat response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"Core rejected heartbeat: HTTP %d: %s",
			response.StatusCode,
			string(responseBody),
		)
	}

	return nil
}

func heartbeatLoop(ctx context.Context, config *AgentConfig) {
	send := func() {
		if err := sendHeartbeat(config); err != nil {
			log.Printf("Heartbeat failed: %v", err)
			return
		}

		log.Printf("Heartbeat sent successfully")
	}

	// Send one immediately rather than waiting 30 seconds
	// after the agent starts.
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

func runAgent() error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf(
			"agent is not configured: %w\nrun 'bnb-agent enroll' first",
			err,
		)
	}

	log.Printf("Loaded node identity: %s", config.NodeID)
	log.Printf("Connected Core: %s", config.CoreURL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go heartbeatLoop(ctx, config)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)

	server := &http.Server{
		Addr:         "127.0.0.1:9090",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Println("Build & Break Agent starting on 127.0.0.1:9090")

	return server.ListenAndServe()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "enroll" {
		if err := enroll(); err != nil {
			log.Fatalf("enrollment failed: %v", err)
		}

		return
	}

	if err := runAgent(); err != nil {
		log.Fatal(err)
	}
}
