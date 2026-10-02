package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictJSON(t *testing.T) {
	for _, body := range []string{`{"extra":1}`, `{"name":"ok"} {"name":"two"}`, `{"name":1}`, strings.Repeat("x", 70000)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var req struct {
			Name string `json:"name"`
		}
		if Decode(w, r, &req) {
			t.Errorf("accepted invalid body")
		}
		if w.Code != 400 {
			t.Errorf("status=%d", w.Code)
		}
	}
}
func TestUUID(t *testing.T) {
	if !UUID("c6076de5-90d4-4842-b903-df48d453637c") || UUID("bad") {
		t.Fatal("UUID validation")
	}
}
