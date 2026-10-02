package main

import (
	bnbruntime "buildandbreak/platform/internal/runtime"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type assignment struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"server_id"`
	RuntimeID   *string   `json:"runtime_id"`
	Revision    time.Time `json:"revision"`
	RuntimeName *string   `json:"runtime_name"`
}
type observation struct {
	ServiceID string    `json:"service_id"`
	RuntimeID string    `json:"runtime_id"`
	Status    string    `json:"status"`
	Revision  time.Time `json:"revision"`
}

func syncServices(ctx context.Context, config *AgentConfig, rt *bnbruntime.DockerRuntime) error {
	base := fmt.Sprintf("%s/api/v1/nodes/%s/services", config.CoreURL, config.NodeID)
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", base, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+config.NodeToken)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("inventory HTTP %d", res.StatusCode)
	}
	var assignments []assignment
	if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&assignments); err != nil {
		return err
	}
	observations := []observation{}
	for _, a := range assignments {
		target := ""
		if a.RuntimeID != nil {
			target = *a.RuntimeID
		} else if a.RuntimeName != nil {
			target = *a.RuntimeName
		}
		if target == "" {
			continue
		}
		state, err := rt.InspectOwned(ctx, target, a.ServerID, a.ID)
		status := "unknown"
		rid := target
		if err == nil {
			status = runtimeStatus(state)
			rid = state.ID
		} else if a.RuntimeID != nil && bnbruntime.IsNotFound(err) {
			status = "missing"
		} else if a.RuntimeID == nil {
			continue
		}
		observations = append(observations, observation{a.ID, rid, status, a.Revision})
	}
	for start := 0; start < len(observations); start += 200 {
		end := min(start+200, len(observations))
		body, _ := json.Marshal(map[string]any{"services": observations[start:end]})
		req, err = http.NewRequestWithContext(ctx, "POST", base+"/observations", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+config.NodeToken)
		req.Header.Set("Content-Type", "application/json")
		res, err = client.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("observation HTTP %d", res.StatusCode)
		}
	}
	return nil
}
func serviceSyncLoop(ctx context.Context, config *AgentConfig, rt *bnbruntime.DockerRuntime) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		if err := syncServices(ctx, config, rt); err != nil {
			log.Printf("Service synchronization failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
