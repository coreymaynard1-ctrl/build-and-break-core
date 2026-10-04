package support

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/testutil"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestTicketOwnershipAndConversationLifecycle(t *testing.T) {
	db := testutil.Database(t)
	h := &Handler{DB: db}
	ctx := context.Background()
	user := func(email, role string) accounts.User {
		t.Helper()
		var u accounts.User
		err := db.QueryRow(ctx, `INSERT INTO accounts(email,name,password_hash,role) VALUES($1,$1::text,'fixture',$2) RETURNING id,email,name,role`, email, role).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, other, admin := user("one@example.test", "customer"), user("two@example.test", "customer"), user("admin@example.test", "admin")
	call := func(u accounts.User, id, method, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
		r.SetPathValue("ticketID", id)
		r = r.WithContext(accounts.WithUser(ctx, u))
		w := httptest.NewRecorder()
		switch method {
		case "POST":
			h.Create(w, r)
		case "PATCH":
			h.Update(w, r)
		case "GET":
			if id == "" {
				h.List(w, r)
			} else {
				h.Read(w, r)
			}
		}
		return w
	}
	require := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("want %d got %d: %s", code, w.Code, w.Body.String())
		}
	}
	w := call(owner, "", "POST", `{"subject":"My server","category":"technical","message":"Please help"}`)
	require(w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	id := result["id"]
	require(call(other, id, "GET", ""), 404)
	require(call(other, id, "PATCH", `{"message":"Unauthorized reply"}`), 404)
	w = call(other, "", "GET", "")
	require(w, 200)
	if w.Body.String() != "[]\n" {
		t.Fatal("other customer saw a ticket")
	}
	require(call(admin, id, "PATCH", `{"message":"Support answer"}`), 200)
	w = call(owner, id, "GET", "")
	require(w, 200)
	var ticket struct {
		Status   string `json:"status"`
		Messages []struct {
			Message string `json:"message"`
			Role    string `json:"author_role"`
		} `json:"messages"`
	}
	json.Unmarshal(w.Body.Bytes(), &ticket)
	if ticket.Status != "answered" || len(ticket.Messages) != 2 || ticket.Messages[1].Role != "admin" {
		t.Fatal("admin answer missing")
	}
	require(call(owner, id, "PATCH", `{"status":"closed"}`), 200)
	require(call(owner, id, "PATCH", `{"message":"Reply while closed"}`), 409)
	require(call(owner, id, "PATCH", `{"status":"open","message":"One more question"}`), 200)
	w = call(owner, id, "GET", "")
	json.Unmarshal(w.Body.Bytes(), &ticket)
	if ticket.Status != "open" || len(ticket.Messages) != 3 {
		t.Fatal("reopened conversation incorrect")
	}
}
