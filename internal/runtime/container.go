package runtime

import "time"

const (
	ManagedLabel   = "com.buildandbreak.managed"
	ServerIDLabel  = "com.buildandbreak.server_id"
	JobIDLabel     = "com.buildandbreak.job_id"
	ServiceIDLabel = "com.buildandbreak.service_id"
)

type PortBinding struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type ContainerSpec struct {
	ExtraPorts []PortBinding
	Name       string
	Image      string
	ServerID   string
	ServiceID  string
	JobID      string

	MemoryBytes int64
	CPUThreads  int

	Environment   []string
	Command       []string
	ContainerPort int
	PortProtocol  string
	Port          int
	VolumeName    string
}

type ContainerState struct {
	ID        string
	ServerID  string
	ServiceID string
	JobID     string
	Name      string
	Image     string
	Status    string
	Running   bool
	ExitCode  int
	StartedAt time.Time
}

func (spec ContainerSpec) Labels() map[string]string {
	return map[string]string{
		ManagedLabel:   "true",
		ServerIDLabel:  spec.ServerID,
		ServiceIDLabel: spec.ServiceID,
		JobIDLabel:     spec.JobID,
	}
}
