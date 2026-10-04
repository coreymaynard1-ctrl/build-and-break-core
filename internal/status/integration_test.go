package status

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMeasuredHealthAndIncidentHistory(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	h := &Handler{DB: db, StartedAt: time.Now()}
	var admin string
	if err := db.QueryRow(ctx, `INSERT INTO accounts(email,name,password_hash,role) VALUES('admin@example.test','Operator','test','admin') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	node := "11111111-1111-4111-8111-111111111111"
	if _, err := db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location,cpu_threads,total_memory,total_disk,last_seen_at,runtime_available) VALUES($1,'Node','private-host','London',4,4294967296,4294967296,NOW(),true)`, node); err != nil {
		t.Fatal(err)
	}
	read := func(want string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		h.Public(w, httptest.NewRequest("GET", "/status", nil))
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var data map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data["status"] != want {
			t.Fatalf("want %s got %s", want, data["status"])
		}
		if bytes.Contains(w.Body.Bytes(), []byte("private-host")) {
			t.Fatal("internal hostname exposed")
		}
		return data
	}
	read("operational")
	if _, err := db.Exec(ctx, `UPDATE nodes SET last_seen_at=NOW()-INTERVAL '10 minutes' WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	read("degraded")
	if _, err := db.Exec(ctx, `UPDATE nodes SET last_seen_at=NOW() WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/incidents", bytes.NewBufferString(`{"title":"Maintenance","message":"Testing incident history","impact":"maintenance","status":"investigating"}`))
	r = r.WithContext(accounts.WithUser(r.Context(), accounts.User{ID: admin, Role: "admin"}))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created map[string]string
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"]
	read("degraded")
	update := func(want int) {
		t.Helper()
		r := httptest.NewRequest("PATCH", "/incidents/"+id, bytes.NewBufferString(`{"message":"Maintenance complete","status":"resolved"}`))
		r.SetPathValue("incidentID", id)
		r = r.WithContext(accounts.WithUser(r.Context(), accounts.User{ID: admin, Role: "admin"}))
		w := httptest.NewRecorder()
		h.Update(w, r)
		if w.Code != want {
			t.Fatalf("update %d: %s", w.Code, w.Body.String())
		}
	}
	update(200)
	data := read("operational")
	if len(data["incidents"].([]any)) != 1 {
		t.Fatal("resolved incident history lost")
	}
	update(409)
}
