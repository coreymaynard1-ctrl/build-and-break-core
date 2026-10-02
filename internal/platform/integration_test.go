package platform

import (
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestConcurrentServiceBudgetAndActions(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	node := "c6076de5-90d4-4842-b903-df48d453637c"
	server := "1d1029df-f543-467f-84c4-20d7759a0622"
	_, err := db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location,cpu_threads,total_memory,total_disk,last_seen_at,runtime_available) VALUES($1,'fixture','fixture','fixture',4,4294967296,4294967296,NOW(),true)`, node)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO servers(id,node_id,name,status,memory_mb,disk_mb,cpu_threads) VALUES($1,$2,'fixture','active',128,128,1)`, server, node)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{DB: db}
	codes := make(chan int, 2)
	ids := make(chan string, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			b, _ := json.Marshal(CreateServiceRequest{ServerID: server, Name: "concurrent", Template: "alpine-test", MemoryMB: 64, CPUThreads: 1})
			r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
			w := httptest.NewRecorder()
			h.CreateService(w, r)
			codes <- w.Code
			var result map[string]string
			json.Unmarshal(w.Body.Bytes(), &result)
			if result["id"] != "" {
				ids <- result["id"]
			}
		})
	}
	wg.Wait()
	close(codes)
	close(ids)
	accepted, conflicts := 0, 0
	for code := range codes {
		if code == 202 {
			accepted++
		} else if code == 409 {
			conflicts++
		} else {
			t.Errorf("status %d", code)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatal("oversubscribed allocation")
	}
	service := <-ids
	_, err = db.Exec(ctx, `UPDATE provisioning_jobs SET status='completed' WHERE service_id=$1`, service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `UPDATE services SET status='running',runtime_id='fixture-runtime' WHERE id=$1`, service)
	if err != nil {
		t.Fatal(err)
	}
	action := func(want int) {
		r := httptest.NewRequest("POST", "/", nil)
		r.SetPathValue("serviceID", service)
		r.SetPathValue("action", "stop")
		w := httptest.NewRecorder()
		h.Action(w, r)
		if w.Code != want {
			t.Fatalf("action %d: %s", w.Code, w.Body.String())
		}
	}
	action(202)
	action(409)
}
