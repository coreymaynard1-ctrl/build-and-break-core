ALTER TABLE billing_plans ADD COLUMN is_demo BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE billing_invoices DROP CONSTRAINT billing_invoices_mode_check;
ALTER TABLE billing_invoices ADD CONSTRAINT billing_invoices_mode_check CHECK(mode IN ('sandbox','test','live'));
ALTER TABLE billing_payments DROP CONSTRAINT billing_payments_provider_check;
ALTER TABLE billing_payments ADD CONSTRAINT billing_payments_provider_check CHECK(provider IN ('sandbox','stripe','paypal'));
ALTER TABLE billing_payments DROP CONSTRAINT billing_payments_provider_reference_key;
ALTER TABLE billing_payments ADD UNIQUE(provider,provider_reference);
CREATE TABLE billing_checkouts (
 invoice_id UUID PRIMARY KEY REFERENCES billing_invoices(id),
 provider TEXT NOT NULL CHECK(provider IN ('stripe','paypal')),
 remote_id TEXT,
 checkout_url TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(provider,remote_id)
);
CREATE TABLE billing_payment_events (
 provider TEXT NOT NULL,
 event_id TEXT NOT NULL,
 payment_reference TEXT NOT NULL,
 invoice_id UUID NOT NULL REFERENCES billing_invoices(id),
 fingerprint TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY(provider,event_id)
);
-- Payment commits before provisioning: an unavailable node cannot undo a payment.
CREATE TABLE billing_fulfillments (
 invoice_id UUID PRIMARY KEY REFERENCES billing_invoices(id),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','error')),
 attempts INTEGER NOT NULL DEFAULT 0,
 error_message TEXT,
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
