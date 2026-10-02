package jobs

import "testing"

func TestResultSemantics(t *testing.T) {
	for _, tc := range []struct {
		kind, status, id string
		ok               bool
	}{
		{"create_container", "running", "runtime", true}, {"stop_service", "running", "runtime", false},
		{"delete_service", "deleted", "runtime", true}, {"delete_service", "running", "runtime", false},
		{"start_service", "running", "", false}, {"restart_service", "stopped", "runtime", true},
	} {
		if got := validCompletedResult(tc.kind, ResultRequest{RuntimeID: tc.id, ServiceStatus: tc.status}); got != tc.ok {
			t.Errorf("%+v got %v", tc, got)
		}
	}
}
