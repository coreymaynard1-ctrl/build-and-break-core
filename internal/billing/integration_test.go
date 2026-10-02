package billing

import (
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestOrderPaymentIdempotencyAndFulfillment(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	node := "c6076de5-90d4-4842-b903-df48d453637c"
	_, err := db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location,cpu_threads,total_memory,total_disk,last_seen_at,runtime_available) VALUES($1,'fixture','fixture','fixture',4,4294967296,4294967296,NOW(),true)`, node)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{DB: db, Sandbox: true}
	order := OrderRequest{PlanID: "dev-alpine", NodeID: node, Name: "test", Email: "test@example.com"}
	callOrder := func(req OrderRequest, want int) map[string]string {
		t.Helper()
		b, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
		r.Header.Set("Idempotency-Key", "test-order-unique")
		w := httptest.NewRecorder()
		h.Order(w, r)
		if w.Code != want {
			t.Fatalf("order status=%d: %s", w.Code, w.Body.String())
		}
		var out map[string]string
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	created := callOrder(order, 201)
	replay := callOrder(order, 200)
	if created["invoice_id"] != replay["invoice_id"] {
		t.Fatal("duplicate invoice")
	}
	changed := order
	changed.Name = "changed"
	callOrder(changed, 409)
	pay := func(want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest("POST", "/", nil)
		r.SetPathValue("invoiceID", created["invoice_id"])
		w := httptest.NewRecorder()
		h.PaySandbox(w, r)
		if w.Code != want {
			t.Fatalf("payment status=%d: %s", w.Code, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	// Capacity/offline rejection must not mark invoice paid or create a payment.
	_, err = db.Exec(ctx, `UPDATE nodes SET runtime_available=false WHERE id=$1`, node)
	if err != nil {
		t.Fatal(err)
	}
	pay(409)
	var count int
	var status string
	if db.QueryRow(ctx, `SELECT count(*) FROM billing_payments`).Scan(&count) != nil || count != 0 {
		t.Fatal("failed fulfillment created payment")
	}
	if db.QueryRow(ctx, `SELECT status FROM billing_invoices WHERE id=$1`, created["invoice_id"]).Scan(&status) != nil || status != "unpaid" {
		t.Fatal("failed fulfillment changed invoice")
	}
	_, err = db.Exec(ctx, `UPDATE nodes SET runtime_available=true WHERE id=$1`, node)
	if err != nil {
		t.Fatal(err)
	}
	paid := pay(200)
	again := pay(200)
	if paid["service_id"] != again["service_id"] {
		t.Fatal("duplicate fulfillment")
	}
	for _, table := range []string{"billing_orders", "billing_invoices", "billing_payments", "servers", "services", "provisioning_jobs"} {
		if db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count) != nil || count != 1 {
			t.Fatalf("%s count=%d", table, count)
		}
	}
	var memory int
	var payload []byte
	if db.QueryRow(ctx, `SELECT memory_mb FROM servers WHERE id=$1`, paid["server_id"]).Scan(&memory) != nil || memory != 128 {
		t.Fatal("wrong entitlement")
	}
	if db.QueryRow(ctx, `SELECT payload FROM provisioning_jobs WHERE id=$1`, paid["job_id"]).Scan(&payload) != nil {
		t.Fatal("job missing")
	}
	var p map[string]any
	json.Unmarshal(payload, &p)
	if p["image"] != "alpine:3.20" {
		t.Fatal("template bypass")
	}
}
