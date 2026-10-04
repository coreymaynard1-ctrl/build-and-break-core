package billing

import (
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"net/http/httptest"
	"testing"
)

func TestCataloguePublicationAndValidation(t *testing.T) {
	db := testutil.Database(t)
	h := &Handler{DB: db, Mode: "live"}
	ctx := context.Background()
	save := func(body string, want int) {
		t.Helper()
		r := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))
		r.SetPathValue("planID", "minecraft-starter")
		w := httptest.NewRecorder()
		h.SavePlan(w, r)
		if w.Code != want {
			t.Fatalf("plan %d: %s", w.Code, w.Body.String())
		}
	}
	save(`{"name":"Starter","currency":"GBP","amount_minor":1500,"memory_mb":4096,"disk_mb":4096,"cpu_threads":2,"template":"minecraft-java","active":true,"is_demo":false}`, 200)
	w := httptest.NewRecorder()
	h.Plans(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("minecraft-starter")) || bytes.Contains(w.Body.Bytes(), []byte("dev-alpine")) {
		t.Fatal("live catalogue leaked demo plans or hid published plan")
	}
	save(`{"name":"Starter","currency":"GBP","amount_minor":-1,"memory_mb":4096,"disk_mb":4096,"cpu_threads":2,"template":"minecraft-java","active":true,"is_demo":false}`, 400)
	var amount int64
	if err := db.QueryRow(ctx, `SELECT amount_minor FROM billing_plans WHERE id='minecraft-starter'`).Scan(&amount); err != nil || amount != 1500 {
		t.Fatal("invalid save changed published price")
	}
	save(`{"name":"Starter","currency":"GBP","amount_minor":1500,"memory_mb":64,"disk_mb":4096,"cpu_threads":2,"template":"minecraft-java","active":true,"is_demo":false}`, 400)
}
