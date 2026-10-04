package platform

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/jobs"
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestMinecraftOwnedConfigurationAndReplacementResult(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	other := "22222222-2222-4222-8222-222222222222"
	node := "33333333-3333-4333-8333-333333333333"
	server := "44444444-4444-4444-8444-444444444444"
	service := "55555555-5555-4555-8555-555555555555"
	_, err := db.Exec(ctx, `INSERT INTO accounts(id,email,name,password_hash) VALUES($1,'owner@test.invalid','Owner','test'),($2,'other@test.invalid','Other','test')`, owner, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location,last_seen_at,runtime_available) VALUES($1,'test','test','test',NOW(),true)`, node)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("test-node"))
	_, err = db.Exec(ctx, `INSERT INTO node_credentials(node_id,token_hash) VALUES($1,$2)`, node, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO servers(id,node_id,name,status,memory_mb,disk_mb,cpu_threads,account_id) VALUES($1,$2,'test','active',2048,4096,2,$3)`, server, node, owner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO services(id,server_id,name,template,status,memory_mb,cpu_threads,port,runtime_id) VALUES($1,$2,'test','minecraft-java','running',2048,2,25565,'original')`, service, server)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{DB: db, MinecraftJavaResolver: func(context.Context, string) (int, error) { return 21, nil }}
	cfg := DefaultMinecraftConfig()
	cfg.Type = "PAPER"
	cfg.Projects = []string{"luckperms"}
	request := func(actor, method string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(cfg)
		r := httptest.NewRequest(method, "/", bytes.NewReader(b))
		r.SetPathValue("serviceID", service)
		r = r.WithContext(accounts.WithUser(r.Context(), accounts.User{ID: actor, Role: "customer"}))
		w := httptest.NewRecorder()
		h.Minecraft(w, r)
		return w
	}
	if request(other, "GET").Code != 404 || request(other, "PUT").Code != 404 {
		t.Fatal("another customer accessed settings")
	}
	w := request(owner, "PUT")
	if w.Code != 202 {
		t.Fatalf("queue settings %d %s", w.Code, w.Body.String())
	}
	if request(owner, "PUT").Code != 409 {
		t.Fatal("concurrent reconfiguration accepted")
	}
	var queued map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &queued)
	_, err = db.Exec(ctx, `UPDATE provisioning_jobs SET status='processing' WHERE id=$1`, queued["job_id"])
	if err != nil {
		t.Fatal(err)
	}
	result := jobs.ResultRequest{Status: "completed", RuntimeID: "replacement", ServiceStatus: "running"}
	b, _ := json.Marshal(result)
	r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
	r.SetPathValue("id", node)
	r.SetPathValue("jobID", queued["job_id"])
	r.Header.Set("Authorization", "Bearer test-node")
	j := &jobs.Handler{DB: db}
	w = httptest.NewRecorder()
	j.Result(w, r)
	if w.Code != 200 {
		t.Fatalf("replacement commit %d %s", w.Code, w.Body.String())
	}
	var rid string
	var applied []byte
	if db.QueryRow(ctx, `SELECT runtime_id,minecraft_applied_config FROM services WHERE id=$1`, service).Scan(&rid, &applied) != nil || rid != "replacement" || !bytes.Contains(applied, []byte("PAPER")) {
		t.Fatal("replacement identity/settings not applied")
	}
}
