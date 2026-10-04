package nodes

import (
	"buildandbreak/platform/internal/jobs"
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestMultipleNodeEnrollmentAndJobIsolation(t *testing.T) {
	db := testutil.Database(t)
	h := &Handler{DB: db}
	enroll := func(name string) EnrollResponse {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"name": name, "location": "test"})
		w := httptest.NewRecorder()
		h.CreateEnrollment(w, httptest.NewRequest("POST", "/", bytes.NewReader(b)))
		if w.Code != 201 {
			t.Fatalf("issue token: %d", w.Code)
		}
		var issued struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &issued)
		b, _ = json.Marshal(EnrollRequest{Token: issued.Token, Hostname: name, CPUThreads: 2, TotalMemory: 4294967296, TotalDisk: 10737418240})
		w = httptest.NewRecorder()
		h.Enroll(w, httptest.NewRequest("POST", "/", bytes.NewReader(b)))
		if w.Code != 201 {
			t.Fatalf("enroll: %d %s", w.Code, w.Body.String())
		}
		var result EnrollResponse
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		replay := httptest.NewRecorder()
		h.Enroll(replay, httptest.NewRequest("POST", "/", bytes.NewReader(b)))
		if replay.Code != 401 {
			t.Fatal("one-time token was reused")
		}
		return result
	}
	a, b := enroll("node-a"), enroll("node-b")
	if a.ID == b.ID || a.NodeToken == b.NodeToken {
		t.Fatal("node identities not independent")
	}
	_, err := db.Exec(context.Background(), `INSERT INTO provisioning_jobs(id,node_id,job_type,status) VALUES(gen_random_uuid(),$1,'test','pending')`, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	j := &jobs.Handler{DB: db}
	claim := func(id, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/", nil)
		r.SetPathValue("id", id)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		j.Claim(w, r)
		return w
	}
	if claim(b.ID, a.NodeToken).Code != 401 {
		t.Fatal("node A accessed node B")
	}
	w := claim(a.ID, a.NodeToken)
	var empty jobs.ClaimResponse
	_ = json.Unmarshal(w.Body.Bytes(), &empty)
	if w.Code != 200 || empty.Job != nil {
		t.Fatal("node A claimed another node's job")
	}
	w = claim(b.ID, b.NodeToken)
	var own jobs.ClaimResponse
	_ = json.Unmarshal(w.Body.Bytes(), &own)
	if w.Code != 200 || own.Job == nil || own.Job.NodeID != b.ID {
		t.Fatal("node B did not receive its own job")
	}
}
