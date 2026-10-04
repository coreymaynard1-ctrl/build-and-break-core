package runtime

import (
	"context"
	"encoding/json"
	"github.com/moby/moby/client"
	"io"
	"time"
)

type Metrics struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
	MemoryLimit uint64  `json:"memory_limit"`
}

func (d *DockerRuntime) Metrics(ctx context.Context, id, server, service string) (*Metrics, error) {
	if _, err := d.InspectOwned(ctx, id, server, service); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	res, err := d.client.ContainerStats(ctx, id, client.ContainerStatsOptions{IncludePreviousSample: true})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	type cpu struct {
		Usage struct {
			Total uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		System uint64 `json:"system_cpu_usage"`
		Online uint64 `json:"online_cpus"`
	}
	var sample struct {
		CPU      cpu `json:"cpu_stats"`
		Previous cpu `json:"precpu_stats"`
		Memory   struct {
			Usage uint64            `json:"usage"`
			Limit uint64            `json:"limit"`
			Stats map[string]uint64 `json:"stats"`
		} `json:"memory_stats"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&sample); err != nil {
		return nil, err
	}
	used := sample.Memory.Usage
	cache := sample.Memory.Stats["inactive_file"]
	if cache == 0 {
		cache = sample.Memory.Stats["total_inactive_file"]
	}
	if cache < used {
		used -= cache
	}
	result := &Metrics{MemoryBytes: used, MemoryLimit: sample.Memory.Limit}
	if sample.CPU.System > sample.Previous.System && sample.CPU.Usage.Total >= sample.Previous.Usage.Total {
		result.CPUPercent = float64(sample.CPU.Usage.Total-sample.Previous.Usage.Total) / float64(sample.CPU.System-sample.Previous.System) * float64(sample.CPU.Online) * 100
	}
	return result, nil
}
