package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type CheckoutInvoice struct {
	ID        string
	Name      string
	Email     string
	Currency  string
	Amount    int64
	Mode      string
	ReturnURL string
}
type RemoteCheckout struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}
type PaymentEvent struct {
	ID         string
	CheckoutID string
	Reference  string
	Currency   string
	Amount     int64
	Mode       string
}
type Gateway interface {
	Create(context.Context, CheckoutInvoice) (RemoteCheckout, error)
	Verify(context.Context, http.Header, []byte) (PaymentEvent, error)
}

var ErrInvalidSignature = errors.New("invalid payment signature")
var ErrIgnoredEvent = errors.New("unsupported payment event")
var gatewayHTTP = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("payment API redirect rejected") }}

func decodeAPI(client *http.Client, req *http.Request, out any) error {
	if client == nil {
		client = gatewayHTTP
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("payment provider unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("payment provider returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return errors.New("invalid provider response")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return errors.New("invalid provider response")
	}
	return nil
}

type Stripe struct {
	Secret        string
	WebhookSecret string
	Mode          string
	BaseURL       string
	Client        *http.Client
}

func (s *Stripe) base() string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	return "https://api.stripe.com"
}
func (s *Stripe) Create(ctx context.Context, i CheckoutInvoice) (RemoteCheckout, error) {
	form := url.Values{"mode": {"payment"}, "success_url": {i.ReturnURL + "/?payment=stripe#billing"}, "cancel_url": {i.ReturnURL + "/#billing"}, "customer_email": {i.Email}, "client_reference_id": {i.ID}, "metadata[invoice_id]": {i.ID}, "line_items[0][quantity]": {"1"}, "line_items[0][price_data][currency]": {strings.ToLower(i.Currency)}, "line_items[0][price_data][unit_amount]": {strconv.FormatInt(i.Amount, 10)}, "line_items[0][price_data][product_data][name]": {i.Name}}
	req, err := http.NewRequestWithContext(ctx, "POST", s.base()+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return RemoteCheckout{}, err
	}
	req.SetBasicAuth(s.Secret, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", "bnb-checkout-"+i.ID)
	var response struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		Live bool   `json:"livemode"`
	}
	if err = decodeAPI(s.Client, req, &response); err != nil {
		return RemoteCheckout{}, err
	}
	if response.ID == "" || response.URL == "" || response.Live != (i.Mode == "live") {
		return RemoteCheckout{}, errors.New("provider checkout mode mismatch")
	}
	parsed, err := url.Parse(response.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "checkout.stripe.com" {
		return RemoteCheckout{}, errors.New("invalid provider checkout URL")
	}
	return RemoteCheckout{response.ID, response.URL}, nil
}
func (s *Stripe) Verify(ctx context.Context, headers http.Header, raw []byte) (PaymentEvent, error) {
	var timestamp string
	signatures := []string{}
	for _, part := range strings.Split(headers.Get("Stripe-Signature"), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if key == "t" {
			if timestamp != "" {
				return PaymentEvent{}, ErrInvalidSignature
			}
			timestamp = value
		}
		if key == "v1" {
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || s.WebhookSecret == "" || seconds < time.Now().Unix()-300 || seconds > time.Now().Unix()+300 {
		return PaymentEvent{}, ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(raw)
	expected := mac.Sum(nil)
	valid := false
	for _, signature := range signatures {
		actual, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(actual, expected) {
			valid = true
		}
	}
	if !valid {
		return PaymentEvent{}, ErrInvalidSignature
	}
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Live bool   `json:"livemode"`
		Data struct {
			Object struct {
				ID       string `json:"id"`
				Status   string `json:"payment_status"`
				Currency string `json:"currency"`
				Amount   int64  `json:"amount_total"`
				Intent   string `json:"payment_intent"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &event) != nil || event.ID == "" {
		return PaymentEvent{}, errors.New("invalid payment event")
	}
	if event.Live != (s.Mode == "live") {
		return PaymentEvent{}, errors.New("payment event mode mismatch")
	}
	if (event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded") || event.Data.Object.Status != "paid" {
		return PaymentEvent{}, ErrIgnoredEvent
	}
	o := event.Data.Object
	if o.ID == "" || o.Intent == "" || o.Amount <= 0 {
		return PaymentEvent{}, errors.New("incomplete payment event")
	}
	return PaymentEvent{event.ID, o.ID, o.Intent, strings.ToUpper(o.Currency), o.Amount, s.Mode}, nil
}

type PayPal struct {
	ClientID  string
	Secret    string
	WebhookID string
	Mode      string
	BaseURL   string
	Client    *http.Client
}

func (p *PayPal) base() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	if p.Mode == "live" {
		return "https://api-m.paypal.com"
	}
	return "https://api-m.sandbox.paypal.com"
}
func (p *PayPal) token(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.base()+"/v1/oauth2/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(p.ClientID, p.Secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		Token string `json:"access_token"`
	}
	if err = decodeAPI(p.Client, req, &response); err != nil {
		return "", err
	}
	if response.Token == "" {
		return "", errors.New("provider authentication failed")
	}
	return response.Token, nil
}
func (p *PayPal) api(ctx context.Context, path, key string, body any, out any) error {
	token, err := p.token(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.base()+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("PayPal-Request-Id", key)
	}
	return decodeAPI(p.Client, req, out)
}
func (p *PayPal) Create(ctx context.Context, i CheckoutInvoice) (RemoteCheckout, error) {
	body := map[string]any{"intent": "CAPTURE", "purchase_units": []any{map[string]any{"reference_id": i.ID, "custom_id": i.ID, "invoice_id": i.ID, "description": i.Name, "amount": map[string]string{"currency_code": i.Currency, "value": fmt.Sprintf("%d.%02d", i.Amount/100, i.Amount%100)}}}, "payment_source": map[string]any{"paypal": map[string]any{"experience_context": map[string]string{"return_url": i.ReturnURL + "/?payment=paypal&invoice=" + i.ID, "cancel_url": i.ReturnURL + "/#billing", "user_action": "PAY_NOW"}}}}
	var response struct {
		ID    string `json:"id"`
		Links []struct {
			Rel  string `json:"rel"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := p.api(ctx, "/v2/checkout/orders", i.ID, body, &response); err != nil {
		return RemoteCheckout{}, err
	}
	for _, link := range response.Links {
		if link.Rel == "payer-action" || link.Rel == "approve" {
			parsed, err := url.Parse(link.Href)
			host := "www.sandbox.paypal.com"
			if p.Mode == "live" {
				host = "www.paypal.com"
			}
			if err != nil || parsed.Scheme != "https" || parsed.Hostname() != host || response.ID == "" {
				return RemoteCheckout{}, errors.New("invalid provider checkout URL")
			}
			return RemoteCheckout{response.ID, link.Href}, nil
		}
	}
	return RemoteCheckout{}, errors.New("provider did not return approval URL")
}

type paypalCapture struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount struct {
		Currency string `json:"currency_code"`
		Value    string `json:"value"`
	} `json:"amount"`
	Supplementary struct {
		Related struct {
			Order string `json:"order_id"`
		} `json:"related_ids"`
	} `json:"supplementary_data"`
}

func minorAmount(value string) (int64, error) {
	whole, fraction, ok := strings.Cut(value, ".")
	if !ok || len(fraction) != 2 || whole == "" || len(whole) > 12 {
		return 0, errors.New("invalid payment amount")
	}
	for _, c := range whole + fraction {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid payment amount")
		}
	}
	n, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid payment amount")
	}
	return n, nil
}
func (p *PayPal) Capture(ctx context.Context, checkout string) (PaymentEvent, error) {
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Units  []struct {
			Payments struct {
				Captures []paypalCapture `json:"captures"`
			} `json:"payments"`
		} `json:"purchase_units"`
	}
	if err := p.api(ctx, "/v2/checkout/orders/"+url.PathEscape(checkout)+"/capture", "capture-"+checkout, map[string]any{}, &response); err != nil {
		return PaymentEvent{}, err
	}
	if response.ID != checkout || response.Status != "COMPLETED" || len(response.Units) != 1 || len(response.Units[0].Payments.Captures) != 1 {
		return PaymentEvent{}, errors.New("payment capture is not completed")
	}
	capture := response.Units[0].Payments.Captures[0]
	amount, err := minorAmount(capture.Amount.Value)
	if err != nil || capture.ID == "" || capture.Status != "COMPLETED" {
		return PaymentEvent{}, errors.New("invalid payment capture")
	}
	return PaymentEvent{"capture-" + capture.ID, checkout, capture.ID, capture.Amount.Currency, amount, p.Mode}, nil
}
func (p *PayPal) Verify(ctx context.Context, headers http.Header, raw []byte) (PaymentEvent, error) {
	var event struct {
		ID       string        `json:"id"`
		Type     string        `json:"event_type"`
		Resource paypalCapture `json:"resource"`
	}
	if json.Unmarshal(raw, &event) != nil || event.ID == "" || p.WebhookID == "" {
		return PaymentEvent{}, ErrInvalidSignature
	}
	body := map[string]any{"webhook_id": p.WebhookID, "webhook_event": json.RawMessage(raw)}
	for _, field := range []string{"auth_algo", "cert_url", "transmission_id", "transmission_sig", "transmission_time"} {
		value := headers.Get("Paypal-" + strings.ReplaceAll(field, "_", "-"))
		if value == "" {
			return PaymentEvent{}, ErrInvalidSignature
		}
		body[field] = value
	}
	var verified struct {
		Status string `json:"verification_status"`
	}
	if err := p.api(ctx, "/v1/notifications/verify-webhook-signature", "", body, &verified); err != nil {
		return PaymentEvent{}, err
	}
	if verified.Status != "SUCCESS" {
		return PaymentEvent{}, ErrInvalidSignature
	}
	if event.Type != "PAYMENT.CAPTURE.COMPLETED" {
		return PaymentEvent{}, ErrIgnoredEvent
	}
	c := event.Resource
	amount, err := minorAmount(c.Amount.Value)
	if err != nil || c.Status != "COMPLETED" || c.ID == "" || c.Supplementary.Related.Order == "" {
		return PaymentEvent{}, errors.New("invalid capture event")
	}
	return PaymentEvent{event.ID, c.Supplementary.Related.Order, c.ID, c.Amount.Currency, amount, p.Mode}, nil
}
