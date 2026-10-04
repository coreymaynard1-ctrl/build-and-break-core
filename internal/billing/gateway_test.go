package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func stripeSignature(raw []byte, secret string, seconds int64) string {
	ts := strconv.FormatInt(seconds, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(raw)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}
func TestStripeCheckoutAndSignedEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, _, ok := r.BasicAuth()
		if !ok || key != "sk_test_fixture" {
			t.Error("missing provider authentication")
		}
		if r.URL.Path != "/v1/checkout/sessions" || r.Header.Get("Idempotency-Key") != "bnb-checkout-invoice" {
			t.Error("missing deterministic checkout key")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("line_items[0][price_data][unit_amount]") != "500" || r.Form.Get("client_reference_id") != "invoice" || r.Form.Get("customer_email") != "customer@example.test" {
			t.Error("checkout used incorrect invoice data")
		}
		fmt.Fprint(w, `{"id":"cs_test_fixture","url":"https://checkout.stripe.com/c/test","livemode":false}`)
	}))
	defer server.Close()
	s := &Stripe{Secret: "sk_test_fixture", WebhookSecret: "whsec_fixture", Mode: "test", BaseURL: server.URL}
	checkout, err := s.Create(context.Background(), CheckoutInvoice{ID: "invoice", Name: "Minecraft", Email: "customer@example.test", Currency: "GBP", Amount: 500, Mode: "test", ReturnURL: "https://host.example"})
	if err != nil || checkout.ID != "cs_test_fixture" {
		t.Fatalf("checkout: %v", err)
	}
	raw := []byte(`{"id":"evt_fixture","type":"checkout.session.completed","livemode":false,"data":{"object":{"id":"cs_test_fixture","payment_status":"paid","amount_total":500,"currency":"gbp","payment_intent":"pi_fixture"}}}`)
	headers := http.Header{}
	headers.Set("Stripe-Signature", stripeSignature(raw, s.WebhookSecret, time.Now().Unix()))
	event, err := s.Verify(context.Background(), headers, raw)
	if err != nil || event.Amount != 500 || event.Reference != "pi_fixture" || event.Mode != "test" {
		t.Fatalf("verified event: %+v %v", event, err)
	}
	if _, err = s.Verify(context.Background(), headers, append(raw, ' ')); !errors.Is(err, ErrInvalidSignature) {
		t.Fatal("accepted altered body")
	}
	headers.Set("Stripe-Signature", stripeSignature(raw, s.WebhookSecret, time.Now().Unix()-600))
	if _, err = s.Verify(context.Background(), headers, raw); !errors.Is(err, ErrInvalidSignature) {
		t.Fatal("accepted old signature")
	}
	headers.Set("Stripe-Signature", stripeSignature(raw, s.WebhookSecret, time.Now().Unix()))
	s.Mode = "live"
	if _, err = s.Verify(context.Background(), headers, raw); err == nil {
		t.Fatal("accepted test event in live mode")
	}
}

func TestPayPalCheckoutCaptureAndVerification(t *testing.T) {
	verification := "SUCCESS"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			id, secret, _ := r.BasicAuth()
			if id != "fixture-id" || secret != "fixture-secret" {
				t.Error("invalid OAuth authentication")
			}
			fmt.Fprint(w, `{"access_token":"fixture-token"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing provider access token")
		}
		switch r.URL.Path {
		case "/v2/checkout/orders":
			if r.Header.Get("PayPal-Request-Id") != "invoice" {
				t.Error("missing checkout idempotency key")
			}
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid order request")
			}
			units := body["purchase_units"].([]any)
			amount := units[0].(map[string]any)["amount"].(map[string]any)
			if amount["value"] != "5.00" || amount["currency_code"] != "GBP" {
				t.Error("wrong provider amount")
			}
			fmt.Fprint(w, `{"id":"ORDER_FIXTURE","links":[{"rel":"payer-action","href":"https://www.sandbox.paypal.com/checkoutnow?token=ORDER_FIXTURE"}]}`)
		case "/v2/checkout/orders/ORDER_FIXTURE/capture":
			if r.Header.Get("PayPal-Request-Id") != "capture-ORDER_FIXTURE" {
				t.Error("missing capture idempotency key")
			}
			fmt.Fprint(w, `{"id":"ORDER_FIXTURE","status":"COMPLETED","purchase_units":[{"payments":{"captures":[{"id":"CAPTURE_FIXTURE","status":"COMPLETED","amount":{"value":"5.00","currency_code":"GBP"}}]}}]}`)
		case "/v1/notifications/verify-webhook-signature":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["webhook_id"] != "fixture-webhook" || body["transmission_sig"] != "signature" || body["webhook_event"] == nil {
				t.Error("incomplete verification request")
			}
			fmt.Fprintf(w, `{"verification_status":%q}`, verification)
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p := &PayPal{ClientID: "fixture-id", Secret: "fixture-secret", WebhookID: "fixture-webhook", Mode: "test", BaseURL: server.URL}
	checkout, err := p.Create(context.Background(), CheckoutInvoice{ID: "invoice", Name: "Minecraft", Currency: "GBP", Amount: 500, Mode: "test", ReturnURL: "https://host.example"})
	if err != nil || checkout.ID != "ORDER_FIXTURE" {
		t.Fatalf("checkout: %v", err)
	}
	event, err := p.Capture(context.Background(), checkout.ID)
	if err != nil || event.Amount != 500 || event.Reference != "CAPTURE_FIXTURE" {
		t.Fatalf("capture: %+v %v", event, err)
	}
	raw := []byte(`{"id":"WH-FIXTURE","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAPTURE_FIXTURE","status":"COMPLETED","amount":{"value":"5.00","currency_code":"GBP"},"supplementary_data":{"related_ids":{"order_id":"ORDER_FIXTURE"}}}}`)
	headers := http.Header{}
	for _, field := range []string{"Auth-Algo", "Cert-Url", "Transmission-Id", "Transmission-Sig", "Transmission-Time"} {
		headers.Set("Paypal-"+field, "signature")
	}
	event, err = p.Verify(context.Background(), headers, raw)
	if err != nil || event.CheckoutID != checkout.ID || event.Reference != "CAPTURE_FIXTURE" {
		t.Fatalf("verified webhook: %+v %v", event, err)
	}
	verification = "FAILURE"
	if _, err = p.Verify(context.Background(), headers, raw); !errors.Is(err, ErrInvalidSignature) {
		t.Fatal("accepted unverified webhook")
	}
}

func TestAmountsAndProviderURLValidation(t *testing.T) {
	for _, bad := range []string{"1e3", "-5.00", "5.0", "5.001", "0.00", "999999999999999999.00", " 5.00", "5.00USD"} {
		if _, err := minorAmount(bad); err == nil {
			t.Errorf("accepted amount %q", bad)
		}
	}
	n, err := minorAmount("123.45")
	if err != nil || n != 12345 {
		t.Fatal("incorrect integer money conversion")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"cs_fixture","url":"https://evil.example/checkout","livemode":false}`)
	}))
	defer server.Close()
	s := &Stripe{Secret: "fixture", BaseURL: server.URL}
	if _, err = s.Create(context.Background(), CheckoutInvoice{Mode: "test"}); err == nil || !strings.Contains(err.Error(), "URL") {
		t.Fatal("accepted arbitrary redirect URL")
	}
}
