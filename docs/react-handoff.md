# React developer handoff — 2026-10-04

## Current structure

- `api.js`: same-origin Core client, in-memory CSRF token, typed API errors and secure random idempotency keys compatible with the HTTP LAN preview.
- `App.jsx`: hash routing, persisted account session bootstrap, customer/admin navigation and five-second resource polling.
- `Home.jsx`: public hosting website sections backed by actual templates, locations and billing mode.
- `Catalogue.jsx`: supported workload, plan and location pages.
- `Account.jsx`: customer registration, sign-in and validation feedback.
- `Checkout.jsx`: workload → plan → location/name → review → invoice/payment flow.
- `Billing.jsx`: owned invoices, admin sandbox payment simulation, provider redirects, PayPal capture confirmation and fulfillment feedback.
- `Admin.jsx`: account/infrastructure overview, hosting-plan administration and explicit development deployment entry.
- `Support.jsx`: owned ticket conversations; admin support inbox; replies and close/reopen operations.
- `StatusPage.jsx`: ten-second measured health refresh and incident history; admin publication/update controls.
- `CreateServer.jsx`: admin-only direct development allocation/provisioning flow.
- `components.jsx`: status, explicit accessible field labels, panels and empty states.
- `style.css`: responsive dark website, catalogue, checkout and portal styling.

Frontend build and lint passed. Chromium smoke verification passed against the live
Core for public status, sign-up, session persistence, isolation, complete sandbox
checkout/invoice creation, support conversation/reply/closure and sign-out. No mock
browser APIs are used. Backend integration tests cover ownership and payment behavior.

## Contracts

Use `/api/v1` only. No Docker or agent URLs/tokens belong in React. HTTP 202 means
queued; use actual `job_status`, service state/error, observation time and node status.
Container-running and Minecraft game readiness are different states. Deletion
preserves Minecraft volumes and requires the current confirmation.

The account session has an HttpOnly cookie and a CSRF token returned by Core. Every
protected mutation sends both. A 401 requires sign-in again. Admin routes are enforced
in Core, including direct provisioning and sandbox payment simulation. Public
registration never creates an administrator. Existing legacy allocations remain
unassigned and admin-only. Pricing/email/ownership come from Core snapshots and the
signed-in account, not browser claims.

Provider adapters exist for Stripe Checkout and PayPal Orders/capture. Merchant keys
and real provider callbacks are not configured, so the running preview remains sandbox.
Paid provider invoices and durable provisioning retries have separate transactions.
Never infer successful payment from the redirect URL. No recurring subscriptions exist.

## Product direction and remaining work

The requested target is the complete hosting workflow and game catalogue, WHMCS /
Paymenter billing capability, Pterodactyl-equivalent management, and an Ubuntu base
inside customer-owned resources. See [delivery criteria](product-delivery.md) and the
[92-entry deployment checklist](../internal/catalogue/games.json). This is an ongoing build.

Do not claim unimplemented game installers, realtime log streaming, SFTP,
backups, schedules, sharing, player/disk measurements, disk quotas, Ubuntu/VPS or subscription
operations are functional. Implement and verify their Core/agent operations before
presenting them as ready. Broader game support needs UDP/multiple port allocation,
installer/version contracts, licensing/credentials where required and real lifecycle
verification. Ubuntu environment versus full VM (or both) awaits the user's choice.

See [API contract](api.md), [provider configuration](payment-setup.md), and the
historical [earlier verification](session-status.md).

## Management workspace

`ServicePanel.jsx` owns direct sidebar navigation and is keyed by service ID to reset console/configuration state when switching servers. `FileManager.jsx` handles volume operations; `BedrockAddons.jsx` appears only for Bedrock. `MinecraftChoice.jsx` supplies initial software/version options at checkout. Resource plans are common across workloads and require at least 4 GiB for new purchases.

Files and addons enqueue node operations; `api.waitOperation` polls `GET /jobs/{id}` and surfaces final errors. File job `result.data` is a JSON string containing the operation response. Preserve this asynchronous flow and disable conflicting actions while a job is active. The agent polls at one second and operation polling uses 400 ms. Console snapshots are polled, not streamed.

Startup renders the actual resolved template command when available. Ports shown to customers come from the service allocation and node public address, including Bedrock extra transport ports. An internal container listener is not a customer join address.

For current working tools, tested coverage, limits and prioritized work, use [developer handoff](handoff.md). The earlier roadmap paragraphs above describe the broader target; file access, CPU/memory observations and console commands are now implemented within the limits stated in the handoff.
