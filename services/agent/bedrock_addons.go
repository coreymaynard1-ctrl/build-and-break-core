package main

import (
	"buildandbreak/platform/internal/httpx"
	bnbruntime "buildandbreak/platform/internal/runtime"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func executeBedrockAddonJob(ctx context.Context, config *AgentConfig, rt *bnbruntime.DockerRuntime, job *ProvisioningJob) error {
	if job.ServiceID == nil || job.ServerID == nil {
		return fmt.Errorf("addon job requires service identity")
	}
	var p struct {
		RuntimeID  string          `json:"runtime_id"`
		ArtifactID string          `json:"artifact_id"`
		SHA        string          `json:"sha256"`
		Behavior   json.RawMessage `json:"behavior_packs"`
		Resource   json.RawMessage `json:"resource_packs"`
	}
	if json.Unmarshal(job.Payload, &p) != nil || p.RuntimeID == "" {
		return fmt.Errorf("invalid addon payload")
	}
	state, err := rt.InspectOwned(ctx, p.RuntimeID, *job.ServerID, *job.ServiceID)
	if err != nil {
		return err
	}
	var data []byte
	if job.JobType == "install_bedrock_addons" {
		if !httpx.UUID(p.ArtifactID) {
			return fmt.Errorf("invalid addon artifact")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/v1/nodes/%s/bedrock-artifacts/%s", config.CoreURL, config.NodeID, p.ArtifactID), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+config.NodeToken)
		res, err := (&http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("addon download HTTP %d", res.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(res.Body, (66<<20)+1))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		if len(data) > 66<<20 || hex.EncodeToString(hash[:]) != p.SHA {
			return fmt.Errorf("addon artifact size or hash mismatch")
		}
	}
	job.RuntimeID = state.ID
	if state.Running {
		if err = rt.StopContainer(ctx, state.ID, *job.ServerID, *job.ServiceID); err != nil {
			return err
		}
	}
	if job.JobType == "install_bedrock_addons" {
		err = rt.CopyBedrockAddon(ctx, state.ID, *job.ServerID, *job.ServiceID, data)
	} else {
		err = rt.SetBedrockPacks(ctx, state.ID, *job.ServerID, *job.ServiceID, p.Behavior, p.Resource)
	}
	if err != nil {
		return err
	}
	if state.Running {
		if err = rt.StartContainer(ctx, state.ID, *job.ServerID, *job.ServiceID); err != nil {
			return err
		}
		job.ServiceStatus = "running"
	} else {
		job.ServiceStatus = "stopped"
	}
	return nil
}
