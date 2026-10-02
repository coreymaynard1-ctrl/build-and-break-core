package runtime

import "time"

const (
	ManagedLabel   = "com.buildandbreak.managed"
	ServerIDLabel  = "com.buildandbreak.server_id"
	ServiceIDLabel = "com.buildandbreak.service_id"
)

type ContainerSpec struct {
	Name      string
	Image     string
	ServerID  string
	ServiceID string

	MemoryBytes int64
	CPUThreads  int

	Environment []string
	Command     []string
}

type ContainerState struct {
	ID        string
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
	}
}
