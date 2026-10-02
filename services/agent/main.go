package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Location string `json:"location"`
	Status   string `json:"status"`
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

func healthHandler(w http.ResponseWriter, r *http.Request) {
	system, err := getSystemInfo()
	if err != nil {
		http.Error(w, "failed to determine system information", http.StatusInternalServerError)
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
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
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

	fmt.Println("Build & Break node enrolled successfully")
	fmt.Printf("Node ID:  %s\n", result.ID)
	fmt.Printf("Name:     %s\n", result.Name)
	fmt.Printf("Hostname: %s\n", result.Hostname)
	fmt.Printf("Location: %s\n", result.Location)
	fmt.Printf("Status:   %s\n", result.Status)

	return nil
}

func runAgent() error {
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
