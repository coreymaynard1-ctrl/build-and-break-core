package jobs

import (
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTransactionalResultsAndObservationRevision(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	node := "c6076de5-90d4-4842-b903-df48d453637c"
	server := "1d1029df-f543-467f-84c4-20d7759a0622"
	service := "d2bbe608-8644-44c7-9ddd-18615c6834c4"
	job := "8ac652e3-f860-4670-a9af-90202a5bd2fd"
	sum := sha256.Sum256([]byte("test-only-node-credential"))
	_, err := db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location) VALUES($1,'fixture','fixture','fixture')`, node)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO node_credentials(node_id,token_hash) VALUES($1,$2)`, node, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO servers(id,node_id,name,memory_mb,disk_mb,cpu_threads) VALUES($1,$2,'fixture',512,512,1)`, server, node)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO services(id,server_id,name,template) VALUES($1,$2,'fixture','alpine-test')`, service, server)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO provisioning_jobs(id,node_id,server_id,service_id,job_type,status) VALUES($1,$2,$3,$4,'create_container','processing')`, job, node, server, service)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{DB: db}
	result := func(body ResultRequest, want int) {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
		r.SetPathValue("id", node)
		r.SetPathValue("jobID", job)
		r.Header.Set("Authorization", "Bearer test-only-node-credential")
		w := httptest.NewRecorder()
		h.Result(w, r)
		if w.Code != want {
			t.Fatalf("result status %d: %s", w.Code, w.Body.String())
		}
	}
	result(ResultRequest{Status: "completed", ServiceStatus: "running"}, 400)
	var status string
	if db.QueryRow(ctx, `SELECT status FROM provisioning_jobs WHERE id=$1`, job).Scan(&status) != nil || status != "processing" {
		t.Fatal("invalid result changed job")
	}
	good := ResultRequest{Status: "completed", ServiceStatus: "running", RuntimeID: "test-runtime"}
	result(good, 200)
	result(good, 200)
	result(ResultRequest{Status: "completed", ServiceStatus: "stopped", RuntimeID: "test-runtime"}, 409)
	var rid string
	var revision time.Time
	if db.QueryRow(ctx, `SELECT runtime_id,status,updated_at FROM services WHERE id=$1`, service).Scan(&rid, &status, &revision) != nil || rid != "test-runtime" || status != "running" {
		t.Fatal("identity/state not committed together")
	}
	// Simulate an action/result after agent inventory. The stale observation must not overwrite it.
	if _, err = db.Exec(ctx, `UPDATE services SET status='stopped',updated_at=updated_at+INTERVAL '1 second' WHERE id=$1`, service); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"services": []Observation{{ServiceID: service, RuntimeID: rid, Status: "running", Revision: revision}}})
	r := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	r.SetPathValue("id", node)
	r.Header.Set("Authorization", "Bearer test-only-node-credential")
	w := httptest.NewRecorder()
	h.Observe(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if db.QueryRow(ctx, `SELECT status FROM services WHERE id=$1`, service).Scan(&status) != nil || status != "stopped" {
		t.Fatal("stale observation overwrote newer state")
	}
}
