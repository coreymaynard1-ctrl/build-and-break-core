package runtime

import (
	"context"
	"encoding/json"
	"github.com/moby/moby/client"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLifecycleRefusesWrongAllocation(t *testing.T) {
	for _, labels := range []map[string]string{
		{ManagedLabel: "false", ServerIDLabel: "server", ServiceIDLabel: "service"},
		{ManagedLabel: "true", ServerIDLabel: "another", ServiceIDLabel: "service"},
		{ManagedLabel: "true", ServerIDLabel: "server", ServiceIDLabel: "another"},
	} {
		mutations := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/json") {
				json.NewEncoder(w).Encode(map[string]any{"Id": "immutable-id", "Name": "/test", "Config": map[string]any{"Image": "alpine", "Labels": labels}, "State": map[string]any{"Status": "running", "Running": true}})
				return
			}
			mutations++
			w.WriteHeader(204)
		}))
		cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"))
		if err != nil {
			t.Fatal(err)
		}
		rt := &DockerRuntime{client: cli}
		ctx := context.Background()
		for _, call := range []func() error{
			func() error { return rt.StartContainer(ctx, "test", "server", "service") },
			func() error { return rt.StopContainer(ctx, "test", "server", "service") },
			func() error { return rt.DeleteContainer(ctx, "test", "server", "service") },
		} {
			if call() == nil {
				t.Fatal("unsafe allocation accepted")
			}
		}
		if mutations != 0 {
			t.Fatalf("sent %d unsafe mutations", mutations)
		}
		cli.Close()
		server.Close()
	}
}
func TestSpecRejectsArbitraryVolumes(t *testing.T) {
	spec := ContainerSpec{Name: "x", Image: "alpine", ServerID: "server", ServiceID: "service", MemoryBytes: 64 << 20, CPUThreads: 1, VolumeName: "unrelated"}
	if validateContainerSpec(spec) == nil {
		t.Fatal("accepted arbitrary volume")
	}
	spec.VolumeName = "bnb-service-service"
	if err := validateContainerSpec(spec); err != nil {
		t.Fatal(err)
	}
	spec.Port = 80
	if validateContainerSpec(spec) == nil {
		t.Fatal("accepted low port")
	}
}
