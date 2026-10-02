package billing

import (
	"net/http/httptest"
	"testing"
)

func TestSandboxDisabled(t *testing.T) {
	h := &Handler{}
	w := httptest.NewRecorder()
	h.PaySandbox(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 503 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestOrderValidation(t *testing.T) {
	req := OrderRequest{NodeID: "c6076de5-90d4-4842-b903-df48d453637c", Name: "test", Email: "test@example.com"}
	if !validOrder(req, "valid-key") {
		t.Fatal("valid order rejected")
	}
	req.Email = "Display Name <test@example.com>"
	if validOrder(req, "valid-key") {
		t.Fatal("display address accepted")
	}
	req.Email = "test@example.com"
	if validOrder(req, "x") {
		t.Fatal("short idempotency key accepted")
	}
}
