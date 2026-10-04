package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnrollmentWireContractAndPrivateIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	t.Setenv("BNB_AGENT_CONFIG", path)
	t.Setenv("BNB_ENROLLMENT_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if r.URL.Path != "/api/v1/nodes/enroll" || json.NewDecoder(r.Body).Decode(&req) != nil {
			t.Error("invalid enrollment request")
			w.WriteHeader(400)
			return
		}
		if req["token"] != "test-token" || req["total_memory"] == nil || req["total_disk"] == nil {
			t.Error("Core contract mismatch")
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "node-identity", "node_token": "private-test-credential"})
	}))
	defer server.Close()
	config, err := enroll(context.Background(), server.URL, "test-token", "", "")
	if err != nil || config.NodeID != "node-identity" {
		t.Fatalf("enrollment failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential file is not private")
	}
	loaded, err := loadConfig()
	if err != nil || loaded.NodeToken != config.NodeToken {
		t.Fatal("saved identity cannot be loaded")
	}
	if err := enrollCommand([]string{"--core", server.URL}); err == nil {
		t.Fatal("existing node identity could be overwritten")
	}
}
