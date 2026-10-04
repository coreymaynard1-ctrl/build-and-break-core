# Stripe and PayPal setup

The running development site is sandbox-only. Its invoices can be marked paid by
an admin using **Billing → Simulate payment and provision**, without charging money.
Both provider adapters exist and pass tests against local mock provider APIs.
Real merchant-account integration and webhook delivery remain unverified.

## Provider test mode

Configure these values in the Core environment or your private `.env`; do not commit
or paste credentials into chat:

| Variable | Purpose |
| --- | --- |
| `BNB_PAYMENT_MODE=test` | Stripe test keys and PayPal sandbox endpoints |
| `BNB_PUBLIC_URL` | Exact website origin used for provider return URLs |
| `BNB_STRIPE_SECRET_KEY` | Stripe `sk_test_...` secret key |
| `BNB_STRIPE_WEBHOOK_SECRET` | Endpoint-specific `whsec_...` signing secret |
| `BNB_PAYPAL_CLIENT_ID` | PayPal sandbox application client ID |
| `BNB_PAYPAL_CLIENT_SECRET` | PayPal sandbox application secret |
| `BNB_PAYPAL_WEBHOOK_ID` | Webhook ID belonging to that sandbox application |

Stripe endpoint: `/api/v1/billing/webhooks/stripe`. Subscribe to
`checkout.session.completed` and `checkout.session.async_payment_succeeded`.
Stripe's local CLI can forward signed test events to Core. Configure the secret
issued by that forwarder; a different endpoint secret will not validate.

PayPal endpoint: `/api/v1/billing/webhooks/paypal`. Subscribe to
`PAYMENT.CAPTURE.COMPLETED`; configure the resulting application webhook ID.
PayPal needs a provider-accessible endpoint. The development WSL address is private
and is not a public webhook destination.

Checkout uses the owned invoice's immutable amount/currency. Stripe creates a hosted
Checkout Session. PayPal creates an order; after approval, return to Billing and use
**Confirm PayPal payment**. The browser never asserts that money was received.
Signed Stripe events and remotely verified PayPal events commit payment receipts;
authenticated PayPal capture responses also confirm receipt directly from the API.

Provider selection is fixed once checkout is reserved for an invoice. Retries reuse
the provider's deterministic idempotency key. Checkout expiry/replacement is not
implemented yet; a stale provider session needs operator review.

## Live payments

Live payments require explicit `BNB_PAYMENT_MODE=live`, live merchant credentials,
an HTTPS `BNB_PUBLIC_URL` and configured webhook delivery. Core then requires secure
session cookies. Real prices must be explicitly published through Admin → Hosting
plans by clearing **Demo price**; demo plans are excluded from live checkout.

Do not enable this preview as a production customer launch yet. Recurring subscriptions,
refunds/credits, tax calculation, invoice exports, password recovery, email verification,
MFA and remaining operational requirements in [product delivery](product-delivery.md)
are still outstanding. The current provider flow is a one-time invoice payment.

## Verification

Integration tests verify matching amounts/currencies/modes, signed event validation,
concurrent replay deduplication and durable fulfillment after a node becomes available.
The real sandbox smoke test verified payment → owned container creation and lifecycle.
No real merchant payments have been attempted.

Provider contracts: [Stripe Checkout](https://docs.stripe.com/api/checkout/sessions/create),
[Stripe webhooks](https://docs.stripe.com/webhooks),
[PayPal Orders](https://developer.paypal.com/api/orders/v2),
[PayPal webhook verification](https://developer.paypal.com/api/rest/webhooks/rest/).
