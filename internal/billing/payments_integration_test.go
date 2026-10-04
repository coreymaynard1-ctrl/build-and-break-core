package billing

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestConfirmedPaymentsSurviveOfflineNodesAndReplay(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	h := &Handler{DB: db, Mode: "test"}
	node := "11111111-1111-4111-8111-111111111111"
	var owner string
	if err := db.QueryRow(ctx, `INSERT INTO accounts(email,name,password_hash) VALUES('customer@example.test','Customer','fixture') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location,cpu_threads,total_memory,total_disk,last_seen_at,runtime_available) VALUES($1,'test','test','test',4,4294967296,4294967296,NULL,false)`, node); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/orders", bytes.NewBufferString(`{"plan_id":"dev-minecraft","node_id":"`+node+`","name":"Paid customer server","accept_eula":true}`))
	r.Header.Set("Idempotency-Key", "payment-fixture")
	r = r.WithContext(accounts.WithUser(ctx, accounts.User{ID: owner, Email: "customer@example.test", Role: "customer"}))
	w := httptest.NewRecorder()
	h.Order(w, r)
	if w.Code != 201 {
		t.Fatalf("order: %d %s", w.Code, w.Body.String())
	}
	var order map[string]string
	json.Unmarshal(w.Body.Bytes(), &order)
	invoice := order["invoice_id"]
	if _, err := db.Exec(ctx, `INSERT INTO billing_checkouts(invoice_id,provider,remote_id,checkout_url) VALUES($1,'stripe','cs_fixture','https://checkout.stripe.com/fixture')`, invoice); err != nil {
		t.Fatal(err)
	}
	event := PaymentEvent{ID: "evt_fixture", CheckoutID: "cs_fixture", Reference: "pi_fixture", Amount: 500, Currency: "GBP", Mode: "test"}
	bad := event
	bad.Amount = 1
	if err := h.recordPayment(ctx, "stripe", bad); err == nil {
		t.Fatal("accepted wrong amount")
	}
	bad = event
	bad.Mode = "live"
	if err := h.recordPayment(ctx, "stripe", bad); err == nil {
		t.Fatal("accepted wrong payment mode")
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			if err := h.recordPayment(ctx, "stripe", event); err != nil {
				t.Errorf("concurrent event: %v", err)
			}
		})
	}
	wg.Wait()
	duplicate := event
	duplicate.ID = "evt_async_second"
	if err := h.recordPayment(ctx, "stripe", duplicate); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM billing_payments WHERE invoice_id=$1`, invoice).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate payment recorded")
	}
	worked, err := h.FulfillNext(ctx)
	if err != nil || !worked {
		t.Fatalf("offline fulfillment: %v", err)
	}
	var paid, fulfillment string
	err = db.QueryRow(ctx, `SELECT i.status,f.status FROM billing_invoices i JOIN billing_fulfillments f ON f.invoice_id=i.id WHERE i.id=$1`, invoice).Scan(&paid, &fulfillment)
	if err != nil || paid != "paid" || fulfillment != "error" {
		t.Fatal("offline node lost payment or retry state")
	}
	if _, err = db.Exec(ctx, `UPDATE nodes SET last_seen_at=NOW(),runtime_available=true WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE billing_fulfillments SET next_attempt_at=NOW() WHERE invoice_id=$1`, invoice); err != nil {
		t.Fatal(err)
	}
	worked, err = h.FulfillNext(ctx)
	if err != nil || !worked {
		t.Fatalf("retry: %v", err)
	}
	var assigned string
	var port int
	err = db.QueryRow(ctx, `SELECT s.account_id,v.port FROM billing_orders o JOIN servers s ON s.id=o.server_id JOIN services v ON v.id=o.service_id WHERE o.id=$1`, order["order_id"]).Scan(&assigned, &port)
	if err != nil || assigned != owner || port != 25565 {
		t.Fatalf("ownership/automatic port: %s %d %v", assigned, port, err)
	}
	worked, err = h.FulfillNext(ctx)
	if err != nil || worked {
		t.Fatal("completed fulfillment ran twice")
	}
	if err = h.recordPayment(ctx, "stripe", event); err != nil {
		t.Fatal("replay after fulfillment failed")
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM provisioning_jobs`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate provisioning job")
	}
	h.Mode = "live"
	r = httptest.NewRequest("POST", "/orders", bytes.NewBufferString(`{"plan_id":"dev-alpine","node_id":"`+node+`","name":"Demo must not charge","email":"customer@example.test"}`))
	r.Header.Set("Idempotency-Key", "live-demo-fixture")
	w = httptest.NewRecorder()
	h.Order(w, r)
	if w.Code != 404 {
		t.Fatalf("demo plan available for live charges: %d", w.Code)
	}
}
