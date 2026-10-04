package accounts_test

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/billing"
	"buildandbreak/platform/internal/nodes"
	"buildandbreak/platform/internal/platform"
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionsAndTenantIsolation(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	a := &accounts.Handler{DB: db}
	p := &platform.Handler{DB: db}
	b := &billing.Handler{DB: db, Sandbox: true}
	n := &nodes.Handler{DB: db}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", a.Register)
	mux.HandleFunc("POST /auth/login", a.Login)
	mux.HandleFunc("GET /auth/session", a.Me)
	mux.HandleFunc("POST /auth/logout", a.Protect(false, a.Logout))
	mux.HandleFunc("GET /servers", a.Protect(false, p.Servers))
	mux.HandleFunc("DELETE /servers/{serverID}", a.Protect(false, p.DeleteServer))
	mux.HandleFunc("GET /services", a.Protect(false, p.Services))
	mux.HandleFunc("POST /services", a.Protect(false, p.CreateService))
	mux.HandleFunc("POST /services/{serviceID}/{action}", a.Protect(false, p.Action))
	mux.HandleFunc("GET /jobs", a.Protect(false, p.Jobs))
	mux.HandleFunc("GET /invoices", a.Protect(false, b.Invoices))
	mux.HandleFunc("POST /orders", a.Protect(false, b.Order))
	mux.HandleFunc("POST /invoices/{invoiceID}/pay", a.Protect(true, b.PaySandbox))
	mux.HandleFunc("GET /nodes", a.Protect(true, n.List))
	mux.HandleFunc("POST /deployments", a.Protect(true, p.Deploy))
	type session struct {
		User   accounts.User `json:"user"`
		CSRF   string        `json:"csrf_token"`
		Cookie *http.Cookie
	}
	call := func(method, path string, body any, s *session, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if s != nil {
			r.AddCookie(s.Cookie)
			if csrf {
				r.Header.Set("X-CSRF-Token", s.CSRF)
			}
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	require := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("want %d got %d: %s", code, w.Code, w.Body.String())
		}
	}
	signup := func(email string) *session {
		w := call("POST", "/auth/register", map[string]string{"email": email, "name": "Test Customer", "password": "strong test password 123"}, nil, false)
		require(w, 200)
		var s session
		if json.Unmarshal(w.Body.Bytes(), &s) != nil {
			t.Fatal("decode registration")
		}
		s.Cookie = w.Result().Cookies()[0]
		if !s.Cookie.HttpOnly || s.Cookie.SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe session cookie")
		}
		return &s
	}
	first, second := signup("one@example.test"), signup("two@example.test")
	node, server, service := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	_, err := db.Exec(ctx, `INSERT INTO nodes(id,name,hostname,location,cpu_threads,total_memory,total_disk,last_seen_at,runtime_available) VALUES($1,'test','test','test',8,8589934592,8589934592,NOW(),true)`, node)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO servers(id,node_id,name,status,memory_mb,disk_mb,cpu_threads,account_id) VALUES($1,$2,'private','active',256,128,2,$3)`, server, node, first.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO services(id,server_id,name,template,status,memory_mb,cpu_threads,runtime_id) VALUES($1,$2,'private','alpine-test','running',128,1,'private-runtime')`, service, server)
	if err != nil {
		t.Fatal(err)
	}
	require(call("GET", "/servers", nil, nil, false), 401)
	require(call("GET", "/nodes", nil, second, false), 403)
	require(call("POST", "/deployments", map[string]any{}, second, true), 403)
	for _, path := range []string{"/servers", "/services", "/jobs?service_id=" + service, "/invoices"} {
		w := call("GET", path, nil, second, false)
		require(w, 200)
		if w.Body.String() != "[]\n" {
			t.Fatalf("tenant data leaked at %s: %s", path, w.Body.String())
		}
	}
	require(call("POST", "/services/"+service+"/stop", nil, second, true), 404)
	require(call("POST", "/services", map[string]any{"server_id": server, "name": "attack", "template": "alpine-test", "memory_mb": 32, "cpu_threads": 1}, second, true), 404)
	require(call("DELETE", "/servers/"+server, nil, second, true), 404)
	require(call("POST", "/services/"+service+"/stop", nil, first, false), 403)
	w := call("POST", "/auth/login", map[string]string{"email": first.User.Email, "password": "incorrect"}, nil, false)
	require(w, 401)
	order := map[string]any{"plan_id": "dev-alpine", "node_id": node, "name": "my order", "email": "spoof@example.test"}
	orderCall := func(s *session) *httptest.ResponseRecorder {
		data, _ := json.Marshal(order)
		r := httptest.NewRequest("POST", "/orders", bytes.NewReader(data))
		r.AddCookie(s.Cookie)
		r.Header.Set("X-CSRF-Token", s.CSRF)
		r.Header.Set("Idempotency-Key", "same-browser-key")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	ow := orderCall(first)
	require(ow, 201)
	require(orderCall(second), 201)
	var invoice map[string]string
	json.Unmarshal(ow.Body.Bytes(), &invoice)
	require(call("POST", "/invoices/"+invoice["invoice_id"]+"/pay", nil, first, true), 403)
	var email, owner string
	err = db.QueryRow(ctx, `SELECT customer_email,account_id FROM billing_orders WHERE id=$1`, invoice["order_id"]).Scan(&email, &owner)
	if err != nil || email != first.User.Email || owner != first.User.ID {
		t.Fatal("order ownership or verified email incorrect")
	}
	// Admin payment simulation must fulfill for the invoice's owner, not the admin.
	if _, err = db.Exec(ctx, `UPDATE accounts SET role='admin' WHERE id=$1`, second.User.ID); err != nil {
		t.Fatal(err)
	}
	paid := call("POST", "/invoices/"+invoice["invoice_id"]+"/pay", nil, second, true)
	require(paid, 200)
	var fulfillment map[string]string
	if err = json.Unmarshal(paid.Body.Bytes(), &fulfillment); err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(ctx, `SELECT account_id FROM servers WHERE id=$1`, fulfillment["server_id"]).Scan(&owner)
	if err != nil || owner != first.User.ID {
		t.Fatal("fulfillment assigned to payer instead of customer")
	}
	// Session role changes take effect on the next request; expiry and disabling revoke access.
	if _, err = db.Exec(ctx, `UPDATE accounts SET disabled=true WHERE id=$1`, second.User.ID); err != nil {
		t.Fatal(err)
	}
	require(call("GET", "/nodes", nil, second, false), 401)
	require(call("POST", "/auth/logout", nil, first, true), 200)
	require(call("GET", "/servers", nil, first, false), 401)
	// A cross-origin browser cannot create a session, even before authentication.
	r := httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(`{"email":"one@example.test","password":"strong test password 123"}`))
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	require(w, 403)
}
