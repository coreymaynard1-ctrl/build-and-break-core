package billing

import (
	"buildandbreak/platform/internal/accounts"
	"buildandbreak/platform/internal/httpx"
	"buildandbreak/platform/internal/platform"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h *Handler) paymentMode() string {
	if h.Mode == "test" || h.Mode == "live" {
		return h.Mode
	}
	if h.Sandbox {
		return "sandbox"
	}
	return "disabled"
}

// Configure reads operator configuration without exposing credentials to clients.
func (h *Handler) Configure() {
	h.Mode = os.Getenv("BNB_PAYMENT_MODE")
	h.ReturnURL = strings.TrimRight(os.Getenv("BNB_PUBLIC_URL"), "/")
	h.Gateways = map[string]Gateway{}
	if h.Mode != "test" && h.Mode != "live" {
		return
	}
	parsed, err := url.Parse(h.ReturnURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || (parsed.Scheme != "https" && (h.Mode == "live" || parsed.Scheme != "http")) {
		return
	}
	stripeKey, webhook := os.Getenv("BNB_STRIPE_SECRET_KEY"), os.Getenv("BNB_STRIPE_WEBHOOK_SECRET")
	prefix := "sk_test_"
	if h.Mode == "live" {
		prefix = "sk_live_"
	}
	if strings.HasPrefix(stripeKey, prefix) && strings.HasPrefix(webhook, "whsec_") {
		h.Gateways["stripe"] = &Stripe{Secret: stripeKey, WebhookSecret: webhook, Mode: h.Mode}
	}
	paypalID, paypalSecret, paypalWebhook := os.Getenv("BNB_PAYPAL_CLIENT_ID"), os.Getenv("BNB_PAYPAL_CLIENT_SECRET"), os.Getenv("BNB_PAYPAL_WEBHOOK_ID")
	if paypalID != "" && paypalSecret != "" && paypalWebhook != "" {
		h.Gateways["paypal"] = &PayPal{ClientID: paypalID, Secret: paypalSecret, WebhookID: paypalWebhook, Mode: h.Mode}
	}
}
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("invoiceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid invoice ID")
		return
	}
	var req struct {
		Provider string `json:"provider"`
	}
	if !httpx.Decode(w, r, &req) {
		return
	}
	gateway, ok := h.Gateways[req.Provider]
	if !ok || h.paymentMode() == "sandbox" || h.paymentMode() == "disabled" {
		httpx.Error(w, 503, "selected payment provider is not configured")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		httpx.Error(w, 500, "failed to begin checkout")
		return
	}
	defer tx.Rollback(r.Context())
	var i CheckoutInvoice
	var status string
	i.ReturnURL = h.ReturnURL
	err = tx.QueryRow(r.Context(), `SELECT i.id,i.currency,i.amount_minor,i.mode,i.status,o.name,o.customer_email FROM billing_invoices i JOIN billing_orders o ON o.id=i.order_id WHERE i.id=$1 AND ($2::uuid IS NULL OR o.account_id=$2) FOR UPDATE OF i`, id, accounts.Scope(r.Context())).Scan(&i.ID, &i.Currency, &i.Amount, &i.Mode, &status, &i.Name, &i.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "invoice not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read invoice")
		return
	}
	if status != "unpaid" || i.Mode != h.paymentMode() {
		httpx.Error(w, 409, "invoice is not payable in the configured payment mode")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO billing_checkouts(invoice_id,provider) VALUES($1,$2) ON CONFLICT(invoice_id) DO NOTHING`, id, req.Provider)
	if err != nil {
		httpx.Error(w, 500, "failed to reserve checkout")
		return
	}
	var provider string
	var remoteID, checkoutURL *string
	err = tx.QueryRow(r.Context(), `SELECT provider,remote_id,checkout_url FROM billing_checkouts WHERE invoice_id=$1`, id).Scan(&provider, &remoteID, &checkoutURL)
	if err != nil {
		httpx.Error(w, 500, "failed to read checkout")
		return
	}
	if provider != req.Provider {
		httpx.Error(w, 409, "invoice already has a checkout with another provider")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.Error(w, 500, "failed to save checkout")
		return
	}
	if remoteID != nil && checkoutURL != nil {
		httpx.JSON(w, 200, RemoteCheckout{*remoteID, *checkoutURL})
		return
	}
	// Network requests happen after the reservation commits. Deterministic provider
	// idempotency keys make retries safe even when a response was lost.
	result, err := gateway.Create(r.Context(), i)
	if err != nil {
		httpx.Error(w, 502, err.Error())
		return
	}
	tag, err := h.DB.Exec(r.Context(), `UPDATE billing_checkouts SET remote_id=$2,checkout_url=$3 WHERE invoice_id=$1 AND (remote_id IS NULL OR remote_id=$2)`, id, result.ID, result.URL)
	if err != nil || tag.RowsAffected() != 1 {
		httpx.Error(w, 503, "checkout was created; retry to recover its saved identity")
		return
	}
	httpx.JSON(w, 200, result)
}
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	gateway, ok := h.Gateways[provider]
	if !ok {
		httpx.Error(w, 503, "payment provider is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.Error(w, 400, "invalid webhook body")
		return
	}
	event, err := gateway.Verify(r.Context(), r.Header, raw)
	if errors.Is(err, ErrIgnoredEvent) {
		httpx.JSON(w, 200, map[string]bool{"received": true})
		return
	}
	if errors.Is(err, ErrInvalidSignature) {
		httpx.Error(w, 400, "invalid payment signature")
		return
	}
	if err != nil {
		httpx.Error(w, 503, "payment event could not be verified")
		return
	}
	if err = h.recordPayment(r.Context(), provider, event); err != nil {
		httpx.Error(w, 503, "payment event could not be recorded; provider should retry")
		return
	}
	httpx.JSON(w, 200, map[string]bool{"received": true})
}
func (h *Handler) CapturePayPal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("invoiceID")
	if !httpx.UUID(id) {
		httpx.Error(w, 400, "invalid invoice ID")
		return
	}
	p, ok := h.Gateways["paypal"].(*PayPal)
	if !ok {
		httpx.Error(w, 503, "PayPal is not configured")
		return
	}
	var checkout, status, mode string
	err := h.DB.QueryRow(r.Context(), `SELECT c.remote_id,i.status,i.mode FROM billing_checkouts c JOIN billing_invoices i ON i.id=c.invoice_id JOIN billing_orders o ON o.id=i.order_id WHERE i.id=$1 AND c.provider='paypal' AND c.remote_id IS NOT NULL AND ($2::uuid IS NULL OR o.account_id=$2)`, id, accounts.Scope(r.Context())).Scan(&checkout, &status, &mode)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "PayPal checkout not found")
		return
	}
	if err != nil {
		httpx.Error(w, 500, "failed to read checkout")
		return
	}
	if status == "paid" {
		httpx.JSON(w, 200, map[string]string{"status": "paid"})
		return
	}
	if status != "unpaid" || mode != h.paymentMode() {
		httpx.Error(w, 409, "invoice is not payable")
		return
	}
	event, err := p.Capture(r.Context(), checkout)
	if err != nil {
		httpx.Error(w, 502, err.Error())
		return
	}
	if err = h.recordPayment(r.Context(), "paypal", event); err != nil {
		httpx.Error(w, 503, "payment captured; confirmation is pending and safe to retry")
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "paid"})
}
func (h *Handler) recordPayment(ctx context.Context, provider string, event PaymentEvent) error {
	if event.ID == "" || event.Reference == "" || event.CheckoutID == "" || event.Amount <= 0 || (event.Mode != "test" && event.Mode != "live") {
		return errors.New("invalid payment")
	}
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var invoice, currency, status, mode string
	var amount int64
	err = tx.QueryRow(ctx, `SELECT i.id,i.currency,i.amount_minor,i.status,i.mode FROM billing_checkouts c JOIN billing_invoices i ON i.id=c.invoice_id WHERE c.provider=$1 AND c.remote_id=$2 FOR UPDATE OF i`, provider, event.CheckoutID).Scan(&invoice, &currency, &amount, &status, &mode)
	if err != nil {
		return err
	}
	if currency != event.Currency || amount != event.Amount || mode != event.Mode {
		return errors.New("payment does not match invoice")
	}
	canonical, _ := json.Marshal(event)
	sum := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(sum[:])
	var previous string
	err = tx.QueryRow(ctx, `SELECT fingerprint FROM billing_payment_events WHERE provider=$1 AND event_id=$2`, provider, event.ID).Scan(&previous)
	if err == nil {
		if previous != fingerprint {
			return errors.New("conflicting duplicate payment event")
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if status == "paid" {
		var reference, paidProvider string
		err = tx.QueryRow(ctx, `SELECT provider,provider_reference FROM billing_payments WHERE invoice_id=$1`, invoice).Scan(&paidProvider, &reference)
		if err != nil || paidProvider != provider || reference != event.Reference {
			return errors.New("invoice already has a different payment")
		}
	} else {
		if status != "unpaid" {
			return errors.New("invoice is not payable")
		}
		_, err = tx.Exec(ctx, `INSERT INTO billing_payments(id,invoice_id,provider,provider_reference,currency,amount_minor) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5)`, invoice, provider, event.Reference, currency, amount)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE billing_invoices SET status='paid',paid_at=NOW() WHERE id=$1`, invoice)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE billing_orders SET status='paid' WHERE id=(SELECT order_id FROM billing_invoices WHERE id=$1)`, invoice)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO billing_fulfillments(invoice_id) VALUES($1) ON CONFLICT DO NOTHING`, invoice)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO billing_payment_events(provider,event_id,payment_reference,invoice_id,fingerprint) VALUES($1,$2,$3,$4,$5)`, provider, event.ID, event.Reference, invoice, fingerprint)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FulfillNext uses a separate transaction from payment confirmation. Savepoints
// retain retry state when capacity or provisioning validation fails.
func (h *Handler) FulfillNext(ctx context.Context) (bool, error) {
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var invoice, order, node, name string
	var owner *string
	var snapshot []byte
	var port int
	err = tx.QueryRow(ctx, `SELECT f.invoice_id,o.id,o.node_id,o.name,o.account_id,o.plan_snapshot,COALESCE(o.port,0) FROM billing_fulfillments f JOIN billing_invoices i ON i.id=f.invoice_id JOIN billing_orders o ON o.id=i.order_id WHERE f.status IN ('pending','error') AND f.next_attempt_at<=NOW() AND i.status='paid' ORDER BY f.next_attempt_at FOR UPDATE OF f SKIP LOCKED LIMIT 1`).Scan(&invoice, &order, &node, &name, &owner, &snapshot, &port)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var p plan
	if json.Unmarshal(snapshot, &p) != nil {
		return false, errors.New("invalid paid plan snapshot")
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	server, service, _, fulfillErr := platform.Fulfill(ctx, nested, node, name, p.Template, p.Memory, p.Disk, p.CPU, port, minecraftOptions(p)...)
	if fulfillErr == nil {
		_, fulfillErr = nested.Exec(ctx, `UPDATE servers SET account_id=$2 WHERE id=$1`, server, owner)
	}
	if fulfillErr == nil {
		_, fulfillErr = nested.Exec(ctx, `UPDATE billing_orders SET server_id=$2,service_id=$3 WHERE id=$1`, order, server, service)
	}
	if fulfillErr != nil {
		if err = nested.Rollback(ctx); err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `UPDATE billing_fulfillments SET status='error',attempts=attempts+1,error_message=$2,next_attempt_at=NOW()+INTERVAL '60 seconds',updated_at=NOW() WHERE invoice_id=$1`, invoice, fulfillErr.Error())
	} else {
		if err = nested.Commit(ctx); err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `UPDATE billing_fulfillments SET status='completed',attempts=attempts+1,error_message=NULL,updated_at=NOW() WHERE invoice_id=$1`, invoice)
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
func (h *Handler) RunFulfillment(ctx context.Context) {
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			for n := 0; n < 20; n++ {
				worked, err := h.FulfillNext(ctx)
				if err != nil {
					log.Print("Billing fulfillment pass failed; retrying later")
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}
